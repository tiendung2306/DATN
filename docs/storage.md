# Lớp Lưu trữ (SQLite)

> Xem thêm: [Index](README.md) · [Adapter Layer → Store](adapter-layer.md#store-adapter-adapterstore) · [Crypto Engine](crypto-engine.md) · [Architecture Overview](architecture-overview.md)

## Database Configuration

- **Driver:** `modernc.org/sqlite` (pure Go, không CGO)
- **Mode:** WAL (Write-Ahead Logging) — concurrent reads + single writer
- **Connections:** Single writer (`MaxOpenConns=1`) — tránh SQLITE_BUSY
- **Busy timeout:** 5000ms — wait 5s trước khi return SQLITE_BUSY
- **Synchronous:** NORMAL — balance safety + performance (WAL đảm bảo durability)
- **Foreign keys:** Enabled

**Lợi ích pure Go SQLite:**
- Không cần C compiler — cross-compile dễ dàng
- Single binary deployment — không dependency động
- Consistent behavior across platforms

## Schema

### Core Tables

#### `system_config`
Key-value store cho configuration và secrets:

| Key | Type | Mục đích |
|-----|------|----------|
| `libp2p.key` | bytes | Ed25519 private key (libp2p host identity) |
| `admin.key.encrypted` | bytes | Encrypted Root Admin private key (Argon2id + AES-256-GCM) |
| `admin.key.salt` | bytes | Salt cho admin key encryption |
| `admin.pubkey` | bytes | Root Admin public key (Ed25519) |
| `session.started_at` | int64 | Session start timestamp |
| `session.kill_pending` | bool | Kill-session flag (after identity import) |
| `bootstrap.addr` | string | Bootstrap peer multiaddr |
| `node.name` | string | Node display name |

#### `mls_identity`
Local MLS signing identity — 1 row:

| Column | Type | Mô tả |
|--------|------|-------|
| `id` | int | Primary key (always 1) |
| `signing_key` | bytes | Ed25519 private key (seed, 32 bytes) |
| `public_key` | bytes | Ed25519 public key (32 bytes) |
| `created_at` | int64 | Creation timestamp |

#### `auth_bundle`
InvitationBundle từ Admin — 1 row:

| Column | Type | Mô tả |
|--------|------|-------|
| `id` | int | Primary key (always 1) |
| `token` | bytes | Serialized InvitationToken |
| `bootstrap_addr` | string | Bootstrap peer multiaddr |
| `root_admin_pubkey` | bytes | Root Admin Ed25519 public key |
| `imported_at` | int64 | Import timestamp |

#### `mls_groups`
MLS group state + metadata:

| Column | Type | Mô tả |
|--------|------|-------|
| `group_id` | string | Primary key (UUID) |
| `group_type` | string | `channel`, `group`, `dm` |
| `group_state` | bytes | Serialized MLS group state (PersistedGroupState JSON) |
| `epoch` | int64 | Current MLS epoch |
| `name` | string | Group display name |
| `category_id` | string | Channel category (nullable) |
| `avatar` | bytes | Group avatar (nullable) |
| `created_at` | int64 | Creation timestamp |
| `last_message_at` | int64 | Last message timestamp (for sorting) |

#### `coordination_state`
Coordinator persisted state:

| Column | Type | Mô tả |
|--------|------|-------|
| `group_id` | string | Primary key (FK → mls_groups) |
| `active_view` | bytes | Serialized ActiveView (online peers) |
| `token_holder` | string | Current Token Holder PeerID |
| `pending_proposals` | bytes | Serialized pending proposals buffer |
| `history_chain` | bytes | Serialized commit history chain |
| `mode` | string | Coordinator mode (live, catching_up, frozen) |
| `updated_at` | int64 | Last update timestamp |

#### `stored_messages`
Decrypted messages:

| Column | Type | Mô tả |
|--------|------|-------|
| `id` | string | Primary key (UUID) |
| `group_id` | string | FK → mls_groups |
| `sender` | string | Sender PeerID |
| `content` | text | Message content (plaintext) |
| `is_mine` | bool | Whether sent by local node |
| `status` | string | `pending`, `published`, `failed` |
| `local_echo_token` | string | Optimistic UI correlation token (nullable) |
| `timestamp` | int64 | HLC timestamp |
| `created_at` | int64 | Local creation timestamp |
| `replay_of` | string | Original message ID if replayed (nullable) |

#### `envelope_log`
Raw envelope log cho durable replay:

| Column | Type | Mô tả |
|--------|------|-------|
| `hash` | string | Primary key (SHA256 of envelope bytes) |
| `group_id` | string | FK → mls_groups |
| `envelope_type` | string | Message type (proposal, commit, application, etc.) |
| `epoch` | int64 | Envelope epoch |
| `from_peer` | string | Sender PeerID |
| `payload` | bytes | Raw envelope bytes |
| `processed` | bool | Whether processed |
| `created_at` | int64 | Receipt timestamp |

### Supporting Tables

| Table | Mục đích | Key columns |
|-------|----------|-------------|
| `group_members` | Group member directory | group_id, peer_id, identity, role, joined_at |
| `peer_profiles` | Peer profile cache | peer_id, display_name, avatar, last_seen |
| `channel_categories` | Channel category organization | group_id, category_id, name, position |
| `invite_records` | Invite tracking | id, group_id, requester_peer_id, status, created_at |
| `file_transfers` | File transfer records | id, group_id, file_name, file_hash, chunk_count, status |
| `fork_heal_events` | Fork heal audit log | id, group_id, winning_peer, heal_type, timestamp |
| `runtime_events` | Durable runtime event log | seq, event_type, payload, aggregate_id, revision, created_at |
| `replicated_store_objects` | Blind-store replicated objects | key, value, owner_peer_id, created_at |

## Stateless Persistence Pattern

**Stateless Rust + SQLite persistence** — Go là single source of truth:

```
Go đọc group_state bytes từ SQLite (mls_groups.group_state)
        │
        ▼
Gửi đến Rust qua gRPC (group_state + operation params)
        │
        ▼
Rust: import_state → MLS operation → export_state
        │
        ▼
Go nhận new_group_state bytes
        │
        ▼
Go lưu vào SQLite (UPDATE mls_groups SET group_state = ?, epoch = ?)
        │
        ▼
Go log envelope (INSERT INTO envelope_log)
```

Rust không giữ state vĩnh viễn. Mỗi RPC là một round-trip hoàn toàn độc lập. Điều này đảm bảo:
- **Go là single source of truth** — SQLite là authoritative state
- **Rust crash/restart không mất dữ liệu** — state luôn trong SQLite
- **Không cần distributed consensus** giữa Go và Rust — Go controls all writes

## Idempotent Operations

### `ApplyCommit`
```
1. Check envelope hash in envelope_log
   ├── Exists → skip (no-op, return success)
   └── Not exists → proceed
2. BEGIN TRANSACTION
3. UPDATE mls_groups SET group_state = ?, epoch = ?
4. INSERT INTO envelope_log (hash, ...)
5. UPDATE coordination_state SET active_view, token_holder, ...
6. COMMIT
```

### `ApplyApplication`
```
1. Check envelope hash in envelope_log
   ├── Exists → skip (no-op, return success)
   └── Not exists → proceed
2. BEGIN TRANSACTION
3. INSERT INTO stored_messages (id, group_id, sender, content, ...)
4. INSERT INTO envelope_log (hash, ...)
5. UPDATE mls_groups SET last_message_at = ?
6. COMMIT
```

**At-least-once delivery:** GossipSub có thể deliver trùng tin nhắn. Idempotent check đảm bảo duplicate envelopes không gây duplicate effects.

## History Chain

Mỗi commit ghi `prev_hash` → tạo chain cho audit trail:

```
Commit 1: hash=abc, prev_hash=null
Commit 2: hash=def, prev_hash=abc
Commit 3: hash=ghi, prev_hash=def
```

Phục hồi có thể walk chain để reconstruct state. Fork detection so sánh chain heads.

## Replay Link Resolution

Sau fork healing, messages được re-encrypted và resend. `replay_of` column map replayed messages đến original message IDs:

```
Original: id=msg-001, content="Hello", status=published
Replayed: id=msg-002, content="Hello", status=published, replay_of=msg-001
```

Frontend hiển thị "replayed" thay vì duplicate — `timelineState.ts` handles reconciliation.

## Runtime Events Table

Durable runtime event log — persistent event store cho frontend replay:

| Column | Type | Mô tả |
|--------|------|-------|
| `seq` | int | Auto-increment sequence number |
| `event_type` | string | Event name (e.g., `chat:message`, `group:epoch`) |
| `payload` | text | JSON-encoded event data |
| `aggregate_id` | string | Aggregate identifier (e.g., group_id) |
| `revision` | int | Per-aggregate revision (gap detection) |
| `created_at` | int64 | Event timestamp |

**Frontend polling:** `useRuntimeEventStream` hook polls `GetRuntimeEventsSince(cursor, limit)` — detects gaps (missing seq numbers) and triggers `refreshState`.

**Gap detection:** Nếu seq 5, 6, 8 được nhận (missing 7), frontend triggers full state refresh để đảm bảo không miss event.
