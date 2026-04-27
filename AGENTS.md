# AI Agent Guidelines — Decentralized Coordination Protocol for MLS on P2P Networks

This document outlines the core principles, mandates, and operational guidelines for AI Agents working on this project. Adherence to these guidelines is crucial for maintaining project integrity, security, and consistency.

**Project Nature:** This is a Graduation Thesis with dual objectives — (1) Research: design a Decentralized Coordination Protocol wrapping MLS (RFC 9420) for P2P environments, and (2) Application: build a serverless zero-trust communication platform implementing the protocol.

## Core Mandates

1. **Conventions:** Rigorously adhere to existing project conventions (formatting, naming, style, structure, framework choices, typing, architectural patterns). Analyze surrounding code, tests, and configuration first.
2. **Libraries/Frameworks:** NEVER assume availability. Verify established usage within the project before employing.
3. **Idiomatic Changes:** Understand local context to ensure natural and idiomatic integration.
4. **Comments:** Add comments sparingly, focusing on *why* something is done for complex logic. Do not edit comments separate from your code changes. NEVER communicate with the user via comments.
5. **Proactiveness:** Fulfill requests thoroughly, including adding tests for new features/fixes. Consider all created files permanent.
6. **Confirm Ambiguity/Expansion:** Do not take significant actions beyond clear scope without user confirmation. If a change is implied, ask for confirmation first. If asked *how* to do something, explain first.
7. **Do Not Revert Changes:** Only revert changes if specifically asked by the user, or if your own changes caused an error.

## Operational Guidelines

1. **Understand Requirements:** Analyze the user's request, the `PROJECT_PLAN.md`, and `README.md` thoroughly.
2. **Plan:** Build a coherent plan based on understanding. For complex tasks, break them down into subtasks and use the `write_todos` tool to track progress. Share concise plans with the user when helpful.
3. **Implement:** Use available tools (e.g., `replace`, `write_file`, `run_shell_command`) strictly adhering to project conventions.
4. **Verify (Tests):** If applicable, verify changes using project's testing procedures. NEVER assume standard test commands. Prefer "run once" or "CI" modes.
5. **Verify (Standards):** After code changes, execute project-specific build, linting, and type-checking commands (e.g., `tsc`, `npm run lint`, `go vet`, `cargo check`).
6. **Finalize:** After all verification passes, the task is complete.

## Security and Safety Rules

1. **Explain Critical Commands:** Before executing commands with `run_shell_command` that modify the file system or system state, provide a brief explanation of the command's purpose and potential impact.
2. **Security First:** Always apply security best practices. Never introduce code that exposes, logs, or commits secrets, API keys, or other sensitive information.
3. **Sidecar Pattern:** The Rust binary MUST NOT be started manually. The Go app MUST spawn it using `os/exec` and pass the listening port via CLI flag (e.g., `--port 12345`).
4. **Stateless Rust:** The Rust engine MUST NOT store state permanently. Go retrieves state from SQLite -> Sends to Rust -> Rust computes -> Returns new state -> Go saves to SQLite.
5. **Strict Onboarding:** No node can join the Gossip network without a valid `InvitationToken` signed by the Root Admin Key.
6. **Single Active Device:** A user account is valid on only ONE device at a time.
7. **Manual Identity Migration:** Private Keys are NEVER sent over the network (even encrypted). They must be exported to a file (`.backup`) encrypted with a Passphrase and manually transferred.
8. **Offline Handling:** Messages to offline peers must use encrypted local envelope retention and authenticated direct stream synchronization. The app also supports blind-store replication on `/org/offline-store/v1`: regular nodes retain targeted `k`-nearest replicas by default, while `--store-node` nodes retain all blind-store objects. Kademlia DHT is reserved for discovery/routing and replica target selection, not application mailbox storage.

## Coordination Protocol Rules (CRITICAL for Phase 4+)

9. **Single-Writer Invariant:** At any given epoch, ONLY the deterministically elected Token Holder may issue a Commit. All other nodes MUST route Proposals through GossipSub and wait for the Token Holder's Commit. NEVER allow two nodes to Commit for the same epoch.
10. **Epoch Monotonicity:** A node MUST NOT process any MLS Commit or Proposal with an epoch number lower than its current epoch. Stale messages must be rejected with `CurrentEpochNotification`.
11. **Two-Tier Separation:** The Coordination Layer (Go: `app/coordination/`) handles ordering, election, and fork healing. The Crypto Layer (Rust) handles pure MLS operations. Rust has NO knowledge of Single-Writer, epochs, or ActiveView — it only performs stateless MLS computations.
12. **Fork Healing — Non-repudiation:** During Autonomous Replay after fork healing, a node MUST only re-encrypt and resend its own messages. It MUST NOT resend messages authored by other nodes.
13. **Forward Secrecy on Heal:** When a node joins the winning branch via External Join, all keys from the losing branch MUST be destroyed (crypto-shredding). No old state may be retained.

## Project-Specific References

* **`PROJECT_PLAN.md`**: Detailed execution roadmap. ALWAYS refer to this for task specifics and phasing.
* **`README.md`**: High-level project overview, architecture, protocol design, and critical rules.
* **`CURRENT_STATE.md`**: Short-term memory — current progress, technical decisions, and implementation details. **Trước khi đụng Go/Wails:** đọc mục *Agent — Bản đồ mã nguồn & Wails* (`adapter/*`, `service.Runtime`, import TS `app/frontend/wailsjs/go/service/Runtime`).

## Go layout note (hexagonal)

* **Coordination protocol** remains in `app/coordination/` (ordering, MLS *interfaces* — no Rust binary there).
* **MLS gRPC implementation** lives in `app/adapter/sidecar/` (`NewMLSEngine`, process lifecycle).
* **Composition root** is `app/main.go` (`config`, `cli`, `wailsui`).

## Frontend Rules (React + Wails) — REQUIRED

1. **Architecture Boundary:** Frontend is a thin UI layer. Security, identity, coordination, persistence, and protocol truth live in Go/Rust/SQLite. Do not re-implement backend decisions in UI.
2. **Feature-First Structure:** Place new UI flows under `app/frontend/src/features/<feature>/...` (`screens`, `hooks`, optional `components`). Keep `app/`, `services/`, `stores/`, and `components/` responsibilities separated.
3. **Compatibility Policy:** During refactor, keep legacy wrappers in `src/screens/*` only as temporary adapters. New development must target `features/*`.
4. **Wails Access Rule:** Do not call generated bindings directly from many places. Go through `app/frontend/src/services/runtime/runtimeClient.ts` (or feature service wrappers) for API calls.
5. **State Strategy (Zustand):** Use Zustand slices for shared runtime state. Keep stores focused on state transitions; do not bury backend orchestration logic inside store definitions.
6. **Screen Orchestration Pattern:** For complex screens, split logic into hooks:
   - `use<Feature>Runtime` for loading/sync,
   - `use<Feature>Events` for event subscriptions,
   - `use<Feature>Actions` for user-triggered mutations.
7. **Event Lifecycle Safety:** Every Wails event subscription must clean up via the `unsubscribe` returned by `EventsOn`. Do not use broad/global event-off patterns that can remove listeners owned by other modules.
8. **Smart vs Dumb Components:** `features/*/screens` and feature hooks are smart layers. `components/*` must stay presentational (props in, UI out), with no direct Wails binding calls.
9. **Desktop Routing Rule:** Prefer app-state-driven routing (or `MemoryRouter` only when needed). Do not use `BrowserRouter` assumptions for Wails desktop flows.
10. **UI System Consistency:** Use Tailwind + Shadcn primitives + local UI tokens. Reuse shared `components/ui/*` primitives before creating one-off controls.
11. **Type Safety & Mappers:** Keep strict TypeScript typing. Convert backend DTOs to view models in `lib/` or feature mappers before deep UI usage.
12. **Testing & Verification (Frontend):** After substantive frontend changes, run at least type/lint/build checks from `app/frontend` (`npm run build`; add lint/typecheck commands when available). Fix introduced issues before finalizing.
13. **No Dead Code Drift:** Remove unused placeholders/components after migration phases complete. Keep `ARCHITECTURE.md` updated whenever folder strategy or dependency direction changes.

---
**Note:** This document is a distillation of the primary agent instructions and project-specific rules. In case of conflict, the original system instructions take precedence, followed by the specific rules outlined in `README.md` and `PROJECT_PLAN.md`.
