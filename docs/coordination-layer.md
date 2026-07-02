# Lớp Coordination (Go)

> Xem thêm: [Index](README.md) · [Architecture Overview](architecture-overview.md) · [Adapter Layer](adapter-layer.md) · [Security & Protocol](security-protocol.md) · [Flows](flows.md) · [Single-Writer Protocol](single_writer_protocol.md)

**Vị trí:** `app/coordination/`  
**Vai trò:** Điều phối giao thức phi tập trung cho một nhóm MLS duy nhất — ordering, election, fork healing, heartbeat, causal timestamps.

## Kiến trúc tổng thể

Coordination Layer là "bộ não" của giao thức phi tập trung. Nó bao bọc MLS (RFC 9420) với một layer điều phối đảm bảo:
- **Thứ tự xác định:** Single-Writer Token Holder election tránh concurrent commits
- **Phát hiện phân nhánh:** ForkDetector so sánh branch weights để xác định nhánh thắng
- **Phục hồi tự động:** Fork healing via External Join + autonomous replay
- **Đồng bộ causal:** HLC timestamps không phụ thuộc NTP
- **Tracking peers online:** ActiveView qua heartbeat

```
Coordinator
  ├── ActiveView        — track online peers, evict dead
  ├── SingleWriter      — Token Holder election, proposal buffering
  ├── EpochTracker      — validate epoch, buffer future messages
  ├── ForkDetector      — detect branches, compare weights
  ├── HLC               — Hybrid Logical Clock
  └── Config            — timeouts, intervals, retention mode
```

## Coordinator (`coordinator.go`)

`Coordinator` struct là thành phần trung tâm, liên kết tất cả sub-components. Mỗi MLS group có một Coordinator instance riêng, được quản lý bởi `service.Runtime` qua map `coordinators[groupID]`.

### Lifecycle

| Phase | Method | Mô tả |
|-------|--------|-------|
| **Create** | `CreateGroup` | Tạo MLS group qua Rust, khởi tạo Coordinator mới |
| **Load** | `InitializeGroup` | Nạp state từ SQLite, khởi tạo sub-components từ persisted state |
| **Run** | `Start` | Bắt đầu 3 background loops: heartbeat, announce, key rotation |
| **Stop** | `Stop` | Dừng tất cả goroutines, cleanup resources |

### Background Loops (bắt đầu khi `Start()`)

1. **Heartbeat loop** — broadcast `MsgHeartbeat` mỗi `HeartbeatInterval` (5s production, 50ms test). Peers ghi nhận qua `ActiveView.RecordHeartbeat`.

2. **Announce loop** — broadcast `GroupStateAnnouncement` mỗi `AnnounceInterval` (10s production, 0 = disabled test). Chứa epoch, treeHash, memberCount — dùng bởi ForkDetector.

3. **Key rotation loop** — tự động `CreateUpdateCommit` mỗi `KeyRotationInterval` (5m production, 0 = disabled test). Refresh leaf key cho forward secrecy.

### Callbacks (injected bởi Service layer)

Coordinator thông báo Service layer qua callbacks, cho phép Service emit UI events:

| Callback | Trigger | Service action |
|----------|---------|----------------|
| `OnMessage` | Application message decrypted | Emit `chat:message` event to frontend |
| `OnEpochChange` | Commit processed, epoch advanced | Emit `group:epoch` event, update UI |
| `OnAccessLost` | Node bị remove khỏi group | Emit `group:left`, cleanup UI |
| `OnForkHealEvent` | Fork healing hoàn tất | Audit log, emit `fork:healed` event |
| `OnSyncRequired` | Phát hiện cần catch-up sync | Trigger offline sync |
| `OnAddCommitted` | Add proposal được commit | Emit `group:member_added`, send Welcome |
| `OnPeerObserved` | Peer mới được quan sát | Update contact list, profile sync |

### Coordinator Fields

```go
type Coordinator struct {
    groupID         string
    cfg             CoordinatorConfig
    transport       Transport       // LibP2PTransport
    engine          MLSEngine       // GrpcMLSEngine (Rust)
    storage         CoordinationStorage  // SQLiteCoordinationStorage
    clock           Clock           // HLC

    activeView      *ActiveView
    singleWriter    *SingleWriter
    epochTracker    *EpochTracker
    forkDetector    *ForkDetector

    mode            CoordinatorMode  // Live, CatchingUp, FrozenForApply
    healing         atomic.Bool      // fork healing in progress

    // Callbacks
    onMessage       func(...)
    onEpochChange   func(...)
    // ...

    // Goroutine coordination
    ctx             context.Context
    cancel          context.CancelFunc
    wg              sync.WaitGroup
}
```

## SingleWriter (`single_writer.go`)

**Quy tắc Single-Writer (CRITICAL):** Tại mỗi epoch, CHỈ một Token Holder được phát hành Commit. Tất cả node khác phải route Proposals qua GossipSub và chờ Commit từ Token Holder. Việc này tránh concurrent commits gây MLS fork.

### Token Holder Election

`ComputeTokenHolder` — bầu deterministic dựa trên:
1. Sort ActiveView (online peers) theo PeerID
2. `tokenHolder = sortedView[epoch % len(sortedView)]`
3. Kết quả đồng nhất trên tất cả nodes (vì cùng sorted ActiveView + cùng epoch)

Đảm bảo: nếu 2 nodes có cùng ActiveView và cùng epoch, họ sẽ tính ra cùng Token Holder.

### Proposal Buffering

| Method | Mục đích |
|--------|----------|
| `BufferProposal` | Đệm proposal đến (từ GossipSub hoặc local), kiểm tra duplicate |
| `SnapshotNextBatch` | Chụp batch proposals cho Commit (giới hạn `MaxBatchedProposals`) |
| `DrainBatchByRefs` | Xóa proposals đã commit (by ProposalRef) |
| `DrainBatchByData` | Xóa proposals đã commit (by raw data) |
| `AdvanceEpoch` | Tăng epoch, reset state, clear suspended peers |
| `SuspendPeer` | Cấm peer khỏi việc trở thành Token Holder (violation detected) |

### Batch Flow

```
Any node creates Proposal → Broadcast GossipSub → All nodes BufferProposal
                                                    │
Token Holder: after BatchingDelay                     │
  ├── SnapshotNextBatch() → [propRef1, propRef2, ...]│
  ├── CreateCommit(expectedRefs) via Rust            │
  ├── Broadcast MsgCommit                             │
  └── AdvanceEpoch locally                            │
                                                     │
All other nodes:                                      │
  ├── Receive MsgCommit                               │
  ├── Validate sender == ComputeTokenHolder()        │
  ├── StageCommit via Rust → verify refs             │
  ├── ProcessCommit via Rust → new state             │
  ├── DrainBatchByRefs(committedRefs)                │
  └── AdvanceEpoch                                    │
```

> Chi tiết thêm: [single_writer_protocol.md](single_writer_protocol.md)

## EpochTracker (`epoch.go`)

Đảm bảo **epoch monotonicity** — không xử lý MLS Commit/Proposal với epoch < current epoch.

| Method | Mục đích |
|--------|----------|
| `ValidateEpoch(msgEpoch)` | Phân loại: `ActionProcess` (== current), `ActionRejectStale` (< current), `ActionBufferFuture` (> current) |
| `BufferFuture` | Đệm messages từ epoch tương lai (chờ catch-up) |
| `DrainBuffered` | Xả buffer khi đạt epoch tương ứng |

Stale messages bị reject với `CurrentEpochNotification` — thông báo sender về epoch hiện tại để sender có thể catch-up.

## HLC (`hlc.go`)

Hybrid Logical Clock — kết hợp physical clock + logical counter để đảm bảo thứ tự nhân quả (causal ordering) không phụ thuộc đồng hồ vật lý:

```go
type HLC struct {
    physical time.Time
    logical  uint64
}
```

| Method | Mục đích |
|--------|----------|
| `Now()` | Tạo timestamp cho sự kiện cục bộ: `max(physical, prevPhysical) + 1` nếu cùng physical time |
| `Update(remote)` | Merge timestamp nhận được, kiểm tra clock drift |

**Clock drift protection:** `MaxClockDriftMs = 10000` (10s). Reject messages có sender timestamp lệch quá 10s so với local clock — chống attacks lợi dụng clock skew.

## ActiveView (`active_view.go`)

Theo dõi peers online qua heartbeat mechanism:

| Method | Mục đích |
|--------|----------|
| `RecordHeartbeat(peerID)` | Ghi nhận peer alive, reset miss counter |
| `Snapshot()` | Trả về sorted list online peer IDs (dùng cho Token Holder election) |
| `Contains(peerID)` | Check peer online |

**Eviction:** Peer bị evict sau `PeerDeadAfter` (mặc định 3) lần heartbeat liên tiếp bị miss. Eviction trigger `onChange` callback → tính lại Token Holder.

**onChange callback:** Khi member set thay đổi (peer join/leave/evict), `onChange` kích hoạt tính lại Token Holder vì sorted ActiveView thay đổi.

## ForkDetector (`fork_healing.go`)

Giám sát `GroupStateAnnouncement` messages để phát hiện network partitions (forks):

### Branch Weight Comparison

`CompareBranchWeight(local, remote)` — so sánh nhánh thắng dựa trên priority:

1. **Epoch cao hơn wins** — nhánh có epoch lớn hơn đã xử lý nhiều commits hơn
2. **Member count cao hơn wins** — nếu cùng epoch, nhánh có nhiều members hơn "healthy" hơn
3. **Commit hash** — nếu cùng epoch + member count, so sánh commit hash (deterministic tiebreaker)
4. **Tree hash** — final tiebreaker

### Fork Detection

| Method | Mục đích |
|--------|----------|
| `ProcessRemote(announcement)` | Phân tích remote announcement, tạo `ForkEvent` nếu phát hiện fork |
| `CompareWithPeer(peerID)` | So sánh trực tiếp với peer (query GroupStateAnnouncement) |

`ForkEvent` triggers Coordinator's fork healing pipeline.

## Coordinator Modes (`coordinator_message.go`)

Xử lý tin nhắn đến qua GossipSub hoặc direct stream, tùy thuộc vào mode:

| Mode | Mô tả | Khi nào |
|------|-------|---------|
| **ModeLive** | Xử lý bình thường — heartbeat, announce, proposal, commit, application | Bình thường |
| **ModeCatchingUp** | Cô lập Gossip live, chỉ buffer commits/applications vào DB inbox | Sau fork heal, cần sync missing messages |
| **ModeFrozenForApply** | Buffer tất cả trong quá trình healing | Fork healing đang thực hiện |

**Clock skew protection:** Reject messages có sender timestamp lệch quá `MaxClockDriftMs` so với local clock.

## Commit Handling (`coordinator_commit.go`)

Pipeline xử lý Commit message (7 bước):

1. **Validate epoch** — EpochTracker.ValidateEpoch (reject stale, buffer future)
2. **StageCommit via Rust** — `engine.StageCommit(groupState, commitBytes, includedProposals)` → kiểm tra proposal refs match
3. **Validate Token Holder** — `singleWriter.ComputeTokenHolder() == msg.From` (chỉ holder mới được commit)
4. **ProcessCommit via Rust** — `engine.ProcessCommit(...)` → áp dụng commit, nhận new group state + tree hash
5. **Persist to SQLite** — `storage.ApplyCommit(...)` — atomic transaction: update group state + envelope log
6. **Advance epoch** — `singleWriter.AdvanceEpoch()`, drain committed proposals, trigger batch replay
7. **Reconcile pending operations** — `coordinator_reconcile.go` — update pending add/remove operations

## Application Handling (`coordinator_application.go`)

Pipeline xử lý tin nhắn ứng dụng (encrypted):

1. **Check duplicate** — envelope hash lookup trong `envelope_log`
2. **Validate epoch** — cho phép stale trong `maxPastEpochs` window (MLS retained keys cho late messages)
3. **HLC update** — `clock.Update(timestamp)` merge clock
4. **DecryptMessage via Rust** — `engine.DecryptMessage(groupState, ciphertext)` → plaintext
5. **Persist** — `storage.ApplyApplication(...)` — atomic: save decrypted message + envelope log
6. **Fire callback** — `onMessage(groupID, sender, plaintext)` → Service layer emit UI event
7. **Send DeliveryACK** — confirm nhận cho sender (cho retry logic)

## Fork Healing (`coordinator_heal.go`)

Fork healing pipeline — phục hồi sau network partition:

### Trigger
ForkDetector phát hiện remote branch có weight cao hơn local → tạo `ForkEvent` → `scheduleHeal`

### Pipeline

1. **`scheduleHeal`** — kiểm tra retry count, set `healing` flag, transition sang `ModeFrozenForApply`

2. **Catch-up sync retry** (tối đa 3 lần) — thử non-destructive heal trước:
   - Query missing messages từ winning peer
   - Process buffered commits/applications
   - Nếu catch-up thành công → resume normal, không cần destructive heal

3. **`runHeal`** — destructive heal (nếu catch-up thất bại):
   - **Fetch GroupInfo** từ winning peer (with ratchet tree extension)
   - **ExternalJoin via Rust** — `engine.ExternalJoin(groupInfo, signingKey)` → join winning branch
   - **Crypto-shredding** — hủy TẤT CẢ keys từ losing branch (forward secrecy on heal)
   - **Autonomous Replay** — re-encrypt & resend OWN messages only (non-repudiation)
   - Resume normal operation trên winning branch

4. **Post-heal** — fire `OnForkHealEvent` callback → audit log → emit UI event

> Chi tiết luồng: [Flows → Fork Healing](flows.md#fork-healing-flow)

## Config (`config.go`)

`CoordinatorConfig` — configurable parameters:

| Parameter | Production | Test | Mô tả |
|-----------|-----------|------|-------|
| `TokenHolderTimeout` | 4s | 100ms | Timeout chờ Token Holder commit |
| `HeartbeatInterval` | 5s | 50ms | Khoảng cách heartbeat broadcast |
| `AnnounceInterval` | 10s | 0 (disabled) | Khoảng cách GroupStateAnnouncement |
| `PeerDeadAfter` | 3 | 3 | Số heartbeat miss trước khi evict peer |
| `MaxBatchedProposals` | 10 | 10 | Max proposals per commit batch |
| `KeyRotationInterval` | 5m | 0 (disabled) | Tự động key rotation interval |
| `BatchingDelay` | 1s | 0 | Delay trước khi flush proposal batch |
| `ViewBootstrapGrace` | 2s | 0 | Grace period trước khi ActiveView stable |
| `MLSOperationTimeout` | 20s | 250ms | Timeout cho Rust MLS operations |
| `RetentionMode` | BALANCED | BALANCED | Key retention policy (see below) |
| `ApplicationAckTimeout` | 30s | 5s | Timeout cho delivery ACK |
| `ApplicationDirectRetryLimit` | 3 | 3 | Max retry gửi application qua direct stream |

### Retention Modes

| Mode | `max_past_epochs` | `max_past_age` | Use case |
|------|-------------------|----------------|----------|
| `STRICT_SECURITY` | 0 | 0s | Maximum forward secrecy — không giữ old keys |
| `BALANCED` (default) | 3 | 5 minutes | Production balance — cho phép late messages |
| `HIGH_AVAILABILITY` | 10 | 1 hour | Maximum late message delivery — risk hơn |

## Các file coordinator khác

| File | Nội dung |
|------|----------|
| `coordinator_proposal.go` | Xử lý Proposal messages (Add/Remove/Update) — validate, buffer, broadcast |
| `coordinator_batch.go` | Bidirectional batching logic — gom proposals từ GossipSub + local |
| `coordinator_broadcast.go` | Broadcast heartbeat, announce, proposals qua GossipSub |
| `coordinator_reconcile.go` | Reconcile pending operations sau commit — update add/remove status |
| `coordinator_replay.go` | Autonomous replay sau fork heal — re-encrypt & resend own messages |
| `coordinator_members.go` | Add/remove members, group info sync, member directory |
| `coordinator_crypto.go` | MLS operation context helper — build operation context từ group state |
| `coordinator_helpers.go` | Utility functions — envelope creation, message routing |
| `coordinator_observability.go` | Metrics & tracing — record coordination events |
| `metrics.go` | Coordination metrics recording — counters, histograms |

## Types (`types.go`)

### Core Message Types

```go
type MessageType int
const (
    MsgProposal MessageType = iota
    MsgCommit
    MsgHeartbeat
    MsgAnnounce
    MsgEpochNotify
    MsgApplication
    MsgApplicationBatched
    MsgDeliveryAck
    MsgHistoryQuery
    MsgHistoryReply
)
```

### Key Structs

| Struct | Mô tả |
|--------|-------|
| `Envelope` | Wrapper tin nhắn chung: GroupID, Type, Epoch, Timestamp (HLC), From (PeerID), Payload |
| `CommitMsg` | Commit payload: commitBytes, welcomeBytes, groupInfoBytes, proposalRefs, newTreeHash |
| `ProposalMsg` | Proposal payload: proposalBytes, proposalRef, proposalType |
| `ApplicationMsg` | Application payload: ciphertext, localEchoToken |
| `GroupStateAnnouncement` | Fork detection: epoch, treeHash, memberCount, commitHash |
| `StoredMessage` | Persisted message: groupID, sender, content, timestamp, isMine, status |
| `GroupRecord` | Group metadata: groupID, groupType, epoch, groupState bytes |
| `CoordState` | Coordinator persisted state: activeView, tokenHolder, pendingProposals, historyChain |
| `ReplayEnvelopeState` | Trạng thái durable processing cho replay: pending, processed, failed |
| `ForkHealingJob` | Fork heal job: retryCount, status, winningPeer, healTimestamp |
| `ApplicationEvent` | Application lifecycle event: sent, delivered, failed, retried |
| `OutboundReplay` | Replay queue entry: messageID, originalEpoch, newEpoch, status |
| `PendingOperation` | Pending add/remove operation: targetPeer, operationType, status |

### Sentinel Errors

| Error | Khi nào |
|-------|---------|
| `ErrNotTokenHolder` | Node không phải Token Holder cố tình Commit |
| `ErrStaleEpoch` | Message epoch < current epoch |
| `ErrFutureEpoch` | Message epoch > current epoch (buffered) |
| `ErrGroupNotFound` | Group ID không tồn tại |
| `ErrNoActiveView` | ActiveView rỗng, không thể elect Token Holder |
| `ErrInvalidConfig` | Config không hợp lệ |
| `ErrAccessRevoked` | Node bị remove khỏi group |
| `ErrClockDrift` | Clock skew vượt quá MaxClockDriftMs |

## Interfaces (`interfaces.go`)

### Transport
```go
type Transport interface {
    Publish(topic string, data []byte) error
    Subscribe(topic string) (<-chan []byte, error)
    Unsubscribe(topic string) error
    SendDirect(peerID string, protocol string, data []byte) error
    LocalPeerID() string
    ConnectedPeers() []string
}
```

### Clock
```go
type Clock interface {
    Now() time.Time
}
```

### MLSEngine
```go
type MLSEngine interface {
    CreateGroup(groupID string, signingKey []byte, maxPastEpochs uint32) (CreateGroupResult, error)
    CreateProposal(groupState []byte, proposalType int, targetKeyPackage []byte) (ProposalResult, error)
    ProcessProposal(groupState []byte, proposalBytes []byte) (ProcessProposalResult, error)
    CreateCommit(groupState []byte, includedProposals [][]byte, expectedRefs [][]byte) (CommitResult, error)
    StageCommit(groupState []byte, commitBytes []byte, includedProposals [][]byte) (StageCommitResult, error)
    ProcessCommit(groupState []byte, commitBytes []byte, includedProposals [][]byte) (ProcessCommitResult, error)
    ProcessWelcome(welcomeBytes []byte, signingKey []byte, kpBundlePrivate []byte, maxPastEpochs uint32) (WelcomeResult, error)
    EncryptMessage(groupState []byte, plaintext []byte) (EncryptResult, error)
    DecryptMessage(groupState []byte, ciphertext []byte) (DecryptResult, error)
    ExternalJoin(groupInfo []byte, signingKey []byte, maxPastEpochs uint32) (ExternalJoinResult, error)
    ExportGroupInfo(groupState []byte, withRatchetTree bool) ([]byte, error)
    ExportSecret(groupState []byte, label string, context []byte, length uint32) ([]byte, error)
    GenerateKeyPackage(signingKey []byte) (GenerateKeyPackageResult, error)
    AddMembers(groupState []byte, keyPackages [][]byte) (CommitResult, error)
    RemoveMembers(groupState []byte, identities [][]byte) (CommitResult, error)
    HasMember(groupState []byte, identity []byte) (bool, error)
    ListMemberIdentities(groupState []byte) ([][]byte, error)
}
```

### CoordinationStorage
```go
type CoordinationStorage interface {
    SaveGroupRecord(record GroupRecord) error
    LoadGroupRecord(groupID string) (*GroupRecord, error)
    SaveCoordState(groupID string, state CoordState) error
    LoadCoordState(groupID string) (*CoordState, error)
    ApplyCommit(groupID string, newState []byte, envelope Envelope) error
    ApplyApplication(groupID string, msg StoredMessage, envelope Envelope) error
    GetMessages(groupID string, limit, offset int) ([]StoredMessage, error)
    SaveEnvelopeRecord(record EnvelopeRecord) error
    HasEnvelope(hash string) (bool, error)
    // ... pagination, history chain, replay links
}
```

## Testing

| File | Nội dung |
|------|----------|
| `chaos_e2e_test.go` | Chaos engineering convergence test — simulate network partitions |
| `concurrency_evaluation_test.go` | Đánh giá đồng thời — multiple proposals same epoch |
| `scalability_evaluation_test.go` | Đánh giá khả năng mở rộng — group sizes 16-4096 |
| `coordinator_overhead_bench_test.go` | Benchmark overhead — measure coordination layer cost |
| `fork_heal_*_test.go` (7 files) | Phoenix protocol, crash safety, partition sweep, replay robustness, bidirectional batching, real MLS bench, orchestrator |
| `recovery_replay_robustness_test.go` | Test recovery replay — autonomous replay correctness |
| `messaging_offline_blindstore_e2e_test.go` | E2E offline blind-store messaging |
| `sidecar_helper_test.go` | Test helper cho real sidecar integration |

> Chi tiết: [Evaluation & Testing](evaluation-testing.md)
