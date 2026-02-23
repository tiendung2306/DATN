# Current State of SECURE PRIVATE P2P COMMUNICATION SYSTEM Project

This document serves as a short-term memory for the AI Agent.

## 1. Project Overview
A serverless, zero-trust P2P communication platform using Go (Wails) and Rust (OpenMLS).

## 2. Completed Tasks

### Phase 1: System Architecture & Setup (Completed)
*   Monorepo, Sidecar lifecycle, gRPC IPC, and CGO-free SQLite.

### Phase 2: P2P Networking Layer (Completed)
*   **Persistent PeerID:** Identity is stored in SQLite `system_config` table.
*   **Resilient IP Detection (Hybrid):**
    *   Uses a UDP trick (8.8.8.8) to find the primary internet-facing IP.
    *   **Failover:** If offline, it scans and filters network interfaces, ignoring virtual ones (Docker, WSL, VMWare) to find a valid LAN IP.
*   **Libp2p Host:** Bound to the specific "best" IP found, improving stability and mDNS accuracy.
*   **Log Noise Suppression (2-Layer):**
    *   **Layer 1:** Set libp2p `mdns` log level to `error` via `github.com/ipfs/go-log/v2`.
    *   **Layer 2:** Custom `LogFilterHandler` for `slog` to intercept and drop annoying "no such interface" warnings common on Windows with virtual adapters.
*   **Hybrid Discovery:** mDNS (Local) + Kademlia DHT (Global) + Dynamic Bootstrap (Docker/Manual).
*   **GossipSub:** Global chat topic `/org/chat/global` implemented and tested with periodic Pings.

## 3. Technical Decisions & Knowledge

### Phase 1 & 2
*   **Windows mDNS Issue:** Resolved by binding the Host to a specific IP and filtering out virtual adapters.
*   **Identity Stability:** PeerID persistence is achieved by storing the Marshaled Private Key in the DB.
*   **Headless Mode:** Essential for Docker testing and server-side operations.

### Phase 3 (Design Finalized — Not Yet Implemented)

#### 3a. Hai loại Identity — KHÔNG nhầm lẫn
| Identity | Layer | Ai sinh | Ai quản lý | Mục đích |
|---|---|---|---|---|
| **Libp2p PeerID** | Mạng (P2P) | Go (crypto.GenerateKeyPair) | Go + SQLite | Định danh node trên mạng, mã hóa kênh Noise |
| **MLS Identity** | Ứng dụng (E2EE) | **Rust** (OpenMLS) | Go (lưu SQLite) | Ký/mã hóa tin nhắn trong MLS Group |

*   Libp2p PeerID: đã có từ Phase 2, lưu trong `system_config`.
*   MLS Identity: **chưa có**, là mục tiêu chính của Phase 3.

#### 3b. Luồng Onboarding đúng (CSR-like)
Đây là quy trình CSR (Certificate Signing Request) chuẩn PKI:

```
[Máy Alice - lần đầu]
  1. GetOrCreateLibp2pIdentity() → PeerID_Alice  (đã có)
  2. Rust GenerateIdentity(display_name) → MLS_PrivKey (LƯU LOCAL) + MLS_PubKey
  3. App hiển thị: PeerID_Alice + MLS_PubKey_hex để Alice gửi Admin (Zalo/email)

[Máy Admin]
  4. Nhận PeerID_Alice + MLS_PubKey_Alice từ Alice (out-of-band)
  5. Tạo InvitationToken: { PeerID_Alice, MLS_PubKey_Alice, DisplayName, IssuedAt, ExpiresAt }
  6. Ký token bằng root_private_key (Ed25519)
  7. Đóng gói InvitationBundle: { token + signature, bootstrap_addr, root_public_key }
  8. Gửi file invitation_alice.bundle cho Alice (out-of-band)

[Máy Alice]
  9. Import bundle → verify:
     a. Chữ ký Admin hợp lệ với root_public_key?
     b. token.PeerID == myPeerID?          ← binding tầng mạng
     c. token.PublicKey == myMLSPubKey?    ← binding tầng ứng dụng
     d. Token chưa hết hạn?
  10. Lưu bundle vào SQLite → Kết nối bootstrap_addr → Vào mạng
```

**NGUYÊN TẮC BẤT BIẾN:**
*   MLS Private Key sinh ra trên máy Alice, KHÔNG BAO GIỜ rời khỏi máy Alice.
*   Root Admin Private Key chỉ tồn tại trên máy Admin, KHÔNG nhúng vào client binary.
*   Client binary chỉ chứa Root **Public** Key (để verify token của peers).

#### 3c. Cấu trúc InvitationToken — PHẢI có PeerID
```go
type InvitationToken struct {
    Version     int    // 1
    DisplayName string // "Alice"
    PeerID      string // Libp2p PeerID — BẮTBUỘC để chống Token Replay Attack
    PublicKey   []byte // MLS public key của Alice
    IssuedAt    int64
    ExpiresAt   int64
    Signature   []byte // Admin ký: hash(Version|DisplayName|PeerID|PublicKey|IssuedAt|ExpiresAt)
}

type InvitationBundle struct {
    Token         *InvitationToken
    BootstrapAddr string // multiaddr của Admin node — Alice cần để vào mạng
    RootPublicKey []byte // TOFU: lưu khi import lần đầu, dùng để verify peers sau này
}
```

#### 3d. Bảo vệ chống Token Replay / Spoofing Attack
*   **Vấn đề:** Eve nghe lén, copy token JSON của Alice, mang đi gửi cho Charlie. Charlie verify chữ ký Admin → hợp lệ → Charlie tưởng đang nói chuyện với Alice.
*   **Giải pháp:** Thêm `PeerID` vào token. Trong `HandleStream`, verify:
    ```go
    authenticatedPeerID := stream.Conn().RemotePeer() // Noise Protocol đã xác thực mật mã
    if peerToken.PeerID != authenticatedPeerID.String() {
        reject() // Eve không có Libp2p private key của Alice → bị lộ
    }
    ```
*   **Tại sao an toàn:** Noise Protocol (lớp transport của Libp2p) chứng minh mật mã rằng bên kết nối thực sự sở hữu private key của PeerID đó. Eve không thể giả PeerID của Alice.

#### 3e. Admin Mode — lưu trữ root private key
*   Root private key lưu trong SQLite của Admin (bảng `system_config`), **mã hóa bằng passphrase** (Argon2id + AES-256-GCM).
*   Admin Panel là giao diện trong app (chỉ hiện khi detect có admin key trong DB).
*   Wails bindings cho Admin: `GetPendingRequests()`, `CreateBundle(displayName, peerID, pubKeyHex)`.

#### 3f. App States
```
StateUninitialized  → Chưa có MLS keypair → Hỏi display_name → GenerateIdentity
StateAwaitingBundle → Có MLS keypair, chưa có bundle → Hiển thị PeerID + PubKey để gửi Admin
StateAuthorized     → Có bundle hợp lệ → Kết nối P2P bình thường
StateAdminReady     → StateAuthorized + có root admin key → Hiện Admin Panel
```

## 4. Current Progress & Next Steps

**Phase 2 is fully verified.**
**Phase 3 design is finalized. Ready to implement.**

### Thứ tự implement Phase 3 (có dependency):

```
[Bước 1] Cập nhật proto/mls_service.proto
         → Thêm display_name vào GenerateIdentityRequest
         → Thêm public_key, signing_key_private vào GenerateIdentityResponse
         → Regenerate Go bindings (protoc)

[Bước 2] Rust: Implement GenerateIdentity trong crypto-engine
         → Thêm openmls_rust_crypto, openmls_traits, serde vào Cargo.toml
         → Tạo crypto-engine/src/mls.rs: hàm generate_identity()
         → Implement handler trong main.rs
         (Bước này độc lập, có thể làm song song với Bước 3+4)

[Bước 3] DB: Thêm 2 bảng mới vào backend/db/db.go
         → Bảng mls_identity: (display_name, public_key, signing_key_private, credential)
         → Bảng auth_bundle: (display_name, public_key, token_*, bootstrap_addr, root_public_key)
         → Thêm các DB methods tương ứng

[Bước 4] Admin PKI: Tạo package backend/admin/
         → token.go: struct InvitationToken, InvitationBundle, Sign(), Verify(), Serialize()
         → admin.go: SetupAdminKey(), UnlockAdminKey(), CreateInvitationBundle()
         (Bước này độc lập, có thể làm song song với Bước 2+3)

[Bước 5] App States & Onboarding Flow
         → backend/app_state.go: định nghĩa AppState enum + DetermineAppState()
         → backend/p2p/auth.go: OnboardNewUser(), ImportInvitationBundle(), GetPublicKeyForDisplay()
         (Phụ thuộc vào Bước 2+3+4)

[Bước 6] ConnectionGater + Auth Protocol
         → backend/p2p/gater.go: AuthGater struct implement network.ConnectionGater
         → backend/p2p/auth_protocol.go: HandleStream() + InitiateHandshake()
         → Cập nhật backend/p2p/host.go: thêm gater, auth handler, trigger handshake sau mDNS
         (Phụ thuộc vào Bước 4+5)

[Bước 7] Tích hợp main.go
         → DetermineAppState() → phân nhánh startup
         → Pass bundle token vào NewP2PNode()
         (Phụ thuộc vào tất cả các bước trên)
```

### Files cần tạo mới:
*   `backend/admin/token.go`
*   `backend/admin/admin.go`
*   `backend/app_state.go`
*   `backend/p2p/auth.go`
*   `backend/p2p/gater.go`
*   `backend/p2p/auth_protocol.go`
*   `crypto-engine/src/mls.rs`

### Files cần sửa:
*   `proto/mls_service.proto` (cập nhật GenerateIdentity messages)
*   `backend/mls_service/*.pb.go` (regenerate)
*   `backend/db/db.go` (thêm 2 bảng + methods)
*   `crypto-engine/Cargo.toml` (thêm dependencies)
*   `crypto-engine/src/main.rs` (implement handler)
*   `backend/p2p/host.go` (tích hợp gater + auth protocol)
*   `backend/main.go` (startup flow mới với AppState)
