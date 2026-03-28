# PROJECT PLAN: DECENTRALIZED COORDINATION PROTOCOL FOR MLS ON P2P NETWORKS

**Role:** Graduation Thesis Project
**Nature:** Protocol Research + Application Implementation
**Architecture:** Distributed System, Sidecar Pattern, Local-first
**Core Stack:**
- **Frontend/Host:** Wails (Go) + React/TypeScript
- **Networking:** Go-libp2p
- **Crypto Engine:** Rust (OpenMLS) running as a gRPC Server
- **Database:** SQLite (Managed by Go)
- **IPC:** gRPC (Protobuf) over localhost with dynamic port assignment

**Research Focus:** Design a Decentralized Coordination Protocol wrapping MLS (RFC 9420) that maintains causal consistency and total ordering on a P2P network without a central Delivery Service. The protocol is built on four mechanisms: Single-Writer Protocol, Epoch Consistency, Group Fork Healing, and Hybrid Logical Clock (HLC).

---

## 1. SYSTEM ARCHITECTURE & SETUP (Weeks 1-2) [COMPLETED ✅]

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

## 2. P2P NETWORKING LAYER (Weeks 3-5) [COMPLETED ✅]

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

### 3.7. Wails GUI Integration ✅
- Thư mục `backend/` đã đổi tên thành `app/`, Go module name đổi từ `backend` → `app`.
- Tích hợp Wails v2.11.0 vào Go app (`app/app.go`).
- Scaffold React + TypeScript + Tailwind frontend tại `app/frontend/`.
- 4 màn hình dev/test UI đã hoàn chỉnh: SetupScreen, AwaitingBundleScreen, DashboardScreen, AdminPanel.

---

## 4. MLS GROUP CHAT + DECENTRALIZED COORDINATION PROTOCOL (Weeks 8-12)

**Goal:** Integrate OpenMLS via gRPC to enable E2EE group chat, and implement the Decentralized Coordination Protocol — the **core research contribution** of this thesis — to maintain MLS state consistency without a central Delivery Service.

**Core Problem:** MLS (RFC 9420) requires a Delivery Service to serialize state-changing Commits. Without it, concurrent Commits cause DAG forking and break the Ratchet Tree. The Coordination Protocol solves this by wrapping MLS with four mechanisms: Single-Writer Protocol, Epoch Consistency, Group Fork Healing, and Hybrid Logical Clock.

### 4.1. MLS Proto Definitions & Rust Engine Extensions [COMPLETED ✅ — Stub]
- **Task:** Update `mls_service.proto` with Group operations:
  - `CreateGroup(GroupId, CreatorIdentity) → GroupState`
  - `CreateProposal(GroupState, ProposalType, Data) → ProposalBytes`
  - `CreateCommit(GroupState, Proposals[]) → CommitBytes, WelcomeBytes, NewGroupState`
  - `ProcessCommit(GroupState, CommitBytes) → NewGroupState`
  - `ProcessWelcome(WelcomeBytes, SigningKey) → GroupState`
  - `EncryptMessage(GroupState, Plaintext) → MlsCiphertext, NewGroupState`
  - `DecryptMessage(GroupState, MlsCiphertext) → Plaintext, NewGroupState`
  - `ExternalJoin(GroupInfo, SigningKey) → GroupState, CommitBytes`
  - `ExportSecret(GroupState, Label, Length) → DerivedKey`
- **Task:** Implement all handlers in Rust as **stateless** functions: Receive GroupState bytes → Deserialize → Operate → Serialize → Return.
- **Implementation Note:** GroupState is opaque bytes from Go's perspective. Go stores it in SQLite; Rust deserializes it into the OpenMLS Ratchet Tree internally.
- **Status:** Proto updated with 9 new RPCs (`CreateGroup`, `CreateProposal`, `CreateCommit`, `ProcessCommit`, `ProcessWelcome`, `EncryptMessage`, `DecryptMessage`, `ExternalJoin`, `ExportSecret`). Go protobuf regenerated. Rust handlers implemented as deterministic stubs (string-based group state, prefix-based encrypt/decrypt). Stubs allow full pipeline testing; real OpenMLS integration pending.
- **Go adapter:** `app/coordination/mls_adapter.go` — `GrpcMLSEngine` wraps gRPC client to `MLSEngine` interface.

### 4.2. Database Schema for Groups & Coordination [COMPLETED ✅]
- **Task:** Add `mls_groups` table:
  ```sql
  CREATE TABLE mls_groups (
      group_id       TEXT PRIMARY KEY,
      group_state    BLOB NOT NULL,        -- serialized OpenMLS group state
      epoch          INTEGER NOT NULL,      -- current epoch number
      tree_hash      BLOB,                  -- current tree hash (for fork detection)
      my_role        TEXT DEFAULT 'member',  -- 'creator' | 'member'
      created_at     DATETIME DEFAULT CURRENT_TIMESTAMP,
      updated_at     DATETIME DEFAULT CURRENT_TIMESTAMP
  );
  ```
- **Task:** Add `coordination_state` table:
  ```sql
  CREATE TABLE coordination_state (
      group_id           TEXT PRIMARY KEY,
      active_view        TEXT NOT NULL,     -- JSON array of online peer IDs
      token_holder       TEXT,              -- current epoch's Token Holder PeerID
      last_commit_hash   BLOB,             -- hash of last processed Commit
      last_commit_at     DATETIME,
      pending_proposals  TEXT,              -- JSON array of buffered Proposals
      FOREIGN KEY (group_id) REFERENCES mls_groups(group_id)
  );
  ```
- **Status:** Both tables created in `app/db/db.go`. Added `stored_messages` table with HLC-based index for message ordering. `app/db/coordination_storage.go` implements `CoordinationStorage` interface (UPSERT for GroupRecord/CoordState, HLC-ordered message queries). **8 tests PASS.**
- **Transport adapter:** `app/p2p/transport_adapter.go` — `LibP2PTransport` wraps real libp2p GossipSub + direct streams (`/coordination/direct/1.0.0`) to `Transport` interface.

### 4.3. Mechanism 1 — Single-Writer Protocol (Go Coordination Layer) [COMPLETED ✅]
- **Package:** `app/coordination/single_writer.go`
- **Task: ActiveView Management** (`app/coordination/active_view.go`):
  - Maintain set of online peers per group via Gossip Heartbeat (periodic `PeerAlive` messages).
  - Peer is removed from ActiveView after missing `N` consecutive heartbeats.
  - ActiveView changes trigger Token Holder recomputation.
- **Task: Implicit Token Holder Election:**
  ```go
  func ComputeTokenHolder(activeView []peer.ID, epoch uint64) peer.ID {
      // TokenHolder = argmin_{node ∈ ActiveView} H(nodeID || epoch)
      // H = SHA-256. All nodes compute the same result deterministically.
  }
  ```
- **Task: Proposal Routing:**
  - Any node creates a Proposal → broadcasts via GossipSub topic `{group_id}/proposals`.
  - All nodes buffer received Proposals locally.
  - Only Token Holder calls `CreateCommit(GroupState, bufferedProposals)`.
- **Task: Commit Broadcasting:**
  - Token Holder broadcasts Commit via GossipSub topic `{group_id}/commits`.
  - All nodes process Commit → advance to epoch E+1 → recompute Token Holder.
- **Task: Failover (Token Holder Timeout):**
  - If no Commit is received within `T_timeout` (configurable, default 3–5s):
    - Evict Token Holder from ActiveView.
    - Recompute new Token Holder.
    - New holder assumes Commit authority immediately.
  - **Critical:** Timeout must account for network latency in LAN (typically <1ms).
- **Status:** `app/coordination/single_writer.go` — `ComputeTokenHolder` (argmin SHA-256), `BufferProposal`, `DrainProposals`, `AdvanceEpoch`. `app/coordination/active_view.go` — heartbeat tracking, liveness check, peer eviction, sorted member list, onChange callback. **18 tests PASS (9 + 9).**

### 4.4. Mechanism 2 — Epoch Consistency Checks (Go Coordination Layer) [COMPLETED ✅]
- **Package:** `app/coordination/epoch.go`
- **Task: Epoch Validation on every incoming MLS message:**
  ```go
  func (c *Coordinator) ValidateEpoch(msgEpoch, localEpoch uint64) EpochAction {
      switch {
      case msgEpoch == localEpoch:
          return ActionProcess
      case msgEpoch < localEpoch:
          return ActionRejectStale  // send CurrentEpochNotification
      default:
          return ActionBufferFuture // request state sync
      }
  }
  ```
- **Task:** Implement `CurrentEpochNotification` message (sent via direct P2P stream, not GossipSub).
- **Task:** Implement state sync protocol: node with stale epoch requests `GroupInfo` + recent Commits from an up-to-date peer.
- **Status:** `app/coordination/epoch.go` — `ValidateEpoch`, `EpochTracker`, future buffer with defensive copies, `Advance` returns buffered messages. **9 tests PASS.**

### 4.5. Mechanism 3 — Group Fork Healing (Go Coordination Layer) [COMPLETED ✅]
- **Package:** `app/coordination/fork_healing.go`
- **Task: Gossip Heartbeat with GroupStateAnnouncement:**
  ```go
  type GroupStateAnnouncement struct {
      GroupID    string
      TreeHash   []byte
      Epoch      uint64
      MemberCount int    // online members in this branch
      CommitHash []byte  // hash of last Commit
  }
  ```
  - Broadcast periodically (e.g., every 5 seconds) on topic `{group_id}/heartbeat`.
  - Receiving a `GroupStateAnnouncement` with different `TreeHash` at higher or equal epoch → partition detected.
- **Task: Branch Weight Comparison Function:**
  ```go
  // W = (C_members, E, H_commit) — compared lexicographically
  func CompareBranchWeight(local, remote GroupStateAnnouncement) BranchResult {
      if local.MemberCount != remote.MemberCount {
          return compare(local.MemberCount, remote.MemberCount)
      }
      if local.Epoch != remote.Epoch {
          return compare(local.Epoch, remote.Epoch)
      }
      return compareBytes(local.CommitHash, remote.CommitHash)
  }
  ```
- **Task: Healing Process (losing branch):**
  1. Drop current MlsGroup from Go memory (NOT from SQLite — keep for audit).
  2. Request `GroupInfo` from winning branch peer.
  3. Validate Committer signature in `GroupInfo` (X.509 certificate).
  4. Call Rust `ExternalJoin(GroupInfo, mySigningKey) → NewGroupState`.
  5. Save new GroupState to SQLite → advance to winning branch's epoch.
  6. **Autonomous Replay:** Re-encrypt and resend own messages from the partition period. MUST NOT resend other nodes' messages (Non-repudiation).
- **Security Analysis:**
  - Forward Secrecy: Preserved — losing branch keys are destroyed (crypto-shredding).
  - PCS: Temporarily weakened during partition, restored immediately after External Join.
- **Status:** `app/coordination/fork_healing.go` — `CompareBranchWeight` (W = MemberCount > CommitHash > TreeHash), `ForkDetector` with ProcessRemote/UpdateLocal. **10 tests PASS.**

### 4.6. Group Operations Integration (End-to-End Flow) [COMPLETED ✅]
- **Task:** Implement "Create Group" flow:
  - Creator calls Rust `CreateGroup` → stores GroupState + epoch 0 in SQLite.
  - Creator is Token Holder for epoch 0 (sole member).
  - Creator broadcasts `GroupCreated` announcement.
- **Task:** Implement "Add Member" flow:
  - Any member creates `Add` Proposal → GossipSub.
  - Token Holder collects → Commit → Welcome message sent to new member via direct stream.
  - New member calls Rust `ProcessWelcome` → joins group at current epoch.
- **Task:** Implement "Remove Member" flow:
  - Any member creates `Remove` Proposal → GossipSub.
  - Token Holder collects → Commit → removed member's keys are evicted from tree.
- **Task:** Implement "Continuous Key Rotation" (leveraging MLS O(log N)):
  - Periodic `Update` Proposals generated automatically (configurable interval).
  - Token Holder batches Updates into Commits.
  - Ensures PCS: compromised device keys become stale within one rotation cycle.
- **Status:** Wails bindings created in `app/group_ops.go` exposing `CreateGroupChat`, `SendGroupMessage`, `GetGroupMessages`, `GetGroups`, `GetGroupStatus` to frontend. `app/app.go` modified with coordination stack fields (`transport`, `coordStorage`, `mlsEngine`, `coordinators` map), `initCoordinationStackLocked()` on P2P start, and `stopCoordinatorsLocked()` in teardown. Real OpenMLS implementation in Rust replaces stubs. Frontend `ChatPanel.tsx` component built with group creation, message sending/receiving, and real-time updates via Wails events.

### 4.7. Mechanism 4 — Hybrid Logical Clock (Message Display Ordering) [COMPLETED ✅]
- **Package:** `app/coordination/hlc.go`
- **Problem:** Within a single epoch, multiple users send messages concurrently. GossipSub does not guarantee delivery order. Without an ordering mechanism, each node displays messages in a different sequence. Wall clocks cannot be trusted in air-gapped networks without NTP.
- **Task: Implement HLC type** `HLCTimestamp { WallTimeMs int64, Counter uint32, NodeID string }`:
  - `Before(other)` — lexicographic comparison for total ordering.
  - JSON-serializable for wire transport and SQLite storage.
- **Task: Implement HLC engine** `HLC { clock Clock, mu sync.Mutex, l int64, c uint32, id string }`:
  - `Now() HLCTimestamp` — called on send or local event.
  - `Update(received HLCTimestamp) HLCTimestamp` — called on message receive, merges remote clock.
  - Uses the injectable `Clock` interface — deterministic in tests via `FakeClock`.
- **Status:** `app/coordination/hlc.go` — HLC engine with `Now()`, `Update()`, thread-safe, injectable Clock. `app/coordination/config.go` — `CoordinatorConfig`, `DefaultConfig`, `TestConfig`, `Validate`. `app/coordination/metrics.go` — thread-safe counters + latency samples. `app/coordination/clock_real.go` — `RealClock` for production. **20 tests PASS (8 HLC + 4 config + 4 metrics + 4 clock).**
- **Properties guaranteed:**
  - Causal consistency: if A happened-before B, then `HLC(A) < HLC(B)`.
  - Total order: all nodes sort messages identically.
  - Wall-clock proximity: `L` is always ≥ physical time, bounded drift.
  - NTP-independent: works in air-gapped networks.

### 4.8. Messaging (Encrypt/Decrypt through Coordination Layer) [COMPLETED ✅]
- **Task:** Implement message send flow (see README Section 3.2):
  - Generate HLC timestamp → encrypt via Rust → wrap in Envelope with epoch + HLC → broadcast.
- **Task:** Implement message receive flow (see README Section 3.3):
  - Epoch check → update local HLC → decrypt via Rust → store with HLC timestamp → emit to UI.
- **Task:** All messages tagged with both `epoch_number` (for MLS validation) and `HLCTimestamp` (for display ordering).
- **Task:** UI sorts and displays messages by HLC — uses `WallTimeMs` for human-readable time.
- **Status:** Full pipeline wired end-to-end. `app/group_ops.go` exposes `SendGroupMessage` and `GetGroupMessages` via Wails bindings. `ChatPanel.tsx` renders messages sorted by HLC timestamp with real-time updates via `EventsOn("group:message")`. Real OpenMLS encryption (`MlsGroup::create_message`) used in Rust — forward secrecy preserved. **62 Go tests + 4 Rust tests PASS.**

### 4.9. Phase 4 Summary [COMPLETED ✅]

**All Phase 4 tasks completed (66 tests total: 62 Go + 4 Rust):**
- Coordination Layer: all 4 mechanisms + Coordinator orchestrator (54 Go tests)
- Infrastructure: Proto (13 RPCs), DB schema + storage (8 Go tests), Transport adapter, gRPC adapter
- Real OpenMLS: `MlsGroupStore` (in-memory `Arc<Mutex<HashMap>>`), `create_group`, `encrypt_message`, `decrypt_message`, `create_commit` (self_update), `process_commit`, `process_welcome`, `export_secret` — all using real OpenMLS 0.8 crypto (4 Rust tests)
- Wails Bindings: `app/group_ops.go` with `CreateGroupChat`, `SendGroupMessage`, `GetGroupMessages`, `GetGroups`, `GetGroupStatus` + coordination stack initialization in `app/app.go`
- Frontend Chat UI: `ChatPanel.tsx` with group creation, message display (HLC-sorted), real-time updates via Wails events
- All code: `go vet` clean, `go build ./...` clean, `cargo build` clean, `cargo test` clean, `tsc --noEmit` clean

**Remaining for full end-to-end validation (Phase 7):**
- Multi-node testing on 2-3 real instances (manual test)
- Add Member flow requires KeyPackage generation (not yet implemented in proto)
- External Join requires verifiable GroupInfo from winning branch (stub only)

---

## 5. ADVANCED FEATURES: MIGRATION & OFFLINE (Weeks 13-14)

**Goal:** Secure Identity Migration (Manual) and Offline Messaging (Store-and-Forward).

### 5.1. Secure Identity Export/Import (File-based)
- **CRITICAL DESIGN NOTE:** The `.backup` file MUST include the Libp2p private key in addition to the MLS key.
  - **Reason:** `InvitationToken` binds `PeerID` (derived from Libp2p key) to the MLS public key. If a new device generates a new PeerID, the token becomes invalid and auth handshake fails.
  - **Solution:** Exporting the Libp2p private key allows the new device to restore the exact same PeerID, making the existing token valid.
- **Task:** Go UI: Prompt User for a strong Passphrase.
- **Task:** Go Logic: Read BOTH private keys from DB:
  - `libp2p_private_key` from `system_config` table
  - `mls_signing_key` + `mls_credential` + `invitation_token` from `mls_identity` and `auth_bundle` tables
- **Task:** Send all of the above to Rust `ExportIdentity(data, passphrase)`.
- **Task:** Rust Logic: Serialize all fields → Encrypt with AES-256-GCM (Key derived from Passphrase via Argon2id) → Return `EncryptedBlob`.
- **Task:** Go Logic: Save `EncryptedBlob` to a `.backup` file.
- **Task:** Import Flow: Read `.backup` file → Send to Rust `ImportIdentity(blob, passphrase)` → Restore ALL keys → Store in DB → Broadcast `KILL_SESSION`.

### 5.2. Session Takeover (Single Active Device)
- **Task:** On successful Import & Connect: Broadcast `KILL_SESSION` (Signed by User Key).
- **Task:** Active clients listening to `KILL_SESSION`: Verify signature → Self-destruct if valid.

### 5.3. Offline Messaging (Store-and-Forward via Neighborhood Storage)
- **Task:** If recipient is offline: `dht.Put(Key=Hash(RecipientID), Value=EncryptedMsg)`.
- **Task:** On recipient connect: `dht.Get(Key=Hash(MyID))` → retrieve and process buffered messages.
- **Task:** Messages are encrypted with MLS group key — only the intended recipient's MLS state can decrypt.
- **Task:** Automatic cleanup: DHT entries expire after configurable TTL.

---

## 6. FILE TRANSFER & FINALIZATION (Weeks 15-16)

**Goal:** Secure high-speed file transfer and thesis documentation.

### 6.1. Secure Direct Swarming (MLS Exporter-based)
- **Task:** Derive one-time symmetric key using `MLS Exporter` (label: `"file-transfer"`, context: `file_hash`).
- **Task:** Sender encrypts file with derived key → splits into fixed-size chunks.
- **Task:** Announce file metadata (hash, size, chunk count, derived key label) via GossipSub.
- **Task:** Implement chunk exchange protocol `/app/file/1.0.0`:
  - Receivers request chunks from sender AND from other receivers who already have them (swarming).
  - Multi-stream parallel download — optimizes LAN bandwidth.
- **Task:** Receiver reassembles chunks → decrypt → verify hash.
- **Task:** UI: Progress bar, file selection dialog.

### 6.2. Thesis Report & Defense Prep
- **Task:** Finalize diagrams (architecture, protocol flow, fork healing sequence).
- **Task:** Prepare Demo environment (multi-node Docker / multi-process on single machine).
- **Task:** Write evaluation results (see Phase 7).

---

## 7. EVALUATION & TESTING (Throughout + Final Weeks)

**Goal:** Prove correctness, performance, and security of the Decentralized Coordination Protocol.

### 7.1. Correctness & Consistency (Concurrency Chaos Test)
- **Task:** Simulate 10+ nodes sending Proposals and Commits simultaneously within the same millisecond.
- **Success Criteria:** All nodes converge to the same Epoch number AND their TreeHash values match perfectly across every device.
- **Task:** Verify Single-Writer invariant: at no point do two Commits exist for the same epoch.

### 7.2. Partition & Healing Test
- **Task:** Physically disconnect network into 2 independent branches (split-brain simulation).
- **Task:** Let each branch evolve independently for multiple epochs.
- **Task:** Reconnect and measure:
  - Branch Weight W correctly selects the winning branch.
  - External Join success rate for losing branch nodes.
  - Time to full convergence (all nodes on same epoch + TreeHash).

### 7.3. Performance & Latency
- **Task:** Measure epoch finalization time (Proposal → Commit → all nodes at E+1).
- **Task:** Compare overhead vs. a baseline system using a centralized Delivery Service.
- **Task:** Scalability: measure CPU and bandwidth consumption as group size increases during periodic key rotation — empirically validate O(log N) advantage of MLS.

### 7.4. Security & Threat Validation
- **Task:** Extract SQLite database of a "losing branch" node after a Partition & Healing test.
- **Task:** Verify that staged Commit keys have been destroyed (crypto-shredding) — Forward Secrecy proof.
- **Task:** Verify Token Replay attack is blocked (Eve replaying Alice's token → rejected).
- **Task:** Verify expired tokens are rejected at auth handshake.

**Validation Criteria:**
> All nodes converge to identical state after concurrent operations and network partitions. O(log N) scaling empirically demonstrated. Forward Secrecy preserved after fork healing. Identity migration succeeds with session takeover. File transfer completes via swarming protocol.
