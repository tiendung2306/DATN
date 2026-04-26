# FRONTEND IMPLEMENTATION PLAN

Project: Zero-Trust P2P Internal Secure Communication App

Purpose: This document is the working context for implementing the production frontend of the application. The current frontend is mainly a backend/dev test UI. The goal is to rebuild it into an end-user product UI while preserving the security and protocol rules defined in `README.md`, `PROJECT_PLAN.md`, and `CURRENT_STATE.md`.

---

## 0. Mandatory Architecture Guardrails (Apply From FE-1)

These guardrails are mandatory coding conventions for all frontend implementation phases. They are not optional UI preferences.

### 0.1. Frontend Role: Thin Layer Over Go/Rust Core

- Frontend is a display and interaction layer.
- Security, identity, coordination, and persistence authority remain in Go/Rust/SQLite.
- Frontend must avoid re-implementing backend logic or deriving protocol truth heuristically.

### 0.2. State Management Strategy (Zustand Required)

- Use `zustand` as the primary client state store.
- Do not use React Context as the main dynamic runtime store for high-frequency P2P events.
- Split store by feature slices (recommended):
  - `app/runtime` (app state, startup health, fatal error)
  - `network` (peer status, reconnect/sync status)
  - `groups` (group list, selected group)
  - `chat` (messages, send status, pending retries)
  - `invites` (pending invites lifecycle)
  - `admin` (admin capability/status)
- Event handlers from Wails should be able to update Zustand directly without forcing unrelated UI re-renders.

### 0.3. Wails Event Lifecycle Rule (Memory Leak Prevention)

- Any `EventsOn(...)` subscription must always register cleanup/unsubscribe in `useEffect` return.
- Never leave long-lived listeners attached after component unmount.
- Prefer a shared wrapper/hook for event subscription to enforce consistent cleanup behavior.
- Review checklist for every PR touching events:
  - where is listener attached?
  - where is listener detached?
  - can this screen mount/unmount multiple times?

### 0.4. Smart/Dumb Component Boundary

- `screens/*` and `features/*` are Smart/Container layers:
  - call Wails bindings
  - read/write Zustand stores
  - orchestrate async flows and event reactions
- `components/*` are Dumb/Presentational layers:
  - render from props only
  - no direct Wails binding calls
  - no protocol/business orchestration

### 0.5. Desktop Routing Rule (No Browser History Assumptions)

- Wails app is desktop-first, not browser-navigation-first.
- Prefer state-based routing using app/runtime/session state.
- Avoid `BrowserRouter`.
- Use `MemoryRouter` only when internal nested navigation is needed; otherwise keep direct state-driven screen rendering.

### 0.6. UI Library Decision

- Preferred stack: **Shadcn UI + Tailwind CSS**.
- Reason: full source-level component control for secure dark-theme customization, while keeping implementation speed and consistency.

---

## 1. Product Direction

The app is a serverless, zero-trust, end-to-end encrypted internal communication platform for high-security organizations. It runs as a Wails desktop app with:

- Frontend: React + TypeScript + Tailwind in `app/frontend/`.
- Host/backend: Go Wails runtime in `app/service`.
- Crypto engine: Rust/OpenMLS sidecar, spawned by Go.
- Network: libp2p P2P, GossipSub, DHT/mDNS discovery, authenticated direct streams.
- Storage: local SQLite.

The frontend must make the product understandable to non-technical users while still exposing enough diagnostic detail for thesis demo/debug through Developer Mode.

---

## 2. UX Principles

### 2.1. Minimalist Cyber Security

- Use a serious dark theme.
- Keep the UI calm, direct, and not visually noisy.
- Avoid exposing technical terms such as `Epoch`, `Ratchet Tree`, `KeyPackage`, `TreeHash`, `Commit`, and `HLC` in normal user flows.
- Show technical details only in Developer Mode, tooltips, or diagnostics panels.

### 2.2. Zero-Trust Identity

- Users do not choose their own official display name.
- Display name is issued by the Admin Node through a signed `.bundle`.
- The first-run UI creates local device identity only.
- All identity and private keys stay local unless exported manually through encrypted `.backup`.

### 2.3. P2P Transparency

- Always show network status clearly.
- Users must understand whether the app is connected, syncing, reconnecting, or offline.
- Message lifecycle must not use Web2 semantics such as "read receipt" unless the backend explicitly supports it.
- Use P2P-safe statuses: encrypted, published, queued, stored offline, failed.

### 2.4. Secure Defaults

- Dangerous actions require explicit confirmation.
- Backup/export screens must warn clearly.
- Import/restore flows must explain session takeover.
- Admin actions must validate data before signing.

---

## 3. Frontend Technical Ground Rules

### 3.1. Wails Binding Rules

Wails binds exported methods on `*service.Runtime`.

Frontend imports must use:

```ts
import { SomeMethod } from "../wailsjs/go/service/Runtime";
import { service } from "../wailsjs/go/models";
```

Do not import from old paths such as `wailsjs/go/main/App`.

After changing exported Go runtime methods:

```powershell
cd app
wails generate module
cd frontend
npm run build
```

### 3.2. Current Frontend Location

- Root app: `app/frontend/src/App.tsx`
- Screens: `app/frontend/src/screens/`
- Components: `app/frontend/src/components/`
- Generated Wails bindings: `app/frontend/wailsjs/`

### 3.3. Suggested Frontend Structure

Target structure:

```text
app/frontend/src/
├── App.tsx
├── main.tsx
├── index.css
├── lib/
│   ├── runtime.ts
│   ├── appState.ts
│   ├── formatting.ts
│   └── errors.ts
├── components/
│   ├── ui/
│   │   ├── Button.tsx
│   │   ├── Card.tsx
│   │   ├── Modal.tsx
│   │   ├── Input.tsx
│   │   ├── TextArea.tsx
│   │   ├── Toast.tsx
│   │   ├── Dropzone.tsx
│   │   ├── StatusBadge.tsx
│   │   └── ProgressBar.tsx
│   ├── layout/
│   │   ├── AppShell.tsx
│   │   ├── Sidebar.tsx
│   │   └── TopBar.tsx
│   ├── network/
│   │   └── NetworkStatusIndicator.tsx
│   ├── chat/
│   │   ├── ChatView.tsx
│   │   ├── MessageBubble.tsx
│   │   ├── MessageComposer.tsx
│   │   ├── SystemMessage.tsx
│   │   └── FileMessageCard.tsx
│   └── diagnostics/
│       └── DeveloperOverlay.tsx
├── screens/
│   ├── StartupScreen.tsx
│   ├── WelcomeScreen.tsx
│   ├── AwaitingBundleScreen.tsx
│   ├── ImportBackupScreen.tsx
│   ├── MainAppScreen.tsx
│   ├── SettingsScreen.tsx
│   ├── AdminUnlockScreen.tsx
│   └── SessionReplacedScreen.tsx
└── features/
    ├── groups/
    ├── invites/
    ├── admin/
    ├── backup/
    └── diagnostics/
```

This structure may be adjusted during implementation, but keep feature boundaries clear.

---

## 4. App State Routing

The root UI should route based on application state and runtime health.

Known backend app states:

- `UNINITIALIZED`: no MLS identity yet.
- `AWAITING_BUNDLE`: local identity exists, but no signed bundle.
- `AUTHORIZED`: valid bundle imported, normal user mode.
- `ADMIN_READY`: authorized user with admin root key available.
- `ERROR`: unrecoverable local app state error.

Frontend-level additional states:

- `STARTING`: app is initializing database/sidecar/runtime.
- `FATAL_ERROR`: startup failed.
- `SESSION_REPLACED`: local device session has been replaced by a newer active device.
- `ADMIN_LOCKED`: admin key exists but passphrase is required before signing.

Routing priority:

1. `STARTING` -> `StartupScreen`
2. `FATAL_ERROR` -> startup fatal error screen
3. `SESSION_REPLACED` -> `SessionReplacedScreen`
4. `UNINITIALIZED` -> `WelcomeScreen`
5. `AWAITING_BUNDLE` -> `AwaitingBundleScreen`
6. `AUTHORIZED` / `ADMIN_READY` -> `MainAppScreen`

---

## 5. Screen Specifications

## Screen 0: App Initializing

Purpose: Show startup progress while the Go host initializes local services.

Layout:

- Centered logo.
- Spinner.
- One-line technical progress text.

Progress states:

- "Khởi tạo SQLite Database..."
- "Khởi động Lõi Bảo Mật..."
- "Thiết lập kết nối IPC/gRPC..."
- "Kiểm tra trạng thái định danh..."
- "Đang khởi động mạng P2P..." when authorized.

Fatal errors:

- Database error: "Không thể đọc dữ liệu định danh. Vui lòng kiểm tra quyền truy cập ổ đĩa hoặc dung lượng trống."
- Crypto engine error: "Lõi bảo mật không phản hồi. Vui lòng khởi động lại ứng dụng."
- IPC error: "Không thể mở cổng IPC cục bộ."

Actions:

- `[Khởi động lại]`
- `[Mở thư mục Log]` only if supported or in Developer Mode.

Implementation notes:

- If the current backend does not expose granular startup progress, initially implement a simple staged loading UI and later wire it to runtime events.

Acceptance checklist:

- Startup screen appears before app state routing.
- Fatal error state is clear and recoverable through restart.
- User is not shown raw stack traces.

---

## Screen 1: Welcome & Create Device Identity

State: `UNINITIALIZED`

Purpose: Create local security identity for a new device.

Layout:

- Minimal centered view.
- App logo using lock/network visual language.
- No input fields.

Text:

- Title: "Bảo Mật Bắt Đầu Từ Thiết Bị Của Bạn"
- Explanation: "Ứng dụng sẽ tạo định danh bảo mật riêng cho thiết bị này. Khóa bảo mật chỉ được lưu cục bộ trên máy của bạn."
- Note: "Tên hiển thị của bạn sẽ do Quản trị viên cấp phép sau khi xác thực."

Actions:

- Primary: `[Tạo định danh bảo mật]`
- Secondary link: `[Khôi phục từ file định danh (.backup)]`

Backend mapping:

- Primary calls existing identity generation flow, likely `GenerateKeys()`.
- Backup link opens `ImportBackupScreen`.

Rules:

- Do not ask user for display name.
- Do not start P2P after identity generation. Route to `AWAITING_BUNDLE`.

Acceptance checklist:

- User can create identity without entering a name.
- After success, user sees Screen 2.
- Errors from identity generation are shown with friendly messages.

---

## Screen 2: Awaiting Bundle

State: `AWAITING_BUNDLE`

Purpose: Let user send device identity request to Admin and import signed `.bundle`.

Layout:

- Title: "Thông tin định danh thiết bị"
- QR code block.
- PeerID and MLS public key fields.
- Request export actions.
- Bundle dropzone.

QR JSON:

```json
{
  "version": 1,
  "peer_id": "12D3KooW...",
  "mls_public_key": "hex..."
}
```

Actions:

- `[Copy tất cả]`
- `[Copy PeerID]`
- `[Copy MLS Public Key]`
- `[Tải file request.json]`
- Drop `.bundle` file into dropzone.
- `[Nhập file cấp phép & Kết nối]`

Validation errors:

- "File sai định dạng."
- "Bundle không khớp với thiết bị này."
- "Chứng thư đã hết hạn."
- "Chữ ký không hợp lệ."

Backend mapping:

- `GetOnboardingInfo()`
- `OpenAndImportBundle()` if using native dialog, or add/import a path/file-content method if drag-and-drop requires it.

Acceptance checklist:

- PeerID and MLS public key are visible and copyable.
- `request.json` contains both required values and version.
- Invalid bundle errors are specific.
- Successful import routes to main app.

---

## Screen 3: Main Layout & Group List

State: `AUTHORIZED` or `ADMIN_READY`

Purpose: Main shell for chat and network awareness.

Layout:

- Left sidebar:
  - app/user identity summary
  - group list
  - pending invites entry
  - settings/admin links
  - network status at bottom
- Main content:
  - selected chat or empty state.

Network status:

- Green: `Connected: X peers`
- Gray: `Authorized: No peers found`
- Yellow: `Syncing: Y messages`
- Yellow: `Reconnecting...`
- Red: `Offline / No network`

Tooltips:

- Explain each status in user-friendly language.

Group list item:

- group name
- last decrypted message preview
- wall-time timestamp
- sync/error fallback:
  - "Đang đồng bộ..."
  - "Không thể giải mã"
  - "Chưa có tin nhắn"

Empty state:

- "Bạn chưa tham gia nhóm bảo mật nào."
- CTA: `[Tạo nhóm mới]`
- Optional CTA: `[Xem lời mời]`

Backend mapping:

- `GetNodeStatus()`
- `GetGroups()`
- `GetGroupMessages(groupID)` for last message if needed
- Runtime events for peer/group/message updates.

Acceptance checklist:

- Network status is always visible.
- Empty user can create first group or check pending invites.
- Sidebar remains usable when no peers are connected.

---

## Screen 4: Chat View E2EE

Purpose: Send and receive encrypted group messages.

Header:

- Group name.
- Lock icon.
- Text: "Được mã hóa đầu cuối bằng OpenMLS"
- Button/icon to open Group Info Panel.
- Developer Mode area may show epoch/tree hash/token holder.

Message bubble:

- plaintext content
- wall-time
- sender display name
- compact status icon

Normal statuses:

- Lock icon: "Đã mã hóa an toàn"
- Network icon: "Đã phát lên mạng nội bộ"

Abnormal statuses:

- "Đang chờ đồng bộ với các thiết bị khác"
- "Đã lưu offline (Chờ có mạng để gửi)"
- "Gửi thất bại"

Failed message actions:

- `[Thử gửi lại]`
- `[Xóa]`

System messages:

- "Bạn đã tạo nhóm bảo mật."
- "Bob đã tham gia nhóm. Khóa bảo mật đã được cập nhật."
- "Khóa nhóm đã được cập nhật."
- "Đang đồng bộ 15 tin nhắn bị lỡ..."

Developer Mode-only details:

- Epoch number.
- Commit applied.
- HLC timestamp.
- sender PeerID.

Input area:

- text input
- send button
- attach file button. If file transfer is not implemented yet, show disabled/planned tooltip.

Backend mapping:

- `SendGroupMessage(groupID, text)`
- `GetGroupMessages(groupID)`
- Wails event `group:message` if available.

Acceptance checklist:

- UI never claims "read" or "delivered to all" unless supported.
- Own sent message appears immediately with appropriate pending/published state.
- Incoming messages appear sorted consistently.
- Failed send can be retried or removed locally.

---

## Screen 4b: File Transfer UI (Planned / Phase 7)

Purpose: UI plan for secure file transfer once backend support is implemented.

Important: This is planned for a later phase. Do not imply this is currently complete unless the backend APIs exist.

Sender flow:

- User clicks attachment.
- Selects file.
- App shows encryption/preparation state.
- File message card appears in chat.

File card:

- file name
- file size
- progress bar
- transfer speed
- peer assistance count, e.g. "Đang tải qua mạng nội bộ (3 peers hỗ trợ)..."
- actions: pause/cancel/retry/open folder when supported.

Receiver flow:

- File card appears.
- User clicks download or auto-download based on settings.
- App verifies hash after download.

Error states:

- "Tải file thất bại."
- "Không thể xác minh toàn vẹn file."
- "Không còn peer nào giữ file này."

Acceptance checklist:

- File transfer UI is visually separate from text messages.
- Large files show progress and speed.
- Integrity verification result is clear.

---

## Screen 5: Group Info Panel

Purpose: Show secure group members and membership actions.

Layout:

- Slide-out panel or modal from Chat View.
- Title: "Thành viên nhóm bảo mật"
- Member list with online/offline indicators.

Member row:

- display name
- online/offline status
- optional short PeerID in Developer Mode

Actions:

- `[Thêm thành viên]`
- `[Rời nhóm bảo mật]`
- `[Xóa khỏi nhóm]` only for users allowed by group policy.

Do not label removal as "Admin removes member" unless group roles are explicitly implemented. Admin Node controls network issuance, not necessarily every group.

Backend mapping:

- `GetGroupMembers(groupID)`
- `GetGroupStatus(groupID)`
- Add/invite methods in `service.Runtime`.

Acceptance checklist:

- Normal users see friendly group membership language.
- Developer Mode can reveal technical membership details.
- Unauthorized actions are hidden or disabled.

---

## Screen 6a: Create Secure Group

Purpose: Create a new encrypted group.

Entry:

- Sidebar empty state.
- Sidebar `[Tạo nhóm mới]` button.

Modal fields:

- Group name.
- Optional short description.

Actions:

- `[Tạo nhóm bảo mật]`
- cancel/close.

Success behavior:

- Create group.
- Navigate to new Chat View.
- Show system message: "Bạn đã tạo nhóm bảo mật."

Backend mapping:

- `CreateGroupChat(groupID)` currently uses `groupID`; may need product-level group name metadata if not already supported.

Implementation note:

- If backend only supports `groupID`, frontend can initially use group name as group ID or request a backend DTO for display metadata later.

Acceptance checklist:

- User can create first group from empty state.
- Newly created group appears in sidebar immediately.
- Chat View opens after creation.

---

## Screen 8a: Generate Join Code

Purpose: Let an invitee generate a one-time group join code for an existing member to use.

User-facing term:

- "Mã tham gia nhóm"

Technical meaning:

- KeyPackage.

Layout:

- Explanation text:
  - "Mã này chỉ có tác dụng một lần để gia nhập nhóm."
  - "Hãy gửi mã này cho người đang ở trong nhóm."
- Button: `[Tạo mã tham gia]`
- QR code.
- Full text code.
- optional checksum/hash for verification.

Actions:

- `[Copy]`
- `[Lưu file .join-request]`

Warnings:

- "Mã này chỉ dùng một lần."
- "Nếu tạo mã mới, mã cũ có thể không còn hiệu lực."

Backend mapping:

- `GenerateKeyPackage()` or equivalent runtime binding.

Acceptance checklist:

- User can generate, copy, and export join request.
- UI does not expose "KeyPackage" as the primary label.
- Generated code is clearly tied to current device identity.

---

## Screen 5b: Add Member Modal

Purpose: Existing group member adds another user to a group.

Entry:

- Group Info Panel -> `[Thêm thành viên]`

Tabs:

1. `Dán mã tham gia`
   - large text area
   - helper: "Người được mời có thể tạo mã này từ ứng dụng của họ."
2. `Chọn thiết bị đã cấp quyền`
   - list of authorized/known peers currently online
   - do not show arbitrary unauthenticated DHT peers.

Action:

- `[Tạo lời mời & Thêm vào nhóm]`

Technical meaning:

- Create MLS Add commit and Welcome data.

Backend mapping:

- `AddMemberToGroup`
- `InvitePeerToGroup`
- `GenerateKeyPackage`/invite store methods as needed.

Acceptance checklist:

- Manual join code paste path works.
- Known peer selection path only shows authorized peers.
- Success produces user-facing system message in chat.

---

## Screen 10: Pending Invites

Purpose: Invitee accepts or rejects pending group invites.

Entry:

- Sidebar item "Lời mời"
- Badge when pending invites exist.

Invite row:

- group name if known
- inviter if known
- time received
- status

Actions:

- `[Tham gia nhóm]`
- `[Từ chối]`

User-facing errors:

- "Lời mời không còn hiệu lực. Vui lòng yêu cầu tạo lời mời mới."
- "Lời mời không khớp với định danh hiện tại."
- "Không thể tham gia nhóm. Vui lòng thử lại hoặc yêu cầu lời mời mới."

Developer Mode details:

- stale welcome
- epoch mismatch
- identity mismatch
- process welcome failure

Backend mapping:

- Existing Welcome fetch/process methods, e.g. `CheckDHTWelcome` legacy naming and `JoinGroupWithWelcome`.

Acceptance checklist:

- Pending invite badge is visible.
- Accepting invite joins group and opens Chat View.
- Rejecting invite removes it locally.

---

## Screen 16: Leave Group & Remove Member

Purpose: Manage group departure and member removal.

Leave group:

- Entry: Group Info Panel -> `[Rời nhóm bảo mật]`
- Confirm modal:
  - "Bạn sẽ không nhận được tin nhắn mới trong nhóm này sau khi rời."
- Action: `[Xác nhận rời nhóm]`

Remove member:

- Entry: member row action for allowed users.
- Confirm modal:
  - "Thành viên này sẽ bị loại khỏi nhóm và khóa bảo mật nhóm sẽ được cập nhật."
- Action: `[Xóa khỏi nhóm]`

Rules:

- Do not claim Admin Node can remove any member unless group role policy supports that.
- Use "khóa bảo mật được cập nhật" instead of "epoch/commit" in normal UI.

Acceptance checklist:

- Destructive actions require confirmation.
- User-facing copy explains security effect.
- Removed/left members update group member list.

---

## Screen 6: Identity Migration - Export Backup

Location: Settings -> "Bảo mật & Thiết bị"

Purpose: Export encrypted `.backup` for manual device migration.

Warnings:

- "Nếu bạn khôi phục file này trên thiết bị khác, thiết bị mới sẽ trở thành phiên hoạt động chính. Thiết bị hiện tại có thể bị ngắt khỏi mạng khi thiết bị mới kết nối."
- "Tuyệt đối KHÔNG chia sẻ file .backup cho bất kỳ ai."
- "Nếu mất mật khẩu giải mã, file backup sẽ không thể khôi phục."

Flow:

1. User checks: "Tôi hiểu rằng file này chứa toàn bộ danh tính của tôi."
2. User enters passphrase twice.
3. Strength indicator: weak / medium / strong.
4. Export button enabled only when passphrases match and strength requirement passes.
5. App exports file.
6. Success state: "File định danh đã được tạo thành công. Hãy cất giữ an toàn."

Backend mapping:

- `ExportIdentity`

Acceptance checklist:

- Export cannot proceed without explicit confirmation.
- Weak/mismatched passphrase blocks export.
- Success state is clear.

---

## Screen 8: Import Identity Backup

Purpose: Restore identity and local content snapshot from encrypted `.backup`.

Entry:

- Welcome screen secondary link.
- Settings -> import backup if app supports replacing current identity.

Flow:

1. Drop/select `.backup`.
2. Enter passphrase.
3. Confirm overwrite if local identity already exists.
4. Decrypt and restore.
5. On next connection, app uses a new SessionClaim and this device becomes the active session.

Warnings:

- Restoring on this device may replace existing local identity.
- Restoring this identity on a new device may cause older devices to be disconnected.

Errors:

- wrong passphrase
- corrupt backup
- unsupported backup version
- restore failed

Backend mapping:

- `ImportIdentityFromFile`

Acceptance checklist:

- Import path is available before onboarding.
- User understands overwrite/session consequences.
- Invalid backup errors are clear.

---

## Screen 12: Session Replaced

Purpose: Lock old device after another device with same identity becomes active.

Trigger:

- A newer valid session for the same identity is accepted by peers through signed `SessionClaim`.
- Avoid saying the new device broadcasts a "kill signal"; this is session arbitration.

Layout:

- Full-screen blocking overlay.
- Dark/red high-severity visual style.

Text:

- Title: "Phiên Làm Việc Đã Kết Thúc"
- Body: "Định danh của bạn vừa được kết nối từ một thiết bị khác với phiên làm việc mới hơn. Thiết bị này đã bị ngắt kết nối, dữ liệu phiên cục bộ đã bị vô hiệu hóa."

Actions:

- `[Đóng ứng dụng]`
- Link: `[Xem hướng dẫn khôi phục]`

Product decision:

- Decide whether old device can still view local message history after takeover.
- High-security default: block app access after session replacement.

Acceptance checklist:

- User cannot continue normal P2P operations after session replacement.
- Message clearly explains why access ended.
- No confusing "logout from server" wording.

---

## Screen 13: Admin Setup & Unlock

Purpose: Protect Root Admin Key before issuing bundles.

State A: first-time Admin setup

- Text: "Thiết bị này đang được cấu hình làm Thiết Bị Quản Trị. Vui lòng tạo mật khẩu để khóa Root Private Key."
- Passphrase input x2.
- Strength indicator.
- Action: `[Khởi tạo Thiết bị Quản trị]`

State B: Admin key exists but locked

- Text: "Phiên làm việc quản trị đã hết hạn. Vui lòng mở khóa để tiếp tục cấp phép cho thiết bị mới."
- Passphrase input.
- Action: `[Mở khóa quyền quản trị]`

Errors:

- "Sai mật khẩu."
- "Khóa quản trị bị hỏng."

Backend mapping:

- `InitAdminKey`
- `HasAdminKey`
- Backend may need explicit unlock method if not available separately.

Acceptance checklist:

- Admin cannot sign bundle while locked.
- Read-only dashboard can remain visible if useful.
- Passphrase is never persisted in frontend state.

---

## Screen 7: Admin Panel - Bundle Issuance

Purpose: Admin issues signed `.bundle` to authorize a new device.

Visibility:

- Only visible when app has admin capability.
- Signing actions require unlocked Root Key.

Inputs:

- `PeerID`
- `MLS Public Key`
- `Display Name` required
- Expiry, default 365 days
- Optional note if issuance history supports it.

Actions:

- `[Dán từ request.json]` to auto-fill PeerID and public key.
- `[Ký điện tử & Xuất file cấp phép]`

Validation:

- PeerID format invalid.
- MLS public key missing/invalid hex.
- Display name empty.
- Expiry invalid.
- Warning if PeerID was already issued before.

Success checklist:

- "Đã ký bằng Root Admin Key."
- "Bundle bị khóa cứng với PeerID được cấp."
- "Hành động tiếp theo: Gửi file .bundle qua kênh an toàn cho nhân sự."

Backend mapping:

- `CreateBundle`
- `CreateAndImportSelfBundle` for admin quick setup if retained.

Acceptance checklist:

- Admin can issue bundle from pasted request.
- Display name is mandatory and Admin-controlled.
- Output file is clearly named.

---

## Screen 18: Issuance History

Purpose: Let Admin review previously issued bundles.

Table columns:

- display name
- short PeerID
- issued at
- expires at
- status if known
- note

Actions:

- copy PeerID
- export history if needed
- filter/search

Implementation note:

- If backend does not persist issuance history yet, this screen is planned. Do not fake history from unrelated data.

Acceptance checklist:

- Admin can audit who has been issued access.
- Sensitive values are truncated by default.

---

## Screen 17: Network & Bootstrap Settings

Purpose: Help users/admins connect in networks where automatic discovery is limited.

Fields:

- local PeerID
- local multiaddr
- connected peers count
- bootstrap address input
- current bootstrap source

Actions:

- copy local multiaddr
- paste/import bootstrap address
- reconnect
- write/export bootstrap file if supported

Validation:

- invalid multiaddr
- missing `/p2p/PeerID`
- connection failed

Acceptance checklist:

- User can copy local connection information.
- Bootstrap address validation explains required format.
- Reconnect action is clear and safe.

---

## Screen 20: Developer Mode & Diagnostics

Location: Settings -> Advanced

Purpose: Debug P2P behavior and support thesis technical demonstration.

Warning:

- "Chế độ này có thể hiển thị thông tin kỹ thuật nhạy cảm. Chỉ bật khi debug hoặc trình diễn kỹ thuật."

Toggle:

- Developer Mode on/off.

When enabled:

- Sidebar shows inbound/outbound peer counts.
- Chat View shows group epoch, token holder, shortened tree hash.
- Message bubble can show HLC and sender PeerID.
- Sync Queue panel shows pending envelopes/messages.
- Technical system messages are visible.
- Log export button appears.

Actions:

- `[Xuất Log P2P]`
- `[Sao chép thông tin chẩn đoán]`

Acceptance checklist:

- Developer Mode is off by default.
- Technical data is hidden from normal users.
- Diagnostics are useful for demo/debug without exposing private keys.

---

## 6. Shared Components

Build these shared components early:

- `Button`
- `Input`
- `TextArea`
- `Card`
- `Modal`
- `Dropzone`
- `Toast`
- `StatusBadge`
- `ProgressBar`
- `CopyField`
- `QRCodeBlock`
- `ConfirmDialog`
- `NetworkStatusIndicator`
- `EmptyState`
- `ErrorCallout`
- `WarningBox`
- `StrengthMeter`

Rules:

- Components should be dark-theme first.
- Components should support disabled/loading states.
- Copy actions should always show toast feedback.
- Destructive actions must use confirm dialogs.

---

## 7. Data & UI Models

Frontend can define local view models that adapt Wails DTOs.

Suggested models:

```ts
type NetworkStatus =
  | "connected"
  | "authorized_no_peers"
  | "syncing"
  | "reconnecting"
  | "offline";

type MessageDeliveryStatus =
  | "encrypted"
  | "published"
  | "queued_for_sync"
  | "stored_offline"
  | "failed";

type AppRouteState =
  | "starting"
  | "fatal_error"
  | "uninitialized"
  | "awaiting_bundle"
  | "authorized"
  | "admin_ready"
  | "session_replaced";
```

Do not let UI models leak cryptographic assumptions. Keep labels user-facing.

---

## 8. Implementation Phases

### Phase FE-1: Design System & App Shell

- Establish dark theme tokens.
- Bootstrap Shadcn UI foundation on top of Tailwind.
- Build shared UI components.
- Implement app shell layout.
- Implement desktop-safe state router skeleton.
- Implement Zustand stores and slice boundaries.
- Implement Wails event bridge pattern with strict subscribe/unsubscribe cleanup.
- Enforce smart/dumb component boundaries from first screen.

Done when:

- App can render startup, welcome, awaiting bundle, and main shell with mocked data.
- No direct Wails binding calls exist inside `components/*`.
- Event listeners demonstrate deterministic cleanup in mount/unmount tests.
- Store updates from high-frequency events do not trigger full app re-render patterns.

#### FE-1 Execution Plan (thứ tự làm code — bắt đầu từ đây)

**Trạng thái repo hiện tại (điểm xuất phát):** `app/frontend` đã có Vite + React 18 + Tailwind; `App.tsx` dùng `useState` + polling `GetAppState`; chưa có Zustand / Shadcn.

**Bước 1 — Phụ thuộc npm**

- Thêm `zustand`.
- Thêm Shadcn qua CLI (theo hướng dẫn shadcn + Vite): sinh `components.json`, alias `@/` (hoặc tương đương trong `tsconfig` + `vite.config`), cài `class-variance-authority`, `clsx`, `tailwind-merge`, `@radix-ui/react-slot` (và các peer Radix theo từng component khi add).
- Giữ `npm run build` (`tsc && vite build`) xanh sau mỗi bước.

**Bước 2 — Design tokens & theme dark (Tailwind + CSS variables)**

- Trong `index.css` (hoặc file theme shadcn): định nghĩa biến màu semantic (`background`, `foreground`, `muted`, `destructive`, `border`, `ring`, `primary` …) cho dark theme cybersecurity.
- Cập nhật `tailwind.config` để map `colors` tới CSS variables (chuẩn Shadcn).
- Không cần polish pixel-perfect trong FE-1; cần **một bảng màu ổn định** để màn sau không đổi token liên tục.

**Bước 3 — Base UI components (Shadcn “first wave”)**

- Add tối thiểu: `Button`, `Card`, `Input`, `Label`, `Separator`, `Skeleton` (tuỳ chọn `Dialog`, `DropdownMenu` nếu shell cần menu).
- Đặt source tại `src/components/ui/*` (theo shadcn), **chỉ presentation** — không import `wailsjs` ở đây.

**Bước 4 — Cấu trúc thư mục & “router”**

- Tạo `src/lib/` nếu chưa có: `wails.ts` (export type-only helpers nếu cần), `routerState.ts` hoặc gom vào store.
- Tạo `src/stores/`:
  - `useAppRuntimeStore`: `appState` (LOADING | UNINITIALIZED | …), `runtimeHealth` snapshot tùy chọn, `startupStage` từ events.
  - Slice rỗng placeholder: `useNetworkStore`, `useGroupsStore` (chỉ shape + actions no-op) để FE-2+ không refactor store lớn.
- Tạo `src/components/layout/AppShell.tsx`: khung chung (ví dụ top bar + vùng nội dung) nhận `children` và props header; **không** gọi Go.

**Bước 5 — Wails event bridge (bắt buộc có pattern cleanup)**

- Tạo `src/hooks/useWailsEvent.ts` (hoặc `lib/wailsEvents.ts` + hook):
  - `useWailsEvent(name, handler)` dùng `EventsOn` từ `wailsjs/runtime/runtime` (import path theo codegen hiện tại).
  - Trong cleanup `useEffect`: gọi `EventsOff` / API tương ứng Wails v2 — **mỗi listener phải unsubscribe khi unmount**.
- Tạo một màn demo nhỏ hoặc mount tạm trong `DashboardScreen` chỉ để verify: mount → nhận event → unmount → không còn handler (có thể manual test trước; unit test optional).

**Bước 6 — Refactor `App.tsx` sang thin root**

- Chuyển polling `GetAppState` + nhánh màn hình vào **container** (ví dụ `src/screens/RootRouter.tsx` hoặc `src/app/AppRoot.tsx`) gọi store.
- `App.tsx` chỉ: `import './index.css'`, bọc provider tối thiểu nếu cần (Shadcn Toaster), render `<AppRoot />`.
- Điều hướng theo state backend: vẫn **state-based** (không `BrowserRouter`); bổ sung trạng thái `STARTING` khi bắt đầu wire `GetRuntimeHealth` + `startup:*` (có thể stub trong FE-1, hoàn thiện ở FE-2).

**Bước 7 — Tách Smart/Dumb trên 1 màn mẫu (không cần migrate hết repo trong FE-1)**

- Chọn **một** màn: ví dụ `SetupScreen` hoặc màn loading mới.
- Dumb: `components/setup/SetupView.tsx` nhận props `onCreateIdentity`, `error`, `loading`.
- Smart: `screens/SetupScreen.tsx` gọi `GenerateKeys` / Wails, cập nhật store, truyền xuống view.
- Các màn còn lại có thể để nguyên tạm thời; ghi `// TODO FE-2: extract dumb view` nếu cần — tránh thay đổi hàng loạt ngoài phạm vi FE-1.

**Bước 8 — Kiểm tra & nghiệm thu**

- `cd app/frontend && npm run build` — pass.
- `wails build` từ `app/` (hoặc `wails dev`) — mở app, qua flow LOADING → UNINITIALIZED / AWAITING_BUNDLE / DASHBOARD không crash.
- Checklist: không file nào dưới `components/ui/*` import `../wailsjs`; mọi `EventsOn` qua hook có cleanup.

**Phạm vi tách bạch (không làm trong FE-1):**

- Rebuild toàn bộ màn theo spec Screen 0–20; chỉ chuẩn bị nền.
- Thêm `react-router-dom` trừ khi thật sự cần nested route — ưu tiên state router đến hết FE-4.

**Thứ tự commit gợi ý (nhỏ, dễ review):** (1) deps + shadcn scaffold (2) tokens (3) stores + AppRoot (4) wails event hook (5) AppShell + một màn smart/dumb mẫu.

### Phase FE-2: Onboarding & Bundle Flow

- Rebuild Welcome screen.
- Rebuild Awaiting Bundle screen.
- Add QR/request JSON export.
- Implement bundle import UX and errors.
- Implement Import Backup entry point.

Done when:

- A new user can create identity, export request, import bundle, and enter main app.

### Phase FE-3: Main Chat UX

- Implement sidebar group list.
- Implement network indicator.
- Implement chat view.
- Implement message statuses and system messages.
- Implement failed message retry UI.

Done when:

- User can create/open group and send/receive messages through existing Wails methods.

**Implementation status checkpoint (current):**
- FE-3 core đã được implement ở mức functional:
  - `MainAppScreen` thay placeholder cho `AUTHORIZED/ADMIN_READY`
  - sidebar groups + network indicator + chat timeline/composer
  - realtime event wiring qua `useWailsEvent` (`group:message`, `group:epoch`, `group:joined`)
  - failed send local retry/remove
- Đang trong giai đoạn UI polish để nâng visual parity gần mock product (spacing, typography, iconography, panel hierarchy).
- FE-4 nên bắt đầu sau khi chốt vòng polish hiện tại để tránh refactor chồng chéo.

#### FE-3 Detailed Execution Plan (main shell + chat core)

**Mục tiêu FE-3 (scope cứng):**
- Thay `MainShellPlaceholder` bằng Main App Shell thật cho trạng thái `AUTHORIZED/ADMIN_READY`.
- Hoàn thiện vòng lặp chat core: danh sách nhóm -> mở nhóm -> đọc lịch sử -> gửi tin -> nhận realtime.
- Thể hiện network trạng thái rõ ràng cho desktop P2P app.
- Không claim các semantics backend chưa có (ví dụ read receipts).

**Điểm xuất phát hiện tại:**
- Router/state onboarding đã chạy qua `RootRouter`, `Welcome`, `AwaitingBundle`, `ImportBackup`.
- FE-1 foundation đã có: `zustand` stores, `useWailsEvent`, shadcn primitives, dark tokens.
- API sẵn có trên Wails bindings:
  - `GetGroups`, `CreateGroupChat`, `GetGroupMessages`, `SendGroupMessage`
  - `GetNodeStatus`, `GetRuntimeHealth`, `TriggerOfflineSync`
  - events đã dùng ở code cũ: `group:message`, `group:epoch`, `group:joined`.

**Phạm vi FE-3 / ngoài phạm vi FE-3:**
- **Trong phạm vi:** shell chính, group list, network indicator, chat timeline/composer, realtime updates, trạng thái gửi thất bại + retry cục bộ.
- **Ngoài phạm vi:** group invite full UX (FE-4), admin issuance UX (FE-6), file transfer (FE-8), advanced diagnostics (FE-7).

##### Bước 1 — Main shell architecture
- Tạo màn `MainAppScreen` để thay `MainShellPlaceholder`.
- Tách layout thành 3 khối:
  1. Sidebar (identity + group list + create group + pending invites entry placeholder),
  2. Main chat panel (timeline + composer),
  3. Optional right panel placeholder (group info) để FE-4 nối tiếp.
- Giữ screen-level orchestration ở `screens/*`, presentation ở `components/*`.

**Files đề xuất:**
- `app/frontend/src/screens/MainAppScreen.tsx`
- `app/frontend/src/components/layout/MainSidebar.tsx`
- `app/frontend/src/components/chat/ChatView.tsx`
- `app/frontend/src/components/chat/MessageComposer.tsx`
- `app/frontend/src/components/chat/MessageList.tsx`

##### Bước 2 — Zustand store expansion cho FE-3
- Mở rộng store theo slice (không nhồi logic vào component):
  - `useGroupsStore`: thêm loading/error + `refreshGroups()` action adapter.
  - `useNetworkStore`: map từ `NodeStatus` sang `NetworkStatus`.
  - thêm `useChatStore` mới: `messagesByGroup`, `sendingQueue`, `failedQueue`, `activeGroupId`.
- Chuẩn hóa DTO adapter ở `src/lib` để map Wails model -> UI model.

**Files đề xuất:**
- `app/frontend/src/stores/useGroupsStore.ts`
- `app/frontend/src/stores/useNetworkStore.ts`
- `app/frontend/src/stores/useChatStore.ts` (new)
- `app/frontend/src/lib/chatModel.ts` (new)
- `app/frontend/src/lib/networkModel.ts` (new)

##### Bước 3 — Sidebar + create/open group flow
- Sidebar hiển thị:
  - display name + short peer id,
  - network badge luôn visible,
  - list groups từ `GetGroups`.
- Add “create group” UX tối giản:
  - input group id/name + action `CreateGroupChat`,
  - success: refresh groups + auto-open group vừa tạo.
- Nếu chưa có group:
  - show empty state có CTA tạo nhóm.

##### Bước 4 — Chat timeline và message composer
- Khi chọn group:
  - load `GetGroupMessages(groupId)`,
  - sort theo timestamp hiện có,
  - render bubble phân biệt `is_mine`.
- Composer:
  - Enter gửi / Shift+Enter xuống dòng (nếu dùng textarea),
  - disable khi empty hoặc không có active group,
  - optimistic insert cho tin nhắn local với trạng thái tạm.

##### Bước 5 — Realtime events + cleanup
- Dùng `useWailsEvent` cho các sự kiện:
  - `group:message`: append message đúng active group hoặc tăng badge unread.
  - `group:epoch`: update metadata nhóm nhẹ.
  - `group:joined`: refresh groups và điều hướng nhóm mới.
- Tuyệt đối không gọi trực tiếp `EventsOn` trong component UI presentation.

##### Bước 6 — Message status và system message policy
- Chuẩn hóa trạng thái user-facing:
  - `sending`, `published`, `failed` (dựa trên khả năng backend hiện tại),
  - không hiển thị “seen/read”.
- System messages FE-side tối thiểu:
  - “Bạn đã tạo nhóm…”
  - “Đang đồng bộ…”
  - “Gửi thất bại, vui lòng thử lại.”

##### Bước 7 — Failed send + retry policy
- Nếu `SendGroupMessage` reject:
  - giữ message ở failed queue cục bộ (không mất nội dung),
  - UI hiển thị `Retry` và `Remove`.
- Retry:
  - gửi lại qua `SendGroupMessage`,
  - nếu thành công thì chuyển trạng thái published.
- Lưu ý: đây là UI recovery cục bộ FE-3, không giả định backend message-id semantics đầy đủ.

##### Bước 8 — Wire router và thay placeholder
- Cập nhật `RootRouter`:
  - `AUTHORIZED/ADMIN_READY` -> `MainAppScreen`.
- Giữ state-based routing, không dùng `BrowserRouter`.

**Files chính:**
- `app/frontend/src/screens/RootRouter.tsx`
- `app/frontend/src/screens/MainAppScreen.tsx`

##### Bước 9 — Quality gates FE-3
- Build gate:
  - `cd app/frontend && npm run build` pass.
- Functional smoke checklist:
  1. Vào app với `AUTHORIZED` thấy sidebar + network status.
  2. Tạo nhóm mới thành công, auto-select nhóm.
  3. Gửi tin nhắn thành công, timeline cập nhật.
  4. Nhận event tin nhắn realtime không duplicate listener khi đổi màn/group.
  5. Khi send fail, hiện retry/remove đúng.
- Architecture gate:
  - `components/ui/*` không import `wailsjs`.
  - mọi event subscription đi qua `useWailsEvent`.
  - smart/dumb boundary giữ sạch.

**Định nghĩa hoàn thành FE-3:**
- Main chat shell usable cho demo core (create/open/send/receive).
- Network status luôn hiển thị và phản ánh trạng thái node.
- Không còn placeholder ở trạng thái `AUTHORIZED/ADMIN_READY`.
- Realtime update hoạt động ổn định, không leak listeners.
- Build pass và không có lint error ở phần file vừa chỉnh.

### Phase FE-4: Group & Invite UX

- Create group modal.
- Group info panel.
- Generate join code screen.
- Add member modal.
- Pending invites screen.
- Leave/remove member UX.

Done when:

- Basic membership flows can be completed or clearly marked as planned if backend method is missing.

### Phase FE-5: Settings, Migration, and Session UX

- Export backup screen.
- Import backup screen.
- Session replaced screen.
- Network/bootstrap settings.

Done when:

- User can safely export/import identity and understand session takeover.

### Phase FE-6: Admin UX

- Admin setup/unlock.
- Bundle issuance panel.
- Request JSON paste/parser.
- Issuance history screen if backend supports persistence.

Done when:

- Admin can create bundle from user request without manual copy mistakes.

### Phase FE-7: Developer Mode & Diagnostics

- Add Developer Mode toggle.
- Add diagnostic overlays.
- Add log export/copy diagnostics UI.

Done when:

- Normal users see clean UI; technical demo can expose protocol state.

### Phase FE-8: File Transfer UI

- Implement planned UI once backend APIs exist.
- Add file cards, progress, speed, retry/cancel.

Done when:

- Secure file transfer is usable end-to-end.

---

## 8.1. Execution Order (Updated, Practical Rollout)

This sequence is the recommended implementation order to keep risk low and maintain demo progress.

1. FE-1 foundation first:
   - design tokens + Shadcn base components
   - app shell + state-based router
   - Zustand slices + event bridge with cleanup contract
2. FE-2 onboarding flow:
   - welcome + awaiting bundle + request export/import bundle UX
3. FE-3 core chat shell:
   - sidebar groups + network indicator + chat timeline/composer
4. FE-4 group/invite lifecycle:
   - pending invites + add member + leave/remove UX aligned with backend policy
5. FE-5 safety and recovery UX:
   - backup export/import + session replaced + network/bootstrap settings
6. FE-6 admin product UX:
   - admin setup/unlock + request parser + bundle issuance
7. FE-7 developer mode:
   - diagnostics and technical overlays
8. FE-8 file transfer UI:
   - only after backend APIs are confirmed production-ready

---

## 9. Backend/API Gaps To Confirm

Use `BACKEND_IMPLEMENTATION_PLAN.md` as the source of truth for backend priority, API shape, and done criteria before wiring production UI screens.

During implementation, verify whether these Wails methods already exist. If not, add backend methods before wiring final UI:

- startup progress events
- open log folder
- drag/drop import for `.bundle` by file path or content
- export `request.json`
- group display metadata separate from `groupID`
- generate join code / KeyPackage as user-facing DTO
- list pending invites
- reject pending invite
- leave group
- remove member
- retry failed message
- session replaced event/state
- network/bootstrap settings
- admin root key unlock as explicit runtime method
- issuance history persistence/listing
- diagnostics snapshot/export logs
- file transfer APIs

Do not fake completed functionality in the UI. If a backend gap exists, either implement it or mark the control as planned/disabled with clear copy.

---

## 10. Quality Checklist

Before considering the frontend rebuild complete:

- `npm run build` passes in `app/frontend`.
- Wails bindings are current after backend API changes.
- Normal UI hides advanced cryptographic terms.
- All copy/download actions give feedback.
- All destructive actions require confirmation.
- Onboarding never asks user for display name.
- Admin bundle issuance requires Admin-controlled display name.
- Network status is always visible in main app.
- Message UI never claims read receipts.
- Backup/import flows clearly warn about session takeover.
- Developer Mode is off by default.
- No private keys, passphrases, or secrets are logged or shown.

