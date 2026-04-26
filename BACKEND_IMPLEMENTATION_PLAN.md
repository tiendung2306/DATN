# BACKEND IMPLEMENTATION PLAN

Project: Zero-Trust P2P Internal Secure Communication App

Purpose: This document defines the backend work that should be completed before the production frontend rebuild. The frontend plan is documented in `FRONTEND_IMPLEMENTATION_PLAN.md`; this file answers which backend capabilities must be product-ready first, which APIs should be exposed to Wails, and which lower-priority backend features can be completed alongside frontend work.

---

## 1. Planning Principles

### 1.1. Backend-First For Core Product Flows

Finish backend foundations before building polished UI for them. The frontend should not fake critical security, identity, group, or session behavior.

Core backend flows that must be real before frontend productization:

- Identity onboarding and signed bundle authorization.
- Admin issuance flow.
- Group lifecycle and invite/join flow.
- Message send/receive with offline recovery.
- Single active device/session takeover.
- Runtime health and diagnostics needed for P2P UX.

### 1.2. Small UI-Driven Backend Gaps Can Be Done Later

Small convenience APIs can be implemented while coding frontend:

- frontend-specific DTO formatting
- small validation helpers
- copy/export helpers
- display metadata refinements
- toast-friendly error strings

Do not block frontend design on these unless they affect security or data consistency.

### 1.3. Do Not Overbuild

Keep implementation aligned with the thesis product:

- No central server.
- No private key transfer over network.
- Admin issues identity, user does not self-name.
- Rust remains stateless; Go owns persistence and coordination.
- Coordination remains in Go; Rust does not know about epochs/token holder/fork healing.

---

## 2. Current Backend Coverage Summary

The backend already has strong foundations:

- App state: `UNINITIALIZED`, `AWAITING_BUNDLE`, `AUTHORIZED`, `ADMIN_READY`.
- Identity setup: `GenerateKeys`, `GetOnboardingInfo`.
- Bundle import: `OpenAndImportBundle`.
- Admin key and bundle issuance: `InitAdminKey`, `HasAdminKey`, `CreateBundle`, `CreateAndImportSelfBundle`.
- Group chat: `CreateGroupChat`, `GetGroups`, `SendGroupMessage`, `GetGroupMessages`.
- MLS invite primitives: `GenerateKeyPackage`, `AddMemberToGroup`, `JoinGroupWithWelcome`, `InvitePeerToGroup`, `CheckDHTWelcome`.
- Group inspection: `GetGroupMembers`, `GetGroupStatus`.
- Node status: `GetNodeStatus`.
- Offline sync: `TriggerOfflineSync`, `GetOfflineSyncStatus`.
- Identity migration: `ExportIdentity`, `ImportIdentityFromFile`.
- Blind-store/offline replication foundation.

The remaining work is mostly about turning backend primitives into stable product flows and frontend-safe APIs.

---

## 3. Priority Levels

### P0 - Must Finish Before Frontend Rebuild

These are core product/security flows. The production frontend should not be built around mocks for these.

1. Invite and pending invite lifecycle.
2. Group membership lifecycle: leave/remove member.
3. Session takeover state and lockout behavior.
4. Startup/runtime health events.
5. Admin issuance readiness: unlock/session model and request parsing.

### P1 - Should Finish Before Frontend Is Feature-Complete

These are important for a robust product UI, but can be implemented in parallel with frontend screens.

1. Network/bootstrap runtime controls.
2. Diagnostics snapshot and log export.
3. Message delivery state/retry model.
4. Issuance history persistence.

### P2 - Later Phase

These belong after chat/group/migration/admin UX is stable.

1. Secure file transfer backend.
2. Advanced evaluation automation and benchmark harness.
3. Optional admin revocation model.

---

## 4. P0 Workstream A - Invite & Pending Invite Lifecycle

### Goal

Make invite/join a complete backend product flow, not just individual primitives.

### Why This Matters

The frontend plan includes:

- Generate join code.
- Add member using join code or known peer.
- Pending invites list.
- Accept/reject invite.

Current backend has many pieces (`GenerateKeyPackage`, `InvitePeerToGroup`, welcome delivery/store/fetch), but the frontend still needs a stable pending invite model.

### Proposed Backend APIs

```go
type PendingInviteInfo struct {
    ID          string `json:"id"`
    GroupID     string `json:"group_id"`
    GroupName   string `json:"group_name,omitempty"`
    InviterPeer string `json:"inviter_peer,omitempty"`
    ReceivedAt  int64  `json:"received_at"`
    Status      string `json:"status"` // pending, accepted, rejected, expired, invalid
}

func (r *Runtime) GenerateJoinCode() (JoinCodeResult, error)
func (r *Runtime) ListPendingInvites() ([]PendingInviteInfo, error)
func (r *Runtime) AcceptInvite(inviteID string) error
func (r *Runtime) RejectInvite(inviteID string) error
```

`GenerateJoinCode` can wrap existing `GenerateKeyPackage`, but should expose user-facing fields:

```go
type JoinCodeResult struct {
    CodeHex      string `json:"code_hex"`
    Checksum     string `json:"checksum"`
    CreatedAt    int64  `json:"created_at"`
    OneTimeUse   bool   `json:"one_time_use"`
}
```

### Implementation Notes

- Keep private KeyPackage bundle material local. Do not expose it as normal UI state unless strictly necessary.
- Persist pending welcome/invite metadata so the UI can list invites without requiring manual group ID input.
- Make accept idempotent. If the invite was already accepted and the group exists, return success.
- Make reject local-only unless protocol-level rejection is explicitly designed later.
- Existing method `CheckDHTWelcome` has legacy naming. Keep for compatibility, but prefer new frontend-facing methods.

### Error Model

Return typed/sentinel errors where possible:

- invite not found
- invite expired/stale
- identity mismatch
- already in group
- process welcome failed
- storage failure

Frontend copy should be user-friendly:

- "Lời mời không còn hiệu lực. Vui lòng yêu cầu tạo lời mời mới."
- "Lời mời không khớp với định danh hiện tại."

### Tests

- Generate join code stores/returns usable data.
- Pending invite appears after welcome is received or fetched.
- Accept invite creates group record and coordinator.
- Accept invite twice is safe.
- Reject invite removes/hides invite.
- Identity mismatch is rejected.

### Done Criteria

- UI can list pending invites without user typing group ID.
- UI can accept/reject invite from one screen.
- Invite errors are specific enough for user-facing messages.

---

## 5. P0 Workstream B - Group Membership Lifecycle

### Goal

Complete group lifecycle actions beyond create/send/add:

- leave group
- remove member
- refresh member list/events

### Proposed Backend APIs

```go
func (r *Runtime) LeaveGroup(groupID string) error
func (r *Runtime) RemoveMemberFromGroup(groupID string, peerID string) error
```

Optional event payloads:

```go
event: "group:members_changed"
payload: { "group_id": "...", "reason": "joined|left|removed|updated" }
```

### Implementation Notes

- Do not assume Admin Node is group admin. Admin controls network access, not necessarily every group.
- If group role/permission model is not implemented, document current policy clearly:
  - either every current member may propose removal
  - or removal is disabled until role policy exists
- Removing a member should go through MLS proposal/commit and update group keys.
- Leaving group should stop local coordinator and mark/remove local group state according to chosen product policy.

### Product Policy To Decide

For `LeaveGroup`, choose one:

1. Hard leave: remove local group state and no longer receive messages.
2. Soft leave: mark group inactive but keep local history.

Recommended for product UX: soft leave by default, with optional "delete local history" later.

### Tests

- Leave group stops coordinator and group disappears or becomes inactive.
- Remove member emits commit and updates member list.
- Removed member cannot decrypt future messages.
- Unauthorized/remove-not-supported path returns clear error.

### Done Criteria

- Group Info Panel can call real leave/remove APIs.
- Member list refreshes after membership changes.
- Security effect is real, not just UI removal.

---

## 6. P0 Workstream C - Session Takeover Lifecycle

### Goal

Productize single active device behavior so frontend can lock old sessions correctly.

### Current Foundation

The backend already has `SessionClaim` in auth handshake and import side effects. The missing part is a clear runtime state/event for the old device and a product-level lockout policy.

### Proposed Backend APIs / Events

```go
func (r *Runtime) GetSessionStatus() (SessionStatus, error)
func (r *Runtime) AcknowledgeSessionReplaced() error
```

```go
type SessionStatus struct {
    State             string `json:"state"` // active, replaced, unknown
    SessionStartedAt  int64  `json:"session_started_at"`
    ReplacedDetectedAt int64 `json:"replaced_detected_at,omitempty"`
}
```

Event:

```text
session:replaced
```

### Implementation Notes

- Avoid describing this as a "kill signal". It is session arbitration via signed `SessionClaim`.
- Decide and implement local lockout behavior:
  - high-security default: block normal app access after replacement
  - local DB remains on disk unless explicit secure deletion is implemented
- Store a local replaced flag if needed so restart still shows `SESSION_REPLACED`.
- Do not delete local history unless explicitly implemented and tested.

### Tests

- Newer session supersedes older session.
- Older session receives/derives replaced state.
- Replaced state survives restart if product policy requires.
- Replaced device cannot continue P2P operations.

### Done Criteria

- Frontend can route to Session Replaced screen using real backend state/event.
- User cannot continue sending messages after replacement.
- Behavior is documented and deterministic.

---

## 7. P0 Workstream D - Startup & Runtime Health Events

### Goal

Expose startup and runtime health state so the UI can show accurate loading/error/network states.

### Proposed Events

```text
startup:progress
startup:error
p2p:status
offline_sync:status
runtime:health
```

Example payload:

```go
type StartupProgressEvent struct {
    Stage   string `json:"stage"`   // db, sidecar, grpc, identity, p2p
    Message string `json:"message"`
}

type StartupErrorEvent struct {
    Code    string `json:"code"`    // database, crypto_engine, ipc, identity, p2p
    Message string `json:"message"`
    Fatal   bool   `json:"fatal"`
}
```

### Implementation Notes

- `Runtime.Startup` currently logs failures but does not expose granular UI state.
- Add internal startup state fields protected by mutex so frontend can query latest state after Wails is ready.
- Do not expose raw stack traces to frontend.
- Keep logs detailed for developer diagnostics.

### Proposed API

```go
func (r *Runtime) GetRuntimeHealth() RuntimeHealth
```

```go
type RuntimeHealth struct {
    StartupStage string `json:"startup_stage"`
    AppState     string `json:"app_state"`
    P2PRunning   bool   `json:"p2p_running"`
    CryptoReady  bool   `json:"crypto_ready"`
    LastError    string `json:"last_error,omitempty"`
    LastErrorCode string `json:"last_error_code,omitempty"`
}
```

### Tests

- Startup health reports DB failure.
- Startup health reports crypto unavailable.
- Authorized startup reports P2P running or P2P error.
- Frontend can poll `GetRuntimeHealth` safely.

### Done Criteria

- Screen 0 can show real progress or latest health state.
- Fatal startup failures have stable error codes.

---

## 8. P0 Workstream E - Admin Issuance Readiness

### Goal

Make Admin issuance safe and efficient for production UI.

### Current Foundation

Existing methods:

- `InitAdminKey`
- `HasAdminKey`
- `CreateBundle`
- `CreateAndImportSelfBundle`

Current `CreateBundle` unlocks the admin key using passphrase on each call. This works, but the frontend plan expects an admin locked/unlocked session.

### Proposed APIs

```go
func (r *Runtime) GetAdminStatus() (AdminStatus, error)
func (r *Runtime) UnlockAdmin(passphrase string) error
func (r *Runtime) LockAdmin() error
func (r *Runtime) ParseDeviceRequestJSON(data string) (DeviceAccessRequest, error)
func (r *Runtime) CreateBundleFromRequest(req IssueBundleRequest) (string, error)
```

```go
type AdminStatus struct {
    HasAdminKey bool `json:"has_admin_key"`
    Unlocked    bool `json:"unlocked"`
}

type DeviceAccessRequest struct {
    Version     int    `json:"version"`
    PeerID      string `json:"peer_id"`
    PublicKeyHex string `json:"mls_public_key"`
}

type IssueBundleRequest struct {
    DisplayName string `json:"display_name"`
    PeerID      string `json:"peer_id"`
    PublicKeyHex string `json:"public_key_hex"`
    ExpiresAt   int64  `json:"expires_at,omitempty"`
    Note        string `json:"note,omitempty"`
}
```

### Security Notes

- If caching unlocked admin private key in memory, wipe it on lock/shutdown when possible.
- Never persist passphrase.
- If in-memory unlock is too much scope, keep current passphrase-per-sign model but adapt UI text: "Nhập mật khẩu để ký bundle".

Recommended path:

1. Short term: keep passphrase-per-sign, add request JSON parser and validation.
2. Later: add explicit unlock session if needed.

### Tests

- Parse valid request JSON.
- Reject missing/invalid PeerID or public key.
- Create bundle requires display name.
- Wrong admin passphrase fails.
- Bundle remains bound to PeerID and MLS public key.

### Done Criteria

- Admin UI can paste `request.json` and issue bundle without manual copy mistakes.
- Display name remains Admin-controlled.
- Signing flow is explicit and safe.

---

## 9. P1 Workstream F - Network & Bootstrap Runtime Controls

### Goal

Allow real users to inspect and repair P2P connectivity without CLI flags.

### Proposed APIs

```go
func (r *Runtime) GetNetworkSettings() (NetworkSettings, error)
func (r *Runtime) SetBootstrapAddress(addr string) error
func (r *Runtime) ReconnectP2P() error
func (r *Runtime) ValidateMultiaddr(addr string) error
```

```go
type NetworkSettings struct {
    LocalPeerID       string `json:"local_peer_id"`
    LocalMultiaddr    string `json:"local_multiaddr"`
    BootstrapAddr     string `json:"bootstrap_addr"`
    ConnectedPeers    int    `json:"connected_peers"`
    VerifiedPeers     int    `json:"verified_peers"`
}
```

### Rules

- Bootstrap address must include `/p2p/PeerID`.
- Runtime override should be persisted only if product policy wants it.
- Reconnect must not corrupt current coordinators or group state.

### Done Criteria

- Settings screen can show local multiaddr and current bootstrap.
- User can paste a valid bootstrap address and reconnect.

---

## 10. P1 Workstream G - Diagnostics Snapshot & Log Export

### Goal

Support Developer Mode and thesis demo/debugging.

### Proposed APIs

```go
func (r *Runtime) GetDiagnosticsSnapshot() (DiagnosticsSnapshot, error)
func (r *Runtime) ExportDiagnostics() (string, error)
func (r *Runtime) OpenLogFolder() error
```

Diagnostics should include:

- local PeerID
- app state
- connected peers, inbound/outbound if available
- verified peers
- groups with epoch/token holder/tree hash short value
- offline sync queue/status
- blind-store status if available
- last runtime errors

### Security Rules

- Never export private keys.
- Never export admin passphrase.
- Redact full tokens unless explicitly required for developer-only debug.
- Prefer shortened PeerID/tree hash in UI; full values can be copied intentionally.

### Done Criteria

- Developer Mode has one backend snapshot call.
- Logs/diagnostics can be exported for bug reports/demo.

---

## 11. P1 Workstream H - Message Delivery State & Retry

### Goal

Give frontend reliable message status instead of guessing.

### Proposed Changes

Add message/envelope state fields if not already persisted:

- encrypted
- published
- queued_for_sync
- stored_offline
- failed

Proposed APIs:

```go
func (r *Runtime) RetryMessage(groupID string, messageID string) error
func (r *Runtime) DeleteLocalMessage(groupID string, messageID string) error
```

### Implementation Notes

- Current `MessageInfo` is minimal and does not include message ID/status.
- Add stable message IDs for frontend actions.
- Be careful with MLS sender behavior: own sent messages may be stored locally as plaintext while remote encrypted envelope travels over network.

### Done Criteria

- Chat UI can show failed/retry state based on backend data.
- Retry does not duplicate successfully applied messages.

---

## 12. P1 Workstream I - Admin Issuance History

### Goal

Allow Admin to audit issued bundles.

### Proposed Persistence

SQLite table:

```sql
CREATE TABLE IF NOT EXISTS admin_issuance_history (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    peer_id TEXT NOT NULL,
    public_key_hex TEXT NOT NULL,
    issued_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL,
    note TEXT,
    bundle_path TEXT
);
```

Proposed API:

```go
func (r *Runtime) ListIssuanceHistory() ([]IssuanceRecord, error)
```

### Rules

- This is an Admin local audit log, not a network-wide source of truth.
- Do not infer online authorization solely from this table.

### Done Criteria

- Admin can see previous bundle issuance records.
- Creating a bundle records an audit entry.

---

## 13. P2 Workstream J - Secure File Transfer Backend

### Goal

Implement Phase 7 secure swarming file transfer after chat/group/admin/migration flows are stable.

### Required Backend Capabilities

- MLS exporter-based per-file key derivation.
- File metadata announcement.
- Chunk protocol `/app/file/1.0.0`.
- Parallel chunk download from sender and peers.
- Progress events.
- Pause/cancel/retry.
- Hash verification.

### Proposed APIs

```go
func (r *Runtime) SendFile(groupID string, path string) (FileTransferInfo, error)
func (r *Runtime) AcceptFileTransfer(transferID string, savePath string) error
func (r *Runtime) CancelFileTransfer(transferID string) error
func (r *Runtime) RetryFileTransfer(transferID string) error
func (r *Runtime) GetFileTransfers(groupID string) ([]FileTransferInfo, error)
```

Events:

```text
file:transfer_progress
file:transfer_completed
file:transfer_failed
```

### Done Criteria

- End-to-end transfer works across at least 2 nodes.
- Receiver verifies file hash after reassembly/decryption.
- Transfer state survives temporary network disconnect where feasible.

---

## 14. Recommended Sprint Plan

### Sprint Backend-1: Group Invite Foundation

Scope:

- `GenerateJoinCode`
- pending invite persistence/list
- accept/reject invite
- invite error model

Why first:

- Without this, the product cannot complete the core group membership loop.

### Sprint Backend-2: Membership & Session Safety

Scope:

- leave group
- remove member or explicit "not supported yet" policy
- session replaced state/event
- message/group member change events

Why second:

- Completes group lifecycle and enforces single active device UX.

### Sprint Backend-3: Runtime Health & Admin Readiness

Scope:

- startup/runtime health API/events
- admin request JSON parser
- admin status/unlock decision
- admin issuance validation

Why third:

- Enables reliable onboarding/admin frontend and startup/error screens.

### Sprint Backend-4: Network & Diagnostics

Scope:

- network/bootstrap settings
- reconnect controls
- diagnostics snapshot
- log export/open log folder

Why fourth:

- Supports operations, demos, and troubleshooting.

### Sprint Backend-5: Message Status & Audit

Scope:

- message ID/status/retry
- admin issuance history

Why fifth:

- Improves frontend quality and auditability without blocking core chat.

### Sprint Backend-6: File Transfer

Scope:

- secure swarming file transfer backend

Why last:

- Large feature, should start after chat/membership/session/admin foundations are stable.

---

## 15. Backend Completion Checklist Before Frontend Product Rebuild

Minimum backend readiness:

- [ ] User can generate identity and import signed bundle.
- [ ] Admin can create bundle from request JSON safely.
- [ ] User can create group and send/receive messages.
- [ ] User can generate join code.
- [ ] Existing member can invite user.
- [ ] Invitee can list and accept/reject pending invite.
- [ ] User can leave group or feature is explicitly disabled with backend error.
- [ ] Member removal policy is implemented or explicitly deferred.
- [ ] Offline sync status can be queried.
- [ ] Session replaced state/event exists.
- [ ] Startup/runtime health can be queried by UI.
- [ ] Network status can distinguish running/no peers/syncing/offline enough for UI.
- [ ] Backup export/import works with clear errors.
- [ ] Developer diagnostics snapshot exists or is explicitly deferred.

Do not start full frontend polish until the first 10 items are real.

---

## 16. Test Strategy

For every P0 backend feature, add tests at the narrowest useful layer:

- Store tests for new persistence tables/state transitions.
- Service tests for Runtime-facing methods where possible.
- Coordination tests when MLS group state/epoch behavior changes.
- P2P integration/manual tests for multi-node behavior.

Manual validation matrix:

- one node first-run onboarding
- admin creates bundle
- two authorized nodes connect
- create group
- generate join code
- add member
- accept invite
- send message online
- send while peer offline, then reconnect
- import backup on second device
- old device becomes replaced or blocked

Before frontend starts depending on a new API:

- method exists in Wails bindings
- method has stable request/response DTOs
- method returns user-mappable errors
- `go test ./...` passes

