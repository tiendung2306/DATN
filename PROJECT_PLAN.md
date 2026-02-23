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

**Goal:** Implement the "Root of Trust" PKI to restrict network access. No node may join the Gossip network without a valid `InvitationToken` signed by the Root Admin Key.

### CRITICAL DESIGN RULES FOR THIS PHASE
- **No CSR on the wire:** MLS Private Key is generated on the user's machine and NEVER leaves it.
- **Admin tool is separate logic:** Root Admin Private Key MUST NOT exist in the regular client code path. It is stored encrypted in the Admin's own SQLite, protected by a passphrase.
- **Token must bind both identity layers:** `InvitationToken` MUST contain both `PeerID` (Libp2p layer) AND `PublicKey` (MLS layer). Missing either enables spoofing attacks.
- **Token Replay defense:** Auth handshake MUST verify `token.PeerID == stream.Conn().RemotePeer()`. The Noise Protocol cryptographically proves PeerID ownership, making replay impossible.

### 3.1. Proto & Rust: Implement `GenerateIdentity` [Step 1 & 2]
- **Task:** Update `GenerateIdentityRequest` to include `display_name: string`.
- **Task:** Update `GenerateIdentityResponse` to return `public_key: bytes`, `signing_key_private: bytes`, `credential: bytes`.
- **Task:** Re-run `protoc` to regenerate Go bindings.
- **Task:** Add `openmls_rust_crypto`, `openmls_traits`, `serde` to `crypto-engine/Cargo.toml`.
- **Task:** Create `crypto-engine/src/mls.rs` implementing `generate_identity(display_name)`.
- **Task:** Wire up handler in `crypto-engine/src/main.rs`.

### 3.2. Database Schema Expansion [Step 3]
- **Task:** Add `mls_identity` table: `(display_name, public_key, signing_key_private, credential)`.
- **Task:** Add `auth_bundle` table: `(display_name, public_key, token_issued_at, token_expires_at, token_signature, bootstrap_addr, root_public_key)`.
- **Task:** Add DB methods: `SaveMLSIdentity`, `GetMLSIdentity`, `HasMLSIdentity`, `SaveAuthBundle`, `GetAuthBundle`, `HasAuthBundle`, `SaveEncryptedAdminKey`, `GetEncryptedAdminKey`, `HasAdminKey`.

### 3.3. Admin PKI Package [Step 4]
- **Task:** Create `backend/admin/token.go`:
  - `InvitationToken` struct: `{ Version, DisplayName, PeerID, PublicKey, IssuedAt, ExpiresAt, Signature }`.
  - `InvitationBundle` struct: `{ Token, BootstrapAddr, RootPublicKey }`.
  - `VerifyTokenSignature(token, rootPubKey) bool`.
  - `SerializeBundle / DeserializeBundle`.
- **Task:** Create `backend/admin/admin.go`:
  - `SetupAdminKey(db, passphrase)`: Generate Ed25519 root keypair, encrypt private key (Argon2id + AES-256-GCM), store in DB.
  - `UnlockAdminKey(db, passphrase)`: Decrypt and return private key.
  - `CreateInvitationBundle(privKey, displayName, peerID, pubKeyHex, bootstrapAddr)`: Sign token + package bundle.

### 3.4. App States & Onboarding Flow [Step 5]
- **Task:** Create `backend/app_state.go` with `AppState` enum: `Uninitialized`, `AwaitingBundle`, `Authorized`, `AdminReady`.
- **Task:** Implement `DetermineAppState(db)`.
- **Task:** Create `backend/p2p/auth.go`:
  - `OnboardNewUser(ctx, db, mlsClient, displayName)`: Call Rust `GenerateIdentity`, store in DB.
  - `ImportInvitationBundle(db, bundleJSON)`: Validate all fields + binding checks, store in DB.
  - `GetPublicKeyForDisplay(db)`: Return PeerID + MLS PubKey hex for the user to send to Admin.

### 3.5. Connection Gating & Auth Protocol [Step 6]
- **Task:** Create `backend/p2p/gater.go`:
  - `AuthGater` struct implementing `network.ConnectionGater`.
  - Maintains `verifiedPeers sync.Map` and `bootstrapPeers sync.Map` (temporary allow during handshake).
- **Task:** Create `backend/p2p/auth_protocol.go` (Protocol: `/app/auth/1.0.0`):
  - `HandleStream`: Exchange tokens, verify signature, verify expiry, **verify `token.PeerID == stream.Conn().RemotePeer()`**, add to `verifiedPeers` or disconnect.
  - `InitiateHandshake(ctx, peerID)`: Proactively open auth stream to a newly discovered peer.
- **Task:** Update `backend/p2p/host.go`: pass `AuthGater` to `libp2p.New`, register stream handler, trigger handshake in `mdnsNotifee.HandlePeerFound`.

### 3.6. Integration [Step 7]
- **Task:** Update `backend/main.go` to use `DetermineAppState` and branch startup accordingly.
- **Task:** Pass `bundle.Token` and `bundle.RootPublicKey` into `NewP2PNode`.
- **Validation:**
  - Node A (Admin) + Node B (Alice with valid bundle): connect successfully ✅
  - Node C (no bundle): rejected by `ConnectionGater` ✅
  - Node D (expired token): rejected during auth handshake ✅
  - Eve replaying Alice's token: rejected because `token.PeerID != stream.Conn().RemotePeer()` ✅

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