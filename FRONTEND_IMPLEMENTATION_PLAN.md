# FRONTEND IMPLEMENTATION PLAN (Condensed)

Project: Zero-Trust P2P Internal Secure Communication App  
Audience: AI Coding Agents and Frontend Developers  
Purpose: Keep only implementation-critical context for day-to-day execution.

---

## 1) Non-Negotiable Product Context

This app is a Wails desktop client for a Zero-Trust, Local-First, P2P secure communication system.

### Security and domain constraints

- No central server assumptions.
- No Web2 login patterns (`email/password`, OAuth, SSO web redirects).
- Identity is locally generated (`PeerID` + MLS key), then authorized by admin via signed `.bundle`.
- Private key material is never sent over the network; migration is manual via encrypted `.backup`.
- Backend (Go/Rust/SQLite) is source of truth for security, coordination, persistence.

### Frontend boundary (thin UI only)

- Frontend renders state and triggers actions.
- Do not re-implement protocol/coordination logic in React.
- Do not infer protocol truth from UI heuristics.

---

## 2) Mandatory Frontend Rules (Agent Guardrails)

### Runtime and data access

- Data must come from Wails IPC bindings (Go runtime), not REST.
- Do not use `fetch`, `axios`, or custom HTTP APIs for core app data.
- Route runtime calls through `app/frontend/src/services/runtime/runtimeClient.ts`.

### Architecture and state

- Feature-first structure under `app/frontend/src/features/<feature>/`.
- Keep `components/*` presentational (props in, UI out).
- Use Zustand for shared runtime state; avoid heavy orchestration inside UI components.
- Prefer state-driven routing (or `MemoryRouter` when needed). Do not use `BrowserRouter`.

### Events and lifecycle

- Every Wails event subscription must clean up via returned `unsubscribe`.
- Never use broad global event removal that may affect other modules.

### Tech stack consistency

- React 18 + TypeScript + Tailwind + shadcn/ui patterns + `lucide-react`.
- Dark mode only.

---

## 3) UI Design Contract (Short Form)

### Visual direction

- Minimal, security-focused dark UI.
- Primary colors:
  - Background/surfaces: zinc/slate dark tones.
  - Secure/connected accent: emerald.
  - Warning: amber.
  - Critical actions: rose/red.
- Monospace only for technical identifiers (`PeerID`, hashes, keys, diagnostics).

### Desktop shell layout (3 columns)

- Left: global navigation (`Chats`, `Lời mời`, `Quản trị` if admin, `Cài đặt`).
- Center: context-dependent content (chat or invite flows).
- Right: collapsible group info panel.

### Product semantics

- Use P2P-safe message states (encrypted/published/offline stored/failed).
- Do not add Web2 read receipts unless backend explicitly supports them.

---

## 4) Information Architecture and Minimal Screen Set

Keep this section as a compact map, not full pixel spec.

### Core app states

- `STARTING` -> splash/progress.
- `FATAL_ERROR` -> blocking startup error with retry/restart CTA.
- `UNINITIALIZED` -> welcome/create local identity.
- `AWAITING_BUNDLE` -> QR/request view + import `.bundle`.
- `AUTHORIZED` / `ADMIN_READY` -> main shell.
- `SESSION_REPLACED` -> lockout screen (full-screen blocking state).

### Essential flows only

1. Startup and onboarding:
   - Splash.
   - Welcome (no manual display-name input).
   - Awaiting bundle (QR + copy raw request + import bundle).
2. Main collaboration:
   - Group list + chat panel + group info sidebar.
   - Invite list with accept/reject and create join code action.
3. Group operations:
   - Create group.
   - Add member (from network or invite code).
   - Leave group / member removal (role-based).
4. Security and settings:
   - Export identity backup (clear takeover warning).
   - Network/bootstrap settings.
   - Session replaced lockout.
5. Admin operations:
   - Unlock admin key.
   - Issue signed bundle from request payload.

Note: Detailed per-screen micro-spec is intentionally excluded from this file.

---

## 5) Developer Mode (Required Behavior)

Global toggle: `isDevMode`.

When enabled:

- Show MLS diagnostics (epoch, tree hash) in group context.
- Show raw `PeerID` and key identifiers where useful.
- Show technical event traces otherwise hidden in user mode.
- Keep default user mode clean and non-technical.

---

## 6) Frontend Structure (Current Target)

```text
app/frontend/src/
  features/
    admin/
    chat/
    invites/
    settings/
  components/
    ui/
    layout/
  services/runtime/
  stores/
  lib/
```

Rules:

- New behavior goes into `features/*` first.
- Legacy wrappers in `src/screens/*` are temporary adapters only.
- DTO -> ViewModel mapping should live in `lib/` or feature mappers.

---

## 7) Implementation Phases (Execution-Focused)

### Phase FE-1: Shell and runtime foundation

- Build/clean `DesktopShell`, left rail, main area, right sidebar container.
- Establish runtime client wrappers and baseline Zustand slices.
- Wire startup/app-state routing skeleton.

### Phase FE-2: Onboarding and authorization

- Implement welcome, awaiting bundle, import bundle flow.
- Ensure copy/QR/request UX is usable and clear.
- Add fatal startup error handling with deterministic fallbacks.

### Phase FE-3: Core chat and groups

- Implement group list, chat view, composer, system messages.
- Add message status rendering aligned with backend event model.
- Add group info sidebar with member presence and role-gated actions.
- **Limits UX:** DM/channel composers use `GetMessageLimits()` (Unicode rune count after trim), soft warnings near the cap, disabled submit when over, and toasts mapped from backend error codes (`TEXT_TOO_LONG`, etc.). Over-limit copy defers very long content to **Phase 8** encrypted file transfer (see `README.md`, Section 3.2.1).

### Phase FE-4: Invite and membership operations

- Pending invites list and accept/reject actions.
- Generate join code and add-member modal with two entry modes.

### Phase FE-5: Settings and security

- Identity export modal (passphrase + warnings).
- Network/bootstrap settings.
- Session replaced lockout UI.

### Phase FE-6: Admin workflows

- Admin unlock screen.
- Bundle issuance flow from request payload.
- Validate mandatory fields and result UX.

### Phase FE-7: Developer mode and diagnostics

- Gate diagnostics via `isDevMode`.
- Add low-level panels without polluting normal UX.

### Phase FE-8: Hardening and cleanup

- Remove stale wrappers/placeholders.
- Improve empty/loading/error states across features.
- Final architecture/document sync.

---

## 8) Architectural Decisions (ADR-lite)

1. Frontend remains thin; backend owns security and protocol truth.
2. Zustand is the shared runtime state mechanism.
3. Wails event subscription must always be lifecycle-safe with cleanup.
4. Runtime calls are centralized through runtime service wrappers.
5. Feature-first folder strategy is canonical for new code.
6. Desktop routing avoids browser-history assumptions.
7. Default UX is non-technical; diagnostics are Dev Mode only.

---

## 9) Active Workboard (Fully Completed ✅)

The entire frontend application implementation, integration, and UI polishing are 100% completed.

### Completed Feature Phases
- [x] FE-1: Shell and runtime foundation.
- [x] FE-2: Onboarding and authorization.
- [x] FE-3: Core chat, groups, and message limit pre-flights.
- [x] FE-4: Invite list, add-member workflows, and auto-join notifications.
- [x] FE-5: Settings, profile saving, avatar upload, and passphrase-encrypted `.backup` export/import.
- [x] FE-6: High-security locked admin dashboard, request paste parser, display name sign, and bundle issuance history.
- [x] FE-7: MS Teams-like Activity panel and developer diagnostics views.
- [x] FE-8: Hardening, loading states, empty screen boundaries, and total code structure cleanup.

### Blockers / backend confirmations
- [x] All backend Wails bindings and integration contracts have been fully confirmed, generated, and verified as stable.

---

## 10) Done Criteria (Definition of Ready-to-Merge)

- Build succeeds in `app/frontend` (`npm run build`).
- No introduced lint/type errors in touched files.
- All event listeners in edited flows have explicit cleanup.
- No direct protocol logic duplicated in frontend.
- UI follows dark-theme + security semantics defined above.

---

## 11) Reference Documents

- `README.md`
- `PROJECT_PLAN.md`
- `CURRENT_STATE.md`
- `AGENTS.md`

If any detailed screen spec is needed later, create a separate short file per feature (for example `docs/frontend/chat-screen-notes.md`) instead of expanding this plan.
