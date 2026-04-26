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
*   Tích hợp Wails v2.11.0: composition root `app/main.go`, bindings trên `app/service.Runtime`, `app/adapter/wailsui` gọi `wails.Run` + `EventSink`.
*   Scaffold React + TypeScript + Tailwind frontend tại `app/frontend/`.
*   4 màn hình dev/test UI đã hoàn chỉnh.

### Hexagonal layout (Go) ✅
*   `app/domain`, `app/port`, `app/adapter/{store,p2p,sidecar,wailsui}`, `app/service`, `app/config`, `app/cli`, `app/pkg/log`; SQLite tại `adapter/store/`, libp2p tại `adapter/p2p/`, Rust sidecar tại `adapter/sidecar/`.

### Agent — Bản đồ mã nguồn & Wails (đọc trước khi sửa) 📌

| Vùng | Đường dẫn | Ghi chú |
|------|-----------|---------|
| Composition root | `app/main.go` | `config.Parse()`, nhánh `cli.Run` vs `wailsui.Run`; `//go:embed all:frontend/dist` |
| Cấu hình flag | `app/config` | `Config`, `Parse()`, `IsCommand()` — dùng chung cho `main` và `service` (**không** đặt `Parse` trong `cli` để tránh import cycle `service` ↔ `cli`) |
| CLI | `app/cli` | `runner.go`, `commands.go` — gọi `service.*` (headless node, backup, bundle, …) |
| Ứng dụng + Wails | `app/service` | `Runtime`; lifecycle export: `Startup`, `DomReady`, `BeforeClose`, `Shutdown`. File theo nghiệp vụ: `runtime.go`, `identity.go`, `admin.go`, `node_status.go`, `group.go`, `messaging.go`, `invite.go`, `session.go`, `app_state.go`, `identity_backup.go`, `bootstrap.go`, `cli_node.go`, `events.go` |
| SQLite | `app/adapter/store` | Thay cho `app/db` cũ |
| P2P | `app/adapter/p2p` | Thay cho `app/p2p` cũ |
| Crypto gRPC | `app/adapter/sidecar` | `StartCryptoEngine`, `NewMLSEngine` — thay cho `coordination/mls_adapter.go` + `crypto_engine.go` cũ |
| Wails vỏ | `app/adapter/wailsui` | `Run`, `EventSink` → `runtime.EventsEmit` |
| Log | `app/pkg/log` | `Setup(headless)` |
| Hex ports / domain | `app/port`, `app/domain` | Interfaces + kiểu thuần |
| Coordination protocol | `app/coordination` | Chỉ logic giao thức; **không** gắn binary Rust trực tiếp |

**Wails → TypeScript (quan trọng):** Bind target là `*service.Runtime`. Codegen tạo `frontend/wailsjs/go/service/Runtime.js|.d.ts` và `models.ts` với namespace **`service`** (không còn `main`). Import FE: `from '.../wailsjs/go/service/Runtime'`, `import { service } from '.../wailsjs/go/models'`. Sau đổi API Go: `cd app && wails generate module`, rồi `npm run build` trong `frontend/` nếu cần.

**Đường dẫn lịch sử (không dùng nữa):** `backend/`, `app/app.go`, `app/group_ops.go`, `app/node.go`, `app/commands.go`, `frontend/wailsjs/go/main/App`, `app/db`, `app/p2p` (root).

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
// app/adapter/p2p/auth_protocol.go — verifyPeerToken()
if token.PeerID != authenticatedPeerID.String() {
    reject() // Eve không có Libp2p private key của Alice → bị lộ ngay
}
```

### 3e. Auth Protocol — `/app/auth/1.0.0`

**Wire format (current):** `[4 bytes big-endian uint32: JSON length][JSON bytes of AuthHandshakeMsg]`

```go
type AuthHandshakeMsg struct {
  Token   *InvitationToken
  Session SessionClaim // started_at + nonce + MLS-signed proof
}
```

Backward compatibility: parser still accepts legacy token-only payload.

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
├── invitation_token     ← token Admin đã ký
├── mls_groups           ← serialized group_state snapshot
├── stored_messages      ← local decrypted chat history
├── kp_bundles           ← pending invite private material
└── pending_welcomes_out ← welcomes chưa giao
```
Implementation note: backup/import is now fully handled in Go (no Rust `ExportIdentity/ImportIdentity` RPC required).

### 3i. Wails Integration — Kiến trúc GUI (cập nhật sau refactor)

**Nguyên tắc:** Wails binding — UI gọi exported methods trên `*service.Runtime`. Codegen: `frontend/wailsjs/go/service/Runtime.d.ts` + `Runtime.js`; DTO trong `frontend/wailsjs/go/models.ts` (namespace `service`).

**Phân nhánh CLI vs GUI trong `app/main.go`:**
```go
cfg := config.Parse()
if cfg.Headless || cfg.IsCommand() {
    cli.Run(cfg) // app/cli
} else {
    wailsui.Run(cfg, distFS) // app/adapter/wailsui — embed frontend/dist
}
```

**`IsCommand()`:** xem `app/config/config.go` (setup, admin, bundle, import/export identity, …).

**Wails lifecycle:** `Runtime.Startup` → DB + identity + sidecar + P2P nếu AUTHORIZED/ADMIN_READY; `Runtime.Shutdown` → teardown. `adapter/wailsui` gắn `EventSink` để `Runtime.emit` → `EventsEmit`.

**State chính (thread-safe, `mu sync.Mutex`):** `ctx`, `cfg`, `db`, `privKey`, `mlsClient`, `conn`, `stopEngine`, `node`, `transport`, `coordStorage`, `mlsEngine`, `coordinators`, … — định nghĩa trong `app/service/runtime.go`.

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

**Package `app/coordination/` — ĐÃ IMPLEMENT (`go test ./coordination` — PASS; ~63 hàm `Test*` + subtests table-driven):**
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
*   **MLS ↔ gRPC:** `GrpcMLSEngine` nằm ở `app/adapter/sidecar/engine.go` (implements `coordination.MLSEngine`) — không còn file `app/coordination/mls_adapter.go`.
*   `testutil_test.go` — FakeNetwork (queue + DrainAll), FakeTransport, MockMLSEngine, MockStorage
*   `coordinator_test.go` — 10 integration tests: group creation, token holder election, message send/receive, proposal/commit, epoch consistency, heartbeats, HLC ordering, fork detection

---

## 4. Current Progress

### Phase 5.1 + 5.2 + 5.3 (Identity migration + session takeover + offline messaging) — COMPLETE ✅

#### Implemented in this update

- **MLS Atomic Apply (consistency hardening):**
  - Root cause fixed: `SQLITE_BUSY` during `SaveMessage` could previously leave MLS message-key consumption and DB persistence out of sync, causing replay-time `SecretReuseError`.
  - New atomic persistence API in coordination storage (`app/coordination/interfaces.go`):
    - `ApplyCommit(...)`
    - `ApplyApplication(...)`
  - SQLite implementation (`app/adapter/store/coordination_storage.go`) now persists group-state/message/applied-marker/envelope-log in one transaction for commit/application apply paths.
  - Coordinator refactor (`app/coordination/coordinator.go`):
    - in-memory state is updated only after atomic DB apply succeeds;
    - local send/commit paths persist first, then publish/emit;
    - inbound replay paths share the same idempotent apply boundary.
  - PeerID serialization cleanup:
    - canonical `peer.ID.String()` in envelope sender and storage write paths;
    - decode fallback for legacy/non-canonical values remains in read paths.
  - Regression coverage:
    - atomic apply/idempotency tests in `app/adapter/store/coordination_storage_test.go`;
    - coordinator guard test for persist-failure non-advancement in `app/coordination/coordinator_test.go`.

- **SQLite contention hardening:**
  - `app/adapter/store/db.go` now sets `WAL`, `busy_timeout`, and a single-writer pool (`SetMaxOpenConns(1)`) to reduce write contention bursts under concurrent goroutines.

- **Dev multi-instance scripts (Windows) now default to latest build:**
  - `scripts/dev-second-instance.ps1`, `scripts/dev-third-instance.ps1`, `scripts/dev-fourth-instance.ps1` support `-AutoBuild` (default true).
  - Default launch behavior: run `wails build` before starting node 2/3/4 exe; can disable via `-AutoBuild:$false`.

- **Identity backup/import (Phase 5.1, Go-side):**
  - Implementation: `app/service/identity_backup.go` (+ tests `identity_backup_test.go`)
    - `ExportIdentityBackup(...)` and `ImportIdentityBackup(...)`
    - Wire format: `[16B salt][12B nonce][AES-GCM ciphertext]`
    - Argon2id params aligned with admin key encryption.
  - Backup format v2: identity + snapshot (`mls_groups`, `stored_messages`, `kp_bundles`, `pending_welcomes_out`); import tương thích v1.
  - CLI: `app/cli/commands.go` + `runner.go` (`--export-identity`, `--import-identity`, …).
  - Wails: `Runtime.ExportIdentity`, `Runtime.ImportIdentityFromFile`
  - Side effects sau import: `service.ApplyIdentityImportSideEffects` (`session.go`)

- **Session takeover hardening (Phase 5.2, auth-handshake session claim model):**
  - `app/adapter/p2p/session_claim.go` + tests; auth wire trong `app/adapter/p2p/auth_protocol.go` (`AuthHandshakeMsg { token, session }`, tương thích token-only cũ).
  - Session keys / flags: `app/service/session.go` (`buildLocalAuthHandshake`, `resetSessionStartedAt`, `killSessionPendingConfigKey`, …).
  - Node startup: `app/service/runtime.go` (`launchP2PNode`) + `app/adapter/p2p/host.go`.

- **Offline messaging (Phase 5.3, store-and-forward):**
  - Offline sync stream + ACK cursor: `app/service/offline_sync.go`, `app/adapter/p2p/offline_wire.go`.
  - Envelope log + sync ack persistence: `app/adapter/store/coordination_storage.go`, schema `envelope_log` / `envelope_dedup` / `sync_acks` / `pending_delivery_acks` / `offline_sync_pull_state` trong `app/adapter/store/db.go`.
  - Runtime trigger: peer connect + manual trigger (`TriggerOfflineSync`) để pull missed envelopes từ connected verified peers.
  - `scheduleOfflineSyncPull` thêm retry/backoff ngắn để tránh race lúc peer vừa connect nhưng chưa advertise protocol.
  - Đã bỏ hoàn toàn DHT mailbox data-path (`app/adapter/p2p/offline_dht.go`, `offlineDHTPushLoop`, `offlineDHTCheckLoop`).

- **Invite offline store (KeyPackage / Welcome) — post-refactor:**
  - DHT application-data path đã loại bỏ (`app/adapter/p2p/kp_dht.go` removed).
  - Custom store protocols: `/app/kp-store/1.0.0`, `/app/kp-fetch/1.0.0`, `/app/welcome-store/1.0.0`, `/app/welcome-fetch/1.0.0` (wire: `app/adapter/p2p/invite_store_wire.go`).
  - `invite.go` vẫn replicate qua store streams (fanout mặc định 3), đồng thời publish blind-store object để tăng xác suất lưu hộ khi peer đích offline.
  - `CheckDHTWelcome` giữ tên để tương thích Wails UI cũ, nhưng implementation hiện fetch từ store peers (không còn gọi DHT).

- **Universal Blind-Store layer (new):**
  - Topic global: `/org/offline-store/v1` (`app/service/blind_store.go`).
  - Object types: `group-envelope`, `key-package`, `welcome`.
  - Runtime policy: regular nodes subscribe blind-store by default and only retain targeted replica objects; `--store-node` retains all objects; `--blind-store-participant=false` explicitly opts out (`app/config/config.go`).
  - Replica selection: ưu tiên Kademlia `GetClosestPeers(routingKey)` + fallback XOR-distance; chỉ nhận từ verified peers.
  - Coordinator hook: `OnEnvelopeBroadcast` publish `MsgCommit/MsgApplication` sang blind-store khi local node broadcast.

#### Validation

- `go test ./...` PASS
- `go vet ./...` PASS
- `go build ./...` PASS

### Phase 4 Coordination Layer — COMPLETE ✅ (xác minh: `cd app && go test ./...`; `cd crypto-engine && cargo test`)

**Coordination:** `app/coordination/*.go` — Transport, Clock, MLSEngine (interface), Coordinator, HLC, fork healing. **Không** chứa binary Rust; bridge MLS: `app/adapter/sidecar/engine.go`.

**SQLite + transport:** `app/adapter/store` (`db.go`, `coordination_storage.go` — 8 tests store), `app/adapter/p2p/transport_adapter.go` (LibP2PTransport).

**Rust:** `crypto-engine/` — OpenMLS stateless qua gRPC (xem `crypto-engine/src/mls.rs`).

**Wails + FE:** Methods trên `app/service.Runtime` (tách file: `group.go`, `messaging.go`, `invite.go`, …). Bindings TS: `frontend/wailsjs/go/service/Runtime.*`, models namespace `service`. UI: `frontend/src/**/*.tsx` (import Runtime, không dùng `go/main/App`).

**Kiểm tra nhanh:** `cd app && go vet ./... && go test ./...` ; `cd frontend && npm run build`.

### Wails bindings (receiver: `*service.Runtime`)

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
| `GetGroupMembers`, `AddMemberToGroup`, `JoinGroupWithWelcome`, `GenerateKeyPackage` | MLS / invite UI (ChatPanel) |
| `InvitePeerToGroup`, `CheckDHTWelcome`, `GetKPStatus` | Luồng invite offline-friendly (store-peer based; `CheckDHTWelcome` là tên legacy API) |
| `ExportIdentity`, `ImportIdentityFromFile` | Backup `.backup` (GUI) |

*(Danh sách đầy đủ: `wails generate module` → `Runtime.d.ts`.)*

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

*   **Module name:** `module app`. Import nội bộ: `"app/..."`.
*   **Wails embed:** `//go:embed all:frontend/dist` trong `app/main.go` (composition root). Source: `app/frontend/src/`; build Vite: `app/frontend/dist/`. Repo có thể có `dist/index.html` tối thiểu để `go build` không lỗi embed khi chưa `npm run build`.
*   **wails generate module:** Chạy từ `app/` sau khi thêm/sửa exported method trên `service.Runtime` → cập nhật `frontend/wailsjs/go/service/Runtime*` và `models.ts`. Đồng bộ import TS (`service/Runtime`, namespace `service`).
*   **protoc command đúng:** `protoc --go_out=. --go-grpc_out=. --proto_path=./proto mls_service.proto` (chạy từ project root)
*   **openmls_rust_crypto version:** phải dùng `0.5` (khớp với `openmls_traits 0.5`)
*   **bootstrap_addr format bắt buộc:** `/ip4/IP/tcp/PORT/p2p/PEERID`
*   **display_name trong MLS credential:** Hiện tại lưu raw UTF-8 bytes; có thể nâng cấp sau sang TLS-serialized `BasicCredential` nếu cần.
*   **Backup `.backup` (Phase 5.1):** Payload **phải** gồm `libp2p_private_key` (cùng các trường MLS/bundle) để khôi phục đúng PeerID — đã implement trong Go (`identity_backup.go`), không phụ thuộc RPC `ExportIdentity`/`ImportIdentity` của proto.
*   **Blacklisting policy:** `rejectSecurity` (có blacklist) chỉ gọi khi `verifyPeerToken` thất bại. `rejectTransient` (không blacklist) cho mọi lỗi IO/timeout. KHÔNG bao giờ blacklist khi `NewStream` fail.
*   **GetNodeStatus mutex:** Khi đã giữ lock `Runtime.mu`, dùng `getAppStateUnlocked()` thay vì gọi lại `GetAppState()` (tránh deadlock).

---

## 6. Next Step — Backend Productization → Frontend → File Transfer

Phase 4 hoàn tất. **Phase 5.1 (`.backup`), 5.2 (`SessionClaim` / single active device), 5.3 (offline store-and-forward)** đã implement — xem §4.

Hệ thống đã có:
- OpenMLS (nhóm, tin nhắn, KeyPackage / AddMembers / Welcome, …) qua sidecar
- Coordination đầy đủ (Single-Writer, Epoch, Fork healing, HLC); **`go test ./...`** và **`cargo test`** để xác minh
- Offline sync + blind-store nền tảng cho envelope / KeyPackage / Welcome
- Identity migration `.backup` + session claim foundation

**Roadmap tài liệu hiện tại:**
- `PROJECT_PLAN.md`: roadmap tổng thể đã đổi thành Phase 6 backend productization, Phase 7 frontend, Phase 8 file transfer, Phase 9 evaluation.
- `BACKEND_IMPLEMENTATION_PLAN.md`: kế hoạch backend chi tiết cần làm trước frontend.
- `FRONTEND_IMPLEMENTATION_PLAN.md`: đặc tả màn hình / luồng UI production-ready.

**Để manual test core hiện tại:**
1. `cd crypto-engine; cargo build --release`
2. `cd app; wails generate module; wails dev` (hoặc `go run . --headless` cho CLI)
3. Trong UI: nhập Group ID → Create / Join → gửi tin nhắn; kiểm tra HLC sort
4. Hai instance (hai DB / port) để thử P2P

**Tiếp theo (ưu tiên):**

1.  **Phase 6 — Backend Productization trước frontend (P0):**
    - Invite / Pending Invite lifecycle: `GenerateJoinCode`, `ListPendingInvites`, `AcceptInvite`, `RejectInvite`.
    - Group membership lifecycle: `LeaveGroup`, `RemoveMemberFromGroup` hoặc policy disable rõ ràng nếu defer role/remove.
    - Session takeover lifecycle: `GetSessionStatus`, event `session:replaced`, local replaced/lockout state.
    - Startup/runtime health: `GetRuntimeHealth`, startup progress/error events, P2P status events.
    - Admin issuance readiness: parse `request.json`, validate PeerID/PublicKey, admin signing flow rõ ràng.

2.  **Phase 6 — Backend Productization (P1, có thể làm song song frontend):**
    - Network/bootstrap runtime controls: local multiaddr, validate/set bootstrap, reconnect.
    - Diagnostics snapshot/export logs cho Developer Mode.
    - Message status/retry model nếu UI cần failed-message recovery.
    - Admin issuance history nếu audit table vào scope.

3.  **Phase 7 — Frontend Application UI:**
    - Rebuild UI từ dev/test sang product UI theo `FRONTEND_IMPLEMENTATION_PLAN.md`.
    - Không fake critical backend behavior; nếu backend gap còn thiếu thì disable/mark planned rõ ràng.

4.  **Phase 8–9:** file transfer (MLS exporter / swarming), evaluation đa node / partition / báo cáo luận văn.

**Lưu ý thiết kế quan trọng:**
*   **KHÔNG DÙNG "smallest hash" nữa** — phương pháp cũ đã bị thay thế bằng Single-Writer Protocol.
*   **GroupState trong Rust:** blob bytes chứa full persisted OpenMLS storage + metadata/signing key. Rust deserialize blob để load group và serialize lại sau mỗi operation (stateless giữa các RPC/process restart).
*   **Coordination Layer chạy hoàn toàn ở Go** — Rust không biết gì về Single-Writer hay Epoch.
*   **Real OpenMLS 0.8:** create_group dùng `MlsGroup::new_with_group_id`, encrypt dùng `group.create_message`, decrypt dùng `group.process_message`. Forward secrecy enforced (sender CANNOT decrypt own messages — own messages stored as plaintext directly).
*   **LibP2PTransport:** Wraps real GossipSub + direct streams qua protocol `/coordination/direct/1.0.0`. Auto-skips messages from self.
*   **Shared transport:** All Coordinators share a single `LibP2PTransport` instance.
