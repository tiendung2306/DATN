# Current State — Decentralized Coordination Protocol for MLS on P2P Networks

This document serves as a short-term memory for the AI Agent.

## 1. Project Overview

**Thesis with dual objectives:**
1. **Research (Core):** Design a Decentralized Coordination Protocol that wraps MLS (RFC 9420) for P2P environments — solving the open problem of maintaining causal consistency and total ordering without a central Delivery Service.
2. **Application:** Build a serverless, zero-trust P2P communication platform using Go (Wails) + Rust (OpenMLS) that implements the protocol.

**Four Core Mechanisms of the Coordination Protocol:**
- **Single-Writer Protocol:** Only one node (Epoch Token Holder) may Commit per epoch. Eliminates concurrent Commits entirely.
- **Epoch Consistency:** Every MLS operation carries an epoch number; stale/future operations are rejected or buffered.
- **Group Fork Healing:** Network partitions are detected via Gossip Heartbeat; losing branch performs External Join into winning branch.
- **Hybrid Logical Clock (HLC):** Causal consistency and total ordering for application messages without NTP synchronization.

## 2. Completed Tasks

### Phase 1: System Architecture & Setup ✅
*   Monorepo, Sidecar lifecycle, gRPC IPC, CGO-free SQLite.

### Phase 2: P2P Networking Layer ✅
*   **Persistent PeerID:** Stored in SQLite `system_config` table.
*   **Resilient IP Detection (Hybrid):** UDP trick (8.8.8.8) → fallback interface scan (ignores Docker/WSL/VMWare).
*   **Libp2p Host:** Bound to the specific "best" IP found.
*   **Log Noise Suppression (2-Layer):** `go-log/v2` sets mdns to `error`; custom `LogFilterHandler` drops Windows virtual adapter warnings.
*   **Hybrid Discovery:** mDNS + Kademlia DHT + Dynamic Bootstrap.
*   **GossipSub:** Global chat topic `/org/chat/global`.

### Phase 3: Identity & Admin Onboarding ✅
*   Tất cả 7 bước đã implement và build clean (Go + Rust).

### Phase 3.5: Wails GUI Integration ✅
*   Thư mục `backend/` đã đổi tên thành `app/`, Go module name đổi từ `backend` → `app`.
*   Tích hợp Wails v2.11.0 vào Go app (`app/app.go`).
*   Scaffold React + TypeScript + Tailwind frontend tại `app/frontend/`.
*   4 màn hình dev/test UI đã hoàn chỉnh.

### Phase 4.1: Add member / KeyPackage (MLS) ✅
*   Proto: `GenerateKeyPackage`, `AddMembers`; `ProcessWelcome` extended with `epoch` and `key_package_bundle_private` (OpenMLS requires the invitee to retain the `KeyPackageBundle` private material until Welcome, not just the public KeyPackage).
*   Rust: `generate_key_package`, `add_members`; gRPC handlers; tests `test_generate_key_package`, `test_add_member_and_welcome`.
*   Go: `MLSEngine` + adapter + mock; `Coordinator.AddMember` added; `CommitMsg` broadcast only carries `CommitData` + `NewTreeHash` (Welcome is delivered out-of-band, not broadcast).
*   Wails: `KeyPackageResult`, `AddMemberToGroup`, `JoinGroupWithWelcome`, `GetGroupMembers`, `MemberInfo`; ChatPanel **Members & keys** panel.

---

## 3. Technical Decisions & Knowledge

### 3a. Hai loại Identity — KHÔNG nhầm lẫn

| Identity | Layer | Ai sinh | Ai quản lý | Mục đích |
|---|---|---|---|---|
| **Libp2p PeerID** | Mạng (P2P) | Go (`crypto.GenerateKeyPair`) | Go + SQLite | Định danh node, mã hóa kênh Noise |
| **MLS Identity** | Ứng dụng (E2EE) | **Rust** (OpenMLS) | Go (lưu SQLite) | Ký/mã hóa tin nhắn MLS Group |

### 3b. Luồng Onboarding đúng (CSR-like) — THIẾT KẾ CUỐI CÙNG

**NGUYÊN TẮC BẤT BIẾN:**
*   Admin là người cấp danh tính — **tên hiển thị do Admin đặt**, không phải user tự đặt.
*   MLS Private Key sinh ra trên máy user, KHÔNG BAO GIỜ rời máy.
*   Root Admin Private Key chỉ tồn tại trên máy Admin.

```
[Máy User - lần đầu]
  1. app --setup
     → GetOrCreateLibp2pIdentity() → PeerID (đã có từ Phase 2)
     → Rust GenerateIdentity() → MLS keypair (credential rỗng ban đầu)
     → Output: PeerID + MLS_PubKey_hex (KHÔNG có tên — Admin sẽ đặt)

[Máy Admin]
  2. Nhận PeerID_Alice + MLS_PubKey_Alice (Zalo/email)
  3. app --create-bundle \
       --bundle-name "Alice" \          ← Admin đặt tên cho user
       --bundle-peer-id <PeerID> \
       --bundle-pub-key <PubKeyHex> \
       --admin-passphrase "secret" \
       --bundle-output alice.bundle
  4. Gửi alice.bundle cho Alice (out-of-band)

[Máy User]
  5. app --import-bundle alice.bundle
     → Verify: chữ ký Admin + PeerID binding + PublicKey binding + expiry
     → SaveAuthBundle() vào SQLite
     → UpdateMLSDisplayName("Alice") ← tên từ token ghi đè vào mls_identity
     → App → StateAuthorized

  6. app (chạy bình thường — GUI mode)
     → Load bundle → BuildLocalToken() → NewP2PNode() với auth
     → Kết nối bootstrap_addr từ bundle
     → Auth handshake với mọi peer
```

**Admin Quick Setup (GUI shortcut):** Nếu máy đã có admin key (`--admin-setup` đã chạy) và đang ở trạng thái `AWAITING_BUNDLE`, UI hiện card "Admin Quick Setup" — nhập displayName + passphrase → tự tạo và import bundle cho chính mình trong 1 bước. Binding: `CreateAndImportSelfBundle(displayName, passphrase string) error`.

### 3c. Cấu trúc InvitationToken và InvitationBundle

```go
// app/admin/token.go
type InvitationToken struct {
    Version     int    `json:"version"`     // = 1
    DisplayName string `json:"display_name"` // do Admin đặt
    PeerID      string `json:"peer_id"`      // BẮTBUỘC — chống Token Replay Attack
    PublicKey   []byte `json:"public_key"`   // MLS public key (hex khi hiển thị)
    IssuedAt    int64  `json:"issued_at"`
    ExpiresAt   int64  `json:"expires_at"`   // = IssuedAt + 365 ngày
    Signature   []byte `json:"signature,omitempty"` // Ed25519 ký payload trên
}

type InvitationBundle struct {
    Token         *InvitationToken `json:"token"`
    BootstrapAddr string           `json:"bootstrap_addr"` // /ip4/IP/tcp/PORT/p2p/PEERID
    RootPublicKey []byte           `json:"root_public_key"` // TOFU khi import lần đầu
}
```

**bootstrap_addr PHẢI có `/p2p/PeerID`** — thiếu PeerID thì Noise Protocol không thể xác thực danh tính bootstrap node.

### 3d. Bảo vệ chống Token Replay / Spoofing Attack

```go
// app/p2p/auth_protocol.go — verifyPeerToken()
if token.PeerID != authenticatedPeerID.String() {
    reject() // Eve không có Libp2p private key của Alice → bị lộ ngay
}
```

### 3e. Auth Protocol — `/app/auth/1.0.0`

**Wire format:** `[4 bytes big-endian uint32: JSON length][JSON bytes of InvitationToken]`

**Quy tắc tránh deadlock:**
*   **Client (outbound, gọi `InitiateHandshake`):** SEND token trước → READ token peer
*   **Server (inbound, `handleIncoming` qua `SetStreamHandler`):** READ token peer trước → SEND token

**Auth state machine (per connection attempt):**
```
Connected → Handshaking → Verified       (crypto ok → lưu vào verifiedPeers)
                        → SecurityFail   (sai chữ ký / hết hạn / PeerID mismatch
                                          → rejectSecurity: blacklist TTL + close peer)
                        → TransientFail  (IO error / timeout / stream reset
                                          → rejectTransient: reset stream only, KHÔNG blacklist)
```

**Vòng đời verifiedPeers:**
*   Set khi handshake thành công (inbound hoặc outbound).
*   Xóa khi peer đóng **tất cả** connection (TCP + QUIC) — xử lý trong `Disconnected` notifee.
*   Đảm bảo peer reconnect/restart luôn phải handshake lại, không bị skip do stale state.

**AuthGater (TTL-based):**
*   `Blacklist(id, reason)` CHỈ được gọi từ `rejectSecurity` — khi `verifyPeerToken` thất bại.
*   KHÔNG gọi khi `NewStream` fail, IO error, hoặc timeout — đây là `rejectTransient`.
*   Blacklist entry tự hết hạn sau **30 phút** (peer được thử lại mà không cần restart app).
*   `isBlacklisted` evict entry hết hạn lazily khi check.

**Root cause bug đã fix (restart → "gater disallows connection"):**
Trước đây `handleIncoming` có `if IsVerified(peer) { return }`. Khi node A restart, node B
(còn chạy) vẫn giữ A trong verifiedPeers nên skip handshake mà không đọc/ghi token. A đang
chờ đọc token thì nhận EOF → gọi `reject()` (cũ) → blacklist B oan. Fix: bỏ early-return ở
inbound + thêm `Disconnected` handler xóa verifiedPeers + tách `rejectSecurity`/`rejectTransient`.

### 3f. App States — THIẾT KẾ CUỐI CÙNG

```
StateUninitialized  → Chưa có MLS keypair → GUI: SetupScreen
StateAwaitingBundle → Có keypair, chưa có bundle → GUI: AwaitingBundleScreen
StateAuthorized     → Có bundle hợp lệ → GUI: DashboardScreen
StateAdminReady     → StateAuthorized + có root admin key → GUI: DashboardScreen + AdminPanel
```

### 3g. display_name — Admin là người cấp, không phải user

*   `display_name` trong `InvitationToken` do **Admin đặt** — đây là tên chính thức.
*   Khi user chạy `--setup`, credential MLS ban đầu **rỗng**.
*   Khi user chạy `--import-bundle`, hàm `ImportInvitationBundle` tự động gọi `database.UpdateMLSDisplayName(token.DisplayName)`.
*   Định danh kỹ thuật thực sự: **PeerID** (mạng) và **MLS PublicKey** (crypto).

### 3h. CRITICAL — Device Migration phải export cả Libp2p Private Key

```
identity.backup (mã hóa AES-256-GCM + Argon2id)
├── libp2p_private_key   ← BẮT BUỘC — để PeerID giống hệt trên máy mới
├── mls_signing_key      ← MLS private key
├── mls_credential       ← display_name bytes (sau khi import bundle)
└── invitation_token     ← token Admin đã ký
```

**Cần cập nhật Phase 5 ExportIdentity proto:**
```protobuf
message ExportIdentityRequest {
  bytes libp2p_private_key = 1; // THÊM — để PeerID được khôi phục
  bytes mls_signing_key    = 2;
  bytes mls_credential     = 3;
  bytes invitation_token   = 4;
  string passphrase        = 5;
}
```

### 3i. Wails Integration — Kiến trúc GUI

**Nguyên tắc:** Wails binding (không phải CLI spawn, không phải REST API). Mọi tương tác UI đều gọi exported method trên `App` struct → Wails auto-gen TypeScript bindings tại `frontend/wailsjs/go/main/App.d.ts`.

**Phân nhánh CLI vs GUI trong `main.go`:**
```go
if cfg.Headless || cfg.IsCommand() {
    // CLI mode — existing behavior
    run(cfg)
} else {
    // GUI mode
    runWailsApp(cfg)
}
```

**`IsCommand()` = bất kỳ flag nào trong:** `--setup`, `--admin-setup`, `--create-bundle`, `--import-bundle`.

**Wails lifecycle:** `startup(ctx)` → init DB + identity + crypto engine + auto-start P2P nếu AUTHORIZED/ADMIN_READY. `shutdown(ctx)` → dọn dẹp tất cả resources.

**App struct state (thread-safe qua `mu sync.Mutex`):**
```go
type App struct {
    ctx, cfg, db, privKey, mlsClient, conn, stopEngine, node, nodeCancel, mu
}
```

### 3j. Decentralized Coordination Protocol — Thiết kế mới (Phase 4)

**THAY ĐỔI QUAN TRỌNG so với thiết kế ban đầu:**
Phiên bản cũ dùng "Deterministic Conflict Resolution" — cho phép xung đột xảy ra rồi chọn commit có hash nhỏ nhất, commit thua bị rollback. **Phiên bản mới LOẠI BỎ HOÀN TOÀN xung đột** bằng Single-Writer Protocol.

**Bốn cơ chế cốt lõi:**

**1. Single-Writer Protocol (Giao thức Người ghi duy nhất):**
*   Tại mọi thời điểm, chỉ **một node duy nhất** — Epoch Token Holder — có quyền tạo Commit.
*   Election tất định (không cần giao tiếp): `TokenHolder = argmin_{node ∈ ActiveView} H(nodeID || epoch)`
*   Phân tách Proposal/Commit: mọi node có thể tạo Proposal, chỉ Token Holder đóng gói thành Commit.
*   Failover: Token Holder không commit trong `T_timeout` (3-5s) → bị loại khỏi ActiveView → bầu lại.

**2. Epoch Consistency (Nhất quán nhân quả qua kiểm tra Epoch):**
*   `msg.epoch == local.epoch` → xử lý bình thường
*   `msg.epoch < local.epoch` → từ chối, gửi `CurrentEpochNotification`
*   `msg.epoch > local.epoch` → buffer, request state sync

**3. Group Fork Healing (Hàn gắn phân mảnh mạng):**
*   Phát hiện qua Gossip Heartbeat (khác TreeHash).
*   Hàm trọng số: `W = (C_members, E, H_commit)` — so sánh lexicographic.
*   Nhánh thua: Drop MlsGroup → External Join vào nhánh thắng → Autonomous Replay (chỉ gửi lại tin nhắn của chính mình).
*   Forward Secrecy bảo toàn (khóa nhánh thua bị hủy). PCS suy yếu tạm thời, khôi phục ngay sau External Join.

**4. Hybrid Logical Clock (HLC) — Thứ tự hiển thị tin nhắn:**
*   Epoch number chỉ ordering MLS state changes. Trong cùng 1 epoch, nhiều user gửi tin nhắn đồng thời → cần HLC để sắp xếp.
*   `HLCTimestamp = (L, C, NodeID)`: L = max(physical_time, received_L), C = logical counter, NodeID = tiebreaker.
*   Đảm bảo: causal consistency, total order, NTP-independent, human-readable (L là unix ms).
*   Mỗi application message mang HLC timestamp. UI sort bằng `Before()`.

**Hệ thống clock đầy đủ:**
| Clock | Mục đích |
|---|---|
| **Epoch Number** (logical counter) | MLS state ordering, Token Holder election, Fork Healing |
| **HLC** (hybrid logical) | Application message display ordering |
| **Local wall clock** | Liveness detection (heartbeat, T_timeout), feeds HLC |

**Package `app/coordination/` — ĐÃ IMPLEMENT (54/54 tests PASS):**
*   `interfaces.go` — Contracts: Transport, Clock, MLSEngine (with treeHash returns), CoordinationStorage
*   `types.go` — Data types, wire messages, enums, sentinel errors
*   `config.go` — CoordinatorConfig + DefaultConfig + TestConfig + Validate
*   `clock_real.go` — RealClock (production); FakeClock (test-only, trong `clock_fake_test.go`)
*   `hlc.go` — Hybrid Logical Clock: Now(), Update(), thread-safe, injectable Clock
*   `metrics.go` — Thread-safe instrumentation for Phase 7 evaluation + Snapshot + Reset
*   `active_view.go` — ActiveView: heartbeat tracking, liveness check, eviction, sorted members, onChange
*   `single_writer.go` — ComputeTokenHolder (argmin SHA-256(nodeID||epoch)), BufferProposal, DrainProposals
*   `epoch.go` — ValidateEpoch, EpochTracker, future buffer with defensive copies, Advance returns buffered
*   `fork_healing.go` — CompareBranchWeight (W = MemberCount > CommitHash > TreeHash), ForkDetector
*   `coordinator.go` — Central orchestrator: ties ActiveView + SingleWriter + EpochTracker + ForkDetector + HLC into message processing pipeline. Public API: CreateGroup, Start, Stop, SendMessage, ProposeAdd/Remove/Update
*   `mls_adapter.go` — GrpcMLSEngine: adapts gRPC MLSCryptoServiceClient → MLSEngine interface
*   `testutil_test.go` — FakeNetwork (queue + DrainAll), FakeTransport, MockMLSEngine, MockStorage
*   `coordinator_test.go` — 10 integration tests: group creation, token holder election, message send/receive, proposal/commit, epoch consistency, heartbeats, HLC ordering, fork detection

---

## 4. Current Progress

### Phase 4 Coordination Layer — COMPLETE ✅ (68 tests: 62 Go + 6 Rust)

#### Coordination Mechanisms (54 Go tests)

| File | Tests | Ghi chú |
|------|-------|---------|
| `app/coordination/types.go` | — | HLCTimestamp, Envelope, wire messages, persistence types, enums, sentinel errors |
| `app/coordination/interfaces.go` | — | Transport, Clock, MLSEngine (with treeHash), CoordinationStorage contracts |
| `app/coordination/config.go` | 4/4 ✅ | CoordinatorConfig, DefaultConfig, TestConfig, Validate |
| `app/coordination/clock_real.go` | — | RealClock (production), FakeClock (test-only) |
| `app/coordination/hlc.go` | 8/8 ✅ | HLC engine: Now(), Update(), monotonic + causal ordering |
| `app/coordination/metrics.go` | 4/4 ✅ | Thread-safe counters + latency samples + Snapshot + Reset |
| `app/coordination/active_view.go` | 9/9 ✅ | Peer liveness, heartbeat, eviction, sorted member list, onChange callback |
| `app/coordination/single_writer.go` | 9/9 ✅ | ComputeTokenHolder (argmin SHA-256), BufferProposal, DrainProposals, AdvanceEpoch |
| `app/coordination/epoch.go` | 9/9 ✅ | ValidateEpoch, EpochTracker, future buffer with defensive copies |
| `app/coordination/fork_healing.go` | 10/10 ✅ | CompareBranchWeight (W = MemberCount > CommitHash > TreeHash), ForkDetector |
| `app/coordination/coordinator.go` | 10/10 ✅ | Central orchestrator: message pipeline, periodic tasks, public API |
| `app/coordination/mls_adapter.go` | — | GrpcMLSEngine: gRPC client → MLSEngine interface |
| `app/coordination/testutil_test.go` | — | FakeNetwork, FakeTransport, MockMLSEngine, MockStorage |

#### Infrastructure — Proto + DB + Transport (8 Go tests)

| File | Tests | Ghi chú |
|------|-------|---------|
| `proto/mls_service.proto` | — | 13 RPCs: 4 existing (Phase 2) + 9 new (Phase 4) |
| `app/mls_service/*.pb.go` | — | Auto-generated from proto (protoc) |
| `app/db/db.go` | — | New tables: mls_groups, coordination_state, stored_messages |
| `app/db/coordination_storage.go` | 8/8 ✅ | SQLiteCoordinationStorage: GroupRecord, CoordState, StoredMessage CRUD |
| `app/p2p/transport_adapter.go` | — | LibP2PTransport: GossipSub + direct streams → Transport interface |

#### Real OpenMLS Crypto Engine (6 Rust tests)

| File | Tests | Ghi chú |
|------|-------|---------|
| `crypto-engine/src/mls.rs` | 6/6 ✅ | Stateless persisted `group_state` (serialize/deserialize OpenMLS storage), real OpenMLS 0.8: create_group, encrypt_message, decrypt_message, create_commit (self_update), process_commit, process_welcome, export_secret |
| `crypto-engine/src/main.rs` | — | Stateless gRPC server (no shared group map), all 13 RPC handlers |
| `crypto-engine/Cargo.toml` | — | Added: openmls_basic_credential, ed25519-dalek, serde, serde_json, tls_codec, sha2 |

#### Wails Bindings + Frontend Chat UI

| File | Ghi chú |
|------|---------|
| `app/group_ops.go` | CreateGroupChat, SendGroupMessage, GetGroupMessages, GetGroups, GetGroupStatus, initCoordinationStackLocked, loadExistingGroupsLocked, Wails event handlers |
| `app/app.go` | Added coordination fields (transport, coordStorage, mlsEngine, coordinators map), initCoordinationStackLocked() call in launchP2PNode, stopCoordinatorsLocked() in teardown |
| `app/frontend/src/components/ChatPanel.tsx` | Group creation UI, group tabs, HLC-sorted message list, real-time updates via EventsOn("group:message"), message input |
| `app/frontend/src/screens/DashboardScreen.tsx` | Integrated ChatPanel below existing grid |
| `app/frontend/wailsjs/go/main/App.js` | Added exports: CreateGroupChat, SendGroupMessage, GetGroupMessages, GetGroups, GetGroupStatus |
| `app/frontend/wailsjs/go/main/App.d.ts` | TypeScript declarations for new group chat functions |
| `app/frontend/wailsjs/go/models.ts` | Added MessageInfo, GroupInfo types |

**Grand total: 68 tests PASS (62 Go + 6 Rust), `go vet` clean, `go build ./...` clean, `cargo build` clean, `cargo test` clean, `tsc --noEmit` clean.**

### All files implemented (Phase 1-4):

| File | Trạng thái | Ghi chú |
|------|-----------|---------|
| `proto/mls_service.proto` | ✅ | 13 RPCs: Phase 2 (4) + Phase 4 (9 new) |
| `app/mls_service/*.pb.go` | ✅ | Auto-generated from proto |
| `crypto-engine/src/mls.rs` | ✅ | Real OpenMLS 0.8 stateless engine: persisted `group_state` blob + all group operations |
| `crypto-engine/src/main.rs` | ✅ | Stateless gRPC server (no shared in-memory group map) |
| `app/db/db.go` | ✅ | Tables: system_config, mls_identity, auth_bundle, messages, mls_groups, coordination_state, stored_messages |
| `app/db/coordination_storage.go` | ✅ | SQLiteCoordinationStorage (8 tests) |
| `app/p2p/transport_adapter.go` | ✅ | LibP2PTransport: GossipSub + direct streams |
| `app/coordination/mls_adapter.go` | ✅ | GrpcMLSEngine: gRPC → MLSEngine interface |
| `app/coordination/*.go` | ✅ | All 4 mechanisms + Coordinator orchestrator (54 tests) |
| `app/group_ops.go` | ✅ | **Wails bindings for group chat operations** |
| `app/p2p/identity.go` | ✅ | GetOrCreateIdentity |
| `app/admin/token.go` | ✅ | SignToken, VerifyToken, SerializeBundle, DeserializeBundle |
| `app/admin/admin.go` | ✅ | SetupAdminKey, UnlockAdminKey, CreateInvitationBundle |
| `app/app_state.go` | ✅ | AppState enum, DetermineAppState() |
| `app/p2p/auth.go` | ✅ | OnboardNewUser, GetOnboardingInfo, ImportInvitationBundle, BuildLocalToken |
| `app/p2p/gater.go` | ✅ | AuthGater blacklist-based |
| `app/p2p/auth_protocol.go` | ✅ | /app/auth/1.0.0 handshake |
| `app/p2p/host.go` | ✅ | NewP2PNode với localToken + rootPubKey |
| `app/main.go` | ✅ | Phân nhánh GUI vs CLI |
| `app/cli.go` | ✅ | Config struct + parseCLI() + IsCommand() |
| `app/runner.go` | ✅ | run() CLI orchestration |
| `app/commands.go` | ✅ | cmdAdminSetup, cmdCreateBundle, cmdSetup, cmdImportBundle |
| `app/node.go` | ✅ | startNode, runP2PNode, connectBootstrap, pingLoop |
| `app/crypto_engine.go` | ✅ | startCryptoEngine, waitForCryptoEngine |
| `app/log.go` | ✅ | LogFilterHandler, setupLogging |
| `app/process.go` | ✅ | ProcessManager, StartCryptoEngine, StopCryptoEngine |
| **`app/app.go`** | ✅ | **Wails App struct + coordination stack + all bindings** |
| `app/wails.json` | ✅ | Wails config, frontend:dir = "frontend" |
| `app/frontend/src/App.tsx` | ✅ | Root: polls GetAppState, state-based routing |
| `app/frontend/src/screens/SetupScreen.tsx` | ✅ | UNINITIALIZED |
| `app/frontend/src/screens/AwaitingBundleScreen.tsx` | ✅ | AWAITING_BUNDLE + Admin Quick Setup |
| `app/frontend/src/screens/DashboardScreen.tsx` | ✅ | AUTHORIZED/ADMIN_READY + peer list + **ChatPanel** |
| `app/frontend/src/components/AdminPanel.tsx` | ✅ | Init admin key + Create bundle tabs |
| `app/frontend/src/components/ChatPanel.tsx` | ✅ | **Group chat UI with HLC ordering** |
| `app/frontend/src/components/CopyField.tsx` | ✅ | Copy-to-clipboard field |
| `app/frontend/src/components/StatusBadge.tsx` | ✅ | Colored state pill |
| `app/frontend/src/components/PeerList.tsx` | ✅ | Connected peers table |
| `app/frontend/wailsjs/` | ✅ | Bindings + MessageInfo/GroupInfo types |

### Wails Bindings hiện tại (`app/app.go` + `app/group_ops.go`):

| Method | Mô tả |
|--------|-------|
| `GetAppState() string` | UNINITIALIZED / AWAITING_BUNDLE / AUTHORIZED / ADMIN_READY / ERROR |
| `GetOnboardingInfo() OnboardingInfo` | PeerID + PublicKeyHex |
| `GenerateKeys() OnboardingInfo` | Tạo MLS keypair qua Rust engine |
| `OpenAndImportBundle() error` | File dialog → import bundle → start P2P |
| `HasAdminKey() (bool, error)` | Kiểm tra có admin key không |
| `CreateAndImportSelfBundle(name, passphrase) error` | Admin tự cấp bundle cho mình |
| `InitAdminKey(passphrase) error` | Khởi tạo Root Admin key |
| `CreateBundle(req) (string, error)` | Tạo bundle cho user mới → save dialog |
| `GetNodeStatus() NodeStatus` | State, PeerID, DisplayName, ConnectedPeers |
| **`CreateGroupChat(groupID) error`** | **Tạo MLS group + Coordinator + subscribe GossipSub** |
| **`SendGroupMessage(groupID, text) error`** | **Encrypt + broadcast qua Coordinator** |
| **`GetGroupMessages(groupID) []MessageInfo`** | **Lấy messages sorted by HLC** |
| **`GetGroups() []GroupInfo`** | **Danh sách groups đã tham gia** |
| **`GetGroupStatus(groupID) map[string]interface{}`** | **Epoch, token holder, member count, metrics** |

### CLI Commands hiện tại (từ `app/`):

```powershell
# Lần đầu — User tạo key pair
go run . --setup

# Admin: khởi tạo root key (chỉ chạy 1 lần trên máy Admin)
go run . --admin-setup --admin-passphrase "MySecret"

# Admin: tạo bundle cho user mới
go run . --create-bundle `
  --bundle-name "Alice" `
  --bundle-peer-id "12D3KooW..." `
  --bundle-pub-key "a3f7c2..." `
  --admin-passphrase "MySecret" `
  --bundle-output alice.bundle

# User: import bundle từ Admin
go run . --import-bundle alice.bundle

# Headless mode (không GUI)
go run . --headless
go run . --headless --db mydb.db --p2p-port 4002

# GUI mode (mặc định khi không có flag)
wails dev        # development (hot-reload)
wails build      # production build
```

---

## 5. Lưu ý kỹ thuật quan trọng

*   **Module name:** `module app` (đổi từ `module backend`). Tất cả import paths dùng `"app/..."`.
*   **Wails embed:** `//go:embed all:frontend/dist` trong `app/app.go`. Frontend source ở `app/frontend/`, build output ở `app/frontend/dist/`. KHÔNG thể dùng `../` trong Go embed.
*   **wails generate module:** Chạy từ `app/` sau mỗi lần thêm/sửa exported method trên `App` struct để cập nhật `frontend/wailsjs/`.
*   **protoc command đúng:** `protoc --go_out=. --go-grpc_out=. --proto_path=./proto mls_service.proto` (chạy từ project root)
*   **openmls_rust_crypto version:** phải dùng `0.5` (khớp với `openmls_traits 0.5`)
*   **bootstrap_addr format bắt buộc:** `/ip4/IP/tcp/PORT/p2p/PEERID`
*   **display_name trong MLS credential:** Hiện tại lưu raw UTF-8 bytes. Phase 4 sẽ dùng proper TLS-serialized `BasicCredential`.
*   **ExportIdentity proto (Phase 5):** PHẢI bao gồm `libp2p_private_key` để PeerID được khôi phục khi chuyển thiết bị.
*   **Blacklisting policy:** `rejectSecurity` (có blacklist) chỉ gọi khi `verifyPeerToken` thất bại. `rejectTransient` (không blacklist) cho mọi lỗi IO/timeout. KHÔNG bao giờ blacklist khi `NewStream` fail.
*   **GetNodeStatus mutex:** Gọi `getAppStateUnlocked()` (không acquire mutex) thay vì `GetAppState()` khi đang giữ `a.mu`.

---

## 6. Next Step — Phase 5: Advanced Features

Phase 4 hoàn tất. Hệ thống đã có:
- Real OpenMLS crypto (create group, encrypt/decrypt messages, self-update commit, export secret)
- Full coordination pipeline (Single-Writer, Epoch Consistency, Fork Healing, HLC) — 54 Go tests
- Wails bindings + Chat UI cho manual testing
- 68 tests pass (62 Go + 6 Rust), all builds clean

**Để manual test:**
1. `cd crypto-engine && cargo build --release`
2. `cd app && wails generate module && wails dev`
3. Trong UI: nhập Group ID → "Create / Join" → gõ tin nhắn → xem tin nhắn hiển thị với HLC timestamp
4. Chạy 2 instance trên 2 máy/port để test P2P messaging

**Tiếp theo (Phase 5):**

1.  **Secure Identity Export/Import:** `.backup` file chứa libp2p_private_key + mls_signing_key + credential + invitation_token, mã hóa AES-256-GCM (Argon2id). Hỗ trợ chuyển thiết bị.

2.  **Session Takeover (Single Active Device):** Broadcast `KILL_SESSION` signed message khi import identity trên thiết bị mới.

3.  **Offline Messaging (DHT Store-and-Forward):** `dht.Put(Hash(RecipientID), EncryptedMsg)` cho peer offline. Auto-retrieve on connect.

4.  **Hardening Add Member flow:** bind `newMemberPeerID` with KeyPackage identity before add, and reduce exposure of `key_package_bundle_private` on frontend (prefer local secure handling over clipboard/UI state).

**Lưu ý thiết kế quan trọng:**
*   **KHÔNG DÙNG "smallest hash" nữa** — phương pháp cũ đã bị thay thế bằng Single-Writer Protocol.
*   **GroupState trong Rust:** blob bytes chứa full persisted OpenMLS storage + metadata/signing key. Rust deserialize blob để load group và serialize lại sau mỗi operation (stateless giữa các RPC/process restart).
*   **Coordination Layer chạy hoàn toàn ở Go** — Rust không biết gì về Single-Writer hay Epoch.
*   **Real OpenMLS 0.8:** create_group dùng `MlsGroup::new_with_group_id`, encrypt dùng `group.create_message`, decrypt dùng `group.process_message`. Forward secrecy enforced (sender CANNOT decrypt own messages — own messages stored as plaintext directly).
*   **LibP2PTransport:** Wraps real GossipSub + direct streams qua protocol `/coordination/direct/1.0.0`. Auto-skips messages from self.
*   **Shared transport:** All Coordinators share a single `LibP2PTransport` instance.
