# Tổng quan Kiến trúc

> Xem thêm: [Index](README.md) · [Coordination Layer](coordination-layer.md) · [Adapter Layer](adapter-layer.md) · [Service Layer](service-layer.md) · [Crypto Engine](crypto-engine.md) · [Frontend](frontend.md)

## Tổng quan

Dự án là một **nền tảng giao tiếp zero-trust serverless** triển khai **Decentralized Coordination Protocol** bao bọc MLS (RFC 9420) cho môi trường P2P. Kiến trúc sử dụng **hexagonal architecture** với 5 layers chính:

| Layer | Ngôn ngữ | Vai trò |
|-------|----------|---------|
| **Frontend** | React + TypeScript (Wails) | Desktop UI — thin layer, không chứa logic bảo mật |
| **Service** | Go | Orchestration — bridge giữa Coordination và Frontend |
| **Coordination** | Go | Giao thức phi tập trung — ordering, election, fork healing |
| **Adapter** | Go | Infrastructure adapters — P2P, gRPC, SQLite |
| **Crypto Engine** | Rust | Pure MLS operations — stateless, không biết coordination |

## Sơ đồ kiến trúc chi tiết

```
┌──────────────────────────────────────────────────────────────────────┐
│                        FRONTEND (React + Wails)                       │
│                                                                       │
│  app/AppRoot.tsx                                                      │
│    └─ features/runtime/screens/RootRouterScreen.tsx                   │
│         ├─ UNINITIALIZED → WelcomeScreen / ImportBackupScreen         │
│         ├─ AWAITING_BUNDLE → AwaitingBundleScreen                     │
│         ├─ ADMIN_READY / AUTHORIZED → MainChatModuleScreen            │
│         │    ├─ WorkspaceRail (activity/chat/admin/settings)          │
│         │    ├─ MainSidebar (group list, channels, DMs)               │
│         │    ├─ ChatView / RoomPanel (messages, posts, comments)      │
│         │    ├─ AdminPanelScreen (admin key, bundles, invites)        │
│         │    ├─ SettingsScreen (network, profile, diagnostics)        │
│         │    └─ ActivityScreen (notifications)                        │
│         └─ Toaster (toast notifications)                              │
│                                                                       │
│  stores/ (Zustand)     ←  services/runtimeClient.ts  ←  Wails Bindings│
│  useChatStore              (~80 methods wrapping Runtime)             │
│  useAppRuntimeStore                                                   │
│  useGroupsStore                                                      │
│  useNetworkStore                                                     │
│  useContactStore                                                     │
│  useNotificationStore                                                │
│  useMessageLimitsStore                                               │
│  useToastStore                                                       │
│                                                                       │
│  hooks/useRuntimeEventStream — durable event polling + gap detection  │
│  hooks/useWailsEvent — typed Wails event subscriptions               │
└────────────────────────────────┬─────────────────────────────────────┘
                                 │ Wails Bindings (generated Go → TS)
┌────────────────────────────────┴─────────────────────────────────────┐
│                     SERVICE LAYER (Go: app/service/)                  │
│                                                                       │
│  Runtime struct — bound với Wails, quản lý toàn bộ ứng dụng:          │
│  ├── Lifecycle: Startup → DomReady → BeforeClose → Shutdown           │
│  ├── Map: coordinators[groupID] → *Coordinator                        │
│  ├── Session management (single active device)                        │
│  ├── Group ops: create, join, leave, members, DM, channels           │
│  ├── Messaging: send, retry, delete, posts, comments                 │
│  ├── Identity: generate, import, export, onboarding                  │
│  ├── Invite: multi-node approval, KeyPackage exchange, Welcome       │
│  ├── File transfer: MLS-encrypted chunked (AES-256-GCM)              │
│  ├── Profile: avatar, display name, peer profile sync                 │
│  ├── Notifications: mention, reply, group_add, invite events         │
│  ├── Offline sync: blind-store replication, direct stream sync       │
│  └── Control API: demo REST endpoints                                │
│                                                                       │
│  EventSink interface — emit events to frontend via Wails runtime      │
└──────────┬───────────────────────────────────────┬───────────────────┘
           │                                       │
┌──────────┴──────────────────┐          ┌────────┴───────────────────────┐
│  COORDINATION LAYER         │          │     ADAPTER LAYER               │
│  (Go: app/coordination/)    │          │  (Go: app/adapter/)             │
│                             │          │                                 │
│  Coordinator                │          │  p2p/                           │
│  ├── ActiveView             │          │  ├── P2PNode (libp2p host)      │
│  ├── SingleWriter           │          │  ├── LibP2PTransport            │
│  ├── EpochTracker           │          │  ├── AuthProtocol               │
│  ├── ForkDetector           │          │  ├── Wire protocols (14)        │
│  ├── HLC                    │          │                                 │
│  └── Config                 │          │  sidecar/                       │
│                             │          │  ├── GrpcMLSEngine              │
│  Message types:             │          │  ├── ProcessManager             │
│  Proposal, Commit,          │          │  └── gRPC adapter               │
│  Heartbeat, Announce,       │          │                                 │
│  Application, DeliveryAck,  │          │  store/                         │
│  HistoryQuery/Reply         │          │  ├── Database (SQLite WAL)      │
│                             │          │  ├── SQLiteCoordinationStorage  │
│  Modes: Live, CatchingUp,   │          │  └── db_*.go (18 files)         │
│  FrozenForApply             │          │                                 │
│                             │          │  wailsui/                       │
│  Fork healing: detect →     │          │  ├── Run (Wails app setup)      │
│  compare → heal → replay    │          │  └── EventSink impl             │
└─────────────────────────────┘          └────────┬────────────────────────┘
                                                  │ gRPC (127.0.0.1:{port})
                                         ┌────────┴────────────────────────┐
                                         │   CRYPTO ENGINE (Rust)           │
                                         │   (crypto-engine/)               │
                                         │                                  │
                                         │   OpenMLS 0.8.0 + tonic 0.14.3   │
                                         │   Ciphersuite: MLS_128_DHKEMX25519│
                                         │     _AES128GCM_SHA256_Ed25519    │
                                         │                                  │
                                         │   30 gRPC RPCs:                  │
                                         │   ├── 17 stateless (production)  │
                                         │   ├── 9 cached (benchmark)       │
                                         │   └── 4 identity (Ed25519)       │
                                         │                                  │
                                         │   Stateless: import state →      │
                                         │     MLS op → export new state    │
                                         │   Cached: DashMap in-memory      │
                                         │     groups with OCC validation   │
                                         └──────────────────────────────────┘
```

## Nguyên tắc Two-Tier Separation

Đây là nguyên tắc cốt lõi nhất của kiến trúc, đảm bảo separation of concerns giữa coordination logic và cryptography:

- **Coordination Layer (Go — `app/coordination/`):** Xử lý thứ tự tin nhắn, bầu cử Token Holder deterministic, phát hiện/phục hồi fork, heartbeat/ActiveView, HLC timestamps. **Không thực hiện bất kỳ MLS crypto operation nào trực tiếp** — tất cả crypto đi qua `MLSEngine` interface.

- **Crypto Layer (Rust — `crypto-engine/`):** Thuần túy MLS stateless. **Không biết về Single-Writer, epochs, ActiveView, Token Holder, hay fork healing.** Chỉ nhận `group_state` bytes → thực hiện MLS operation → trả về `new_group_state` bytes. Rust là "computational black box" — tính toán đúng, không quyết định thứ tự.

Lợi ích:
1. Rust code đơn giản, ít bug surface — không cần hiểu distributed systems
2. Go có thể thay đổi coordination strategy mà không đụng crypto
3. Test crypto độc lập với coordination logic
4. Upgrade OpenMLS version chỉ ảnh hưởng Rust, không phá Go

## Sidecar Pattern

Go spawn Rust binary qua `os/exec`, truyền port qua CLI flag `--port {freePort}`:

```
Go ProcessManager.StartEngine()
  ├── GetFreePort() — xin OS free TCP port
  ├── Tìm binary: crypto-engine / crypto-engine.exe
  │   (cwd, ../crypto-engine/target/{release,debug}/...)
  ├── spawn với --port flag
  ├── Pipe stdout/stderr → Go logger
  └── GrpcMLSEngine connect to 127.0.0.1:{port}

Rust main()
  ├── clap::Parser → port (default 50051)
  ├── bind 127.0.0.1:{port}
  ├── max message size: 64 MiB
  └── serve gRPC
```

**Stateless round-trip:**
```
Go đọc group_state bytes từ SQLite
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
Go lưu vào SQLite (atomic transaction)
```

Rust **không lưu state vĩnh viễn**. Mỗi RPC là một round-trip hoàn toàn độc lập. Điều này đảm bảo:
- Go là single source of truth (SQLite)
- Rust crash/restart không mất dữ liệu
- Không cần distributed consensus giữa Go và Rust

**Cached path (benchmark only):** `RuntimeCache` (DashMap) giữ group in-memory để đo overhead của stateless serialization. Production dùng stateless path.

## Cấu trúc thư mục chi tiết

```
DATN/
├── app/                              # Go backend chính
│   ├── main.go                       # Composition root — parse flags, chọn mode
│   ├── go.mod / go.sum               # Go module "app" + deps (libp2p, wails, grpc, sqlite)
│   │
│   ├── adapter/                      # Hexagonal adapters
│   │   ├── p2p/                      # P2P communication
│   │   │   ├── host.go               # P2PNode — libp2p host (Ed25519, TCP+QUIC, DHT, GossipSub, mDNS)
│   │   │   ├── transport_adapter.go  # LibP2PTransport — implements coordination.Transport
│   │   │   ├── auth.go               # OnboardNewUser — MLS key generation via Rust
│   │   │   ├── auth_protocol.go      # Auth handshake (/app/auth/1.0.0) — InvitationToken verify
│   │   │   ├── session_claim.go      # SessionClaim — single active device
│   │   │   ├── gater.go              # AuthGater — connection gating + blacklist
│   │   │   ├── identity.go           # GetOrCreateIdentity, GetOnboardingInfo
│   │   │   ├── pubsub.go             # Join chat room helper
│   │   │   ├── kp_direct.go          # KeyPackage direct delivery
│   │   │   ├── user_profile_push.go  # Profile sync (/app/user-profile/1.0.0)
│   │   │   ├── replicated_store_wire.go  # Blind-store replication (/org/replicated-store/1.0.0)
│   │   │   ├── invite_store_wire.go  # Invite store/lookup (/app/invite-store/1.0.0)
│   │   │   ├── group_info_wire.go    # GroupInfo exchange (/app/group-info/1.0.0)
│   │   │   ├── group_invite_request_wire.go  # Group invite (/app/group-invite/1.0.0)
│   │   │   ├── channel_category_wire.go      # Channel category sync (/app/channel-cat/1.0.0)
│   │   │   ├── file_transfer_wire.go         # File transfer (/app/file-transfer/1.0.0)
│   │   │   └── offline_wire.go               # Offline sync (/app/offline-sync/1.0.0)
│   │   │
│   │   ├── sidecar/                  # Rust MLS engine adapter
│   │   │   ├── engine.go             # GrpcMLSEngine — MLSEngine interface impl
│   │   │   ├── grpc.go               # gRPC connection helper
│   │   │   ├── process.go            # ProcessManager — spawn/stop Rust binary
│   │   │   ├── cached_benchmark_engine.go  # Cached benchmark engine (research)
│   │   │   ├── sys_proc_attr_windows.go   # Windows process attributes
│   │   │   └── sys_proc_attr_other.go     # Linux/macOS process attributes
│   │   │
│   │   ├── store/                    # SQLite storage
│   │   │   ├── db.go                 # Database — WAL, single conn, schema init
│   │   │   ├── coordination_storage.go  # SQLiteCoordinationStorage — CoordinationStorage impl
│   │   │   ├── portrepos.go          # Repository ports backed by SQLite
│   │   │   ├── db_group_members.go   # Group member CRUD
│   │   │   ├── db_group_metadata.go  # Group metadata storage
│   │   │   ├── db_identity.go        # MLS identity storage
│   │   │   ├── db_invite_assets.go   # Invite assets storage
│   │   │   ├── db_peer_directory.go  # Peer directory storage
│   │   │   ├── db_messages.go        # Message storage
│   │   │   ├── db_admin.go           # Admin key storage
│   │   │   ├── db_config.go          # System config storage
│   │   │   ├── profile.go            # Peer profile cache
│   │   │   ├── channel_categories.go # Channel category storage
│   │   │   ├── file_transfer.go      # File transfer storage
│   │   │   ├── notifications.go      # Notification storage
│   │   │   ├── runtime_events.go     # Runtime event log storage
│   │   │   ├── replicated_store.go   # Blind-store replicated objects
│   │   │   ├── group_add_operations.go  # Group add operations storage
│   │   │   ├── group_event_log.go    # Group event log storage
│   │   │   ├── group_invite_requests.go # Group invite requests storage
│   │   │   └── backup_data.go        # Backup data storage
│   │   │
│   │   └── wailsui/                  # Wails UI binding
│   │       ├── run.go                # Run — Wails app setup, window config, lifecycle hooks
│   │       └── sink.go               # EventSink — Wails runtime events
│   │
│   ├── admin/                        # Root Admin key & InvitationToken
│   │   ├── admin.go                  # SetupAdminKey, UnlockAdminKey, CreateInvitationBundle
│   │   └── token.go                  # InvitationToken — Ed25519 signed credential
│   │
│   ├── cli/                          # CLI command handlers
│   │   ├── runner.go                 # Run — CLI entry point, command dispatch
│   │   └── commands.go               # cmdAdminSetup, cmdCreateBundle, cmdSetup, cmdImportBundle...
│   │
│   ├── config/                       # CLI flag parsing
│   │   └── config.go                 # Config struct + flag definitions
│   │
│   ├── coordination/                 # Decentralized Coordination Protocol
│   │   ├── coordinator.go            # Coordinator — central orchestrator
│   │   ├── coordinator_message.go    # Message handling (GossipSub + direct)
│   │   ├── coordinator_commit.go     # Commit processing pipeline
│   │   ├── coordinator_proposal.go   # Proposal handling
│   │   ├── coordinator_application.go  # Application message handling
│   │   ├── coordinator_heal.go       # Fork healing pipeline
│   │   ├── coordinator_replay.go     # Autonomous replay post-heal
│   │   ├── coordinator_batch.go      # Bidirectional proposal batching
│   │   ├── coordinator_broadcast.go  # Broadcast heartbeat/announce/proposals
│   │   ├── coordinator_reconcile.go  # Reconcile pending ops after commit
│   │   ├── coordinator_members.go    # Add/remove members, group info sync
│   │   ├── coordinator_crypto.go     # MLS operation context helper
│   │   ├── coordinator_helpers.go    # Utility functions
│   │   ├── coordinator_observability.go  # Metrics & tracing
│   │   ├── single_writer.go          # Token Holder election + proposal buffering
│   │   ├── epoch.go                  # EpochTracker — validate, buffer future
│   │   ├── hlc.go                    # Hybrid Logical Clock
│   │   ├── active_view.go            # Online peer tracking
│   │   ├── fork_healing.go           # ForkDetector — branch comparison
│   │   ├── metrics.go                # Coordination metrics recording
│   │   ├── types.go                  # Core types (Envelope, MessageType, etc.)
│   │   ├── interfaces.go             # Transport, Clock, MLSEngine, CoordinationStorage
│   │   └── config.go                 # CoordinatorConfig
│   │
│   ├── domain/                       # Domain types
│   │   ├── errors.go                 # ErrNotFound, ErrUnauthorized, ErrInvalidInput...
│   │   ├── identity.go               # Identity, OnboardingInfo
│   │   ├── invite.go                 # AuthBundle, PendingWelcome, KPStatus, CreateBundleRequest
│   │   ├── notification.go           # Notification types + struct
│   │   └── session.go                # Session config keys
│   │
│   ├── frontend/                     # React frontend (Wails-embedded)
│   │   ├── package.json              # React 18, Tailwind, Shadcn, Zustand, Vite
│   │   ├── tsconfig.json             # Strict TS, ES2020, bundler resolution
│   │   ├── vite.config.ts            # Vite + React plugin
│   │   ├── wailsjs/                  # Generated Wails bindings (Go → TS)
│   │   │   ├── go/models.ts          # TS types for Go structs
│   │   │   ├── go/service/Runtime.js # Generated Runtime bindings
│   │   │   └── runtime/              # Wails runtime (EventsOn, etc.)
│   │   └── src/                      # Frontend source
│   │       ├── main.tsx              # Entry — dark mode, StrictMode
│   │       ├── App.tsx               # Wraps AppRoot
│   │       ├── app/AppRoot.tsx       # RootRouterScreen + Toaster
│   │       ├── features/             # Feature-first modules
│   │       ├── stores/               # Zustand stores (8)
│   │       ├── services/             # runtimeClient.ts
│   │       ├── components/           # Shared UI (30 items)
│   │       ├── lib/                  # Utilities (7 files)
│   │       ├── hooks/                # Shared hooks (2 files)
│   │       └── screens/              # Legacy adapters (5 files)
│   │
│   ├── mls_service/                  # Generated gRPC Go code (from proto)
│   │
│   ├── pkg/                          # Shared packages
│   │   ├── log/log.go               # Structured logging (slog + IPFS filter)
│   │   └── filetransfer/crypto.go   # MLS exporter-based file encryption (AES-256-GCM)
│   │
│   ├── port/                         # Repository port interface
│   │   └── repository.go             # Repository interface definition
│   │
│   └── service/                      # Application service layer
│       ├── runtime.go                # Runtime — core struct, lifecycle, Wails binding
│       ├── group.go                  # Group operations
│       ├── messaging.go              # Message send/receive/retry
│       ├── identity.go               # Identity generation, onboarding, backup
│       ├── identity_backup.go        # Encrypted backup export/import
│       ├── session.go                # Single active device enforcement
│       ├── invite.go                 # Multi-node invite approval (70KB, largest file)
│       ├── invite_lifecycle.go       # Invite lifecycle management
│       ├── admin.go                  # Admin key setup, bundle creation
│       ├── app_state.go              # DetermineAppState state machine
│       ├── blind_store.go            # Blind-store replication layer
│       ├── bootstrap.go              # Bootstrap peer connection
│       ├── channel_categories.go     # Channel category CRUD + sync
│       ├── channel_payload.go        # Channel post/comment validation
│       ├── cli_node.go               # CLI headless mode support
│       ├── control_api.go            # Demo control REST API
│       ├── events.go                 # EventSink interface definition
│       ├── file_transfer.go          # MLS-encrypted file transfer
│       ├── fork_heal_history.go      # Fork heal audit log
│       ├── group_admins.go           # Group admin management
│       ├── group_avatar_replicate.go # Group avatar P2P replication
│       ├── group_event_log.go        # Group event log
│       ├── group_info_sync.go        # GroupInfo sync for fork healing
│       ├── group_invite_request_p2p.go # P2P invite request protocol
│       ├── group_invite_requests.go  # Invite request management
│       ├── group_invite_policy_replicate.go # Invite policy replication
│       ├── group_member_directory.go  # Member directory sync
│       ├── group_permissions.go      # Permission checks
│       ├── membership.go             # Membership operations
│       ├── message_limits.go         # Message length limits
│       ├── network_diagnostics.go    # Network diagnostic info
│       ├── node_status.go            # Node status reporting
│       ├── notifications.go          # Notification system
│       ├── offline_sync.go           # Offline message sync
│       ├── profile.go                # User profile + avatar
│       ├── recovery_replay.go        # Recovery replay APIs
│       ├── replicated_sync.go        # Replicated store sync
│       ├── runtime_events.go         # Durable runtime event log
│       └── runtime_health.go         # Health status reporting
│
├── crypto-engine/                    # Rust MLS sidecar
│   ├── Cargo.toml                    # openmls 0.8, tonic 0.14, tokio, dashmap, ed25519-dalek
│   ├── build.rs                      # tonic-prost-build → generate gRPC server code
│   └── src/
│       ├── main.rs                   # gRPC server (tonic) — MyMlsService impl
│       ├── lib.rs                    # pub mod mls
│       ├── mls.rs                    # Core MLS logic (2173 lines, 30+ tests)
│       └── bin/
│           └── mls_bench.rs          # Benchmark binary (MLS optimization research)
│
├── proto/                            # gRPC protobuf definition
│   └── mls_service.proto             # MLSCryptoService — 30 RPCs
│
├── evaluation/                       # Benchmark & chart scripts
│   ├── *.py                          # Python analysis scripts (10 files)
│   ├── data/                         # CSV/JSON benchmark data (11 files)
│   └── plots/                        # PNG charts (14 files)
│
├── demo-control/                     # Demo control app (separate Wails app)
├── scripts/                          # Dev instance scripts (multi-node)
├── docs/                             # This documentation
├── thesis_drafts/                    # Thesis LaTeX/markdown drafts
├── Dockerfile / docker-compose.yml   # Container deployment
├── README.md                         # High-level overview
├── PROJECT_PLAN.md                   # Execution roadmap
├── CURRENT_STATE.md                  # Current progress + source map
└── AGENTS.md                         # AI agent guidelines
```

## Entry Point

`app/main.go` — Composition root, 3 chế độ hoạt động:

### Wails GUI (mặc định)
```go
wailsui.Run(cfg, dist)
```
- Embed `frontend/dist` (React build output)
- Bind `service.Runtime` struct → Wails generates TS bindings
- EventSink → Wails runtime events → frontend `EventsOn`
- Window: 1200x800, dark theme

### Headless managed
```go
runManagedHeadless(cfg)
```
- `Runtime.Startup()` — init SQLite, P2P, sidecar, coordinators
- Chờ OS signal (SIGINT/SIGTERM)
- `Runtime.Shutdown()` — cleanup

### CLI command
```go
cli.Run(cfg)
```
- `--setup` — generate MLS keys
- `--admin-setup` — create Root Admin key
- `--create-bundle` — sign InvitationToken
- `--import-bundle` — import bundle, launch P2P
- `--export-identity` / `--import-identity` — encrypted backup/restore

## Dependency Direction

```
app/main.go (composition root)
    │
    ├── config (flag parsing)
    ├── cli (CLI commands)
    ├── wailsui (Wails UI adapter)
    │       │
    │       └── service.Runtime (orchestration)
    │               │
    │               ├── coordination.Coordinator (protocol logic)
    │               │       │
    │               │       └── interfaces: Transport, MLSEngine, CoordinationStorage, Clock
    │               │
    │               ├── adapter.p2p (LibP2PTransport → Transport)
    │               ├── adapter.sidecar (GrpcMLSEngine → MLSEngine)
    │               └── adapter.store (SQLiteCoordinationStorage → CoordinationStorage)
    │                       │
    │                       └── crypto-engine (Rust gRPC → MLSEngine impl)
    │
    └── domain (shared types)
```

**Key principle:** Dependencies luôn đi từ ngoài vào trong. Service phụ thuộc coordination interfaces, không phụ thuộc cụ thể adapter. Adapter implement coordination interfaces. Rust chỉ implement MLS math, không phụ thuộc Go.

## Key Design Decisions

1. **Pure Go SQLite (`modernc.org/sqlite`):** Không cần CGO, cross-compile dễ dàng, single binary deployment
2. **Ed25519 everywhere:** Signing keys cho MLS identity, admin keys, InvitationTokens — nhất quán
3. **GossipSub cho group messaging:** Pub/sub topology, không cần central broker
4. **Kademlia DHT cho discovery:** Peer routing, KeyPackage advertisement, blind-store replica targeting — không cho application mailbox
5. **HLC cho causal ordering:** Không phụ thuộc NTP, tolerant clock drift (10s max)
6. **Stateless Rust:** Go quản lý persistence, Rust là computational black box
7. **Single-Writer Commit:** Deterministic Token Holder election, tránh concurrent commits gây fork
8. **Fork healing via External Join:** Losing branch re-joins winning branch, crypto-shredding old keys, autonomous replay own messages only
