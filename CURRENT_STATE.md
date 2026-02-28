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
*   Xem chi tiết bên dưới.

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
  1. backend --setup
     → GetOrCreateLibp2pIdentity() → PeerID (đã có từ Phase 2)
     → Rust GenerateIdentity() → MLS keypair (credential rỗng ban đầu)
     → Output: PeerID + MLS_PubKey_hex (KHÔNG có tên — Admin sẽ đặt)

[Máy Admin]
  2. Nhận PeerID_Alice + MLS_PubKey_Alice (Zalo/email)
  3. backend --create-bundle \
       --bundle-name "Alice" \          ← Admin đặt tên cho user
       --bundle-peer-id <PeerID> \
       --bundle-pub-key <PubKeyHex> \
       --admin-passphrase "secret" \
       --bundle-output alice.bundle
  4. Gửi alice.bundle cho Alice (out-of-band)

[Máy User]
  5. backend --import-bundle alice.bundle
     → Verify: chữ ký Admin + PeerID binding + PublicKey binding + expiry
     → SaveAuthBundle() vào SQLite
     → UpdateMLSDisplayName("Alice") ← tên từ token ghi đè vào mls_identity
     → App → StateAuthorized

  6. backend (chạy bình thường)
     → Load bundle → BuildLocalToken() → NewP2PNode() với auth
     → Kết nối bootstrap_addr từ bundle
     → Auth handshake với mọi peer
```

### 3c. Cấu trúc InvitationToken và InvitationBundle

```go
// backend/admin/token.go
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

**bootstrap_addr PHẢI có `/p2p/PeerID`** — thiếu PeerID thì Noise Protocol không thể xác thực danh tính bootstrap node. Đây là bug đã được phát hiện và sửa trong session này.

### 3d. Bảo vệ chống Token Replay / Spoofing Attack

*   **Vấn đề:** Eve nghe lén, copy token JSON của Alice, dùng token đó kết nối vào Charlie.
*   **Giải pháp — PeerID binding trong auth handshake:**
    ```go
    // backend/p2p/auth_protocol.go — verifyPeerToken()
    if token.PeerID != authenticatedPeerID.String() {
        // authenticatedPeerID = stream.Conn().RemotePeer()
        // Noise Protocol đã chứng minh mật mã: bên kia SỞ HỮU private key của PeerID đó
        reject() // Eve không có Libp2p private key của Alice → bị lộ ngay
    }
    ```

### 3e. Auth Protocol — `/app/auth/1.0.0`

**Wire format:** `[4 bytes big-endian uint32: JSON length][JSON bytes of InvitationToken]`

**Quy tắc tránh deadlock:**
*   **Client (outbound, gọi `InitiateHandshake`):** SEND token trước → READ token peer
*   **Server (inbound, `handleIncoming` qua `SetStreamHandler`):** READ token peer trước → SEND token

**Network Notifee:** `authNetworkNotifee` trigger `InitiateHandshake` chỉ với **outbound connections** (`c.Stat().Direction == network.DirOutbound`). Inbound connections tự được server phục vụ qua stream handler.

**AuthGater:** Blacklist-based. Peer mới được phép qua. Sau khi fail **token verification** → bị `Blacklist()` → chặn vĩnh viễn ở `InterceptSecured` và `InterceptUpgraded`.
**QUAN TRỌNG:** `Blacklist` KHÔNG được gọi khi `NewStream` fail (lỗi mạng tạm thời). Chỉ gọi khi `verifyPeerToken` fail. Gọi sớm sẽ block peer hợp lệ vừa reconnect.

### 3f. App States — THIẾT KẾ CUỐI CÙNG

```
StateUninitialized  → Chưa có MLS keypair → Chạy: backend --setup
StateAwaitingBundle → Có keypair, chưa có bundle → Gửi PeerID + PubKey cho Admin, chờ bundle
StateAuthorized     → Có bundle hợp lệ → Kết nối P2P + auth handshake
StateAdminReady     → StateAuthorized + có root admin key → Có thể tạo bundle cho người khác
```

### 3g. display_name — Admin là người cấp, không phải user

*   `display_name` trong `InvitationToken` do **Admin đặt** — đây là tên chính thức.
*   Khi user chạy `--setup`, credential MLS ban đầu **rỗng**.
*   Khi user chạy `--import-bundle`, hàm `ImportInvitationBundle` tự động gọi `database.UpdateMLSDisplayName(token.DisplayName)` để ghi tên Admin đặt vào DB.
*   Hai user khác nhau dù có cùng `display_name` vẫn có keypair khác nhau hoàn toàn (CSPRNG).
*   Định danh kỹ thuật thực sự: **PeerID** (mạng) và **MLS PublicKey** (crypto).

### 3h. CRITICAL — Device Migration phải export cả Libp2p Private Key

**Vấn đề:** `InvitationToken` chứa `PeerID` của máy cũ. Máy mới nếu tạo PeerID mới sẽ không khớp → fail auth handshake.

**Giải pháp bắt buộc cho Phase 5:** File `.backup` phải chứa **cả hai** loại private key:

```
identity.backup (mã hóa AES-256-GCM + Argon2id)
├── libp2p_private_key   ← BẮT BUỘC — để PeerID giống hệt trên máy mới
├── mls_signing_key      ← MLS private key
├── mls_credential       ← display_name bytes (sau khi import bundle)
└── invitation_token     ← token Admin đã ký
```

**Admin chuyển thiết bị:** Chỉ cần copy file `admin.db` sang máy mới. Tất cả key đều nằm trong DB (Root Admin Key đã mã hóa bằng passphrase). Không cần tool riêng.

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

---

## 4. Current Progress — Phase 3 COMPLETED

### Files đã implement (Phase 3 + Refactor):

| File | Trạng thái | Ghi chú |
|------|-----------|---------|
| `proto/mls_service.proto` | ✅ | `GenerateIdentityRequest` có `display_name` (bị ignore), Response có `public_key`, `signing_key_private`, `credential` |
| `backend/mls_service/*.pb.go` | ✅ | Regenerated bằng `protoc --go_out=. --go-grpc_out=.` |
| `crypto-engine/Cargo.toml` | ✅ | `openmls_rust_crypto=0.5`, `openmls_traits=0.5` |
| `crypto-engine/src/mls.rs` | ✅ | `generate_identity()` không nhận display_name, trả credential rỗng |
| `crypto-engine/src/main.rs` | ✅ | Handler bỏ qua display_name từ request |
| `backend/db/db.go` | ✅ | Bảng `mls_identity` + `auth_bundle` (có cột `peer_id`), methods: Save/Get/Has + `UpdateMLSDisplayName` |
| `backend/p2p/identity.go` | ✅ | Dùng `SetConfig/GetConfig` thay inline SQL |
| `backend/admin/token.go` | ✅ | `InvitationToken`, `InvitationBundle`, `SignToken`, `VerifyToken`, `SerializeBundle`, `DeserializeBundle` |
| `backend/admin/admin.go` | ✅ | `SetupAdminKey`, `UnlockAdminKey`, `CreateInvitationBundle` (Argon2id + AES-256-GCM) |
| `backend/app_state.go` | ✅ | `AppState` enum, `DetermineAppState()` |
| `backend/p2p/auth.go` | ✅ | `OnboardNewUser`, `GetOnboardingInfo` → `OnboardingInfo{PeerID, PublicKeyHex}`, `ImportInvitationBundle` (4 checks + `UpdateMLSDisplayName`), `BuildLocalToken` |
| `backend/p2p/gater.go` | ✅ | `AuthGater` blacklist-based implement `network.ConnectionGater` |
| `backend/p2p/auth_protocol.go` | ✅ | `AuthProtocol`, `handleIncoming` (server), `InitiateHandshake` (client), `authNetworkNotifee` |
| `backend/p2p/host.go` | ✅ | `NewP2PNode` có tham số `localToken` + `rootPubKey`, tích hợp gater + auth |
| **`backend/main.go`** | ✅ | **Thin entry point — 11 dòng** (refactored) |
| **`backend/cli.go`** | ✅ | `Config` struct + `parseCLI()` |
| **`backend/runner.go`** | ✅ | `run()` — orchestration + dispatch |
| **`backend/commands.go`** | ✅ | Command handlers: `cmdAdminSetup`, `cmdCreateBundle`, `cmdSetup`, `cmdImportBundle` + print helpers |
| **`backend/node.go`** | ✅ | `startNode`, `runP2PNode`, `connectBootstrap`, `joinChatRoom`, `pingLoop`, `waitForShutdown`, `writeBootstrapFile` |
| **`backend/crypto_engine.go`** | ✅ | `startCryptoEngine` (returns stopFn), `waitForCryptoEngine` |
| **`backend/log.go`** | ✅ | `LogFilterHandler`, `setupLogging` |

### CLI Commands hiện tại:

```powershell
# Lần đầu — User tạo key pair
backend --setup

# Admin: khởi tạo root key (chỉ chạy 1 lần trên máy Admin)
backend --admin-setup --admin-passphrase "MySecret"

# Admin: tạo bundle cho user mới
backend --create-bundle `
  --bundle-name "Alice" `
  --bundle-peer-id "12D3KooW..." `
  --bundle-pub-key "a3f7c2..." `
  --admin-passphrase "MySecret" `
  --bundle-output alice.bundle

# User: import bundle từ Admin
backend --import-bundle alice.bundle

# Chạy bình thường (StateAuthorized hoặc AdminReady)
backend
backend --db mydb.db --p2p-port 4002
```

---

## 5. Lưu ý kỹ thuật quan trọng

*   **protoc command đúng:** `protoc --go_out=. --go-grpc_out=. --proto_path=./proto mls_service.proto` (chạy từ project root, KHÔNG dùng `--go_out=./backend`)
*   **openmls_rust_crypto version:** phải dùng `0.5` (khớp với `openmls_traits 0.5` mà `openmls 0.8.0` cần), KHÔNG dùng `0.2`
*   **bootstrap_addr format bắt buộc:** `/ip4/IP/tcp/PORT/p2p/PEERID` — thiếu phần `/p2p/PEERID` thì Noise không xác thực được peer
*   **display_name trong MLS credential:** Hiện tại lưu raw UTF-8 bytes. Phase 4 sẽ dùng proper TLS-serialized `BasicCredential`.
*   **golang.org/x/crypto:** Đã được promote lên direct dependency trong `go.mod` (dùng Argon2id trong admin.go)
*   **ExportIdentity proto (Phase 5):** PHẢI bao gồm `libp2p_private_key` để PeerID được khôi phục khi chuyển thiết bị
*   **`process.go` StdoutPipe/StderrPipe:** Phải kiểm tra lỗi trước khi tạo Scanner. Scanner với `nil` pipe → panic. Goroutines quét log PHẢI khởi động **sau** `cmd.Start()`.
*   **Blacklisting policy trong `auth_protocol.go`:** `Blacklist` chỉ được gọi khi `verifyPeerToken` thất bại. Không gọi khi `NewStream` fail (transient error) — sẽ block reconnect của peer hợp lệ.
*   **Refactor `main.go`:** Tách thành 6 file đơn trách nhiệm: `cli.go`, `log.go`, `crypto_engine.go`, `commands.go`, `node.go`, `runner.go`. `main.go` chỉ còn ~11 dòng.

---

## 6. Next Step — Phase 4: MLS Secure Group Chat

Xem `PROJECT_PLAN.md` section 4 để biết chi tiết.

Bước đầu tiên của Phase 4:
1. Cập nhật `proto/mls_service.proto` với các gRPC methods: `CreateGroup`, `AddMember`, `RemoveMember`, `EncryptMessage`, `DecryptMessage`, `ProcessCommit`
2. Thiết kế data model MLS state trong SQLite (`mls_groups` table)
3. Implement Rust handlers (stateless — Go truyền state vào, Rust tính toán, trả state mới)
