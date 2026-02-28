# Current State of SECURE PRIVATE P2P COMMUNICATION SYSTEM Project

This document serves as a short-term memory for the AI Agent.

## 1. Project Overview
A serverless, zero-trust P2P communication platform using Go (Wails) and Rust (OpenMLS).

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

**AuthGater:** Blacklist-based. `Blacklist` CHỈ được gọi khi `verifyPeerToken` thất bại. KHÔNG gọi khi `NewStream` fail — sẽ block reconnect của peer hợp lệ.

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

---

## 4. Current Progress

### Files đã implement (Phase 3 + Wails GUI):

| File | Trạng thái | Ghi chú |
|------|-----------|---------|
| `proto/mls_service.proto` | ✅ | GenerateIdentity, Ping |
| `app/mls_service/*.pb.go` | ✅ | Auto-generated |
| `crypto-engine/src/mls.rs` | ✅ | generate_identity, credential rỗng |
| `app/db/db.go` | ✅ | Tables: system_config, mls_identity, auth_bundle, messages |
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
| **`app/app.go`** | ✅ | **Wails App struct + tất cả bindings** |
| `app/wails.json` | ✅ | Wails config, frontend:dir = "frontend" |
| `app/frontend/src/App.tsx` | ✅ | Root: polls GetAppState, state-based routing |
| `app/frontend/src/screens/SetupScreen.tsx` | ✅ | UNINITIALIZED |
| `app/frontend/src/screens/AwaitingBundleScreen.tsx` | ✅ | AWAITING_BUNDLE + Admin Quick Setup |
| `app/frontend/src/screens/DashboardScreen.tsx` | ✅ | AUTHORIZED/ADMIN_READY + peer list |
| `app/frontend/src/components/AdminPanel.tsx` | ✅ | Init admin key + Create bundle tabs |
| `app/frontend/src/components/CopyField.tsx` | ✅ | Copy-to-clipboard field |
| `app/frontend/src/components/StatusBadge.tsx` | ✅ | Colored state pill |
| `app/frontend/src/components/PeerList.tsx` | ✅ | Connected peers table |
| `app/frontend/wailsjs/` | ✅ | Auto-generated bindings (wails generate module) |

### Wails Bindings hiện tại (`app/app.go`):

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
*   **Blacklisting policy:** `Blacklist` chỉ gọi khi `verifyPeerToken` thất bại, KHÔNG gọi khi `NewStream` fail.
*   **GetNodeStatus mutex:** Gọi `getAppStateUnlocked()` (không acquire mutex) thay vì `GetAppState()` khi đang giữ `a.mu`.

---

## 6. Next Step — Phase 4: MLS Secure Group Chat

Xem `PROJECT_PLAN.md` section 4 để biết chi tiết.

Bước đầu tiên của Phase 4:
1. Cập nhật `proto/mls_service.proto` với các gRPC methods: `CreateGroup`, `AddMember`, `RemoveMember`, `EncryptMessage`, `DecryptMessage`, `ProcessCommit`
2. Thiết kế bảng `mls_groups` trong SQLite
3. Implement Rust handlers stateless
4. **Lưu ý dMLS:** MLS chuẩn cần Delivery Service tập trung. Project này dùng approach "Deterministic Conflict Resolution": khi có concurrent commits, tất cả node buffer lại và chọn commit có hash nhỏ nhất. Commit thua bị roll back.
