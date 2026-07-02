# Frontend (React + Wails)

> Xem thêm: [Index](README.md) · [Architecture Overview](architecture-overview.md) · [Service Layer](service-layer.md)

**Vị trí:** `app/frontend/src/`  
**Vai trò:** Thin UI layer — security, identity, coordination, persistence, và protocol truth live in Go/Rust/SQLite. Frontend chỉ display và user interaction.

## Tech Stack

- **React 18** + **TypeScript** (strict mode)
- **Wails v2** — Go desktop framework, generates TS bindings từ Go struct
- **Tailwind CSS** + **Shadcn/UI** primitives (dark theme)
- **Zustand** cho state management (lightweight, no boilerplate)
- **Vite** build tool (fast HMR, tree-shaking)
- **Lucide React** icons

## Cấu trúc chi tiết

```
src/
├── main.tsx                         # Entry — dark mode, StrictMode
├── App.tsx                          # Wraps AppRoot
├── app/
│   └── AppRoot.tsx                  # RootRouterScreen + Toaster
│
├── features/                        # Feature-first modules
│   ├── chat/                        # Chat UI (main feature)
│   │   ├── components/              # Chat-specific modals (2 files)
│   │   │   ├── AddMemberModal.tsx   # Add member dialog
│   │   │   └── CreateGroupModal.tsx # Create group dialog
│   │   ├── hooks/
│   │   │   ├── chatTypes.ts         # Event payload interfaces
│   │   │   ├── useChatRuntime.ts    # Core chat state: groups, messages, network
│   │   │   ├── useChatEvents.ts     # Wails event subscriptions for chat
│   │   │   ├── useChatActions.ts    # User actions: send, create, retry
│   │   │   ├── useChannelCategories.ts  # Category CRUD + sync
│   │   │   └── useMentions.tsx      # Mention candidates + rendering
│   │   ├── lib/
│   │   │   ├── timelineState.ts     # Message reconciliation, unread anchors
│   │   │   └── replayBlocked.ts     # Detect silent replay blocking
│   │   └── screens/
│   │       └── MainChatModuleScreen.tsx  # Main chat screen orchestrator
│   │
│   ├── onboarding/                  # Onboarding flow
│   │   └── screens/
│   │       ├── WelcomeScreen.tsx        # Initial — generate keys or import backup
│   │       ├── AwaitingBundleScreen.tsx # Show PeerID, wait for admin bundle
│   │       └── ImportBackupScreen.tsx   # Import .backup file
│   │
│   ├── runtime/                     # Runtime router
│   │   └── screens/
│   │       └── RootRouterScreen.tsx     # App state router (loading/error/onboarding/chat)
│   │
│   ├── admin/                       # Admin UI
│   │   └── screens/
│   │       └── AdminPanelScreen.tsx     # Admin key, bundle creation, invite history
│   │
│   ├── settings/                    # Settings UI
│   │   └── screens/
│   │       └── SettingsScreen.tsx       # Network, profile, diagnostics
│   │
│   └── activity/                    # Activity feed
│       └── screens/
│           └── ActivityScreen.tsx       # Notifications list
│
├── stores/                          # Zustand stores (8)
│   ├── useChatStore.ts              # Chat messages, posts, comments, active group
│   ├── useAppRuntimeStore.ts        # App state, startup stage, errors
│   ├── useGroupsStore.ts            # Group list, group metadata
│   ├── useNetworkStore.ts           # Network status, connected peers, local peer ID
│   ├── useContactStore.ts           # Contact display names, online status
│   ├── useNotificationStore.ts      # Notifications list, unread count
│   ├── useMessageLimitsStore.ts     # Channel message length limits
│   └── useToastStore.ts             # Toast notification queue (auto-dismiss)
│
├── services/
│   └── runtime/
│       └── runtimeClient.ts         # Centralized Wails binding wrapper (~80 methods)
│
├── components/                      # Shared UI components
│   ├── ui/                          # Shadcn primitives (8 files)
│   │   ├── button.tsx
│   │   ├── card.tsx
│   │   ├── dialog.tsx
│   │   ├── input.tsx
│   │   ├── label.tsx
│   │   ├── separator.tsx
│   │   ├── skeleton.tsx
│   │   └── toaster.tsx
│   ├── chat/                        # Chat-specific shared (7 files + posts/)
│   │   ├── ChatView.tsx         # Message list + input
│   │   ├── MessageList.tsx      # Message list rendering
│   │   ├── MessageComposer.tsx  # Message input
│   │   ├── RoomPanel.tsx        # Channel post/comment view (71KB, largest)
│   │   ├── PostView.tsx         # Post detail view
│   │   ├── ChatListAvatar.tsx   # Chat avatar component
│   │   ├── FileAttachmentCard.tsx # File attachment display
│   │   └── posts/               # Post components (5 files)
│   │       ├── PostCard.tsx
│   │       ├── PostComposerCard.tsx
│   │       ├── CommentList.tsx
│   │       ├── CommentComposer.tsx
│   │       └── MentionTextarea.tsx
│   ├── layout/                      # Layout components (4 files)
│   │   ├── AppShell.tsx             # Full-screen container wrapper
│   │   ├── MainSidebar.tsx          # Group list, channels, DMs (23KB)
│   │   ├── PrimaryRail.tsx          # Vertical nav rail (Admin/Chats/Settings)
│   │   └── WorkspaceRail.tsx        # Module nav with notification badges
│   ├── onboarding/                  # Onboarding shared components
│   ├── backup/                      # Backup components
│   ├── network/                     # Network status indicator
│   └── welcome/                     # Welcome screen components
│
├── lib/                             # Utility libraries (7 files)
│   ├── utils.ts                     # cn() — Tailwind class merge (clsx + tailwind-merge)
│   ├── textLimits.ts                # countRunes() — Unicode code point count (align Go)
│   ├── networkModel.ts              # Map backend node status → frontend connection state
│   ├── chatModel.ts                 # Chat data transforms: message conversion, mentions, formatting
│   ├── avatarImage.ts               # Avatar validation: max size, mime types, dimensions
│   ├── formatSendError.ts           # Map backend send errors → user-friendly toast messages
│   └── formatRemoveMemberError.ts   # Map backend remove member errors → toast messages
│
├── hooks/                           # Shared hooks (2 files)
│   ├── useRuntimeEventStream.ts     # Durable event polling + gap detection
│   └── useWailsEvent.ts             # Typed Wails event subscription with cleanup
│
└── screens/                         # Legacy screen adapters (5 files)
    ├── AwaitingBundleScreen.tsx     # Wraps onboarding AwaitingBundleView
    ├── ImportBackupScreen.tsx       # Wraps backup ImportBackupView
    ├── MainAppScreen.tsx            # Wraps chat MainChatModuleScreen
    ├── RootRouter.tsx               # Wraps runtime RootRouterScreen
    └── WelcomeScreen.tsx            # Wraps onboarding WelcomeView
```

## Architecture Rules

### 1. Wails Access Rule
Tất cả Wails binding calls đi qua `services/runtime/runtimeClient.ts` — không gọi generated bindings trực tiếp từ components/hooks.

```typescript
// ❌ Bad — direct binding call
import { GetGroups } from '@/wailsjs/go/service/Runtime'
const groups = await GetGroups()

// ✅ Good — via runtimeClient
import { runtime } from '@/services/runtime/runtimeClient'
const groups = await runtime.getGroups()
```

### 2. Smart vs Dumb Components
- **Smart:** `features/*/screens` và feature hooks — gọi `runtimeClient`, mutate stores, orchestrate flows
- **Dumb:** `components/*` — props in, UI out, không gọi Wails bindings

### 3. State Strategy (Zustand)
Zustand slices cho shared state. Stores tập trung vào state transitions, không bury backend orchestration logic.

```typescript
// Store = state + simple actions
interface ChatStore {
  messages: Message[]
  activeGroupID: string | null
  setMessages: (msgs: Message[]) => void
  addMessage: (msg: Message) => void
  // NO backend calls here — hooks do that
}
```

### 4. Screen Orchestration Pattern
Complex screens split thành 3 hook types:

| Hook | Responsibility |
|------|----------------|
| `use<Feature>Runtime` | Load/sync state from backend |
| `use<Feature>Events` | Subscribe to Wails events |
| `use<Feature>Actions` | User-triggered mutations |

### 5. Event Lifecycle Safety
Mỗi Wails event subscription phải cleanup via `unsubscribe`:

```typescript
useEffect(() => {
  const unsub = EventsOn('chat:message', handler)
  return () => { unsub() }  // Cleanup on unmount
}, [])
```

### 6. Desktop Routing
App-state-driven routing — không dùng `BrowserRouter`. `RootRouterScreen` renders dựa trên `appState` từ `useAppRuntimeStore`.

## runtimeClient.ts

Centralized API wrapper — re-exports tất cả Wails-generated bindings (~80+ methods), wrap trong typed functions:

| Nhóm | Methods |
|------|---------|
| **Identity** | `generateKeys`, `getOnboardingInfo`, `openAndImportBundle`, `exportIdentity`, `importIdentityFromFile`, `exportDeviceRequestJSON` |
| **Group** | `createGroupChat`, `getGroups`, `getGroupMessages`, `sendGroupMessage`, `leaveGroup`, `addMemberToGroup`, `removeMemberFromGroup`, `startDirectMessage`, `getGroupMembers` |
| **Messaging** | `sendGroupMessageWithLocalEchoToken`, `retryMessage`, `deleteLocalMessage`, `getGroupPosts`, `getPostComments` |
| **Admin** | `initAdminKey`, `verifyAdminPassphrase`, `createBundle`, `createBundleFromRequest`, `getAdminStatus`, `listIssuanceHistory` |
| **Network** | `getNodeStatus`, `getNetworkSettings`, `validateMultiaddr`, `setBootstrapAddress`, `reconnectP2P` |
| **Session** | `getSessionStatus` |
| **Profile** | `getMyProfile`, `saveMyProfile`, `updateMyProfile`, `getPeerProfile`, `applySignedPeerProfile` |
| **File transfer** | `prepareGroupFile`, `prepareOutgoingFileTransfer`, `sendGroupFile`, `downloadGroupFile`, `openDownloadedFile` |
| **Invite** | `invitePeerToGroup`, `requestGroupInvite`, `generateJoinCode`, `listGroupInviteRequests`, `approveGroupInviteRequest`, `rejectGroupInviteRequest` |
| **Diagnostics** | `getDiagnosticsSnapshot`, `exportDiagnostics`, `getForkHealHistory`, `getRuntimeHealth`, `getRuntimeEventsSince`, `getAppState` |
| **Offline** | `getOfflineSyncStatus`, `triggerOfflineSync` |
| **Channel** | `listChannelCategories`, `createChannelCategory`, `removeChannelCategory` |
| **Notifications** | `getNotifications`, `getUnreadNotificationCount`, `markNotificationRead`, `markAllNotificationsRead` |

## Zustand Stores

### `useChatStore.ts`
Chat messages, posts, comments, active group selection. Optimistic UI support — messages có status `pending`, `published`, `failed`.

### `useAppRuntimeStore.ts`
App state machine: `loading`, `error`, `uninitialized`, `awaiting_bundle`, `authorized`, `admin_ready`. Startup stage tracking.

### `useGroupsStore.ts`
Group list with metadata (groupID, type, name, member count, epoch). Category organization.

### `useNetworkStore.ts`
Network connection state: `disconnected`, `connecting`, `connected`. Connected peers list, local peer ID.

### `useContactStore.ts`
Contact display names + online status. `getDisplayName(peerID)` with fallback to shortened PeerID.

### `useNotificationStore.ts`
Notifications list, unread count, loading state. Async fetch, mark read, mark all read, add new (dedup by ID).

### `useMessageLimitsStore.ts`
Message length limits from backend (DM, channel title, channel body, comment). Falls back to defaults on failure.

### `useToastStore.ts`
Toast queue with auto-dismiss (6.5s). Variants: `default`, `destructive`. Push + dismiss functions.

## Key Hooks

### `useRuntimeEventStream.ts`
Durable event polling — fetch runtime events from backend, manage cursor via localStorage:

```
1. Read cursor from localStorage ('runtime:lastSeq')
2. Poll GetRuntimeEventsSince(cursor, limit)
3. For each event: call onEvent callback
4. Update cursor
5. Detect gaps (missing seq) → trigger refreshState
6. Also listen to 'runtime:event_available' Wails event for immediate poll
```

### `useWailsEvent.ts`
Typed Wails event subscription with automatic cleanup:

```typescript
function useWailsEvent<T>(eventName: string, handler: (data: T) => void) {
  useEffect(() => {
    const unsub = EventsOn(eventName, handler)
    return () => { unsub() }
  }, [eventName, handler])
}
```

### `useChatRuntime.ts`
Core chat state management:
- Fetch node status → update `useNetworkStore`
- Fetch groups → update `useGroupsStore`
- Select active group → load messages → update `useChatStore`
- Update contacts from group members → `useContactStore`

### `useChatEvents.ts`
Subscribe to chat-related Wails events:
- `chat:message` → add to chat store
- `chat:message_sent` → update status to `published`
- `chat:message_failed` → update status to `failed`
- `group:epoch` → refresh group info
- `group:left` → remove group, show toast
- `group:member_added` / `group:member_removed` → refresh members
- `group:replay_blocked` → handle stale epoch (silent or error)
- `notification:new` → add to notification store

### `useChatActions.ts`
User-triggered chat actions:
- `selectGroup(groupID)` — switch active group, load messages
- `createGroup(name, type, categoryID)` — create group via `runtime.createGroupChat`
- `sendMessage(text)` — optimistic send via `runtime.sendGroupMessageWithLocalEchoToken`
- `sendFile(filePath)` — file transfer via `runtime.sendGroupFile`
- `retryMessage(messageID)` — retry failed message
- `deleteMessage(messageID)` — delete local message
- Error handling → toast notifications via `formatSendError`

## Lib Utilities

### `chatModel.ts` (371 lines)
Chat data transformations:
- `detectConversationKind` — classify group as DM, channel, or group
- `convertMessage` — map backend message DTO to frontend view model
- `formatTimestamp` — relative time formatting
- `parseMessageContent` — parse structured payloads (posts, comments, files)
- `extractMentions` / `renderMentions` — mention extraction and highlighted rendering
- `formatFileSize` / `formatFileType` — file metadata formatting

### `timelineState.ts` (184 lines)
Message timeline management:
- `compareTimelines` — diff old vs new message lists
- `mergeTimelines` — merge optimistic + canonical messages
- `reconcileOptimistic` — replace optimistic messages with canonical (match by localEchoToken)
- `computeUnreadAnchor` — determine scroll position for unread messages

### `networkModel.ts`
Map backend `NodeStatus` to frontend connection states:
- `node_online` → `connected`
- `node_starting` → `connecting`
- `node_offline` → `disconnected`
- User-friendly labels for display

### `textLimits.ts`
`countRunes(str)` — count Unicode code points, aligned với Go's `utf8.RuneCountInString`. JavaScript's `.length` counts UTF-16 code units, không chính xác cho emoji/CJK.

### `avatarImage.ts`
Client-side avatar validation:
- Max file size: 256 KiB
- Allowed MIME types: `image/png`, `image/jpeg`, `image/webp`
- Max dimensions: 512x512px
- Auto-resize if needed

### `formatSendError.ts` / `formatRemoveMemberError.ts`
Map backend error codes to user-friendly toast messages:
- `ERR_TEXT_TOO_LONG` → "Tin nhắn quá dài. Giới hạn: {limit} ký tự"
- `ERR_EMPTY_MESSAGE` → "Tin nhắn không được để trống"
- `ERR_FILE_TOO_LARGE` → "File quá lớn. Giới hạn: {limit}"
- `ERR_PERMISSION_DENIED` → "Bạn không có quyền thực hiện thao tác này"
- `ERR_CANNOT_REMOVE_CREATOR` → "Không thể xóa người tạo nhóm"

## Config

### `package.json`
```json
{
  "dependencies": {
    "react": "^18", "react-dom": "^18",
    "zustand": "^4",
    "lucide-react": "^0.3xx",
    "clsx": "^2", "tailwind-merge": "^2"
  },
  "devDependencies": {
    "vite": "^5", "@vitejs/plugin-react": "^4",
    "typescript": "^5",
    "tailwindcss": "^3", "autoprefixer": "^10"
  }
}
```

### `tsconfig.json`
- Target: ES2020, JSX: react-jsx
- Strict mode: true
- Module resolution: bundler
- Path alias: `@/*` → `./src/*`

### `vite.config.ts`
- React plugin
- Path alias: `@` → `/src`
- Build output: `dist/`
- `emptyOutDir: true`

## Wails Bindings

Generated files trong `frontend/wailsjs/`:
- `go/models.ts` — TypeScript types cho tất cả Go structs (27000+ lines)
- `go/service/Runtime.d.ts` — Type declarations cho Runtime methods (7400+ lines)
- `go/service/Runtime.js` — JavaScript implementations (11400+ lines)
- `runtime/` — Wails runtime (EventsOn, EventsOff, etc.)

Wails auto-generates these files khi `wails dev` hoặc `wails build` chạy. **Không edit manually.**
