# Luồng Hoạt Động Chính

> Xem thêm: [Index](README.md) · [Coordination Layer](coordination-layer.md) · [Security & Protocol](security-protocol.md) · [Admin & Config](admin-config.md) · [Service Layer](service-layer.md)

## Onboarding Flow

```
User (new device)                    Admin (Root Admin)
  │                                    │
  ├─ 1. GenerateKeys()                 │
  │    └── Rust: Ed25519 key pair      │
  │    └── Store in SQLite mls_identity│
  │                                    │
  ├─ 2. GetOnboardingInfo()            │
  │    └── Return PeerID + PubKeyHex   │
  │                                    │
  ├─ 3. Send PeerID + PubKey to Admin  │
  │    (out-of-band: Zalo/email)       │
  │                                    │
  │                                    ├─ 4. VerifyAdminPassphrase(pw)
  │                                    │    └── Decrypt admin private key
  │                                    │
  │                                    ├─ 5. CreateBundle(peerID, pubKey, name)
  │                                    │    ├── Sign InvitationToken (Ed25519)
  │                                    │    ├── Package: token + bootstrap addr + root pubkey
  │                                    │    └── Export .bundle file
  │                                    │
  ├─ 6. OpenAndImportBundle(.bundle)   │
  │    ├── Verify token signature      │
  │    │   (Root Admin public key)     │
  │    ├── Store auth_bundle in SQLite │
  │    ├── Launch P2P node             │
  │    │    ├── Build session claim    │
  │    │    ├── Create P2PNode         │
  │    │    └── Connect to bootstrap   │
  │    └── Emit app:state_changed      │
  │       → AUTHORIZED                 │
  │                                    │
  └─ 7. P2P auth handshake             │
       ├── Send AuthMessage            │
       │   {token, peerID, sessionClaim}│
       ├── Server verifies:            │
       │   ├── Token signature         │
       │   ├── PeerID match            │
       │   └── Session claim           │
       └── Auth success → join network │
```

## Group Creation Flow

```
Creator node
  │
  ├─ 1. CreateGroupChat(groupID, type, categoryID)
  │
  ├─ 2. MLS: CreateGroup() via Rust
  │    ├── Send signing_key + max_past_epochs
  │    ├── Rust: create MLS group (OpenMLS)
  │    └── Return group_state bytes
  │
  ├─ 3. Store in SQLite
  │    ├── INSERT INTO mls_groups (group_state, epoch=0, ...)
  │    └── INSERT INTO coordination_state (active_view, ...)
  │
  ├─ 4. Coordinator: InitializeGroup()
  │    ├── Load state from SQLite
  │    ├── Init ActiveView (just creator)
  │    ├── Init SingleWriter (creator = Token Holder)
  │    ├── Init EpochTracker (epoch=0)
  │    └── Init ForkDetector
  │
  ├─ 5. Coordinator: Start()
  │    ├── Heartbeat loop (5s interval)
  │    ├── Announce loop (10s interval)
  │    └── Key rotation loop (5m interval)
  │
  ├─ 6. Subscribe GossipSub topic "group/{groupID}"
  │
  └─ 7. Emit UI event: group created
       └── Frontend: add to group list, switch to it
```

## Message Send Flow

```
Sender                                          Receiver
  │                                                │
  ├─ 1. SendGroupMessage(groupID, text)            │
  │    └── Or: SendGroupMessageWithLocalEchoToken  │
  │       (optimistic UI)                          │
  │                                                │
  ├─ 2. Coordinator: sendMessage()                 │
  │    ├─ Check not healing                        │
  │    ├─ HLC.Now() → timestamp                    │
  │    ├─ MLS: EncryptMessage(groupState, text)    │
  │    │    └── via Rust gRPC                      │
  │    ├─ Build Envelope:                          │
  │    │    {MsgApplication, epoch, timestamp,     │
  │    │     from, ciphertext}                     │
  │    ├─ Publish to GossipSub topic               │
  │    └─ Track pending delivery (for ACK retry)   │
  │                                                │
  ├─ 3. Local: persist to SQLite                   │
  │    ├─ INSERT INTO stored_messages              │
  │    │   (status=pending, local_echo_token)      │
  │    └─ Emit chat:message_sent event             │
  │       └── Frontend: optimistic UI              │
  │                                                │
  │                                                ├─ 4. GossipSub readLoop
  │                                                │    → handleRawMessage()
  │                                                │
  │                                                ├─ 5. Validate epoch
  │                                                │    ├─ == current → process
  │                                                │    ├─ < current → reject (stale)
  │                                                │    └─ > current → buffer (future)
  │                                                │
  │                                                ├─ 6. HLC.Update(timestamp)
  │                                                │    └── Merge clock
  │                                                │
  │                                                ├─ 7. MLS: DecryptMessage()
  │                                                │    └── via Rust gRPC
  │                                                │
  │                                                ├─ 8. Persist to SQLite
  │                                                │    ├─ Check envelope hash (idempotent)
  │                                                │    ├─ INSERT INTO stored_messages
  │                                                │    ├─ INSERT INTO envelope_log
  │                                                │    └─ Fire OnMessage callback
  │                                                │
  │                                                ├─ 9. Emit chat:message event
  │                                                │    └── Frontend: add to chat
  │                                                │
  │                                                └─ 10. Send DeliveryAck
  │                                                     └── Direct stream to sender
  │
  └─ 11. Receive DeliveryAck
       ├─ Update message status → published
       └─ Emit chat:message_sent event
          └── Frontend: update status
```

## Commit Flow (Single-Writer)

```
Any node (has proposal, e.g., Add member)
  │
  ├─ 1. CreateProposal() via Rust
  │    └── Return proposal bytes + proposal ref
  │
  ├─ 2. Broadcast MsgProposal via GossipSub
  │
  └─ 3. All nodes: BufferProposal() in SingleWriter
       └── Check duplicate (by proposal ref)

Token Holder (deterministically elected)
  │  ComputeTokenHolder = sortedView[epoch % len(view)]
  │
  ├─ 4. After BatchingDelay (1s): SnapshotNextBatch()
  │    └── Get batch of proposal refs (max 10)
  │
  ├─ 5. CreateCommit(expectedRefs) via Rust
  │    ├─ Send group_state + proposal refs
  │    ├─ Rust: create MLS Commit
  │    └─ Return: commit bytes + welcome bytes + new state + tree hash
  │
  ├─ 6. Broadcast MsgCommit via GossipSub
  │
  └─ 7. AdvanceEpoch locally
       ├─ SingleWriter.AdvanceEpoch()
       ├─ DrainBatchByRefs(committedRefs)
       └─ Trigger batch replay

All other nodes
  │
  ├─ 8. Receive MsgCommit
  │
  ├─ 9. Validate:
  │    ├─ Epoch OK (not stale, not future)
  │    ├─ Sender == ComputeTokenHolder() ← CRITICAL
  │    └─ Proposal refs match
  │
  ├─ 10. StageCommit() via Rust
  │     └── Verify proposal refs in commit
  │
  ├─ 11. ProcessCommit() via Rust
  │     └── Apply commit → new group state + tree hash
  │
  ├─ 12. Persist to SQLite (atomic)
  │     ├─ UPDATE mls_groups SET group_state, epoch
  │     ├─ INSERT INTO envelope_log
  │     └─ UPDATE coordination_state
  │
  ├─ 13. AdvanceEpoch, drain proposals
  │
  ├─ 14. Reconcile pending operations
  │     └── Update add/remove status
  │
  └─ 15. Emit group:epoch event
       └── Frontend: refresh group info
```

## Fork Healing Flow

```
Network partition → two branches diverge
  │
  ├─ 1. Each node broadcasts GroupStateAnnouncement
  │    (every AnnounceInterval = 10s)
  │    {epoch, treeHash, memberCount, commitHash}
  │
  ├─ 2. ForkDetector: CompareBranchWeight(local, remote)
  │    Priority:
  │    ├─ Higher epoch wins
  │    ├─ If same epoch: more members win
  │    ├─ If same: commit hash (deterministic)
  │    └─ If same: tree hash (final tiebreaker)
  │
  ├─ 3. Losing branch node: scheduleHeal()
  │    ├─ Check retry count (max 3)
  │    ├─ Set healing flag (atomic.Bool)
  │    └─ Transition to ModeFrozenForApply
  │
  ├─ 4. Catch-up sync retry (non-destructive, max 3)
  │    ├─ Query missing messages from winning peer
  │    ├─ Process buffered commits/applications
  │    └─ If success → resume normal (no destructive heal)
  │
  ├─ 5. runHeal() — destructive heal (if catch-up failed)
  │    ├─ Fetch GroupInfo from winning peer
  │    │   └── With ratchet tree extension
  │    │
  │    ├─ ExternalJoin() via Rust
  │    │   ├─ Send GroupInfo + signing key
  │    │   ├─ Rust: MLS ExternalJoin operation
  │    │   └─ Return new group state (on winning branch)
  │    │
  │    ├─ Crypto-shredding
  │    │   └─ Destroy ALL keys from losing branch
  │    │       (forward secrecy on heal)
  │    │
  │    ├─ Autonomous Replay
  │    │   ├─ Re-encrypt OWN messages only
  │    │   │   (non-repudiation — NOT others' messages)
  │    │   ├─ Resend via GossipSub on winning branch
  │    │   └─ Set replay_of = original message ID
  │    │
  │    └─ Resume normal operation
  │       ├─ Transition to ModeLive
  │       └─ Start heartbeat, announce loops
  │
  └─ 6. Fire OnForkHealEvent callback
       ├─ Audit log (fork_heal_events table)
       └─ Emit fork:healed event
          └── Frontend: show notification
```

## Invite Flow (Multi-Node Approval)

```
Invitee (wants to join group)        Group Member(s)
  │                                     │
  ├─ 1. RequestGroupInvite(groupID)     │
  │    └── Send invite request via     │
  │       /app/group-invite/1.0.0      │
  │                                     │
  │                                     ├─ 2. ListGroupInviteRequests()
  │                                     │    └── See pending requests
  │                                     │
  │                                     ├─ 3. ApproveGroupInviteRequest(reqID)
  │                                     │    ├─ Fetch KeyPackage from DHT
  │                                     │    │   (or direct stream)
  │                                     │    ├─ Create Add proposal
  │                                     │    ├─ Token Holder commits
  │                                     │    ├─ ProcessCommit → new state
  │                                     │    └─ Send Welcome to invitee
  │                                     │       (direct stream or offline)
  │                                     │
  ├─ 4. Receive Welcome                 │
  │    └── ProcessWelcome() via Rust   │
  │       └── Join group               │
  │                                     │
  ├─ 5. Coordinator: InitializeGroup   │
  │    └── Start coordination          │
  │                                     │
  └─ 6. Emit group:invite_auto_joined  │
       └── Frontend: add group, switch │
```

## File Transfer Flow

```
Sender                               Receiver
  │                                     │
  ├─ 1. PrepareGroupFile(groupID, path) │
  │    ├─ Read file                     │
  │    ├─ Compute SHA256                │
  │    └─ Split into chunks (1MB)      │
  │                                     │
  ├─ 2. PrepareOutgoingFileTransfer()   │
  │    ├─ ExportSecret() via Rust       │
  │    │   └── Derive per-file key      │
  │    └─ Encrypt each chunk            │
  │       (AES-256-GCM + unique nonce)  │
  │                                     │
  ├─ 3. SendGroupFile(groupID, meta)    │
  │    ├─ Broadcast file metadata       │
  │    │   (via GossipSub)              │
  │    └─ Transfer chunks               │
  │       (via /app/file-transfer/1.0.0)│
  │                                     │
  │                                     ├─ 4. Receive file metadata
  │                                     │    └── Store in file_transfers
  │                                     │
  │                                     ├─ 5. DownloadGroupFile()
  │                                     │    ├─ Receive chunks
  │                                     │    ├─ Decrypt each chunk
  │                                     │    │   (AES-256-GCM)
  │                                     │    └─ Reassemble file
  │                                     │
  │                                     └─ 6. OpenDownloadedFile()
  │                                          └── Open with OS default
```

## Offline Message Delivery Flow

```
Sender → Receiver (offline)
  │
  ├─ 1. SendGroupMessage(groupID, text)
  │    └─ Coordinator: publish to GossipSub
  │       └─ Receiver not connected → no delivery
  │
  ├─ 2. Envelope retention
  │    ├─ Store in envelope_log (SQLite)
  │    └─ Blind-store replication
  │       ├─ Replicate to k-nearest nodes (k=2)
  │       └─ /org/replicated-store/1.0.0
  │
  │           ... time passes ...
  │
  ├─ 3. Receiver comes online
  │    ├─ P2P auth handshake
  │    └─ Auth success → onPeerVerified callback
  │
  ├─ 4. Offline sync trigger
  │    ├─ Direct stream: /app/offline-sync/1.0.0
  │    ├─ Sender delivers buffered envelopes
  │    └─ Receiver processes (idempotent)
  │
  └─ 5. Blind-store sync
       ├─ Receiver queries blind-store nodes
       ├─ Fetches replicated objects
       └─ Processes missed messages
```
