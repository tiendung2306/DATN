# Lớp Adapter (Go)

> Xem thêm: [Index](README.md) · [Architecture Overview](architecture-overview.md) · [Coordination Layer](coordination-layer.md) · [Crypto Engine](crypto-engine.md) · [Storage](storage.md)

**Vị trí:** `app/adapter/`  
**Vai trò:** Infrastructure adapters — implement coordination interfaces với concrete technologies (libp2p, gRPC, SQLite).

Adapter Layer là ranh giới giữa "pure domain logic" (Coordination) và "real world infrastructure" (network, database, external process). Mỗi adapter implement một hoặc nhiều interface từ `coordination/interfaces.go`.

## P2P Adapter (`adapter/p2p/`)

### P2PNode (`host.go`)

Tạo và quản lý libp2p host — nền tảng mạng cho toàn bộ ứng dụng:

```go
type P2PNode struct {
    host       host.Host
    dht        *dht.IpfsDHT
    pubsub     *pubsub.PubSub
    gater      *AuthGater
    authProtocol *AuthProtocol
    // ...
}
```

**Transport configuration:**
- **Ed25519 identity** — libp2p host identity, cùng key với MLS signing key
- **TCP + QUIC** — dual transport, TCP cho compatibility, QUIC cho low-latency UDP
- **Kademlia DHT** (prefix `/datn`) — peer discovery, routing, KeyPackage advertisement
- **GossipSub** — pub/sub cho group messaging, coordination messages
- **mDNS** — local network discovery (zero-config cho same-subnet peers)

**Connection management:**
- **AuthGater** — connection gater block blacklisted peers trước khi handshake
- **GetBestLocalIP** — auto-detect network interface:
  1. UDP trick: connect UDP to bootstrap addr, read local addr
  2. Fallback: iterate interfaces, prefer non-loopback IPv4

### LibP2PTransport (`transport_adapter.go`)

Implement `coordination.Transport` interface — bridge GossipSub + direct streams:

| Method | Implementation |
|--------|----------------|
| `Publish(topic, data)` | `pubsub.Publish(topic, data)` qua GossipSub |
| `Subscribe(topic)` | `pubsub.Subscribe(topic)` → return channel, spawn `readLoop` |
| `Unsubscribe(topic)` | Cancel subscription, close channel |
| `SendDirect(peerID, protocol, data)` | Open libp2p stream `/coordination/direct/1.0.0`, write data |
| `LocalPeerID()` | `host.ID().String()` |
| `ConnectedPeers()` | Iterate `host.Network().Peers()` |

**`readLoop`:** Goroutine đọc messages từ GossipSub subscription channel, decode envelope, route đến Coordinator's `handleRawMessage`.

**`handleDirectStream`:** Handler cho incoming direct streams — decode envelope, route đến Coordinator.

**Retry logic:** `ps.Join` có race condition khi topic chưa tồn tại — retry 3 lần với 100ms delay.

### AuthProtocol (`auth_protocol.go`)

Handshake `/app/auth/1.0.0` trên mỗi connection mới — zero-trust authentication:

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
    │                                └── If fail: blacklist peer
```

**Verification steps:**
1. **InvitationToken signature** — verify bằng Root Admin Ed25519 public key
2. **PeerID match** — PeerID trong token phải khớp với connection's PeerID (chống replay attack)
3. **Session claim** — check `SessionClaim` (signed timestamp) — enforce single active device
4. **Blacklist** — peers fail auth 3 lần → blacklist (SecurityFail) → AuthGater block

**`onPeerVerified` callback** — trigger sau khi auth thành công:
- Profile sync — exchange display name, avatar
- Welcome delivery — deliver pending Welcome messages
- Offline sync — deliver buffered messages
- KeyPackage offer — offer KeyPackage cho invite

### SessionClaim (`session_claim.go`)

```go
type SessionClaim struct {
    PeerID    string
    Timestamp int64  // Unix milliseconds
    Signature []byte  // Ed25519 signed bằng MLS signing key
}
```

- **Single Active Device enforcement:** Khi 2 device cùng identity kết nối, device có timestamp mới hơn wins. Device cũ nhận `session:replaced` event → UI hiển thị error.
- `SessionClaim` signed bằng MLS signing key (không phải admin key) — chứng minh ownership

### Other P2P Files

| File | Mục đích |
|------|----------|
| `auth.go` | `OnboardNewUser` — generate MLS Ed25519 key pair via Rust, persist to SQLite |
| `session_claim.go` | SessionClaim struct + sign/verify logic |
| `gater.go` | AuthGater — block/blacklist peers trước connection |
| `identity.go` | GetOrCreateIdentity, GetOnboardingInfo — P2P identity management |
| `pubsub.go` | Join chat room helper — GossipSub topic management |
| `kp_direct.go` | KeyPackage direct delivery — send KP qua direct stream |

### Wire Protocols

Mỗi wire protocol là một libp2p stream handler, chạy trên một protocol ID cụ thể:

| File | Protocol ID | Mục đích |
|------|-------------|----------|
| `user_profile_push.go` | `/app/user-profile/1.0.0` | User profile sync — exchange display name, avatar giữa peers |
| `replicated_store_wire.go` | `/org/replicated-store/1.0.0` | Blind-store replication — store/lookup replicated objects |
| `invite_store_wire.go` | `/app/invite-store/1.0.0` | Invite store/lookup — DHT-based invite tracking |
| `group_info_wire.go` | `/app/group-info/1.0.0` | GroupInfo exchange — fork healing support |
| `group_invite_request_wire.go` | `/app/group-invite/1.0.0` | Group invite request — multi-node approval protocol |
| `channel_category_wire.go` | `/app/channel-cat/1.0.0` | Channel category sync — organize channels into categories |
| `file_transfer_wire.go` | `/app/file-transfer/1.0.0` | File transfer — chunked, MLS-encrypted |
| `offline_wire.go` | `/app/offline-sync/1.0.0` | Offline sync — deliver buffered messages when peer online |

### BlindStore (`service/blind_store.go`)

Blind-store replication layer cho offline message retention — implemented trong Service layer, không phải P2P adapter:
- Regular nodes: retain targeted k-nearest replicas (mặc định k=2) — Kademlia DHT selects replica targets
- `--store-node` nodes: retain ALL blind-store objects
- Objects stored encrypted — store nodes không thể đọc content
- Kademlia DHT: discovery/routing only, không cho application mailbox storage

## Sidecar Adapter (`adapter/sidecar/`)

### GrpcMLSEngine (`engine.go`)

Adapter từ gRPC `MLSCryptoServiceClient` → `coordination.MLSEngine` interface:

```go
type GrpcMLSEngine struct {
    client mls_service.MLSCryptoServiceClient
}
```

Mỗi method tuân theo pattern:
1. Build gRPC request từ coordination parameters
2. Call gRPC với context timeout
3. Validate response — `requireNonEmptyState` kiểm tra `new_group_state` không rỗng
4. Return coordination result type

**`truncateSigningKey`:** OpenMLS expects 32-byte Ed25519 seed, nhưng Go generates 64-byte (seed + public key). Hàm này cắt xuống 32-byte.

**`requireNonEmptyState`:** Nếu Rust trả về empty state, gợi ý "group state may be stale — rebuild from SQLite". Điều này có thể xảy ra nếu Rust restart giữa operations.

### gRPC Connection (`grpc.go`)

Helper để tạo gRPC connection đến Rust sidecar:
- `DialContext` với `127.0.0.1:{port}`
- `WithInsecure` — không cần TLS (local only)

### Cached Benchmark Engine (`cached_benchmark_engine.go`)

Benchmark-only adapter cho cached MLS path — implement `MLSEngine` using cached gRPC RPCs (`LoadGroup`, `UnloadGroup`, `EncryptMessageCached`, etc.). Không dùng trong production.

### ProcessManager (`process.go`)

Quản lý Rust binary lifecycle — spawn, monitor, stop:

```go
type ProcessManager struct {
    cmd      *exec.Cmd
    port     int
    cancel   context.CancelFunc
    binaryPath string
}
```

| Method | Mục đích |
|--------|----------|
| `StartEngine()` | Tìm và spawn Rust binary, return port |
| `StopCryptoEngine()` | Cancel context → kill process |
| `GetFreePort()` | Xin OS free TCP port (bind 127.0.0.1:0 → read assigned port) |
| `tryStart(path, port)` | Spawn binary với `--port` flag, pipe stdout/stderr |
| `ensureBinaryFresh(path)` | Kiểm tra binary modification time vs source — warn nếu stale |

**Binary search order:**
1. `./crypto-engine` / `./crypto-engine.exe` (cwd)
2. `../crypto-engine/target/release/crypto-engine` (release build)
3. `../crypto-engine/target/debug/crypto-engine` (debug build)

**`setSysProcAttr`:** Platform-specific process attributes:
- **Windows:** `CREATE_NEW_PROCESS_GROUP` — cho phép graceful kill
- **Linux/macOS:** `Setpgid` — kill process group

> Chi tiết Rust engine: [Crypto Engine](crypto-engine.md)

## Store Adapter (`adapter/store/`)

### Database (`db.go`)

SQLite database — pure Go, không CGO:

```go
type Database struct {
    db *sql.DB
}
```

**Configuration:**
- **Driver:** `modernc.org/sqlite` (pure Go implementation, không cần C compiler)
- **Mode:** WAL (Write-Ahead Logging) — concurrent reads + single writer
- **Connections:** `MaxOpenConns=1` — single writer tránh SQLITE_BUSY
- **Busy timeout:** 5000ms — wait 5s trước khi return SQLITE_BUSY
- **Synchronous:** NORMAL — balance giữa safety và performance (WAL đảm bảo durability)

**Schema initialization:** `initSchema()` tạo tất cả tables nếu chưa tồn tại — idempotent, safe to run on every startup.

> Chi tiết schema: [Storage](storage.md)

### SQLiteCoordinationStorage (`coordination_storage.go`)

Implement `coordination.CoordinationStorage` interface — persistence cho coordination state:

| Method | Mục đích |
|--------|----------|
| `SaveGroupRecord` | Lưu group metadata + MLS group state bytes |
| `LoadGroupRecord` | Nạp group record từ SQLite |
| `SaveCoordState` | Lưu coordination state (activeView, tokenHolder, pendingProposals, historyChain) |
| `LoadCoordState` | Nạp coordination state |
| `ApplyCommit` | **Atomic:** update group state + log envelope + update epoch |
| `ApplyApplication` | **Atomic:** save decrypted message + log envelope |
| `GetMessages` | Paginated message retrieval (limit, offset) |
| `SaveEnvelopeRecord` | Lưu raw envelope cho durable replay |
| `HasEnvelope` | Check envelope hash — idempotent check |

**Idempotent operations:** Cả `ApplyCommit` và `ApplyApplication` check envelope hash trước khi apply. Nếu envelope đã tồn tại → skip (no-op). Đảm bảo at-least-once delivery không gây duplicate effects.

**History chain:** Mỗi commit ghi `prev_hash` → tạo chain cho audit trail. Phục hồi có thể walk chain để reconstruct state.

**Replay link resolution:** Map replayed messages (sau fork heal) đến original message IDs — frontend hiển thị "replayed" thay vì duplicate.

### Các file store khác

| File | Nội dung |
|------|----------|
| `portrepos.go` | Repository ports — implement `port.Repository` interface |
| `db_group_members.go` | Group member CRUD — add, remove, list members với online status |
| `db_group_metadata.go` | Group metadata storage — name, avatar, category |
| `db_identity.go` | MLS identity storage — signing key, public key |
| `db_invite_assets.go` | Invite assets — KeyPackages, welcome messages, invite records |
| `db_peer_directory.go` | Peer directory — known peers, connection info |
| `db_messages.go` | Message storage — stored_messages CRUD |
| `db_admin.go` | Admin key storage — encrypted admin private key |
| `db_config.go` | System config storage — key-value pairs |
| `profile.go` | Peer profile cache — display name, avatar, last seen |
| `channel_categories.go` | Channel category storage — category CRUD + ordering |
| `file_transfer.go` | File transfer storage — file metadata, chunk tracking |
| `notifications.go` | Notification storage — CRUD, unread count |
| `runtime_events.go` | Runtime event log storage — durable event store |
| `replicated_store.go` | Blind-store replicated objects — store, lookup, GC |
| `group_add_operations.go` | Group add operations — track pending adds |
| `group_event_log.go` | Group event log — audit trail |
| `group_invite_requests.go` | Group invite requests storage — request CRUD |
| `backup_data.go` | Backup data storage — identity backup metadata |

## Wails UI Adapter (`adapter/wailsui/`)

### Run (`run.go`)

Wails application setup — bridge Go backend với React frontend:

```go
func Run(cfg config.Config, dist string) {
    rt := service.NewRuntime(cfg)
    err := wails.Run(&options.App{
        Bind: []interface{}{rt},
        OnStartup:  rt.Startup,
        OnDomReady: rt.DomReady,
        OnBeforeClose: rt.BeforeClose,
        OnShutdown: rt.Shutdown,
        AssetServer: &assetserver.AssetServer{Handler: distHandler},
    })
}
```

- Bind `service.Runtime` struct → Wails generates TypeScript bindings trong `frontend/wailsjs/`
- Lifecycle hooks: `Startup` (init), `DomReady` (frontend loaded), `BeforeClose` (confirm exit), `Shutdown` (cleanup)
- Window: 1200x800, dark theme, title "Secure P2P Node"

### EventSink (`sink.go`)

Implement `service.EventSink` interface — emit events to frontend via Wails runtime:

```go
type EventSink struct{}

func (s *EventSink) Emit(eventName string, data ...interface{}) {
    wailsruntime.EventsEmit(ctx, eventName, data...)
}
```

Frontend subscribes via `EventsOn(eventName, handler)` — see `hooks/useWailsEvent.ts`.
