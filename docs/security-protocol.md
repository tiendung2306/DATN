# Bảo Mật & Giao Thức

> Xem thêm: [Index](README.md) · [Coordination Layer](coordination-layer.md) · [Admin & Config](admin-config.md) · [Crypto Engine](crypto-engine.md) · [Flows](flows.md)

## Security Rules

### 1. Strict Onboarding

Không node nào join Gossip network không có `InvitationToken` hợp lệ.

**Pipeline:**
1. New user generates Ed25519 key pair (local, via Rust engine)
2. User sends PeerID + public key to Root Admin (out-of-band)
3. Admin signs `InvitationToken` với Root Admin Ed25519 private key
4. Token binds: PeerID + PublicKey + IssuedAt + ExpiresAt (365 days)
5. User imports `.bundle` file → stores auth bundle → launches P2P node
6. Mỗi P2P connection yêu cầu auth handshake (`/app/auth/1.0.0`):
   - Client sends AuthMessage (token + PeerID + SessionClaim)
   - Server verifies token signature bằng Root Admin public key
   - Server verifies PeerID trong token khớp connection's PeerID (anti-replay)
   - Server checks SessionClaim (single active device)
7. Peers fail auth 3 lần → blacklist (AuthGater block)

**Threat model:** Token không thể forge (Ed25519 signature), không thể replay (PeerID binding), không thể steal (private key chỉ trong memory).

### 2. Single Active Device

Một tài khoản chỉ valid trên MỘT thiết bị tại một thời điểm.

**Mechanism:**
- `SessionClaim` — signed timestamp bằng MLS signing key, gửi trong auth handshake
- Khi 2 device cùng identity kết nối:
  - Device có timestamp mới hơn wins
  - Device cũ nhận `session:replaced` event → UI hiển thị error, block operations
- `ensureSessionActive()` guard — check session chưa bị replaced trước mọi operation
- `ApplyIdentityImportSideEffects()` — reset session + set kill-session flag (after identity import on new device)

**Threat model:** Ngăn same identity chạy trên multiple devices — giảm attack surface, enforce accountability.

### 3. Manual Identity Migration

Private keys KHÔNG bao giờ gửi qua network (ngay cả encrypted). Phải export ra file `.backup` encrypted với passphrase, transfer thủ công.

**Pipeline:**
1. `ExportIdentity(outputPath, passphrase)`:
   - Derive key: Argon2id(passphrase, salt=16 bytes random)
   - Encrypt: AES-256-GCM(private_key, nonce=12 bytes random)
   - Write `.backup` file: `[salt][nonce][ciphertext+tag]`
2. User transfers `.backup` manually (USB, secure channel)
3. `ImportIdentityFromFile(filePath, passphrase)`:
   - Read `.backup` file
   - Derive key: Argon2id(passphrase, salt)
   - Decrypt: AES-256-GCM(ciphertext, nonce)
   - Store in SQLite → reset session → set kill-session flag

**Threat model:** Private key không bao giờ trên network — kể cả nếu network compromised, attacker không intercept key. Passphrase brute-force resistant (Argon2id).

### 4. Forward Secrecy on Heal

Khi node join winning branch via External Join, TẤT CẢ keys từ losing branch MUST be destroyed (crypto-shredding).

**Mechanism:**
- `runHeal()` trong `coordinator_heal.go`:
  1. ExternalJoin via Rust → new group state on winning branch
  2. Crypto-shredding — destroy ALL old keys (group state, epoch secrets, message keys)
  3. No old state may be retained — SQLite overwritten với new group state

**Threat model:** Nếu losing branch keys compromised, chúng không thể decrypt future messages trên winning branch. Forward secrecy preserved across fork healing.

### 5. Non-Repudiation in Replay

During Autonomous Replay after fork healing, node MUST only re-encrypt and resend its OWN messages. MUST NOT resend messages authored by other nodes.

**Mechanism:**
- `coordinator_replay.go` — iterate `stored_messages WHERE is_mine = true AND replay_of IS NULL`
- Re-encrypt với new group state (winning branch keys)
- Set `replay_of = original_message_id` — frontend hiển thị "replayed" thay vì duplicate
- Messages từ other nodes KHÔNG được replay — chúng sẽ được re-delivered bởi their original senders

**Threat model:** Nếu node replay messages của other nodes, nó có thể inject/modify content. Non-repudiation đảm bảo mỗi node chỉ accountable cho messages của chính nó.

## Protocol Invariants

### Single-Writer

Tại mỗi epoch, CHỈ Token Holder được phát hành Commit.

**Enforcement:**
- `ComputeTokenHolder` — deterministic: `sortedView[epoch % len(view)]`
- Commit validation: `sender == ComputeTokenHolder()` — reject nếu không phải
- `SuspendPeer` — cấm peer khỏi trở thành Token Holder (violation detected)
- `ErrNotTokenHolder` — sentinel error cho unauthorized commit attempts

**Violation consequence:** Unauthorized commit bị reject, sender bị flag, potential blacklist.

### Epoch Monotonicity

Node MUST NOT process MLS Commit/Proposal với epoch < current epoch.

**Enforcement:**
- `EpochTracker.ValidateEpoch(msgEpoch)`:
  - `ActionProcess` — epoch == current → process normally
  - `ActionRejectStale` — epoch < current → reject với `CurrentEpochNotification`
  - `ActionBufferFuture` — epoch > current → buffer, chờ catch-up
- Stale messages bị reject — sender nhận `CurrentEpochNotification` để catch-up

**Violation consequence:** Stale commits rejected, network converges to latest epoch.

### Two-Tier Separation

Coordination Layer (Go) handles ordering, election, fork healing. Crypto Layer (Rust) handles pure MLS operations. Rust has NO knowledge of Single-Writer, epochs, or ActiveView.

**Enforcement:**
- `coordination.MLSEngine` interface — Rust chỉ thấy MLS operations (CreateGroup, EncryptMessage, etc.)
- Rust không nhận epoch numbers, token holder info, hay active view
- Go gửi `group_state bytes` + operation params → Rust returns `new_group_state bytes`
- Rust không biết về coordination concepts — chỉ MLS crypto

**Benefit:** Separation of concerns. Rust có thể thay thế bằng different MLS implementation mà không affect coordination logic.

### Stateless Rust

Rust engine MUST NOT store state permanently. Go retrieves state from SQLite → Sends to Rust → Rust computes → Returns new state → Go saves to SQLite.

**Enforcement:**
- Mọi stateless RPC nhận `group_state: bytes`, trả về `new_group_state: bytes`
- Rust không có database, không persistent storage
- `RuntimeCache` (DashMap) chỉ cho benchmark — production uses stateless path
- Rust crash/restart → Go re-sends state from SQLite → no data loss

**Benefit:** Go là single source of truth. Rust crash không mất dữ liệu. Không cần distributed consensus giữa Go và Rust.

## Cryptographic Choices

| Component | Algorithm | Parameters | Mục đích |
|-----------|-----------|------------|----------|
| **MLS Ciphersuite** | `MLS_128_DHKEMX25519_AES128GCM_SHA256_Ed25519` | — | Group encryption, key agreement |
| **KEM** | X25519 | — | Diffie-Hellman key exchange |
| **AEAD** | AES-128-GCM | 128-bit keys | Authenticated encryption |
| **Hash** | SHA-256 | — | MLS transcript hash, tree hash |
| **Signature** | Ed25519 | 32-byte seed, 64-byte sig | MLS signing, admin key, auth |
| **Admin Key Encryption** | Argon2id + AES-256-GCM | time=1, memory=64KB, parallelism=4 | Encrypt admin private key at rest |
| **Identity Backup** | Argon2id + AES-256-GCM | Same as admin | Encrypt `.backup` file |
| **Noise Protocol** | Noise IK | — | libp2p transport encryption |
| **HLC** | Hybrid Logical Clock | MaxClockDriftMs=10000 | Causal ordering, clock drift protection |
| **File Transfer** | MLS Exporter + AES-256-GCM | Per-file derived key | Chunked file encryption |

## Offline Handling

| Mechanism | Mô tả |
|-----------|-------|
| **Encrypted local envelope retention** | Messages to offline peers stored in `envelope_log` (SQLite) — encrypted, không readable by local node |
| **Authenticated direct stream sync** | When peer online, deliver buffered envelopes via `/app/offline-sync/1.0.0` — authenticated, idempotent |
| **Blind-store replication** | Replicate envelopes to k-nearest nodes (k=2) via `/org/replicated-store/1.0.0` — store nodes không thể đọc content |
| **`--store-node` mode** | Nodes với flag retain ALL blind-store objects (high availability) |
| **Kademlia DHT** | Discovery/routing only — NOT for application mailbox storage |

## Key Retention Policies

| Mode | `max_past_epochs` | `max_past_age` | Use case |
|------|-------------------|----------------|----------|
| `STRICT_SECURITY` | 0 | 0s | Maximum forward secrecy — không giữ old keys, late messages rejected |
| `BALANCED` (default) | 3 | 5 minutes | Production balance — cho phép late messages trong 3 epochs / 5 phút |
| `HIGH_AVAILABILITY` | 10 | 1 hour | Maximum late delivery — risk hơn, giữ keys lâu hơn |

**Trade-off:** Higher retention = better late message delivery but longer key exposure window. Default BALANCED là production recommendation.

## Auth Protocol Detail

```
Client (new connection)          Server (existing peer)
    │                                │
    ├── SEND: AuthMessage            │
    │   {token, peerID, sessionClaim}│
    │                                ├── READ: AuthMessage
    │                                ├── Verify token signature (Root Admin key)
    │                                ├── Verify PeerID matches token
    │                                ├── Check session claim (single active device)
    │                                ├── SEND: AuthResponse
    │   ├── READ: AuthResponse       │
    │   ├── If success: OK           │
    │   └── If fail: close conn      │
    │                                └── If fail: blacklist peer (after 3 fails)
```

**Security guarantees:**
- **Authentication:** Token signed bởi Root Admin — chỉ admin có thể issue tokens
- **Authorization:** Token binds PeerID — không thể dùng token của node khác
- **Anti-replay:** PeerID trong token phải khớp connection's PeerID
- **Freshness:** SessionClaim có timestamp — detect stale sessions
- **Single active device:** SessionClaim comparison — newer timestamp wins
- **Blacklist:** AuthGater block peers fail auth 3 lần — prevent brute-force

## Wire Protocol Security

Mỗi wire protocol chạy trên libp2p stream — đã encrypted bởi Noise IK transport:

| Protocol | Security |
|----------|----------|
| `/app/auth/1.0.0` | Token + PeerID + SessionClaim verification |
| `/app/user-profile/1.0.0` | Profile signed bằng MLS signing key |
| `/org/replicated-store/1.0.0` | Objects encrypted — store nodes không đọc content |
| `/app/invite-store/1.0.0` | DHT-based — KeyPackage public data |
| `/app/group-info/1.0.0` | GroupInfo public (with ratchet tree for fork healing) |
| `/app/group-invite/1.0.0` | Invite request signed — approval required |
| `/app/channel-cat/1.0.0` | Category data — group members only |
| `/app/file-transfer/1.0.0` | Chunks MLS-encrypted (AES-256-GCM via exporter) |
| `/app/offline-sync/1.0.0` | Authenticated, idempotent delivery |
| `/coordination/direct/1.0.0` | Direct stream — same security as GossipSub |
