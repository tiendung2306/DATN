# Lớp Admin & Config (Go)

> Xem thêm: [Index](README.md) · [Architecture Overview](architecture-overview.md) · [Security & Protocol](security-protocol.md) · [Flows → Onboarding](flows.md#onboarding-flow)

## Admin (`app/admin/`)

### InvitationToken (`token.go`)

`InvitationToken` là credential signed bởi Root Admin, binding PeerID + PublicKey. Đây là cơ chế zero-trust onboarding — không node nào join network không có token hợp lệ.

```go
type InvitationToken struct {
    PeerID        string    // Libp2p PeerID của node được mời
    PublicKey     []byte    // MLS signing public key (Ed25519)
    IssuedAt      int64     // Unix timestamp
    ExpiresAt     int64     // Unix timestamp (IssuedAt + 365 days)
    IssuedBy      string    // Admin name/label
    Signature     []byte    // Ed25519 signature bằng Root Admin private key
}
```

| Method | Mục đích |
|--------|----------|
| `SignToken(peerID, pubKey, adminName, adminPrivKey)` | Tạo token: build payload → sign với Ed25519 → return token |
| `VerifyToken(token, rootAdminPubKey)` | Verify: check signature + check expiry + check PeerID matches |

**Token validity:** 365 days từ `IssuedAt`. Token hết hạn bị reject trong auth handshake.

**Anti-replay:** Token绑定 PeerID — không thể dùng token của node khác. AuthProtocol verify PeerID trong token khớp với connection's PeerID.

### InvitationBundle

`InvitationBundle` là package gửi out-of-band (Zalo, email, USB) cho new user:

```go
type InvitationBundle struct {
    Token           InvitationToken  // Signed token
    BootstrapAddr   string           // Bootstrap peer multiaddr (e.g., /ip4/1.2.3.4/tcp/4001/p2p/...)
    RootAdminPubKey []byte           // Root Admin Ed25519 public key (for token verification)
}
```

Bundle là file `.bundle` (JSON format). User import qua UI (`OpenAndImportBundle`) hoặc CLI (`--import-bundle`).

### Admin Key (`admin.go`)

Root Admin key — Ed25519 key pair, encrypted at rest với Argon2id + AES-256-GCM:

| Method | Mục đích |
|--------|----------|
| `SetupAdminKey(passphrase)` | Generate Ed25519 key pair → encrypt private key → store in SQLite `system_config` |
| `UnlockAdminKey(passphrase)` | Decrypt private key → keep in memory cho signing |
| `CreateInvitationBundle(peerID, pubKeyHex, name)` | Build bundle: sign token + package with bootstrap addr + root pub key |
| `GetAdminStatus()` | Return: initialized, unlocked, issuance count |

**Key derivation (Argon2id — OWASP parameters):**

| Parameter | Value | Mô tả |
|-----------|-------|-------|
| `time` | 1 | Iterations |
| `memory` | 64 KB | Memory cost (KiB) |
| `parallelism` | 4 | Parallel threads |
| `output` | 32 bytes | Derived key length |

**Wire format (encrypted private key in SQLite):**
```
[16 bytes salt][12 bytes nonce][ciphertext + GCM tag]
```

- **Salt:** Random 16 bytes per encryption — unique even nếu cùng passphrase
- **Nonce:** Random 12 bytes per encryption — AES-256-GCM require unique nonce
- **Ciphertext:** Ed25519 private key (32 bytes) encrypted với AES-256-GCM

**Security:**
- Private key chỉ tồn tại trong memory sau khi unlock — không bao giờ ghi ra disk unencrypted
- Passphrase không lưu — nếu quên, admin key không thể recover
- Root Admin public key lưu trong `InvitationBundle` — mọi node verify token mà không cần admin online

## Config (`app/config/`)

CLI flag parsing — `config.go` định nghĩa `Config` struct và flag bindings:

```go
type Config struct {
    // Network
    DBPath         string
    RuntimeDir     string
    BindIP         string
    P2PPort        int
    BootstrapAddr  string
    WriteBootstrap string
    Headless       bool
    ControlPort    int
    ControlToken   string
    InstanceLabel  string

    // Setup
    Setup              bool
    ImportBundle       string
    ExportIdentity     bool
    ImportIdentityPath string
    ExportOutputPath   string
    IdentityPassphrase string
    Force              bool

    // Admin
    AdminSetup      bool
    AdminPassphrase string
    CreateBundle    bool
    BundleName      string
    BundlePeerID    string
    BundlePubKey    string
    BundleOutput    string

    // Store
    StoreNode              bool
    BlindStoreParticipant  bool
    OfflineReplicaK        int
    RuntimeEventReplay     bool
    FileTransferChunkBytes int
}
```

### CLI Flags

| Nhóm | Flags | Mặc định | Mô tả |
|------|-------|----------|-------|
| **Network** | `--db`, `--runtime-dir`, `--bind-ip`, `--p2p-port`, `--bootstrap`, `--write-bootstrap`, `--headless`, `--control-port`, `--control-token`, `--instance-label` | port 4001, db `.local/app.db` | P2P network + runtime configuration |
| **Setup** | `--setup`, `--import-bundle <path>`, `--export-identity`, `--export-output <path>`, `--import-identity <path>`, `--identity-passphrase <pw>`, `--force` | — | Identity setup operations |
| **Admin** | `--admin-setup`, `--admin-passphrase <pw>`, `--create-bundle`, `--bundle-name <name>`, `--bundle-peer-id <id>`, `--bundle-pub-key <hex>`, `--bundle-output <path>` | — | Admin key + bundle creation |
| **Store** | `--store-node`, `--blind-store-participant`, `--offline-replica-k <n>` | k=2, blind-store on | Blind-store replication config |
| **File transfer** | `--file-chunk-bytes <n>` | 1MB | File chunk size |
| **Runtime** | `--runtime-event-replay-enabled` | true | Durable runtime event log + replay |

### CLI Commands (`app/cli/`)

`cli/runner.go` — entry point cho CLI mode, dispatch commands:

| Command | Handler | Mô tả |
|---------|---------|-------|
| `--admin-setup` | `cmdAdminSetup` | Generate Root Admin key, encrypt với passphrase |
| `--create-bundle` | `cmdCreateBundle` | Sign InvitationToken, export `.bundle` file |
| `--setup` | `cmdSetup` | Generate MLS key pair (Ed25519) via Rust engine |
| `--import-bundle <path>` | `cmdImportBundle` | Import bundle, verify token, store auth bundle |
| `--export-identity <path>` | `cmdExportIdentity` | Export encrypted backup (`.backup` file) |
| `--import-identity <path>` | `cmdImportIdentity` | Restore from backup file |

`cli/commands.go` — implement từng command, print results to console.

## Domain (`app/domain/`)

Shared domain types — used bởi service, coordination, và adapter layers:

### `errors.go`

```go
var (
    ErrNotFound       = errors.New("not found")
    ErrUnauthorized   = errors.New("unauthorized")
    ErrInvalidInput   = errors.New("invalid input")
    ErrConflict       = errors.New("conflict")
    ErrCryptoRequired = errors.New("crypto operation required")
)
```

### `identity.go`

```go
type Identity struct {
    PeerID      string  // Libp2p PeerID (derived from Ed25519 public key)
    PublicKey   []byte  // Ed25519 public key (MLS signing key)
    PrivateKey  []byte  // Ed25519 private key (encrypted at rest)
    CreatedAt   int64
}

type OnboardingInfo struct {
    PeerID      string  // For admin CSR
    PublicKeyHex string // Hex-encoded public key
}
```

### `invite.go`

```go
type AuthBundle struct {
    Token           []byte  // Serialized InvitationToken
    BootstrapAddr   string
    RootAdminPubKey []byte
    ImportedAt      int64
}

type PendingWelcome struct {
    GroupID    string
    WelcomeBytes []byte
    FromPeer   string
    CreatedAt  int64
}

type KPStatus string  // KeyPackage status: "active", "consumed", "expired"

type CreateBundleRequest struct {
    PeerID    string
    PublicKey string
    Name      string
}
```

### `notification.go`

```go
type NotificationType string
const (
    NotificationMention       NotificationType = "mention"
    NotificationReply         NotificationType = "reply"
    NotificationGroupAdd      NotificationType = "group_add"
    NotificationInviteRequest NotificationType = "invite_request"
    NotificationInviteApproved NotificationType = "invite_approved"
    NotificationInviteRejected NotificationType = "invite_rejected"
)

type Notification struct {
    ID           string
    Type         NotificationType
    GroupID      string
    ActorPeerID  string
    TargetID     string
    Content      string
    IsRead       bool
    CreatedAt    int64
}
```

### `session.go`

```go
const (
    ConfigKeySessionStartedAt   = "session.started_at"
    ConfigKeyKillSessionPending = "session.kill_pending"
)
```

