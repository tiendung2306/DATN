# Lớp Service (Go)

> Xem thêm: [Index](README.md) · [Architecture Overview](architecture-overview.md) · [Coordination Layer](coordination-layer.md) · [Frontend](frontend.md) · [Flows](flows.md)

**Vị trí:** `app/service/`  
**Vai trò:** Orchestration layer — bridge giữa Coordination protocol và Frontend UI. `Runtime` struct là single entry point cho tất cả frontend operations.

## Runtime (`runtime.go`)

`Runtime` struct — lõi ứng dụng, bound với Wails frontend:

```go
type Runtime struct {
    cfg          config.Config
    db           *store.Database
    p2pNode      *p2p.P2PNode
    engine       *sidecar.GrpcMLSEngine
    processMgr   *sidecar.ProcessManager
    coordinators map[string]*coordination.Coordinator
    hlc          *coordination.HLC
    blindStore   *p2p.BlindStore
    eventSink    EventSink
    // ... session, profile, notifications, etc.
}
```

> **Note:** `blindStore` is managed by the Service layer (`blind_store.go`), not the P2P adapter. The P2P adapter provides wire-level replication (`replicated_store_wire.go`), while the Service layer orchestrates blind-store policy (k-nearest, store-node mode).

### Lifecycle

| Phase | Method | Mô tả |
|-------|--------|-------|
| **Init** | `NewRuntime(cfg)` | Construct Runtime với config, chưa start anything |
| **Startup** | `Startup(ctx)` | Wails lifecycle — init DB, P2P identity, sidecar, P2P node, coordinators |
| **Ready** | `DomReady(ctx)` | Frontend loaded — start background loops, emit initial state |
| **Close** | `BeforeClose(ctx)` | Confirm exit — check pending operations |
| **Shutdown** | `Shutdown(ctx)` | Cleanup — stop coordinators, stop P2P, stop sidecar, close DB |

### Startup Flow (chi tiết)

```
Startup()
  │
  ├── 1. Create runtime directory (~/.datn or --runtime-dir)
  │
  ├── 2. Init SQLite database
  │     └── store.NewDatabase(path) → WAL mode, schema init
  │
  ├── 3. Load/create P2P identity
  │     ├── If mls_identity exists → load
  │     └── Else → GetOrCreateIdentity() → generate Ed25519
  │
  ├── 4. Start Rust crypto sidecar
  │     ├── ProcessManager.StartEngine() → spawn binary, get port
  │     └── GrpcMLSEngine → connect to 127.0.0.1:{port}
  │
  ├── 5. DetermineAppState()
  │     ├── No identity → UNINITIALIZED
  │     ├── Identity but no auth_bundle → AWAITING_BUNDLE
  │     ├── Has auth_bundle → AUTHORIZED (or ADMIN_READY if admin key)
  │     └── Emit app:state_changed event
  │
  └── 6. If AUTHORIZED: launch P2P node
        ├── Load auth bundle from DB
        ├── Build session claim (signed timestamp)
        ├── Create P2PNode with auth protocol
        ├── Install wire protocol handlers (8 protocols)
        ├── Start background loops:
        │   ├── Keepalive — periodic ping connected peers
        │   ├── Offline sync — deliver buffered messages
        │   ├── KeyPackage advertise — publish KP for invite
        │   ├── Envelope GC — cleanup old envelopes
        │   └── Pending welcome — process undelivered Welcomes
        ├── Init coordinators for all existing groups
        └── Emit node:status event
```

### P2P Launch Detail

Sau khi auth bundle loaded, P2P node được tạo với:

**Wire protocol handlers installed (8):**
- GroupInfo exchange — provide GroupInfo cho fork healing
- Invite store/lookup — DHT-based invite tracking
- Group invite request — multi-node approval protocol
- File transfer — chunked MLS-encrypted file transfer
- Offline sync — deliver buffered messages when peer online
- Channel category sync — organize channels into categories
- User profile push — exchange display name, avatar
- Replica store — blind-store replication protocol

**Background loops:**
- **Keepalive** — periodic ping to maintain connections
- **Offline sync** — check for peers coming online, deliver buffered messages
- **KeyPackage advertise** — publish KeyPackage to DHT for invite discovery
- **Envelope GC** — cleanup old envelopes beyond retention window
- **Pending welcome** — process undelivered Welcome messages (retry)

## Group Operations (`group.go`)

| Method | Mô tả |
|--------|-------|
| `CreateGroupChat(groupID, groupType, categoryID)` | Tạo MLS group, start Coordinator, subscribe GossipSub |
| `JoinGroupWithWelcome(welcomeBytes)` | Join group từ Welcome message (invite flow) |
| `LeaveGroup(groupID)` | Rời group, cleanup coordinator, unsubscribe GossipSub |
| `GetGroups()` | List all active groups (from SQLite) |
| `GetGroupMembers(groupID)` | List members với online status (from ActiveView) |
| `AddMemberToGroup(groupID, peerID)` | Add member — create Add proposal, send Welcome |
| `RemoveMemberFromGroup(groupID, peerID)` | Remove member — create Remove proposal |
| `StartDirectMessage(peerID)` | Tạo DM group giữa 2 peers (special group type) |
| `GetGroupPosts(groupID)` | List channel posts (paginated) |
| `GetPostComments(postID)` | List comments on a post (paginated) |

**Group types:**
- `channel` — broadcast channel (posts + comments, like Discord/Slack)
- `group` — group chat (message stream)
- `dm` — direct message (2-person private chat)

**Channel categories:** Channels có thể organize vào categories — `channel_categories.go` CRUD + sync.

**Group avatar:** Save/replicate group avatar — `group.go` handles avatar storage + P2P replication.

## Messaging (`messaging.go`)

| Method | Mô tả |
|--------|-------|
| `SendGroupMessage(groupID, text)` | Encrypt + broadcast qua Coordinator |
| `SendGroupMessageWithLocalEchoToken(groupID, text, token)` | Optimistic UI correlation — frontend tracks local echo |
| `GetGroupMessages(groupID, limit, offset)` | Paginated message retrieval |
| `RetryMessage(groupID, messageID)` | Resend persisted message (failed delivery) |
| `DeleteLocalMessage(groupID, messageID)` | Delete local message row (user action) |

**Channel messaging** (`channel_payload.go`):
- Posts — structured content with title, body, mentions, attachments
- Comments — reply to posts, with optional reply-to-comment (threaded)
- Payload validation — `ERR_CHANNEL_PAYLOAD_INVALID` if title/body exceeds limits
- Message limits — `message_limits.go` — configurable per-type limits

**Replay link resolution:** Map replayed messages (sau fork heal) đến original message IDs — frontend hiển thị "replayed" thay vì duplicate.

## Identity (`identity.go`)

| Method | Mô tả |
|--------|-------|
| `GenerateKeys()` | Generate Ed25519 key pair via Rust engine → store in SQLite |
| `GetOnboardingInfo()` | Return PeerID + MLS public key (hex) cho admin CSR |
| `OpenAndImportBundle(bundlePath)` | Import InvitationBundle, verify token, store auth bundle, launch P2P |
| `ExportIdentity(outputPath, passphrase)` | Export encrypted backup — `.backup` file (Argon2id + AES-256-GCM) |
| `ImportIdentityFromFile(filePath, passphrase)` | Restore from backup — reset session, set kill-session flag |
| `ExportDeviceRequestJSON(outputPath)` | Export onboarding request to JSON file (alternative to UI) |

**Security:** Private keys KHÔNG bao giờ gửi qua network. Export ra file `.backup` encrypted với passphrase, transfer thủ công (USB, etc.).

## Session (`session.go`)

**Single Active Device enforcement** — một tài khoản chỉ valid trên MỘT thiết bị:

| Method | Mô tả |
|--------|-------|
| `SessionClaim` | Signed timestamp (MLS signing key) — gửi trong auth handshake |
| `markSessionReplaced()` | Đánh dấu session bị thay thế → emit `session:replaced` event |
| `ensureSessionActive()` | Guard cho tất cả operations — check session chưa bị replaced |
| `ApplyIdentityImportSideEffects()` | Reset session + set kill-session flag (after identity import) |
| `GetSessionStatus()` | Return session state: `active`, `replaced` |

## Invite (`invite.go` — 70KB, largest file)

Multi-node invite approval workflow:

| Method | Mô tả |
|--------|-------|
| `InvitePeerToGroup(groupID, peerID)` | Send invite — create Add proposal, send Welcome |
| `RequestGroupInvite(groupID, reason)` | Request join — send invite request to group members |
| `GenerateJoinCode(groupID)` | Generate join code (human-readable) |
| `ListGroupInviteRequests(groupID)` | List pending invite requests |
| `ApproveGroupInviteRequest(requestID)` | Approve — send Welcome to requester |
| `RejectGroupInviteRequest(requestID, reason)` | Reject — notify requester |

**Invite policies:**
- `creator_approval` — chỉ creator/admin có thể approve invite requests
- `any_member` — bất kỳ member nào có thể approve

**KeyPackage exchange:**
- Inviter fetches KeyPackage from DHT (advertised by invitee)
- Or via direct stream (`kp_direct.go`)
- Welcome message delivered via direct stream or offline buffer

**Invite lifecycle:** pending → approved/rejected → joined (Welcome delivered) → expired

## File Transfer (`file_transfer.go`)

MLS-encrypted file transfer — chunked, AES-256-GCM:

| Method | Mô tả |
|--------|-------|
| `PrepareGroupFile(groupID, filePath)` | Read file, compute SHA256, split into chunks |
| `PrepareOutgoingFileTransfer(groupID, fileMeta)` | Encrypt each chunk với MLS exporter key |
| `SendGroupFile(groupID, fileMeta)` | Broadcast file metadata + transfer chunks via direct stream |
| `DownloadGroupFile(groupID, fileID)` | Download chunks, decrypt, reassemble |
| `OpenDownloadedFile(groupID, fileID)` | Open downloaded file với OS default app |

**Encryption:** MLS exporter-based — `ExportSecret` derives per-file encryption key from group state. Each chunk encrypted với AES-256-GCM + unique nonce. See `pkg/filetransfer/crypto.go`.

**Chunk size:** Configurable via `--file-chunk-bytes` (default 1MB).

## Profile (`profile.go`)

| Method | Mô tả |
|--------|-------|
| `GetMyProfile()` | Return local user profile (display name, avatar, status) |
| `SaveMyProfile(displayName, avatarData)` | Save profile + push to connected peers |
| `UpdateMyProfile(fields)` | Partial update |
| `GetPeerProfile(peerID)` | Get cached peer profile |
| `ApplySignedPeerProfile(peerID, profile, signature)` | Apply received profile (verified signature) |

**Avatar pipeline:**
- Frontend compresses to max 256 KiB (WebP/JPEG, max 512px) — see `lib/avatarImage.ts`
- Backend stores in SQLite + replicates to blind-store
- Profile sync via `/app/user-profile/1.0.0` wire protocol

## Notifications (`notifications.go`)

| Method | Mô tả |
|--------|-------|
| `GetNotifications(limit, offset)` | Paginated notification list |
| `GetUnreadNotificationCount()` | Unread count for badge |
| `MarkNotificationRead(id)` | Mark single notification read |
| `MarkAllNotificationsRead()` | Mark all read |

**Notification types:** `mention`, `reply`, `group_add`, `invite_request`, `invite_approved`, `invite_rejected`

## Offline Sync (`offline_sync.go`)

| Method | Mô tả |
|--------|-------|
| `GetOfflineSyncStatus()` | Return sync status (pending count, last sync time) |
| `TriggerOfflineSync()` | Manually trigger sync |

**Mechanism:**
- Messages to offline peers: encrypted local envelope retention (`envelope_log` in SQLite)
- When peer online: authenticated direct stream synchronization
- Blind-store replication on `/org/offline-store/v1`:
  - Regular nodes: retain targeted k-nearest replicas (default k=2)
  - `--store-node` nodes: retain ALL blind-store objects

## Runtime Events (`runtime_events.go`)

Durable runtime event log — persistent event store cho frontend replay:

| Method | Mô tả |
|--------|-------|
| `GetRuntimeEventsSince(seq, limit)` | Paginated event retrieval (for catch-up) |
| `GetRuntimeEventCursor()` | Current max seq (for frontend cursor) |
| `GetAggregateRevisions()` | Per-aggregate revision map (gap detection) |

**Events stored:** `runtime:health`, `startup:progress`, `startup:error`, `app:state_changed`, `admin:status`, `node:status`, `p2p:status`, `session:replaced`, `chat:message`, `group:epoch`, `group:left`, `group:member_added`, `group:member_removed`, `fork:healed`, `notification:new`

Frontend polls events via `useRuntimeEventStream` hook — detects gaps (missing seq numbers) and triggers `refreshState`.

## Runtime Health (`runtime_health.go`)

| Method | Mô tả |
|--------|-------|
| `GetRuntimeHealth()` | Return health status: startup_stage, app_state, last_error |
| `GetAppState()` | Return current app state (UNINITIALIZED, AWAITING_BUNDLE, AUTHORIZED, ADMIN_READY) |
| `GetDiagnosticsSnapshot()` | Full diagnostic info (peers, groups, messages, network) |
| `ExportDiagnostics(outputPath)` | Export diagnostic snapshot to file |

## Admin (`admin.go`)

| Method | Mô tả |
|--------|-------|
| `InitAdminKey(passphrase)` | Setup Root Admin key (Ed25519, encrypted với Argon2id + AES-256-GCM) |
| `VerifyAdminPassphrase(passphrase)` | Unlock admin key |
| `CreateBundle(peerID, pubKeyHex, name)` | Sign InvitationToken, export `.bundle` file |
| `CreateBundleFromRequest(requestJSON)` | Create bundle from exported request file |
| `GetAdminStatus()` | Return admin key status (initialized, unlocked) |
| `ListIssuanceHistory()` | List all issued tokens (audit trail) |

## Các service files khác

| File | Nội dung |
|------|----------|
| `app_state.go` | `DetermineAppState` state machine — check identity, bundle, admin key |
| `blind_store.go` | Blind-store replication layer — manage replicated objects |
| `bootstrap.go` | Connect to bootstrap peer — initial network entry |
| `channel_categories.go` | Channel category CRUD + P2P sync |
| `channel_payload.go` | Channel post/comment payload validation — title/body limits |
| `cli_node.go` | CLI node operations — headless mode support |
| `control_api.go` | Demo control REST API — for demo-control app |
| `fork_heal_history.go` | Fork heal audit log — record healing events |
| `group_admins.go` | Group admin management — promote/demote admins |
| `group_event_log.go` | Group event log — audit trail for group operations |
| `group_info_sync.go` | GroupInfo sync — exchange GroupInfo for fork healing |
| `group_invite_request_p2p.go` | P2P invite request protocol — wire handler |
| `group_invite_requests.go` | Invite request management — CRUD, approval workflow |
| `group_member_directory.go` | Member directory sync — exchange member info between peers |
| `group_permissions.go` | Permission checks — creator, admin, member roles |
| `membership.go` | Membership operations — join, leave, remove, role management |
| `message_limits.go` | Channel message length limits — configurable per type |
| `network_diagnostics.go` | Network diagnostic info — peers, connections, DHT routing |
| `node_status.go` | Node status reporting — peer ID, display name, connected peers |
| `recovery_replay.go` | Recovery replay APIs — replay messages after fork heal |
| `replicated_sync.go` | Replicated store sync — manage blind-store replication |

## EventSink Interface

Service layer emit events to frontend qua `EventSink` interface:

```go
type EventSink interface {
    Emit(eventName string, data ...interface{})
}
```

**Implementation:** `wailsui.EventSink` — wraps `wailsruntime.EventsEmit`. Frontend subscribes via `EventsOn(eventName, handler)`.

**Key events emitted:**

| Event | Trigger | Frontend action |
|-------|---------|-----------------|
| `chat:message` | Message received/sent | `useChatEvents` → push to chat store |
| `chat:message_sent` | Message sent confirmed | Update message status to `published` |
| `chat:message_failed` | Message send failed | Update message status to `failed` |
| `group:epoch` | Epoch advanced | Refresh group info, update member list |
| `group:left` | User left/removed | Remove group from list, show notification |
| `group:member_added` | Member added | Update member list, show notification |
| `group:member_removed` | Member removed | Update member list, show notification |
| `group:replay_blocked` | Replay blocked (stale epoch) | Silent or show error based on `user_visible` |
| `group:operation_progress` | Group operation progress | Update pending operation status |
| `group:add_operation_stale` | Add operation stale | Retry or cancel pending add |
| `group:invite_auto_joined` | Auto-joined via invite | Add group to list, switch to it |
| `notification:new` | New notification | Add to notification store, update badge |
| `startup:progress` | Startup stage changed | Update loading screen |
| `startup:error` | Startup failed | Show error screen |
| `runtime:health` | Health status changed | Update app state |
| `app:state_changed` | App state changed | Route to appropriate screen |
| `session:replaced` | Session replaced | Show error, block operations |
| `node:status` | Node status changed | Update network indicator |
| `p2p:status` | P2P status changed | Update network indicator |
| `admin:status` | Admin status changed | Update admin panel |
| `runtime:event_available` | New runtime event | Trigger `drainRuntimeEvents` |
| `channel_categories:changed` | Categories changed | Refresh category list |

## Testing

20+ integration test files (`business_*_integration_test.go`):

| Test file | Nội dung |
|-----------|----------|
| `business_admin_integration_test.go` | Admin key setup, bundle creation, token verification |
| `business_app_state_integration_test.go` | App state machine transitions |
| `business_channel_categories_integration_test.go` | Channel category CRUD + sync |
| `business_channel_messaging_integration_test.go` | Channel post/comment flows |
| `business_crosscutting_integration_test.go` | Cross-cutting concerns (auth, session, profile) |
| `business_diagnostics_integration_test.go` | Diagnostic snapshot, export |
| `business_diagnostics_replay_integration_test.go` | Diagnostic replay — runtime event replay |
| `business_dm_realsidecar_integration_test.go` | DM with real Rust sidecar (not mock) |
| `business_e2e_group_integrity_test.go` | End-to-end group integrity |
| `business_fork_heal_integration_test.go` | Fork healing integration |
| `business_identity_backup_integration_test.go` | Identity export/import (encrypted backup) |
| `business_integration_harness_test.go` | Test harness — shared setup, mock sidecar, helpers |
| `business_integration_mls_mock_test.go` | MLS mock integration — mock engine for fast tests |
| `business_invite_integration_test.go` | Multi-node invite approval workflow |
| `business_invite_request_integration_test.go` | Invite request flow — request, approve, reject |
| `business_join_roster_sync_integration_test.go` | Join + roster sync |
| `business_known_peers_integration_test.go` | Known peers management |
| `business_members_integration_test.go` | Member management (add, remove, roles) |
| `business_messaging_integration_test.go` | Message send/receive/retry |
| `business_rejoin_integration_test.go` | Rejoin after leave |
| `business_runtime_events_integration_test.go` | Runtime event log + replay |
| `business_runtime_lifecycle_integration_test.go` | Runtime startup/shutdown lifecycle |
| `business_session_integration_test.go` | Single active device enforcement |
| `business_sprint6_integration_test.go` | Sprint 6 integration (file transfer, etc.) |

> Chi tiết: [Evaluation & Testing](evaluation-testing.md)
