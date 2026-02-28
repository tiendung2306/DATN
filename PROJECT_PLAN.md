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

## 3. IDENTITY & ADMIN ONBOARDING (Weeks 6-7) [COMPLETED ✅]

**Goal:** Implement the "Root of Trust" PKI to restrict network access. No node may join the Gossip network without a valid `InvitationToken` signed by the Root Admin Key.

### CRITICAL DESIGN RULES (Finalized)
- **No CSR on the wire:** MLS Private Key is generated on the user's machine and NEVER leaves it.
- **Admin assigns display names:** User only generates a key pair (`--setup`). The display name (`DisplayName` in token) is assigned by Admin when creating the bundle — the user does NOT set their own name.
- **Admin tool is separate logic:** Root Admin Private Key MUST NOT exist in the regular client code path. Stored encrypted (Argon2id + AES-256-GCM) in Admin's own SQLite only.
- **Token must bind both identity layers:** `InvitationToken` MUST contain both `PeerID` (Libp2p layer) AND `PublicKey` (MLS layer).
- **Token Replay defense:** Auth handshake MUST verify `token.PeerID == stream.Conn().RemotePeer()`. Noise Protocol cryptographically proves PeerID ownership.
- **bootstrap_addr must include PeerID:** Format MUST be `/ip4/IP/tcp/PORT/p2p/PEERID`. Without PeerID, Noise cannot authenticate the bootstrap node's identity.

### 3.1. Proto & Rust: Implement `GenerateIdentity` ✅
- `GenerateIdentityRequest.display_name` field exists in proto but is **ignored** by Rust handler.
- `GenerateIdentityResponse` returns `public_key`, `signing_key_private`, `credential` (empty bytes at generation time).
- `generate_identity()` in Rust takes no `display_name` parameter. Credential is empty `Vec::new()`.
- Go updates `mls_identity.credential` after bundle import via `UpdateMLSDisplayName()`.

### 3.2. Database Schema Expansion ✅
- `mls_identity` table: `(id, display_name, public_key, signing_key_private, credential, created_at)`.
- `auth_bundle` table: `(id, display_name, peer_id, public_key, token_issued_at, token_expires_at, token_signature, bootstrap_addr, root_public_key, imported_at)`. — Note: `peer_id` column added.
- `system_config` table (pre-existing): stores `libp2p_priv_key` and `admin_root_private_key`.
- Methods: `SaveMLSIdentity`, `GetMLSIdentity`, `HasMLSIdentity`, `UpdateMLSDisplayName`, `SaveAuthBundle`, `GetAuthBundle`, `HasAuthBundle`, `SetConfig`, `GetConfig`, `HasConfig`.

### 3.3. Admin PKI Package ✅
- `backend/admin/token.go`: `InvitationToken`, `InvitationBundle`, `SignToken`, `VerifyToken`, `SerializeBundle`, `DeserializeBundle`.
- `backend/admin/admin.go`: `SetupAdminKey`, `UnlockAdminKey`, `CreateInvitationBundle`. Encryption: Argon2id(passphrase, salt) → AES-256-GCM. Wire format: `[16B salt][12B nonce][ciphertext]`.

### 3.4. App States & Onboarding Flow ✅
- `backend/app_state.go`: `StateUninitialized`, `StateAwaitingBundle`, `StateAuthorized`, `StateAdminReady`, `DetermineAppState(db)`.
- `backend/p2p/auth.go`:
  - `OnboardNewUser(ctx, db, mlsClient)` — no `displayName` param; credential set empty.
  - `GetOnboardingInfo(db, privKey) *OnboardingInfo` — returns `{PeerID, PublicKeyHex}` only.
  - `ImportInvitationBundle(db, privKey, bundleJSON)` — 4 checks + calls `UpdateMLSDisplayName`.
  - `BuildLocalToken(bundle) *admin.InvitationToken` — reconstructs full token from stored bundle.

### 3.5. Connection Gating & Auth Protocol ✅
- `backend/p2p/gater.go`: `AuthGater` blacklist-based (not whitelist). New peers allowed through; blacklisted on handshake failure.
- `backend/p2p/auth_protocol.go`: Protocol `/app/auth/1.0.0`. Wire format: `[4B uint32 length][JSON token]`. Client (outbound) sends first; Server (inbound) reads first — avoids deadlock. `authNetworkNotifee` triggers `InitiateHandshake` only for outbound connections.
- `backend/p2p/host.go`: `NewP2PNode` signature updated with `localToken *admin.InvitationToken` and `rootPubKey []byte`. Integrates `libp2p.ConnectionGater(gater)`.

### 3.6. Integration ✅
- `backend/main.go` fully rewritten: `DetermineAppState` branching, new CLI flags (`--setup`, `--admin-setup`, `--create-bundle`, `--import-bundle`), `waitForCryptoEngine` with retry loop, `runAdminSetup`, `runCreateBundle`.
- **Validation scenarios:**
  - Node A (Admin) + Node B (valid bundle): connect + auth ✅
  - Node C (no bundle / `StateUninitialized`): P2P not started ✅
  - Node D (expired token): rejected at `verifyPeerToken` ✅
  - Eve replaying Alice's token: `token.PeerID != noise_peer` → rejected ✅

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
- **CRITICAL DESIGN NOTE:** The `.backup` file MUST include the Libp2p private key in addition to the MLS key.
  - **Reason:** `InvitationToken` binds `PeerID` (derived from Libp2p key) to the MLS public key. If a new device generates a new PeerID, the token becomes invalid and auth handshake fails.
  - **Solution:** Exporting the Libp2p private key allows the new device to restore the exact same PeerID, making the existing token valid.
- **Task:** Go UI: Prompt User for a strong Passphrase.
- **Task:** Go Logic: Read BOTH private keys from DB:
  - `libp2p_private_key` from `system_config` table
  - `mls_signing_key` + `mls_credential` + `invitation_token` from `mls_identity` and `auth_bundle` tables
- **Task:** Send all of the above to Rust `ExportIdentity(data, passphrase)`.
- **Task:** Rust Logic: Serialize all fields -> Encrypt with AES-256-GCM (Key derived from Passphrase via Argon2id) -> Return `EncryptedBlob`.
- **Task:** Go Logic: Save `EncryptedBlob` to a `.backup` file.
- **Task:** Import Flow: Read `.backup` file -> Send to Rust `ImportIdentity(blob, passphrase)` -> Restore ALL keys -> Store in DB -> Broadcast `KILL_SESSION`.

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