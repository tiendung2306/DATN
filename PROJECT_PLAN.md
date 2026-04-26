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

> **Đường dẫn mã (2026):** Go app trong `app/` (module `app`). SQLite: `app/adapter/store`. Libp2p: `app/adapter/p2p`. Rust sidecar + gRPC MLS: `app/adapter/sidecar` (`GrpcMLSEngine` trong `engine.go`). Wails: `app/main.go`, bind `*service.Runtime` (`app/service/`), TS: `app/frontend/wailsjs/go/service/Runtime`. Chi tiết: `CURRENT_STATE.md` mục *Agent — Bản đồ mã nguồn*.

**Research Focus:** Design a Decentralized Coordination Protocol wrapping MLS (RFC 9420) that maintains causal consistency and total ordering on a P2P network without a central Delivery Service. The protocol is built on four mechanisms: Single-Writer Protocol, Epoch Consistency, Group Fork Healing, and Hybrid Logical Clock (HLC).

---

## 1. SYSTEM ARCHITECTURE & SETUP (Weeks 1-2) [COMPLETED ✅]

**Goal:** Establish the IPC infrastructure where the Go application manages the Rust sidecar process lifecycle securely and reliably.

### 1.1. Monorepo Structure & Protobuf Definition
- **Task:** Define the directory structure.
  - `/proto`: Shared `.proto` definitions.
  - `/app`: Go host (Wails, Libp2p, SQLite); frontend nguồn tại `app/frontend/`.
  - `/crypto-engine`: Rust code (gRPC server, OpenMLS logic).
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
- `app/admin/token.go`: `InvitationToken`, `InvitationBundle`, `SignToken`, `VerifyToken`, `SerializeBundle`, `DeserializeBundle`.
- `app/admin/admin.go`: `SetupAdminKey`, `UnlockAdminKey`, `CreateInvitationBundle`. Encryption: Argon2id(passphrase, salt) → AES-256-GCM. Wire format: `[16B salt][12B nonce][ciphertext]`.

### 3.4. App States & Onboarding Flow ✅
- `app/service/app_state.go`: `StateUninitialized`, `StateAwaitingBundle`, `StateAuthorized`, `StateAdminReady`, `DetermineAppState(db)`.
- `app/adapter/p2p/auth.go`:
  - `OnboardNewUser(ctx, db, mlsClient)` — no `displayName` param; credential set empty.
  - `GetOnboardingInfo(db, privKey) *OnboardingInfo` — returns `{PeerID, PublicKeyHex}` only.
  - `ImportInvitationBundle(db, privKey, bundleJSON)` — 4 checks + calls `UpdateMLSDisplayName`.
  - `BuildLocalToken(bundle) *admin.InvitationToken` — reconstructs full token from stored bundle.

### 3.5. Connection Gating & Auth Protocol ✅
- `app/adapter/p2p/gater.go`: `AuthGater` blacklist-based (not whitelist). New peers allowed through; blacklisted on handshake failure.
- `app/adapter/p2p/auth_protocol.go`: Protocol `/app/auth/1.0.0`. Wire format: `[4B uint32 length][JSON payload]` (`AuthHandshakeMsg`: token + optional signed `SessionClaim`). Client (outbound) sends first; Server (inbound) reads first — avoids deadlock.
- `app/adapter/p2p/host.go`: `NewP2PNode` với `localToken *admin.InvitationToken` và `rootPubKey []byte`. Integrates `libp2p.ConnectionGater(gater)`.

### 3.6. Integration ✅
- `app/main.go` (composition root) + `app/cli` (`--setup`, `--admin-setup`, `--create-bundle`, `--import-bundle`, headless, …) + `service.Runtime` lifecycle; spawn sidecar / crypto client qua `adapter/sidecar`.
- **Validation scenarios:**
  - Node A (Admin) + Node B (valid bundle): connect + auth ✅
  - Node C (no bundle / `StateUninitialized`): P2P not started ✅
  - Node D (expired token): rejected at `verifyPeerToken` ✅
  - Eve replaying Alice's token: `token.PeerID != noise_peer` → rejected ✅

### 3.7. Wails GUI Integration ✅
- Go module `app`; Wails v2.11.0 — `app/adapter/wailsui` gọi `wails.Run`, bind `*service.Runtime`, `EventSink` → UI events.
- Scaffold React + TypeScript + Tailwind tại `app/frontend/`.
- 4 màn hình dev/test UI đã hoàn chỉnh: SetupScreen, AwaitingBundleScreen, DashboardScreen, AdminPanel.

---

## 4. MLS GROUP CHAT + DECENTRALIZED COORDINATION PROTOCOL (Weeks 8-12) [COMPLETED ✅]

**Goal:** Integrate OpenMLS via gRPC to enable E2EE group chat, and implement the Decentralized Coordination Protocol — the **core research contribution** of this thesis — to maintain MLS state consistency without a central Delivery Service.

**Core Problem:** MLS (RFC 9420) requires a Delivery Service to serialize state-changing Commits. Without it, concurrent Commits cause DAG forking and break the Ratchet Tree. The Coordination Protocol solves this by wrapping MLS with four mechanisms: Single-Writer Protocol, Epoch Consistency, Group Fork Healing, and Hybrid Logical Clock.

### 4.1. MLS Proto Definitions & Rust Engine Extensions [COMPLETED ✅]
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
- **Status:** Proto có các RPC nhóm (CreateGroup … ExportSecret), sau đó bổ sung `GenerateKeyPackage`, `AddMembers`; `ProcessWelcome` mở rộng (epoch + `key_package_bundle_private` cho invitee). Go protobuf regenerated. Rust OpenMLS 0.8, stateless: nhận `group_state` bytes → operate → trả bytes mới.
- **Go bridge:** `GrpcMLSEngine` trong `app/adapter/sidecar/engine.go` — implements `coordination.MLSEngine` (không còn `app/coordination/mls_adapter.go`).

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
- **Status:** Schema và migration trong `app/adapter/store/db.go`; `app/adapter/store/coordination_storage.go` implements `CoordinationStorage` (UPSERT GroupRecord/CoordState, HLC-ordered messages). **`go test ./adapter/store` — PASS.**
- **Transport adapter:** `app/adapter/p2p/transport_adapter.go` — `LibP2PTransport` (GossipSub + direct streams `/coordination/direct/1.0.0`).

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
- **Status:** Wails methods trên `*service.Runtime` trong `app/service/group.go`, `messaging.go`, `invite.go`, …; stack coordination trong `app/service/runtime.go` + `initCoordinationStackLocked` / `stopCoordinatorsLocked` trong `group.go`. Frontend `ChatPanel.tsx`: tạo nhóm, tin nhắn, members/keys, events Wails.

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
- **Status:** `app/coordination/hlc.go` — HLC engine with `Now()`, `Update()`, thread-safe, injectable Clock. `app/coordination/config.go` — `CoordinatorConfig`, `DefaultConfig`, `TestConfig`, `Validate`. `app/coordination/metrics.go` — thread-safe counters + latency samples. `app/coordination/clock_real.go` — `RealClock`; `FakeClock` trong `clock_fake_test.go` (test-only). **`go test ./coordination` — PASS** (gồm table-driven subtests).
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
- **Status:** Full pipeline end-to-end; `SendGroupMessage` / `GetGroupMessages` trên `service.Runtime` + `ChatPanel.tsx` (HLC sort, `EventsOn("group:message")`). Rust: OpenMLS encrypt/decrypt. **Xác minh:** `cd app && go test ./...` ; `cd crypto-engine && cargo test` — PASS.

### 4.9. Phase 4 Summary [COMPLETED ✅]

**Phase 4 (coordination + MLS group chat) hoàn tất:**
- **Coordination:** `app/coordination/*` — bốn cơ chế + `Coordinator`; **`go test ./coordination` — PASS**.
- **Proto / RPC:** `proto/mls_service.proto` — **15 RPC** (gồm identity, nhóm, `GenerateKeyPackage`, `AddMembers`).
- **Persistence + transport:** `app/adapter/store`, `app/adapter/p2p/transport_adapter.go`; **gRPC:** `app/adapter/sidecar/engine.go`.
- **Rust:** `crypto-engine` — stateless OpenMLS; **`cargo test` — 8 tests PASS** (gồm `test_generate_key_package`, `test_add_member_and_welcome`).
- **Wails:** `app/service/*.go` (group, messaging, invite, …), `app/service/runtime.go`; frontend `ChatPanel.tsx`.
- **Build:** `go vet ./...`, `go build ./...`, `cargo build`, `cargo test` clean (kèm `npm run build` frontend khi cần).

**Remaining for full end-to-end validation (Phase 7/8):**
- Multi-node testing on 2-3 real instances (manual test)
- External Join / verifiable `GroupInfo` từ nhánh thắng — cần kiểm thử đa kịch bản và thu bằng chứng hội tụ
- Legacy groups created with old metadata-only `group_state` must be recreated/migrated

---

## 5. ADVANCED FEATURES: MIGRATION & OFFLINE (Weeks 13-14)

**Goal:** Secure Identity Migration (Manual) and Offline Messaging (Store-and-Forward).

**Trạng thái (đối chiếu code):** **5.1**, **5.2** và **5.3** đã implement (`.backup` + `SessionClaim` + offline store-and-forward qua sync stream). DHT mailbox đã được thay thế bởi stream-sync + local retention.

### 5.1. Secure Identity Export/Import (File-based) [COMPLETED ✅]
- **CRITICAL DESIGN NOTE:** The `.backup` file MUST include the Libp2p private key in addition to the MLS key.
  - **Reason:** `InvitationToken` binds `PeerID` (derived from Libp2p key) to the MLS public key. If a new device generates a new PeerID, the token becomes invalid and auth handshake fails.
  - **Solution:** Exporting the Libp2p private key allows the new device to restore the exact same PeerID, making the existing token valid.
- **Task:** Go UI: Prompt User for a strong Passphrase.
- **Task:** Go Logic: Read BOTH private keys from DB:
  - `libp2p_private_key` from `system_config` table
  - `mls_signing_key` + `mls_credential` + `invitation_token` from `mls_identity` and `auth_bundle` tables
- **Task:** Go Logic: Serialize identity payload and encrypt with AES-256-GCM (Key derived from Passphrase via Argon2id) → Save `.backup`.
- **Task:** Include full user content snapshot for migration UX parity:
  - `mls_groups` (group states)
  - `stored_messages` (local chat history)
  - `kp_bundles` and `pending_welcomes_out` (invite flows in progress)
- **Task:** Import Flow: Read `.backup` → Decrypt in Go → Restore identity + content snapshot in SQLite.
- **Task:** Backward compatibility: `.backup` versioning must support old identity-only backups.

**Implemented:** `app/service/identity_backup.go`, CLI `app/cli`, Wails `ExportIdentity` / `ImportIdentityFromFile`; format v2 + import v1 (xem `CURRENT_STATE.md` §4).

### 5.2. Session Takeover (Single Active Device) [COMPLETED ✅]
- **Task:** Extend `/app/auth/1.0.0` handshake with signed `SessionClaim`:
  - payload includes `session_started_at` + nonce
  - signature verified with MLS public key from `InvitationToken`
- **Task:** Per-peer session arbitration:
  - newer session accepted
  - stale session rejected
  - concurrent old connections for same PeerID closed

**Implemented:** `app/adapter/p2p/session_claim.go`, `auth_protocol.go` (`AuthHandshakeMsg`), `app/service/session.go`, tests trong `session_claim_test.go`.

### 5.3. Offline Messaging (Store-and-Forward via Stream Sync + Local Retention) [COMPLETED ✅]
- **Task:** If recipient is offline: sender (and peers who already received the envelope) persist encrypted envelopes in local SQLite `envelope_log`.
- **Task:** On recipient reconnect: node pulls missed envelopes via authenticated direct stream `/app/offline-sync/1.0.0` and replays in-order.
- **Task:** Messages are encrypted with MLS group key — only recipients with valid MLS state can decrypt.
- **Task:** Automatic cleanup: envelope retention is bounded by TTL and per-group cap (`PruneEnvelopes`).

**Implemented:** `app/service/offline_sync.go`, `app/adapter/p2p/offline_wire.go`, `app/adapter/store/coordination_storage.go` + schema trong `app/adapter/store/db.go`.
- Offline sync stream `/app/offline-sync/1.0.0`: pull envelope log theo `seq`, replay, và ACK cursor.
- Runtime triggers: peer connect (`scheduleOfflineSyncPull` with short retry/backoff) + manual trigger (`TriggerOfflineSync`).
- DHT application-data mailbox path đã loại bỏ (`app/adapter/p2p/offline_dht.go` removed). Kademlia DHT giữ vai trò discovery/routing.

### 5.4. Universal Blind-Store Layer + Store Node Role [COMPLETED ✅]
- **Task:** Add a global blind-store topic for offline replication artifacts: `/org/offline-store/v1`.
- **Task:** Support runtime roles:
  - regular nodes subscribe by default and persist only when selected as replica target.
  - `--store-node`: always persist blind-store objects.
  - `--blind-store-participant=false`: explicit opt-out from selective replica storage.
  - `--offline-replica-k`: number of non-store replica targets.
- **Task:** Publish local `MsgCommit` / `MsgApplication` envelopes to blind-store via coordinator callback hook.
- **Task:** Replicate invite artifacts (`KeyPackage`, `Welcome`) through blind-store in addition to existing store streams.
- **Task:** Select replica targets using Kademlia proximity (`GetClosestPeers`) with XOR-distance fallback.
- **Task:** Add envelope dedup persistence to avoid duplicate replay (`envelope_dedup` + `AppendEnvelope` dedup path).

**Implemented:** `app/service/blind_store.go`, `app/coordination/coordinator.go` (`OnEnvelopeBroadcast`), `app/config/config.go` (new flags), `app/service/invite.go` (blind-store publish + fetch candidates), `app/adapter/store/db.go` + `app/adapter/store/coordination_storage.go` (dedup).

---

## 6. BACKEND PRODUCTIZATION BEFORE FRONTEND (Weeks 15-16)

**Goal:** Complete the core backend product flows required by the production frontend. The current backend already has the protocol, MLS, onboarding, offline sync, blind-store, and backup foundations. This phase turns those foundations into stable Wails-facing product APIs so the frontend does not need to fake critical behavior.

**Detailed execution reference:** `BACKEND_IMPLEMENTATION_PLAN.md`.

### 6.1. Invite & Pending Invite Lifecycle [P0]
- **Task:** Wrap the existing KeyPackage / Welcome / invite-store primitives into user-facing APIs:
  - `GenerateJoinCode`
  - `ListPendingInvites`
  - `AcceptInvite`
  - `RejectInvite`
- **Task:** Persist enough pending invite metadata for the UI: invite id, group id/name if known, inviter if known, received time, status.
- **Task:** Make invite accept idempotent and return stable errors for expired/stale invites, identity mismatch, and already-joined groups.
- **Success Criteria:** User can generate a join code, another member can invite them, and the invitee can accept/reject from a pending invite list without manually typing a group id.

### 6.2. Group Membership Lifecycle [P0]
- **Task:** Add Runtime-facing APIs for:
  - `LeaveGroup(groupID)`
  - `RemoveMemberFromGroup(groupID, peerID)` or explicitly return "not supported yet" if group roles are deferred.
- **Task:** Define product policy for leaving a group:
  - recommended: soft leave by default (stop active participation, keep local history).
- **Task:** Emit group/member change events for frontend refresh.
- **Success Criteria:** Group Info UI can perform real leave/remove actions or display a truthful disabled state backed by backend policy.

### 6.3. Session Takeover Lifecycle [P0]
- **Task:** Productize the existing `SessionClaim` single-active-device mechanism into explicit runtime state/events.
- **Task:** Add:
  - `GetSessionStatus`
  - event `session:replaced`
  - local replaced/lockout flag if needed.
- **Task:** Decide and implement old-device behavior after takeover. Recommended high-security default: block normal app access after session replacement, but do not claim secure deletion unless implemented.
- **Success Criteria:** Frontend can route to a real Session Replaced screen and the old device cannot continue normal P2P operations.

### 6.4. Startup & Runtime Health Events [P0]
- **Task:** Expose startup/runtime health to UI:
  - database init
  - Rust sidecar startup
  - IPC/gRPC readiness
  - app state detection
  - P2P startup.
- **Task:** Add stable error codes for database, crypto engine, IPC, identity, and P2P failures.
- **Task:** Add `GetRuntimeHealth` and/or startup events such as `startup:progress`, `startup:error`, `p2p:status`.
- **Success Criteria:** Splash screen and fatal error UI can show real backend state instead of guessing.

### 6.5. Admin Issuance Readiness [P0]
- **Task:** Make Admin flow safer for UI:
  - parse `request.json`
  - validate PeerID and MLS public key format
  - keep Display Name Admin-controlled and mandatory.
- **Task:** Decide admin unlock model:
  - short-term acceptable: passphrase-per-sign in `CreateBundle`
  - later: explicit `UnlockAdmin` / `LockAdmin` session.
- **Task:** Keep or extend `CreateBundle` so the UI can issue `.bundle` from parsed request data.
- **Success Criteria:** Admin can issue a signed bundle from `request.json` without manually copying long strings.

### 6.6. Network, Diagnostics, Message State, and Audit [P1]
- **Task:** Add runtime network/bootstrap controls:
  - view local multiaddr
  - validate/set bootstrap address
  - reconnect P2P.
- **Task:** Add diagnostics snapshot/export for Developer Mode:
  - peers, groups, epoch, token holder, tree hash, sync queue, recent errors.
- **Task:** Add message IDs/status/retry if the frontend needs failed-message recovery.
- **Task:** Add Admin issuance history if audit table is in scope.
- **Success Criteria:** Developer Mode and settings screens have real backend data, but these can be completed in parallel with frontend after P0 is done.

### 6.7. Backend Readiness Gate for Frontend
- **Required before full frontend rebuild:**
  - invite list/accept/reject works
  - group leave/remove policy is implemented or explicitly disabled
  - session replaced state/event exists
  - startup/runtime health exists
  - admin request JSON issuance path exists.
- **Verification:** `cd app && go test ./...` must pass after each backend slice.

---

## 7. FRONTEND APPLICATION UI (Weeks 17-18)

**Goal:** Upgrade the current dev/test UI into a production-ready frontend experience for real user workflows, using the backend APIs completed in Phase 6.

**Detailed execution reference:** `FRONTEND_IMPLEMENTATION_PLAN.md`.

### 7.1. Product UX Architecture & Navigation
- **Task:** Define app information architecture: startup, onboarding, dashboard, groups, messaging, invite, admin, settings, backup/import, diagnostics.
- **Task:** Standardize navigation model (left sidebar + main content + contextual panels) with responsive breakpoints.
- **Task:** Define design tokens (colors, typography, spacing, elevation, semantic states) and shared UI guidelines.

### 7.2. Rebuild Core Screens for End Users
- **Task:** Replace dev/test-first screens with polished user flows:
  - app initializing / fatal error
  - first-run setup (`UNINITIALIZED`)
  - bundle waiting/import (`AWAITING_BUNDLE`)
  - authorized dashboard (`AUTHORIZED`)
  - admin capabilities (`ADMIN_READY`)
  - session replaced lockout.
- **Task:** Add consistent loading/empty/error states for every major screen.
- **Task:** Add reusable component library (buttons, inputs, modal, toast, table/list, status badges).

### 7.3. Group Chat UX Completion
- **Task:** Improve group lifecycle UX: create/join, pending invites, member list, invite actions, group status/health indicators.
- **Task:** Improve chat UX: message composer, delivery state hints, timeline grouping, readable HLC-based ordering display.
- **Task:** Add clear visual cues for offline sync status and reconnect/replay events.

### 7.4. Settings, Admin, and Diagnostics UX
- **Task:** Build backup/import, session/device, network/bootstrap settings.
- **Task:** Build Admin setup/unlock, request parsing, bundle issuance, and issuance history if backend supports it.
- **Task:** Build Developer Mode overlays for P2P/protocol diagnostics.

### 7.5. Frontend Quality, Accessibility, and Build Stability
- **Task:** Add/expand frontend validation and type safety checks (`npm run build`, lint/typecheck setup if needed).
- **Task:** Improve accessibility baseline (keyboard navigation, focus states, contrast, ARIA labels on key interactions).
- **Task:** Ensure Wails bindings integration remains stable after UI refactor (`wails generate module` + TS import consistency).

### 7.6. Frontend Deliverables
- **Deliverable:** A coherent end-user UI usable for thesis demo without backend-dev-only controls.
- **Deliverable:** Screen-level acceptance checklist for onboarding, messaging, invite, migration, admin, and diagnostics flows.

---

## 8. FILE TRANSFER & FINALIZATION (Weeks 19-20)

**Goal:** Secure high-speed file transfer and thesis documentation.

### 8.1. Secure Direct Swarming (MLS Exporter-based)
- **Task:** Derive one-time symmetric key using `MLS Exporter` (label: `"file-transfer"`, context: `file_hash`).
- **Task:** Sender encrypts file with derived key → splits into fixed-size chunks.
- **Task:** Announce file metadata (hash, size, chunk count, derived key label) via GossipSub.
- **Task:** Implement chunk exchange protocol `/app/file/1.0.0`:
  - Receivers request chunks from sender AND from other receivers who already have them (swarming).
  - Multi-stream parallel download — optimizes LAN bandwidth.
- **Task:** Receiver reassembles chunks → decrypt → verify hash.
- **Task:** UI: Progress bar, file selection dialog.

### 8.2. Thesis Report & Defense Prep
- **Task:** Finalize diagrams (architecture, protocol flow, fork healing sequence).
- **Task:** Prepare Demo environment (multi-node Docker / multi-process on single machine).
- **Task:** Write evaluation results (see Phase 9).

---

## 9. EVALUATION & TESTING (Throughout + Final Weeks)

**Goal:** Prove correctness, performance, and security of the Decentralized Coordination Protocol.

### 9.1. Correctness & Consistency (Concurrency Chaos Test)
- **Task:** Simulate 10+ nodes sending Proposals and Commits simultaneously within the same millisecond.
- **Success Criteria:** All nodes converge to the same Epoch number AND their TreeHash values match perfectly across every device.
- **Task:** Verify Single-Writer invariant: at no point do two Commits exist for the same epoch.

### 9.2. Partition & Healing Test
- **Task:** Physically disconnect network into 2 independent branches (split-brain simulation).
- **Task:** Let each branch evolve independently for multiple epochs.
- **Task:** Reconnect and measure:
  - Branch Weight W correctly selects the winning branch.
  - External Join success rate for losing branch nodes.
  - Time to full convergence (all nodes on same epoch + TreeHash).

### 9.3. Performance & Latency
- **Task:** Measure epoch finalization time (Proposal → Commit → all nodes at E+1).
- **Task:** Compare overhead vs. a baseline system using a centralized Delivery Service.
- **Task:** Scalability: measure CPU and bandwidth consumption as group size increases during periodic key rotation — empirically validate O(log N) advantage of MLS.

### 9.4. Security & Threat Validation
- **Task:** Extract SQLite database of a "losing branch" node after a Partition & Healing test.
- **Task:** Verify that staged Commit keys have been destroyed (crypto-shredding) — Forward Secrecy proof.
- **Task:** Verify Token Replay attack is blocked (Eve replaying Alice's token → rejected).
- **Task:** Verify expired tokens are rejected at auth handshake.

**Validation Criteria:**
> All nodes converge to identical state after concurrent operations and network partitions. O(log N) scaling empirically demonstrated. Forward Secrecy preserved after fork healing. Identity migration succeeds with session takeover. File transfer completes via swarming protocol.
