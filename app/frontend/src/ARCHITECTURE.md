# Frontend Architecture (Scalable Baseline)

This frontend uses a modular, feature-first structure inspired by large React applications:

## Folder Strategy

- `app/`: application composition root (`AppRoot`) and future providers/bootstrap wiring.
- `features/`: business modules by domain (`runtime`, `chat`, `onboarding`, `invites`, `admin`, `settings`).
- `services/`: infrastructure adapters (Wails runtime client, future telemetry/storage adapters).
- `components/`: reusable presentational UI blocks (layout/chat/ui primitives).
- `stores/`: Zustand slices (runtime/network/groups/chat).
- `lib/`: pure utility/mapping functions.
- `screens/`: compatibility layer only; new screens must live under `features/*/screens`.

## Dependency Direction

Preferred dependency flow:

`app -> features -> components -> lib`

And cross-cutting adapters:

`features -> services`

Rules:

- Presentational components in `components/` must not call Wails bindings directly.
- Wails API calls should go through `services/runtime/runtimeClient.ts`.
- Stateful orchestration belongs in `features/*/screens` or dedicated feature hooks.
- Keep stores as thin shared state containers; avoid backend call logic inside stores.
- For complex screens, split orchestration into hooks:
  - `use<Feature>Runtime`: data loading/sync
  - `use<Feature>Events`: event subscriptions
  - `use<Feature>Actions`: user-triggered mutations

## Next Migration Steps

1. Implement real screens for `features/invites`, `features/admin`, `features/settings`.
2. Replace polling loops with event-first sync where backend events are available.
3. Add typed DTO mappers per feature (avoid passing raw backend DTOs deep into UI).
4. Remove legacy wrappers under `screens/` once all imports are migrated.
