# PROJECT PLAN: SECURE PRIVATE P2P COMMUNICATION SYSTEM

**Role:** Graduation Thesis Project
**Architecture:** Distributed System, Sidecar Pattern, Local-first
**Core Stack:**
- **Frontend/Host:** Wails (Go) + React/TypeScript
- **Networking:** Go-libp2p
- **Crypto Engine:** Rust (OpenMLS) running as a gRPC Server
- **Database:** SQLite (Managed by Go)
- **IPC:** gRPC (Protobuf) over localhost with dynamic port assignment

## 1. SYSTEM ARCHITECTURE & SETUP (Weeks 1-2)

**Goal:** Establish the IPC infrastructure where the Go application manages the Rust sidecar process lifecycle securely and reliably.

### 1.1. Monorepo Structure & Protobuf Definition
- **Task:** Define the directory structure.
  - `/proto`: Shared `.proto` definitions.
  - `/backend`: Go code (Wails app, Libp2p host, SQLite manager).
  - `/crypto-engine`: Rust code (gRPC server, OpenMLS logic).
  - `/frontend`: React code.
- **Task:** Define `mls_service.proto`.
  - Service: `MLSCryptoService`
  - Methods (Initial): `GenerateIdentity`, `ExportIdentity`, `ImportIdentity`.
  - Data structures: `KeyPackage`, `MlsMessage`.
- **Task:** Configure `protoc` to generate Go code and Rust code.

### 1.2. Rust gRPC Server Implementation
- **Context:** Headless binary, listens on CLI-provided port.
- **Task:** Implement `main.rs` to parse a `--port` flag.
- **Task:** Implement a basic Tonic gRPC server listening on `127.0.0.1:{port}`.
- **Task:** Implement a dummy `Ping` method to verify connectivity.

### 1.3. Go Process Manager (Sidecar Logic)
- **Context:** Go manages Rust process lifecycle.
- **Task:** Implement `ProcessManager` struct in Go.
  - `GetFreePort()`: Ask OS for random port.
  - `StartCryptoEngine()`: Execute Rust binary with port arg.
  - `StopCryptoEngine()`: Handle cleanup on exit.
- **Task:** Implement gRPC Client in Go to connect to `127.0.0.1:{random_port}`.

### 1.4. Database & Logging Setup
- **Task:** Initialize SQLite (`users`, `messages`).
- **Task:** Setup structured logging (Zap/Slog) capturing both Go and Rust outputs.

---

## 2. P2P NETWORKING LAYER (Weeks 3-5) [COMPLETED]

**Goal:** Enable decentralized node discovery and communication using Go-libp2p.

### 2.1. Libp2p Host Configuration [Done]
- **Task:** Configure `libp2p.New` with TCP/QUIC, Noise, Yamux, AutoNAT.
- **Implementation Note:** Added persistent `PeerID` storage in SQLite (`system_config` table) to ensure stable identity across restarts.

### 2.2. Discovery Mechanism [Done]
- **Task:** Implement mDNS discovery service.
- **Task:** Implement Kademlia DHT (`dht.New`) in server mode.
- **Task:** Implement "Bootstrap" logic to connect to static IPs.
- **Implementation Note:** Implemented a "Hybrid Discovery" model. Fixed Windows mDNS noise issues by binding to specific interfaces and implementing a 2-layer log filter. Added a Shared Volume Bootstrap mechanism for robust Docker testing.

### 2.3. GossipSub Implementation [Done]
- **Task:** Initialize GossipSub (`pubsub.NewGossipSub`).
- **Task:** Define global topic `"/org/chat/global"`.
- **Task:** Implement Subscription handler (Receive -> Log -> Store Pending -> Emit to UI).

---

## 3. IDENTITY & ADMIN ONBOARDING (Weeks 6-7)

**Goal:** Implement the "Root of Trust" PKI to restrict network access.

### 3.1. Admin Tools (Go-based)
- **Task:** Implement `SignIdentity` logic using `root.pem`.
- **Task:** Generate `InvitationToken` (Protobuf/JSON).

### 3.2. User Identity (Go <-> Rust)
- **Task:** Rust: Implement `GenerateIdentity` (OpenMLS).
- **Task:** Go: Display Public Key -> Verify `InvitationToken` -> Store Identity locally.

### 3.3. Network Gating (ConnectionGater)
- **Task:** Implement `ConnectionGater` interface.
- **Logic:** Handshake to exchange Signed Tokens. Drop connection if signature is invalid.

---

## 4. MLS SECURE GROUP CHAT (Weeks 8-11)

**Goal:** Integrate OpenMLS via gRPC to enable E2EE group chat.

### 4.1. MLS Proto Definitions & State Management
- **Task:** Update `mls_service.proto` for Group operations (`CreateGroup`, `ProcessCommit`, etc.).
- **Task:** Implement Stateless Rust logic (State is passed from Go -> Rust -> Go).

### 4.2. Group Operations
- **Task:** Implement "Create Group" and "Join Group" flows via Direct P2P messages (Welcome Msg).

### 4.3. Messaging & Consensus
- **Task:** Implement `EncryptMessage` / `DecryptMessage`.
- **Task:** Implement Deterministic Ordering logic (Buffer commits -> Select lowest Hash).

---

## 5. ADVANCED FEATURES: MIGRATION & OFFLINE (Weeks 12-13)

**Goal:** Secure Identity Migration (Manual) and Offline Messaging.

### 5.1. Secure Identity Export/Import (File-based)
- **Task:** Go UI: Prompt User for a strong Passphrase.
- **Task:** Go Logic: Read Private Key + Cert from DB. Send to Rust `ExportIdentity(passphrase)`.
- **Task:** Rust Logic: Serialize KeyPackage -> Encrypt with AES-GCM (Key derived from Passphrase/Argon2) -> Return bytes.
- **Task:** Go Logic: Save bytes to a `.backup` file.
- **Task:** Import Flow: Read file -> Send to Rust `ImportIdentity` -> Restore Keypair.

### 5.2. Session Takeover (Single Active Device)
- **Task:** On successful Import & Connect: Broadcast `KILL_SESSION` (Signed by User Key).
- **Task:** Active clients listening to `KILL_SESSION`: Verify signature -> Self-destruct if valid.

### 5.3. Offline Messaging (Neighborhood Storage)
- **Task:** If sending fails: `dht.Put(Key=Hash(RecipientID), Value=EncryptedMsg)`.
- **Task:** On Connect: `dht.Get(Key=Hash(MyID))`.

---

## 6. FILE TRANSFER & FINALIZATION (Weeks 14-15)

**Goal:** Direct high-speed file transfer and documentation.

### 6.1. Direct Stream Protocol
- **Task:** Define Libp2p Protocol `/app/file/1.0.0`.
- **Task:** Implement Stream Handler (Send Metadata -> Stream Content).
- **Task:** UI Progress Bar.

### 6.2. Thesis Report & Defense Prep
- **Task:** Finalize diagrams.
- **Task:** Prepare Demo environment.

**Validation Criteria:**
> Successful transfer of identity via file. Old session terminates automatically. Large file transfer works.