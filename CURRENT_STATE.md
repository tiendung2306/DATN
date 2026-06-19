# Current State — Decentralized Coordination Protocol for MLS on P2P Networks

This document serves as a short-term memory for the AI Agent.

## 1. Project Overview

**Thesis with dual objectives:**
1. **Research (Core):** Design a Decentralized Coordination Protocol that wraps MLS (RFC 9420) for P2P environments — solving the open problem of maintaining causal consistency and total ordering without a central Delivery Service.
2. **Application:** Build a serverless, zero-trust P2P communication platform using Go (Wails) + Rust (OpenMLS) that implements the protocol.

**Four Core Mechanisms of the Coordination Protocol:**
- **Single-Writer Protocol:** Only one node (Epoch Token Holder) may Commit per epoch. Eliminates concurrent Commits entirely.
- **Epoch Consistency:** Every MLS operation carries an epoch number; stale/future operations are rejected or buffered.
- **Group Fork Healing:** Network partitions are detected via Gossip Heartbeat; losing branch performs External Join into winning branch.
- **Hybrid Logical Clock (HLC):** Causal consistency and total ordering for application messages without NTP synchronization.

## 2. Completed Tasks

### Latest Delta (2026-06-16) ✅ — Fork Detection Theory Direction Clarified for Future Code Work

- **Theory/documentation clarification completed:** Bất biến phát hiện fork trong Chương 4 không nên tiếp tục diễn đạt theo kiểu "cùng epoch nhưng khác `CommitHash`/`TreeHash`". Lập luận đó không đủ mạnh khi một nhánh đi trước nhanh hơn nhánh còn lại.
- **Chosen direction for future code alignment:**
  - Mỗi node nên lưu thêm một **rolling history summary** cho lịch sử Commit, ví dụ `R(0)=r0`, `R(e)=H(R(e-1) || CommitHash(e))`.
  - `StateSummary`/`GroupStateAnnouncement` vẫn nên giữ nhỏ và mang ít nhất `Epoch`, `TreeHash`, `CommitHash`, và `HistoryHash = R(E)`.
  - Khi peer remote ở epoch cao hơn, node local **không kết luận fork ngay**; local cần thêm một round trip để yêu cầu `R(localEpoch)` từ remote.
  - Nếu `remote.R(localEpoch) == local.R(localEpoch)` thì remote chỉ đang đi trước trên cùng nhánh.
  - Nếu hai giá trị khác nhau thì fork đã xuất hiện tại hoặc trước `localEpoch`.
- **Important design boundary:** Winner rule hiện tại **không đổi**. Detection dùng `HistoryHash` prefix comparison; selection vẫn dùng branch weight `(support/member count > epoch > commit hash/tree hash tiebreakers)` theo tài liệu hiện tại.
- **Docs synchronized:** Updated `README.md`, `PROJECT_PLAN.md`, `thesis_drafts/Chuong_3_De_xuat.md`, `thesis_drafts/Chuong_4_Phan_tich_ly_thuyet.md`, `thesis_drafts/paper_project/Chuong_4_Phan_tich_ly_thuyet.md`, và bản LaTeX của Chương 4 để phản ánh hướng này trước khi code được cập nhật.

### Latest Delta (2026-06-05) ✅ — Replay/Duplicate Hardening for Late-Join + Fork-Heal

- **Problem investigated:** Khi node join muộn hoặc sau fork-heal, máy local có thể thấy duplicate message của chính mình. Root cause không nằm ở render thuần túy mà ở semantics replay: mapping cũ chỉ nối `replayed -> previous copy`, không đảm bảo `replayed -> root original`.
- **Backend contract hardened:**
  - `service.MessageInfo` giữ `replayed_at` và thêm/chuẩn hóa `supersedes_message_id` với nghĩa **canonical original message ID**.
  - Realtime `group:message` và các API list (`GetGroupMessages`, `GetGroupPosts`, `GetPostComments`) dùng cùng mapper để trả replay metadata nhất quán.
  - Resolver replay ở `app/adapter/store/coordination_storage.go` giờ canonicalize replay chain qua `application_event (envelope_hash, replayed_envelope_hash)` để nhiều vòng replay (`A -> B -> C`) vẫn trả `C supersedes A`.
  - Nếu canonicalization lỗi, service log `slog.Warn` thay vì fail-open im lặng.
- **Frontend behavior hardened:**
  - Timeline không còn ẩn message chỉ vì `replayed_at`; chỉ ẩn khi có message khác trong cùng timeline mang `supersedesMessageId` trỏ tới nó.
  - Rule reconcile được dùng chung cho `messagesByGroup`, `postsByGroup`, `commentsByPost`.
  - `group:replay_blocked` có backward-compatible silent handling: event mới dùng `user_visible=false`; event cũ thiếu field này nhưng có `reason=stale_epoch_requires_recovery_snapshot` vẫn bị silent.
- **Files worth remembering for future replay bugs:**
  - Backend contract/mapping: `app/service/group.go`, `app/service/messaging.go`
  - Replay link resolution: `app/adapter/store/coordination_storage.go`
  - Frontend reconcile logic: `app/frontend/src/features/chat/lib/timelineState.ts`
  - Silent replay-blocked helper: `app/frontend/src/features/chat/lib/replayBlocked.ts`
- **Verification completed:**
  - Go targeted tests PASS for storage/service replay mapping and replay_blocked payload behavior.
  - Frontend `npm test` PASS and `npm run build` PASS.
  - Full manual `demo-control` 4-node replay scenario still recommended before high-stakes demo.

### Latest Delta (2026-06-02) ✅ — Locking Boundaries & Deadlock Elimination (Service Quality Hardening)

- **Deadlock & Locking Boundaries Audit & Fixes [COMPLETED]:**
  - **Read-Lock Optimization:** Converted multiple state getters and status queries in `app/service/` (`GetKPStatus`, `GetGroupStatus`, `makeMessageHandler`, `mapStoredMessagesToMessageInfo`) from exclusive `r.mu.Lock()` to read-only `r.mu.RLock()`, avoiding write-lock starvation and UI hangs during high P2P synchronization loads.
  - **Database Close Race Resolution:** Extended the locking scope of `r.mu.RLock()` in `GetOfflineSyncStatus()` using `defer` to encompass the entire function including database read operations. This guarantees connection safety and prevents panics/deadlocks on concurrent teardown.
  - **Asynchronous DB Isolation:** Added explicit error logging to async peer verification persistence `persistVerifiedPeerInfo(pid)` in `runtime.go` to ensure DB issues never block or silently fail Wails UI loops.
  - **Verification:** Successfully validated and ran all unit tests (`go test -v ./service/...` passes 100%). Stopped background processes cleanly and executed a successful `wails build` to produce the deadlock-free `SecureP2P.exe` binary.

### Latest Delta (2026-05-31) ✅ — Senior-Grade Crash-Safe Fork Healing State Machine (Milestone 5)


- **Crash-Safe Fork Healing State Machine [COMPLETED]:**
  - **Unified Job Identity:** Shifted `fork_healing_job` from using `group_id` as the primary key to `job_id TEXT PRIMARY KEY` with active job isolation via conditional unique index `idx_fork_healing_active_group` on group ID. This resolves the critical issue where successive splits/heals in the same group were blocked due to key collisions on completed/cleaned jobs.
  - **Multi-Fork Event Isolation:** Added `job_id` to `application_event` with a compound index `idx_application_event_job_status`. This keeps events associated with older completed/failed healing jobs fully isolated from new active jobs, preventing event leakage or incorrect replay matching during consecutive heals.
  - **Strict Branch Matcher Invariant:** Refactored the `isAlreadyOnWinningBranch` helper to perform precise Epoch + TreeHash matching instead of simple epoch-based comparison, completely preventing nodes from bypassing verification and joining invalid/junk branches.
  - **Lexicographical Phase Transition Fix:** Replaced fragile lexicographical string comparisons (`Status < "STATE_SWAPPED"`) with an explicit helper (`phaseBeforeStateSwapped`) that properly incorporates the `"FROZEN_FOR_APPLY"` phase, guaranteeing correct chronological resume ordering.
  - **Durability Sequence & Error Propagation:** Refactored `broadcastOutboundReplay` to append to the offline sync envelope log before publishing to GossipSub. Database save errors (`SaveOutboundReplay` and `SaveApplicationEvent`) are strictly propagated rather than swallowed, preventing message loss if a node crashes right before broadcasting.
  - **Offline State Recovery:** Hardened the `EXTERNAL_JOINED` phase in `runHeal` to restore the winning branch state directly from the `PendingGroupState` serialized payload in SQLite, enabling the state machine to resume and swap state offline even if the winner peer goes offline.
- **SQLite Integration Testing Hardening:**
  - Added intensive SQLite database integration tests: `TestSQLiteCoordinationStorage_ForkHealingJob_LifecycleAndUniqueConstraint`, `TestSQLiteCoordinationStorage_ApplicationEventsAndPayloadShredding`, `TestSQLiteCoordinationStorage_OutboundReplayQueue`, and `TestSQLiteCoordinationStorage_GetActiveForkHealingJob_ExcludesNewerCleaned`.
  - Hardened the `GetActiveForkHealingJob` query to filter out newer `CLEANED` history records and correctly prioritize the active job.
- **Validation:**
  - 100% test pass on all 12 coordinator unit tests and 24 SQLite integration tests.
  - Successful Go compilation `go build -o DATN.exe main.go` with zero errors.

### Latest Delta (2026-05-16) ✅ — Activity Tab & Offline Notification System (MS Teams style)

- **End-to-End Implementation:** Added a comprehensive notification system ("Hoạt động" tab) spanning backend persistence to enterprise-grade frontend UI.
- **Offline-First Architecture:** 
  - Notifications are generated at the **backend processing layer** (`messaging.go`, `invite.go`, `group_invite_request_p2p.go`).
  - When a node returns online and performs P2P synchronization, incoming messages/invites automatically trigger notification generation in SQLite.
- **Notification Types (6):**
  - `mention`: User tagged via `@Name`.
  - `reply`: Direct reply to a user's message/post/comment.
  - `group_add`: User joined a group/DM via `Welcome` (auto-join).
  - `invite_request`: (For Creator) A new member request is pending.
  - `invite_approved`: (For Requester) Request was approved by creator.
  - `invite_rejected`: (For Requester) Request was denied.
- **Technical Highlights:**
  - **Backend (Go):** Dedicated `notifications` table with deterministic SHA-256 IDs for **strict idempotency** (prevents duplicate alerts during P2P replays). 
  - **UTC Timestamps:** All notifications use UTC storage with relative time formatting ("5m ago") on the frontend to solve cross-node clock skew issues.
  - **Auto-Cleanup:** Maintenance loop automatically prunes notifications older than 30 days.
  - **Frontend (React/Zustand):** Global unread count badge in `WorkspaceRail`, real-time Toast alerts, and a professional `ActivityScreen` with date grouping and smart content previews (extracts plain text from JSON payloads).
- **Validation:** 3-node manual tests (partition/heal/tag) confirmed notifications arrive reliably after sync. Backend/Frontend build PASS.

### Latest Delta (2026-05-16) ✅ — Frontend UI Redesign (Admin & Settings screens)

- **Admin Panel Redesign:** 
  - Shifted from dev/test-first UI to a **professional enterprise-grade UI** utilizing Shadcn/UI primitives and Lucide icons.
  - **Security Gate:** Implemented a dedicated high-security lock screen with PIN/password entry.
  - **Admin Session TTL:** Integrated frontend support for the 15-minute backend unlock TTL. The UI automatically locks and notifies the user via Toast when the session expires.
  - **Issuance Workspace:** Added a tabbed interface switching between "Manual Issuance" and "File-based Import". 
  - **File Request Parser:** Enhanced file import with immediate visual feedback of extracted PeerID and MLS Public Key metadata.
  - **Issuance History:** Added a dedicated history section with formatted Vietnamese timestamps and identity details.
- **Settings Screen Redesign:**
  - **Simplified Navigation:** Refactored to a **tabbed interface** separating "Hồ sơ cá nhân" (Profile) and "Bảo mật & Sao lưu" (Security/Backup).
  - **Decluttered UI:** Removed developer-centric tools (Bootstrap override, Diagnostics) to focus on end-user tasks.
  - **Profile Management:** Modernized the avatar upload area with processing indicators and hover effects. Clearly marked "Display Name" as Admin-assigned (read-only) to align with protocol rules.
  - **Identity Backup:** Improved the backup flow with security info boxes and emphasized passphrase protection for Ed25519 private keys.
- **Feedback & UX Hardening:**
  - Standardized all success/error notifications using the global **Toast system** (`useToastStore`).
  - Added loading states (`isLoading`, `profileSaving`) for all mutation actions to prevent double-submits and improve responsiveness.
  - Applied subtle animations (fade-in, slide-in) for screen and tab transitions, enhancing the modern application feel.
- **Validation:** `npm run build` PASS, all backend integrations (Runtime client) verified and maintained.

### Latest Delta (2026-05-16) ✅ — Empirical Proofs via Chaos Testing & Real-Sidecar Forward Secrecy Validation

- **Chaos Testing (Protocol Correctness):**
  - Implemented `app/coordination/chaos_e2e_test.go` using a 5-node cluster.
  - Simulated continuous randomized network partitions (Nemesis) during concurrent messaging and membership changes.
  - **Results:** 100% convergence achieved (Epoch 23 reached). Proven Invariants: Single-Writer Safety, Fork Healing Convergence, and HLC Causal Ordering.
  - **Artifacts:** Raw metrics in `app/coordination/chaos_metrics.csv`; Professional convergence chart in `evaluation/convergence_chart.png` (generated via `evaluation/plot_chaos.py`).
- **Forward Secrecy (Cryptographic Integrity):**
  - Implemented `TestBusinessP1_E2E_RealSidecar_ForwardSecrecy` in `app/service/business_e2e_group_integrity_test.go`.
  - Used the **real Rust OpenMLS sidecar** to prove that a removed member cannot decrypt messages from future epochs.
  - **Results:** Verified that the decentralized coordination layer does not compromise MLS's core security properties.
- **Validation:**
  - `go test -v ./coordination` PASS (all 50+ tests).
  - `go test -v -tags=business_integration ./service -run ForwardSecrecy` PASS.


- **User report:** *"Vẫn không được nhé, cái cái nhóm vvvvvvvvvvvvvvvvvvvvvv tôi lấy node 2 tạo, node 1 mời node 3 thì không được? Xem db rồi debug thật kỹ cho tôi"*
- **DB inspection cho group `vvvv...`** (3 DB local: `app.db`/`dev-wails-sibling.db`/`dev-wails-peer3.db`):
  - NODE2 (sibling.db, Tester1) = **creator** (`my_role=creator`, `invite_policy=any_member`).
  - NODE1 (app.db, Admin) = member (`my_role=member`).
  - NODE3 (peer3.db, Tester2) = invitee mới, chưa join.
  - `group_invite_requests` ở NODE2: 2 records `failure_code=ERR_INVITE_ADD_MEMBER_FAILED`, `is_mirror=0` → creator nhận wire submit từ NODE1 OK, nhưng tự execute AddMember thất bại.
  - **`source_peer_id` đúng** (= creator/Tester1) ở mọi DB → fix trước (ngày 11/5) hoạt động chính xác. Đây là lớp lỗi MỚI, không phải regression cùng kiểu.
- **Root cause kiến trúc — KP không được forward qua wire:** Khi NODE1 (member) gọi `RequestGroupInvite("vvvv", Tester2)`:
  1. Forward `submit` đến NODE2 (creator) qua `/app/group-invite-request/1.0.0`.
  2. Wire frame `GroupInviteWireClientReqV1` cũ **chỉ chứa `target_peer_id`** — KHÔNG có KeyPackage.
  3. NODE2 auto-approve (`any_member`) → `processInviteRequest` → `r.InvitePeerToGroup(target=Tester2)` → `fetchPeerKeyPackage(Tester2)`.
  4. Direct fetch fail (NODE2 chưa verified với Tester2 / không còn live link) → cache miss → store-peer fallback fail nốt (NODE2 đi tìm khắp nơi, không ai có KP cached).
  5. → AddMember fail với `ERR_INVITE_ADD_MEMBER_FAILED`.
- **Tại sao đây là bug kiến trúc, không phải vá săm:** Topology cho thấy **requester là node có likelihood cao nhất sở hữu KP fresh của target** (vừa fetch lúc user click "Invite"). Bắt creator chạy lại discovery là race với network state — luôn có khả năng fail trong môi trường mDNS phân mảnh, late-join, NAT, v.v. Đính kèm KP vào wire submit biến luồng từ "creator phải tự khám phá" sang "requester chuyển tay KP" — deterministic, một-shot.
- **Fix triệt để (3 điểm chỉnh sửa, additive — backward compat):**
  - **Wire schema** (`app/adapter/p2p/group_invite_request_wire.go`): `GroupInviteWireClientReqV1` thêm field `TargetKeyPackage []byte json:"target_key_package,omitempty"` (omitempty → peer cũ không gửi vẫn parse OK; receiver không có field vẫn hoạt động).
  - **Requester side** (`app/service/group_invite_requests.go::RequestGroupInvite`): Trước khi forward `submit`, gọi `r.fetchPeerKeyPackage(targetID)` (đã sẵn có path direct → cache → store fallback). Đính kèm vào `TargetKeyPackage`. Failure tại đây non-fatal — log debug và vẫn forward (creator vẫn có thể fetch tự lập).
  - **Creator side** (`app/service/group_invite_request_p2p.go::rpcSubmitInviteRequest`): Sau `CreateGroupInviteRequest` thành công và TRƯỚC `processInviteRequest` (auto-approve), nếu `req.TargetKeyPackage` không rỗng thì gọi `database.SaveStoredKeyPackage(targetPeerID, req.TargetKeyPackage, requesterPeerID)`. Subsequent `fetchPeerKeyPackage` → direct fail → **cache hit** → AddMember chạy ngon. Source = requester để audit trail rõ ai đã chuyển KP.
- **Test E2E mới (`app/service/business_e2e_group_integrity_test.go`, 2 test pass):**
  - **BI-119** `TestBusinessP1_E2E_BI119_CreatorMissingKP_RequesterAttachesKP_AutoApproveSucceeds`: Reproduce production bug exactly.
    - Setup: `e2eAliceBobCharlie` 3-node, Alice creator `any_member`.
    - Bug condition: `DELETE FROM stored_keypackages WHERE peer_id = charlie` trên Alice → creator không có KP target.
    - Lấy KP thật của Charlie từ `charlie.db.GetKPBundle`, seed vào Bob (`bobDB.SaveStoredKeyPackage`) — mô phỏng requester đã fetch trước khi forward.
    - Bob gửi wire submit kèm `TargetKeyPackage=rawKP`.
    - Assertions: `resp.OK==true`, `resp.Record.Status=="approved"` (không còn `ERR_INVITE_ADD_MEMBER_FAILED`); Alice's `stored_keypackages` có KP cho Charlie sau xử lý; `pending_welcomes_out` có welcome cho Charlie (smoking gun = AddMember đã thật sự chạy).
  - **BI-120** `TestBusinessP1_E2E_BI120_NoTargetKeyPackage_FallsBackToCreatorFetch`: Backward compat — peer cũ không gửi KP, Alice vẫn có KP cache → vẫn approve. Đảm bảo field thuần additive.
- **Validation:**
  - `go vet ./...`, `go vet -tags=business_integration ./service/...` PASS.
  - `go test -tags=business_integration -run BI119|BI120 ./service/...` PASS 1.244s.
  - **Full business suite** `go test -tags=business_integration -timeout 600s ./service/... -count=1` PASS 31.786s, 162 tests total.
- **Tài liệu:** `docs/testing/BUSINESS_INTEGRATION_TEST_SCENARIOS.md` thêm BI-119 (P0) + BI-120 (P1) vào matrix với mô tả setup/action/expected.
- **Đánh giá best-practice:**
  - **Topology-aware**: data đi theo node có quyền tin cậy nhất (requester vừa verified target để tạo invite UI).
  - **Backward compat**: wire field omitempty + creator side fallback giữ luồng cũ → mixed-version cluster vẫn hoạt động.
  - **No silent failure**: nếu requester không lấy được KP, vẫn forward; creator log error rõ; row chuyển sang `failed` cho retry — không có "vô hình" bị nuốt lỗi.
  - **Audit trail đúng**: `stored_keypackages.source_peer_id = requester` thay vì target — phản ánh ai đã thực sự chuyển KP.

### Latest Delta (2026-05-17) ✅ — Group Fork Healing Epoch-based weight logic + Autonomous Replay verification

- **Group Fork Healing [COMPLETED]:**
    - **Formula:** Branch weight comparison strictly uses $W = (C_{members}, E, H_{commit})$. The `Epoch` ($E$) field acts as the primary evolutionary indicator when member counts are equal.
    - **Autonomous Replay:** Verified implementation where nodes merging into a winning branch automatically re-encrypt their partition-window messages using the newly acquired cryptographic state (Epoch/Key) and re-broadcast them. This ensures zero-data-loss and preserves Forward Secrecy. Strict non-repudiation is maintained (nodes only replay messages they authored).
- **Validation:**
    - `go test -v ./app/coordination/...` PASS (62 tests), including specific TDD cases for Epoch weight and Autonomous Replay.
    - Verified that `EncryptMessage` during replay uses the winning branch's `groupState`, ensuring cryptographic freshness.

### Latest Delta (2026-05-11) ✅ — Source-peer-id invariant hardening + invite-creator hint protocol round-trip + 4 new E2E regression tests

- **User report:** *"sao những lỗi như này mà chạy test không phát hiện? kiểm tra còn đường đi nào sót, hãy code thêm test bao phủ hết đi, để tránh lỗi thêm khi test manual"* — sau khi fix `fetchWelcomeFromStorePeers` không truyền `localID` làm source nữa, user yêu cầu audit toàn diện và code test cover hết đường đi để tránh tái lập tương lai.
- **Audit kết quả — phát hiện 4 gap còn lại trong creator-hint chain (đường đi mà non-creator member dùng để forward `RequestGroupInvite` về creator):**
  1. `WelcomeFetchResponseV1` thiếu `SourcePeerID` — invitee fetch welcome qua **store peer C** (không phải creator A) sẽ lưu C làm source → creator hint sai.
  2. `checkStoredWelcomes` (luồng pull on connect) truyền `""` cho `sourcePeerID` → row mất hint hoàn toàn.
  3. Không có defensive guard ở chokepoint `savePendingInviteFromWelcome` → caller mới ghi nhầm `localID` lần nữa thì silent break (đây là exact bug class vừa fix).
  4. `SaveStoredWelcome` / `SaveStoredWelcomeIfNewer` upsert ghi đè `source_peer_id` không có heal — caller bug ghi `""` sẽ wipe hint tốt sẵn có ở row cũ.
- **Fix triệt để (4 vá kiến trúc, không vá săm):**
  - **Gap 1 — Wire contract:** `app/adapter/p2p/invite_store_wire.go::WelcomeFetchResponseV1` thêm field `SourcePeerID` (`json:"source_peer_id,omitempty"`). Server side `handleWelcomeFetchStream` (trong `invite.go`) fill từ `database.GetStoredWelcome` (đã trả `srcPeerID` ở fix trước). Client side trong `fetchWelcomeFromStorePeers`: `source := strings.TrimSpace(resp.SourcePeerID); if source == "" { source = pid.String() }` — ưu tiên explicit field, fallback responder peer ID chỉ khi field thiếu (legacy responder chưa upgrade).
  - **Gap 2 — Source propagation:** `fetchWelcomeFromStorePeers` đổi return signature từ `([]byte, string, string, error)` thành `([]byte, string, string, string, error)` (welcome, groupType, categoryID, **sourcePeerID**, err). `checkStoredWelcomes` nhận và truyền source thật đó vào `savePendingInviteFromWelcome` thay vì `""`.
  - **Gap 3 — Defensive guard:** trong `app/service/invite.go::savePendingInviteFromWelcome`, sau khi resolve `localID`, thêm:
    ```go
    if sourcePeerID != "" && localID != "" && sourcePeerID == localID {
        slog.Warn("savePendingInviteFromWelcome: dropping self as source (programmer error)", ...)
        sourcePeerID = ""
    }
    ```
    → Self-as-source là programmer bug; thay vì silent break, drop xuống `""` (row vẫn được save, hint resolve fallback sang row khác hoặc fail rõ ràng với ERR_GROUP_CREATOR_UNKNOWN).
  - **Gap 4 — DB heal semantics:** `app/adapter/store/db.go::SaveStoredWelcome` và `SaveStoredWelcomeIfNewer` đổi clause `source_peer_id = excluded.source_peer_id` thành `source_peer_id = CASE WHEN trim(excluded.source_peer_id) <> '' THEN excluded.source_peer_id ELSE stored_welcomes.source_peer_id END` (tương tự `category_id` heal đã có). Một caller blank source không bao giờ wipe hint tốt sẵn có. `IfNewer` còn kết hợp với điều kiện `created_at >= existing` để giữ tính chất "newer wins".
- **Test bao phủ mới (5 test, tất cả pass):**
  - **Unit (`app/adapter/store/invite_lifecycle_test.go`):**
    - `TestGetGroupInviteCreatorHint_SkipsEmptySourceRows`: 2 rows trong stored_welcomes (newer blank, older inviter) → query phải trả inviter, blank không shadow.
    - `TestStoredWelcome_RoundTrip_PreservesInviterIdentity`: SaveStoredWelcome → GetStoredWelcome carry full inviter context, source != invitee.
    - `TestStoredWelcome_BlankSource_DoesNotClobberGoodHint`: heal contract cho upsert chính.
    - `TestStoredWelcomeIfNewer_BlankSource_DoesNotClobberGoodHint`: heal contract cho replication path.
  - **Integration E2E (`app/service/business_e2e_group_integrity_test.go`):**
    - **BI-115** `TestBusinessP1_E2E_BI115_SelfAsSource_NeverPersisted`: pass `bobInfo.PeerID` làm sourcePeerID → defensive guard phải drop, hint vẫn = Alice.
    - **BI-116** `TestBusinessP1_E2E_BI116_MemberResolvesCreatorAfterAutoJoin`: `bob.resolveGroupCreatorPeerID` qua (1) members table và (2) hint fallback (xoá members rồi check) — cả 2 đều = Alice. Đây là exact code path mà `RequestGroupInvite` non-creator phụ thuộc.
    - **BI-117** `TestBusinessP1_E2E_BI117_RestartReplay_PreservesInviterAsSource`: simulate restart bằng cách gọi `bob.fetchWelcomeFromStorePeers(gid)` → kích hoạt nhánh local-row → assert pre-state, replay-state, post-state đều giữ Alice là source.
    - **BI-118** `TestBusinessP1_E2E_BI118_WelcomeFetchResponse_CarriesSourcePeerID`: build `WelcomeFetchResponseV1` từ DB và assert `resp.SourcePeerID` được fill — guard refactor sau drop trường này (silent JSON omit).
- **Validation:** `go vet ./...`, `go vet -tags=business_integration ./...` PASS, unit tests `./adapter/store/...` PASS, integration E2E suite (`-tags=business_integration ./service/... -count=1`) PASS 28.026s với 160+ tests gồm 4 BI mới.
- **Tài liệu:** `docs/testing/BUSINESS_INTEGRATION_TEST_SCENARIOS.md` thêm hàng BI-115..BI-118 vào matrix với mô tả setup/action/expected — kèm flag "Fail = bug 'non-creator can't invite anymore' đã regress" để rõ vai trò regression-guard.
- **Đánh giá best-practice:**
  - **Defense in depth**: 1 cùng invariant ("source != self") được enforce ở 3 lớp — wire (BI-118), service guard (BI-115), DB heal (BlankSource tests). Một lớp fail thì lớp kia còn bắt được.
  - **Wire field omitempty**: backward compat với responder cũ chưa upgrade. Client side fallback sang `pid.String()` — không vỡ tuyệt đối, chỉ degrade về behavior cũ cho cặp legacy.
  - **Heal vs hard-overwrite**: cùng triết lý với `category_id` heal — blank không bao giờ wipe known-good. Áp dụng nhất quán cho cả `SaveStoredWelcome` và `SaveStoredWelcomeIfNewer`.

### Latest Delta (2026-05-10) ✅ — Welcome wire carries `category_id` inline (deterministic restore) + 3-node E2E group integrity tests

- **Bug recurrence reported:** *"Lại bị lỗi khi node 1 tạo nhóm và chỉnh ai cũng có thể mời, node 2 mời node 3 thì cái nhóm của node 3 đã mất danh mục."* — confirms previous fix (post-apply snapshot pull) was a band-aid: pull races with peer verification on first connect, can fail silently. Plus user thông báo thiếu test E2E nghiệp vụ kiểm tra integrity nhóm sau chuỗi action.
- **Root cause architectural:** Welcome bytes (RFC 9420) không carry category metadata; recovery dựa vào async snapshot pull → race với peer verification trong `node.AuthProtocol.IsVerified` → pull silent-fail → Charlie thấy nhóm orphan. Fix vá săm trước (gọi `scheduleChannelCategorySync` ngay sau apply welcome) chỉ giảm xác suất chứ không loại bỏ race.
- **Fix triệt để (deterministic, không phụ thuộc network state):** ship `category_id` inline trong **mọi** wire frame chuyển welcome, persist xuống stored_welcomes / pending_invites để startup recovery cũng có nguyên metadata.
  - Wire frames thêm field `category_id`:
    - `app/service/invite.go::welcomeDeliveryWire` (direct stream `/app/welcome-delivery/1.0.0`).
    - `app/adapter/p2p/invite_store_wire.go::WelcomeStoreRequestV1` / `WelcomeFetchResponseV1` / `WelcomeListItemV1` (offline replication + retrieval + list).
    - `app/service/blind_store.go::blindStoreEnvelopeV1` (blind-store gossip object).
  - Schema migration (`app/adapter/store/db.go`): `stored_welcomes.category_id`, `pending_invites.category_id` (cả 2 có DEFAULT '' để legacy rows OK), heal semantics: incoming non-empty value luôn thắng (chữa được legacy rows lưu trước migration).
  - DB API:
    - `SaveStoredWelcome` / `SaveStoredWelcomeIfNewer` thêm `categoryID` param.
    - `GetStoredWelcome` đổi return → `([]byte, string, string, error)` (welcome, groupType, categoryID, err).
    - `StoredWelcome` / `PendingInvite` struct thêm `CategoryID`.
    - `ReopenRejectedInvite` heal `category_id` qua `CASE WHEN ? != ''` để re-invite carry value mới.
  - Service layer:
    - `InvitePeerToGroup` resolve `categoryID` từ `coordStorage.GetGroupRecord(gid).CategoryID` (đã được `AssignCategoryToGroup` cập nhật trong `mls_groups`) rồi truyền xuống `replicateWelcomeToStorePeers` + `deliverWelcome`.
    - `savePendingInviteFromWelcome` ký mới: `(groupID, groupType, categoryID, welcome, sourcePeerID, reopenRejected)`. Sau `applyWelcome` OK gọi `applyChannelCategoryAfterAutoJoin`:
      - Path 1 (deterministic): nếu `categoryID != ""` → `db.AssignCategoryToGroup(groupID, categoryID)` + emit `channel_categories:changed{reason:"welcome_inline"}` → return.
      - Path 2 (fallback): chỉ chạy khi inline trống → giữ `scheduleChannelCategorySync(sourcePID)` cũ.
    - `processPendingWelcomesOnStartup` đọc `inv.CategoryID` từ pending row và truyền vào helper → startup recovery cũng deterministic không cần network.
    - `CheckDHTWelcome` (legacy API) cũng gọi `applyChannelCategoryAfterAutoJoin` sau khi apply.
  - Helper cũ `maybeSyncChannelCategoryAfterAutoJoin` được thay bằng `applyChannelCategoryAfterAutoJoin` (logic gồm cả inline write + fallback pull, gắn ở 1 chokepoint). Không còn vá săm rải rác.
- **End-to-end test mới (`app/service/business_e2e_group_integrity_test.go`):** đáp ứng yêu cầu user *"chưa có test nào kiểu làm một loạt các hành động nghiệp vụ và cuối cùng check xem đã vào được nhóm chưa và check xem các thông tin nhóm như danh mục, cấu hình security..."*
  - `TestBusinessP1_E2E_BI070_BobInvitesCharlie_AnyMember_GroupIntegrityIntact`: Alice tạo category → tạo channel → set policy `any_member` → Bob auto-join (mới wire-frame có category) → Bob gửi wire submit cho Charlie → Alice (Token Holder) auto-approve → kéo welcome từ Alice's `pending_welcomes_out` + categoryID từ Alice's `coordStorage` (mô phỏng deliverWelcome wire frame) → đẩy vào `charlie.savePendingInviteFromWelcome`. Cuối pipeline assert tất cả: `HasGroup`, `GroupRecord.GroupType`, `GroupRecord.CategoryID`, `ListGroupMembers` (có inviter), Alice's `GroupInvitePolicy` vẫn `any_member`. Đây chính là kịch bản user report — pass = bug fix triệt để.
  - `TestBusinessP1_E2E_BI071_FallbackPath_NoInlineCategory_StillJoins`: cùng setup nhưng deliberately truyền `categoryID = ""` (mô phỏng legacy / blind-store frame chưa upgrade) — assert auto-join vẫn thành công, fallback path không block join (UI sẽ show category sau khi pull thành công ở peer-connect tiếp theo).
  - Helper `assertGroupIntegrity` bundle 4 check (HasGroup, group_type, category_id, members) — tái dùng được ở các test sau.
- **Đánh giá best-practice (không phải fix vá săm nữa):**
  - **Single source of truth**: `category_id` chỉ resolve một lần ở inviter từ `coordStorage.GetGroupRecord(...)`, di chuyển nguyên vẹn trên dây và xuống đĩa, áp ngay khi nhận. Không có duplicate logic, không có cache stale risk.
  - **Backward compatible**: incoming non-empty heal nodes upgrade dần; field `omitempty` không làm vỡ peer cũ.
  - **Deterministic > eventually-consistent** cho metadata UI cần ngay. Pull snapshot vẫn còn nhưng bị degrade thành fallback đúng vai trò của nó.
  - **Test đi đến cuối ống dẫn**: assert state ở receiver runtime, không phải ở midstream (`pending_welcomes_out`). Mọi regression làm welcome mất category sẽ fail BI-070 ngay.
- **Validation:** `go vet ./...`, `go vet -tags=business_integration ./...`, `go build ./...`, `go test ./...` PASS, `go test -tags=business_integration -timeout 300s ./service` PASS (160 tests, 25.866s) gồm BI-070 + BI-071 mới, `npm run build` PASS.

### Latest Delta (2026-05-10) ✅ — Member invite forwards to creator + Welcome carries category

- **User reports cần fix cùng lúc:**
  1. *"Node 2 không mời được node 3, chỉ Node 1 mời được."* — Bug 1.
  2. *"Nhóm chat đang từ có danh mục thành mất danh mục."* — Bug 2.

- **Bug 1 — Root cause sâu:** `any_member` policy trước đây đi nhánh "member tự execute" (`processInviteRequest(id, true)` → `r.InvitePeerToGroup` → `coord.AddMember`). Nhưng theo **Single-Writer Invariant** (`coordinator.go:553-554`), chỉ Token Holder mới được phát Commit. Default Token Holder ở epoch 0/1 là creator → member luôn dính `ErrNotTokenHolder`. Lazy-sync hôm trước chỉ chữa triệu chứng (`ERR_INVITE_REQUEST_FORBIDDEN`), không chữa bệnh. Lazy-sync làm member rơi xuống nhánh self-process → tiếp tục fail ở MLS layer thay vì wire layer. Đây là lý do Node 2 vẫn không mời được sau patch trước.
- **Bug 1 — Fix kiến trúc đúng (2 file):**
  - `app/service/group_invite_requests.go::RequestGroupInvite`: bỏ phân nhánh local policy. Non-creator → **luôn** forward `submit` qua wire tới creator. Member không đọc local `invite_policy` nữa (creator là authority duy nhất + là Token Holder + nắm DB authoritative). Lazy-sync hook bị gỡ (logic mới không phụ thuộc local cache nữa).
  - `app/service/group_invite_request_p2p.go::rpcSubmitInviteRequest`: bỏ block `policy != creator_approval`. Cả 2 policy đều accept. Sau khi tạo pending row:
    - `creator_approval` → row đứng pending, creator quyết qua UI.
    - `any_member` → gọi `processInviteRequest(id, false)` ngay tại creator (Token Holder) để auto-approve. Trả record với status đã update.
- **Bug 1 — Hệ quả (semantics rõ ràng):** "any_member" trong P2P thesis = "creator auto-approves", **không phải** "member tự commit độc lập". Đây là giải thích đúng dưới Single-Writer Invariant. Member vẫn xuất hiện trên UI là người mời; creator's runtime chỉ là execution engine.
- **Bug 2 — Root cause:** `joinGroupWithWelcome` (group.go:519-531) tạo `GroupRecord` với `CategoryID=""` vì Welcome bytes (RFC 9420) không carry category metadata. Production có path `scheduleChannelCategorySync` chạy ở `peer_connected` event (invite.go:1456) — nhưng race với welcome arrival: Welcome đến qua direct stream **trước** khi peer_connected fired (cả 2 đều ở runtime của Tester1 nhưng concurrent). Invitee thấy nhóm xuất hiện "không có danh mục" cho tới khi snapshot pull tiếp theo trigger.
- **Bug 2 — Fix (`app/service/invite.go::savePendingInviteFromWelcome`):** sau `applyWelcome` thành công, nếu group là `channel`, spawn `go r.scheduleChannelCategorySync(sourcePID)` ngay lập tức để đóng race window. `scheduleChannelCategorySync` đã idempotent (3 backoffs, idempotent upsert) nên gọi 2 lần không hại.
- **Tests (`business_invite_request_integration_test.go`):**
  - BI-058 / BI-059 / BI-060 viết lại — không gọi `bob.RequestGroupInvite` (gater chặn dial trong harness) mà invoke trực tiếp `alice.handleGroupInviteWireRPC(bobPID, submit)`. Đây là contract creator-side mới. Tests vẫn cover: (a) any_member auto-approves, (b) duplicate guard, (c) target-already-member reject.
  - `TestBusinessP1_PolicyDrift_WireErrorContractStable` — gỡ (hợp đồng cũ "wire reject any_member với câu cố định" không còn tồn tại). Thay bằng 2 test mới:
    - `TestBusinessP1_AnyMember_WireSubmitAutoApproves` — wire submit any_member → record approved.
    - `TestBusinessP1_CreatorApproval_WireSubmitStaysPending` — wire submit creator_approval → record pending.
  - `business_integration_harness_test.go`: gỡ `businessSeedGroupInvitePolicy`, `sprint4MirrorInvitePolicyAfterCreator`, `sprint4EnsureAliceOnBobMemberTable` (dead helpers — kiến trúc mới member không có local policy state cần seed nữa).
- **Validation:** `go vet ./...`, `go vet -tags=business_integration ./service/...`, `go build ./...`, `go test ./...` PASS, `go test -tags=business_integration -timeout 120s ./service` PASS (25.231s), `npm run build` PASS.
- **Trade-off / future work:** với `any_member`, requester phải online & connected tới creator để submit thành công. Nếu creator offline → `errCreatorUnreachable`. Đây là hạn chế tự nhiên của Single-Writer + serverless: không có ai khác hợp lệ để Commit trong khi creator down. Mở rộng tương lai (ngoài thesis): bầu Token Holder rotate, hoặc designated co-admin.

### Latest Delta (2026-05-10) ✅ — Removed Cancel of group invite request (no CRDT cancel in P2P)

- **Quyết định nghiệp vụ:** trong môi trường P2P serverless, "rút lại yêu cầu" của requester về bản chất là CRDT — phải race với creator approve, đồng bộ qua gossip mesh, và tránh split-brain (cancel cục bộ thành công trong khi creator đã sinh Welcome). Quá phức tạp so với phạm vi luận văn → bỏ tính năng, chỉ giữ Approve / Reject (creator quyết định cuối cùng).
- **Backend (`app/service/group_invite_requests.go`):** xoá hẳn method public `CancelGroupInviteRequest`. Comment giữ lại để giải thích lý do và signpost cho future work nếu thêm CRDT layer.
- **Backend wire (`app/service/group_invite_request_p2p.go`):** xoá case `"cancel"` trong `handleGroupInviteWireRPC` và hàm `rpcCancelInviteRequest`. RPC giờ chỉ còn `submit` + `sync`. Op `cancel` đến sẽ trả `unknown op`.
- **Frontend (`runtimeClient.ts`):** bỏ import `CancelGroupInviteRequest` và wrapper `cancelGroupInviteRequest`. Wails generated bindings sẽ tự drop ở lần `wails generate` kế tiếp.
- **Frontend (`RoomPanel.tsx`):** đơn giản hoá UI yêu cầu tham gia — creator chỉ thấy `Duyệt` + `Từ chối`; mọi vai khác (requester / target / observer) thấy text "Đang chờ người tạo nhóm duyệt." Không còn nút `Hủy yêu cầu`. Handler `handleInviteAction` thu hẹp signature thành `'approve' | 'reject'`.
- **Tests đã gỡ:** `TestBusinessP1_Sprint4_BI063_CancelGroupInviteRequest_ByRequester` (business integration) và `TestCancelGroupInviteRequest_ProcessingReturnsConflict` (unit). Cả 2 thay bằng comment giải thích.
- **Docs:** `BUSINESS_INTEGRATION_TEST_SCENARIOS.md` BI-063 đánh dấu (removed) thay vì xoá hẳn để giữ lịch sử quyết định.
- **Schema:** const `store.InviteRequestStatusCancelled` giữ nguyên để row cũ (nếu có trong DB) vẫn load được; không có code path nào sinh ra status này nữa.
- **Validation:** `go vet ./...`, `go vet -tags=business_integration ./service/...`, `go build ./...`, `go test ./...` (PASS), `go test -tags=business_integration ./service` (PASS, 23.113s), `npm run build` (PASS).

### Latest Delta (2026-05-10) ✅ — Auto-join semantics for incoming Welcome (Discord/Slack-style invite)

- **Quyết định nghiệp vụ:** invitee không cần bấm "Accept" — Welcome đến nơi là vào nhóm luôn, đối xứng với policy phía inviter (`creator_approval` / `any_member`). User opt-out bằng `LeaveGroup`, không còn semantics "từ chối lời mời" trong UI.
- **Backend (`app/service/invite.go`):**
  - `savePendingInviteFromWelcome` mở rộng nhánh auto-apply cho **mọi** group type (không chỉ `dm`). Khi `applyWelcome` thành công, row pending lưu lại với `status=accepted` làm audit trail và emit `invite:auto_joined{id, group_id, group_type, inviter_peer}`. Khi fail (sidecar chưa ready, KP rotated, MLS rejection) → giữ `status=pending` cho retry sau.
  - Method mới `processPendingWelcomesOnStartup(ctx)` chạy goroutine sau `launchP2PNode`: refresh từ store peers rồi sweep `pending_invites` `status=pending`, retry `applyWelcome`. Best-effort, fail giữ pending cho lần startup tiếp theo.
- **Routing event (`app/service/runtime_events.go`):** thêm `invite:auto_joined` vào case aggregate routing để frontend event log nhận đúng `invite:<group_id>`.
- **Frontend (`app/frontend/src/features/chat/hooks/useChatEvents.ts`):** subscribe `invite:auto_joined` qua `useWailsEvent`; handler refresh group list + members rồi `pushToast` "X đã thêm bạn vào nhóm Y" (resolve display name qua `useContactStore`). UI **không** thêm danh sách pending welcomes / nút Accept/Reject — luồng manual coi như fallback của API public.
- **Tests mới (`business_invite_integration_test.go`):**
  - `TestBusinessP1_AutoJoin_OnIncomingWelcome` — `savePendingInviteFromWelcome` → `HasGroup=true` ngay, row `accepted`.
  - `TestBusinessP1_AutoJoin_ProcessesPendingOnStartup` — pre-seed `pending_invites` → gọi `processPendingWelcomesOnStartup` → row chuyển `accepted`, group joined.
  - `TestBusinessP1_AutoJoin_DeferredWhenKPMissing` — không có KP bundle → row giữ `pending`; persist KP rồi sweep lại → join thành công.
- **Test contract đã cập nhật:** `BUSINESS_INTEGRATION_TEST_SCENARIOS.md` BI-046 đổi sang manual seed only; bổ sung BI-046b/c/d cho auto-join (online happy path, startup recovery, deferred fallback). BI-047/048/049 giữ vì `AcceptInvite`/`RejectInvite` còn là idempotent manual-recovery API.
- **Đề xuất A — Wire-path harness + end-of-pipe regression guard (2026-05-10):** sửa `sprint4AliceBobJoinedChannel` (`business_integration_harness_test.go`) chuyển từ `JoinGroupWithWelcome` thủ công → đi qua `savePendingInviteFromWelcome` (cùng chokepoint với direct stream / replication / blind-store), kèm assert `bobDB.HasGroup(gid)` ngay trong helper. Hậu quả: 13 test BI-055..BI-067 (sprint4) tự động trở thành regression guard cho auto-join — nếu auto-join gãy, helper fail → toàn bộ sprint4 đỏ ngay. Bổ sung test mới `TestBusinessP1_WirePath_AliceInvitesBob_BobAutoJoinsEndToEnd` (BI-046e): chạy thật `alice.InvitePeerToGroup` → lấy welcome bytes từ `pending_welcomes_out` → đẩy vào `bob.savePendingInviteFromWelcome` → assert end-of-pipe (Bob trong nhóm + invite `accepted`). Đây chính là loại test mà 100+ test cũ thiếu vì assert dừng ở midstream (`pending_welcomes_out` là endpoint của sender) chứ không đi đến chokepoint receiver.
- **Validation:** `go vet ./...`, `go vet -tags=business_integration ./...`, `go build ./...`, `go test ./...`, `go test -tags=business_integration ./service` (full suite, 24.485s, PASS), `npm run build` — PASS.
- **Side effect tốt:** Welcome cũ kẹt trong `pending_invites` từ phiên trước (vd. kenh1/kenh2 trên DB Tester1) tự được auto-join ngay khi user restart Wails app, không cần xoá DB hay thao tác thủ công.

### Latest Delta (2026-05-09) ✅ — File transfer UX hardening + channel multi-attachments

- **Downloaded file actions (receiver UX):**
  - Added `Runtime.OpenDownloadedFile(groupID, fileID, fallbackPath)` to open previously downloaded files.
  - Download path is persisted in SQLite `file_transfers` for inbound rows (`direction=in`, states: `transferring/completed/failed`) so "Open file" still works after app restart.
  - `FileAttachmentCard` now supports `Mở file` + `Tải lại/Thử lại`; frontend maps `ERR_FILE_NOT_DOWNLOADED`, `ERR_FILE_MISSING_LOCAL`, `ERR_FILE_OPEN_FAILED` to user-friendly messages.
- **Channel post composer now supports multi-file attachments (product flow):**
  - Added backend API `PrepareGroupFile(groupID)` (open dialog + validate + prepare ciphertext, **no auto announce**).
  - Channel UI moved attach action into `PostComposerCard` and supports pending attachment list with remove action.
  - Publishing a channel post now sends a single `post` payload with `attachments[]` (max **10** files/post), replacing the old "one attached file = one separate auto-post" behavior.
  - `PostCard` renders multiple attachment cards per post; file download/open state is tracked per attachment key (`postId:fileId`) to avoid state collision.

### Latest Delta (2026-05-09) ✅ — Phase 8 MVP: file transfer (MLS exporter + direct pull)

- Proto `ExportSecretRequest.context` → Rust/OpenMLS exporter context (RFC 9420); Go `MLSEngine.ExportSecret(..., context []byte, length)`.
- Phase 8 MVP: AES-GCM chunks (`app/pkg/filetransfer`), SQLite `file_transfers`, `/app/file/1.0.0` **direct sender→receiver pull only** (không swarming).
- Runtime: `PrepareOutgoingFileTransfer`, `PullFileTransferFromPeer`; events `file:prepare` / `file:sent` / `file:received`; `-file-chunk-bytes`.
- `PROJECT_PLAN.md` §8: một dòng scope MVP (direct; swarming sau).

### Latest Delta (2026-05-08) ✅ — Hạng mục 3 hardening: local-remove cryptographic detection + revoke enforcement

- **MLS membership query RPC mới (`HasMember`) đã được thêm end-to-end:**
  - Proto: `HasMember(HasMemberRequest{group_state, identity}) -> HasMemberResponse{is_member}`.
  - Rust: `mls::has_member(...)` dùng `member_leaf_index(BasicCredential(identity))`.
  - gRPC/Go bridge: regenerated `app/mls_service/*.pb.go`, sidecar adapter + `coordination.MLSEngine.HasMember`.
  - Test/mocks: `MockMLSEngine` + service test engine stubs cập nhật interface.
- **Coordinator giờ detect local bị remove bằng cryptographic state sau khi apply commit:**
  - `coordinator.go` thêm `updateLocalAccessRevocationLocked(...)` gọi `MLSEngine.HasMember` sau commit path (both inbound process commit và local commit/remove/add).
  - Nếu local không còn trong ratchet tree: set `accessRevoked=true` (idempotent) + gọi callback `OnAccessLost(groupID, epoch, reason)`.
  - Các mutation APIs bị chặn khi revoked: `SendMessage`, `Propose*`, `AddMember`, `RemoveMember` trả `ErrAccessRevoked`.
- **Service event bridge chuẩn hóa theo contract đã chốt:**
  - `group:left` luôn có `reason`:
    - voluntary leave: `reason="left"`,
    - local access lost (removed): `reason="removed"` + `epoch`.
  - `Runtime.makeAccessLostHandler(...)` dừng coordinator, `MarkGroupLeft`, cập nhật roster local left, emit `group:left` + `group:members_changed`.
- **Frontend đồng bộ handling cho removed UX:**
  - Typed payload mới: `GroupLeftPayload`, `GroupMembersChangedPayload`.
  - `useChatEvents` xử lý `group:left(reason=removed)` để clear active room + toast cảnh báo.
  - `MainChatModuleScreen` runtime stream path cũng clear active group khi nhận `group:left` (parity live/replay).
- **Byzantine/replay hardening tests đã bổ sung:**
  - `TestCoordinator_LocalRemovedAfterCommit_BlocksMutations`.
  - `TestCoordinator_LocalRemoved_CallbackFiresOnceOnDuplicateReplay` (idempotent callback under duplicate replay).
  - `TestAccessLostHandler_EmitsGroupLeftRemovedAndMarksGroupLeft`.
- **Validation:** `go test ./...`, `go vet ./...`, `cargo test`, `npm run build` — PASS.

### Latest Delta (2026-05-08) ✅ — Hạng mục 3 polish: stable error-code contract Backend ↔ Frontend

- **Backend (`app/service/membership.go`) định nghĩa stable error codes wire format `"<ERR_CODE>: <english detail>"`:**
  - `ERR_GROUP_NOT_FOUND`, `ERR_REMOVE_MEMBER_FORBIDDEN`, `ERR_REMOVE_MEMBER_SELF`,
    `ERR_REMOVE_MEMBER_PEER_NOT_VERIFIED`, `ERR_REMOVE_MEMBER_ACCESS_REVOKED`,
    `ERR_REMOVE_MEMBER_CRYPTO_FAILURE`, `ERR_REMOVE_MEMBER_INVALID_PEER_ID`,
    `ERR_RUNTIME_NOT_INITIALIZED`.
  - `RemoveMemberFromGroup` map `coordination.ErrAccessRevoked` → `ErrRemoveMemberAccessRevoked` để FE phân biệt rõ "bạn đã bị kick" vs "lỗi crypto khác".
  - Đã bỏ sentinel chết `ErrRemoveMemberNotSupported`.
- **Frontend `app/frontend/src/lib/formatRemoveMemberError.ts` mới:** map error codes → toast tiếng Việt thân thiện với title + description (destructive). Có handler riêng cho `formatLeaveGroupError` và case `session has been replaced`.
- **`RoomPanel.tsx` chuyển `alert(...)` → `useToastStore.pushToast`:** thêm cả success toast cho leave/remove. Không còn message raw kiểu `Lỗi khi xóa thành viên: Error: remove member: …` lọt ra UI.
- **Validation:** `go vet`, `go test ./service ./coordination -count=1`, `npm run build` — PASS.

### Latest Delta (2026-05-07) ✅ — Fork Healing Sprint 2G: Integration tests (multi-node + persistence/failure coverage)

- **Integration test end-to-end cho fork healing đã được bổ sung (`app/coordination/fork_heal_integration_test.go`):**
  - `TestIntegration_ForkHeal_ConvergesReplayAndPersistsHistory`:
    - mô phỏng split-brain 2 node (FakeNetwork partition/heal),
    - trigger heal từ loser branch qua announce winner branch,
    - verify convergence (`epoch` + `tree_hash`),
    - verify autonomous replay chạy cho window partition,
    - verify persisted history tồn tại (`fork_heal_events` + `fork_audit`) với outcome success.
  - `TestIntegration_ForkHeal_FailurePersistsFailedStep`:
    - inject lỗi `GroupInfoFetcher`,
    - verify heal failure path kết thúc sạch (`healing=false`),
    - verify persisted failed event có `outcome=failed`, `failed_step=groupinfo_request`,
    - verify step-level audit rows được ghi cho trace thất bại.
- **Test harness mock được chỉnh để phản ánh convergence thực hơn:**
  - `MockMLSEngine.ExternalJoin` trả về commit payload JSON-compatible với `ProcessCommit` (`mockCommitData`) thay vì byte placeholder, giúp node winner có thể apply external commit và hội tụ epoch/tree hash trong integration assertions.
- **Validation:** `go test ./coordination -count=1` và `go test ./... -count=1 -timeout 240s` — PASS.
- **Trạng thái Sprint 2:** phần code Sprint 2A→2G đã xong; bước còn lại trước khi close toàn bộ Sprint 2 là **manual validation theo script mục 7 của plan**.

### Latest Delta (2026-05-07) ✅ — Fork Healing Sprint 2F: Persistence + history API

- **DB persistence cho fork-heal đã có schema thật (SQLite):**
  - `fork_heal_events`: summary row cho mỗi heal trace (`trace_id`, `group_id`, `winner_*`, `new_*`, `outcome`, `failed_step`, `duration/total`, `replayed_message_count`, timestamps).
  - `fork_audit`: step-level rows (`trace_id`, `group_id`, `step`, `status`, `ts_ms`, `duration_ms`, `error`).
  - Indexes cho truy vấn diagnostics theo group/trace (`idx_fork_heal_events_group_created`, `idx_fork_heal_events_trace`, `idx_fork_audit_trace_id`, ...).
- **Storage interface mở rộng (`coordination.CoordinationStorage`):**
  - `RecordForkHealEvent`, `RecordForkHealAudit`
  - `ListForkHealEvents`, `ListForkHealAudit`
  - `PruneForkHealHistory`
  - `SQLiteCoordinationStorage` + `MockStorage` đều đã implement.
- **Retention policy đã enforce theo quyết định đã chốt:**
  - 30 ngày + cap 10 records/group (hằng số nội bộ `forkHealRetentionDays=30`, `forkHealMaxPerGroup=10`).
  - Trigger prune khi insert event summary (`RecordForkHealEvent`) để tránh drift.
- **Coordinator đã persist trực tiếp từ heal pipeline:**
  - `runHeal` ghi audit rows cho từng step (`started/completed/failed`) qua `recordForkHealAudit`.
  - Success ghi `fork_heal_events` outcome `success` (kèm replayed count).
  - Failure path `logHealFailed` ghi `fork_heal_events` outcome `failed` + `failed_step`.
  - Persistence errors là non-fatal, chỉ warn log (`fork_heal/audit_persist_failed`, `fork_heal/event_persist_failed`) để không block heal.
- **Runtime API mới cho Developer Mode:**
  - `Runtime.GetForkHealHistory(groupID string, limit int)` (`app/service/fork_heal_history.go`)
  - Trả về summary + audit entries (hex tree hash, outcome, failed_step, timing metrics, replay count).
- **Tests mới/cập nhật:**
  - `app/adapter/store/coordination_storage_test.go`
    - `TestSQLiteCoordinationStorage_ForkHealHistory_RecordAndList`
    - `TestSQLiteCoordinationStorage_ForkHealHistory_PruneCap`
  - `app/service/fork_heal_history_test.go`
    - `TestGetForkHealHistory_ReturnsEventsWithAudit`
- **Validation:** `go vet ./...`, `go test ./... -count=1 -timeout 240s`, `go build ./...` — PASS.

### Latest Delta (2026-05-07) ✅ — Fork Healing Sprint 2E: Autonomous Replay

- **Replay đã chạy thật sau heal thành công:**
  - `Coordinator.runHeal` không còn `deferred_to_2e`.
  - Step 6 `fork_heal/replay_started_*`: query replay window từ `stored_messages` dựa trên `PartitionStartedAt`.
  - Step 7 `fork_heal/replay_completed_*`: re-encrypt + re-broadcast các message của **chính local node** trong window.
- **Replay window semantics (non-repudiation):**
  - Chỉ lấy message có `SenderID == localID`.
  - Chỉ lấy message timestamp trong `[partitionStartedAt, healStartedAt]`.
  - Dùng `GetMessagesSince` + filter local sender để giữ thứ tự HLC (causal order) khi replay.
  - Không replay message authored bởi peer khác (đúng invariant fork-healing).
- **Replay publish path:**
  - Encrypt lại từng plaintext bằng state mới sau external join (`MLSEngine.EncryptMessage`).
  - Broadcast `MsgApplication` envelope mới ở epoch healed hiện tại.
  - Append envelope vào offline log để offline-sync/blind-store vẫn thấy bản replay.
  - Persist lại `group_state` cuối replay về `mls_groups` để state crash-safe sau batch replay.
- **Throttle configurable đã được dùng thật:**
  - `CoordinatorConfig.ReplayThrottleMs` (default 100ms) áp dụng delay giữa các replay envelopes.
  - Delay đi qua `clock.After` (không `time.Sleep`) để tests deterministic với FakeClock.
- **Logging/Eval contract ổn định:**
  - `replay_started_completed` có `window_message_count`.
  - `replay_completed_completed` có `replayed_count`.
  - aggregate log có `replayed_message_count`.
  - Failure step logging giữ format `fork_heal/failed` + `failed_step`.
- **Code additions chính (`app/coordination/coordinator.go`):**
  - `collectReplayWindowMessages(...)`
  - `replayWindowMessages(...)`
  - `saveCurrentGroupStateLocked(...)`
  - runHeal now wires these into step 6/7.
- **Test mới/cập nhật:**
  - `TestCoordinator_Heal_ReplaysOwnPartitionWindowMessages`:
    - tạo 2 local messages trong partition window,
    - trigger heal,
    - assert replay publish `MsgApplication` >= 2 và heal success.
  - Existing heal tests vẫn pass sau đổi behavior.
- **Validation:** `go vet ./...`, `go test ./... -count=1 -timeout 240s`, `go build ./...` — PASS.

### Latest Delta (2026-05-07) ✅ — Fork Healing Sprint 2D: Heal orchestrator wired

- **`Coordinator.runHeal` đã chạy pipeline thật (không còn scaffold-only):**
  - Step 1 `fork_heal/groupinfo_request_*`: gọi `GroupInfoFetcher` lấy winner `GroupInfo`.
  - Step 2 `fork_heal/groupinfo_received_*`: verify metadata consistency (tree hash check khi available).
  - Step 3 `fork_heal/external_join_*`: gọi Rust `MLSEngine.ExternalJoin(groupInfo, signingKey)`.
  - Step 4 `fork_heal/state_swap_*`: persist state mới vào `mls_groups` + reset in-memory trackers (`epochTracker`, `singleWriter`, `forkDetector`) theo epoch mới.
  - Step 5 `fork_heal/external_commit_*`: broadcast external commit envelope lên group topic (epoch = winner epoch), append offline envelope log.
  - Step 6-7 replay markers giữ ổn định log contract và để mode `deferred_to_2e` (body replay sẽ cắm ở Sprint 2E).
- **Coordinator wiring mới:**
  - `CoordinatorOpts.GroupInfoFetcher` + `GroupInfoFetchResult` + `GroupInfoFetchFunc`.
  - `Coordinator` dùng callback này để fetch GroupInfo từ winner peer trong heal path.
  - `coordinator.go` thêm `buildEnvelopeWithEpochAndTimestampLocked(...)` để external commit broadcast dùng đúng envelope epoch.
- **Runtime integration (service layer):**
  - `Runtime.fetchGroupInfoForHeal(...)` bridge `requestGroupInfoFromPeer(...)` -> `coordination.GroupInfoFetchResult`.
  - Tất cả điểm tạo coordinator (`CreateGroupChat`, `JoinGroupWithWelcome`, restore existing groups) đều truyền `GroupInfoFetcher`.
- **State transition semantics sau external join:**
  - `newEpoch = winnerEpoch + 1` (epoch của external commit).
  - SaveGroupRecord overwrite atomically (giữ metadata cũ như role/group_type/created_at).
  - `forkDetector.Reset()` + `UpdateLocal(...)` để convergence detector khởi tạo lại theo nhánh mới.
  - Emit `onEpochChange(newEpoch)` để frontend/runtime event stream đồng bộ sau heal.
- **Failure handling/logging:**
  - Mọi lỗi theo step emit `fork_heal/failed` + aggregate outcome `failed` với `failed_step`.
  - Success path emit `fork_heal/completed` + aggregate có `new_epoch`, `new_tree_hash`, `total_ms`.
- **Metrics:**
  - `ForkHealingsAttempted` vẫn tăng khi schedule CAS thành công.
  - `ForkHealingsSucceeded` chỉ tăng khi pipeline 2D hoàn tất.
  - `RecordExternalJoin(duration)` ghi latency heal end-to-end của lần external join.
- **Tests cập nhật:** heal-path tests trong `fork_heal_orchestrator_test.go` inject `groupInfoFetch` mock để verify path thành công sau khi 2D live.
- **Validation:** `go vet ./...`, `go test ./... -count=1`, `go build ./...` — PASS.

### Latest Delta (2026-05-07) ✅ — Fork Healing Sprint 2C: GroupInfo wire protocol

- **New P2P protocol (`/app/group-info/1.0.0`):** thêm wire contract request/response cho fork-heal pre-step (loser fetch GroupInfo từ winner trước khi gọi `ExternalJoin`).
  - `app/adapter/p2p/group_info_wire.go`:
    - `GroupInfoProtocol = "/app/group-info/1.0.0"`.
    - `GroupInfoRequestV1{v, group_id, with_ratchet_tree}`.
    - `GroupInfoResponseV1{v, group_id, epoch, tree_hash, group_info, error}`.
    - `WriteGroupInfoJSONFrame` / `ReadGroupInfoJSONFrame` (length-prefixed JSON, 4 MiB cap, cùng convention với invite/offline wire).
- **Runtime stream handler (auth-gated):**
  - `app/service/group_info_sync.go::registerGroupInfoHandler` đăng ký `SetStreamHandler(p2p.GroupInfoProtocol, ...)`.
  - `handleGroupInfoStream`:
    - reject peer chưa verify (`AuthProtocol.IsVerified`),
    - read + validate `GroupInfoRequestV1`,
    - gọi `exportLocalGroupInfo(groupID, withRatchetTree)`,
    - trả `GroupInfoResponseV1` (happy path hoặc `error` string nếu fail).
  - lifecycle wiring:
    - startup `launchP2PNode()` thêm `r.registerGroupInfoHandler()`,
    - shutdown `stopNetworkLocked()` thêm `r.removeGroupInfoHandler()`.
- **Local export helper cho 2D:**
  - `exportLocalGroupInfo` snapshot local state từ coordinator + gọi `MLSEngine.ExportGroupInfo(...)`.
  - `snapshotGroupForExport` chuẩn hóa validation (`group_id` required, coordinator tồn tại, mls engine ready).
  - `requestGroupInfoFromPeer(ctx, remote, groupID, withRatchetTree)` đã có sẵn để Sprint 2D gọi trực tiếp khi chạy heal pipeline thật.
- **Coordination observability helper:**
  - thêm `Coordinator.GetTreeHash()` để service layer có thể trả metadata winner branch (`tree_hash`) trong response mà không đụng internal fields.
- **Tests mới:**
  - `app/adapter/p2p/group_info_wire_test.go`:
    - round-trip frame encode/decode,
    - reject zero-length / oversized frames.
  - `app/service/group_info_sync_test.go`:
    - export local group info thành công,
    - group not found,
    - mls engine not ready,
    - export error propagation.
- **Validation:** `go vet ./...`, `go test ./... -count=1`, `go build ./...` — PASS.
- **Phạm vi Sprint 2C:** dừng ở protocol + handler + request helper. Chưa cắm vào `runHeal` body (phần đó thuộc Sprint 2D).

### Latest Delta (2026-05-07) ✅ — Fork Healing Sprint 2B: Auto-announce + heal trigger scaffold

- **Independent timers (heartbeat + announce):** `Coordinator.periodicLoop` đã được tách thành **2 goroutine độc lập** — `heartbeatLoop` (cadence `HeartbeatInterval`) và `announceLoop` (cadence `AnnounceInterval`). Tránh pattern "đếm mỗi 2 heartbeat thì announce" — mỗi loop có timer/After riêng, không share counter, không bị skew khi tick này delay tick kia. Khi `AnnounceInterval == 0`, `announceLoop` không spawn (zero overhead trong tests/manual mode).
- **Config additions (`app/coordination/config.go`):**
  - `AnnounceInterval` (default `10s`, test default `0`) — chu kỳ broadcast `MsgAnnounce` cho ForkDetector.
  - `ReplayThrottleMs` (default `100`, test default `0`) — throttle giữa các re-broadcast trong Autonomous Replay (Sprint 2E sẽ tiêu thụ). Configurable để Phase 9.2 có thể sweep load profiles.
  - `Validate()` cộng thêm range checks (`>= 0`).
- **`broadcastAnnounceLocked` helper:** dùng chung giữa `announceLoop` và `BroadcastAnnounce()` (manual trigger trong tests). Nội dung announce = `(treeHash, activeView.Size())`. CommitHash chưa wire vào (Sprint 2D nếu cần để stricter weight comparison).
- **`ForkEvent.PartitionStartedAt` (sticky timestamp):**
  - `branchInfo.firstSeenAt` lưu thời điểm local node thấy TreeHash lạ **lần đầu**; observation tiếp theo cho cùng branch KHÔNG bump → giá trị này chính là partition start observed locally.
  - `ForkDetector.ProcessRemote(observedAt, from, epoch, ann)` — signature mới đẩy `observedAt` từ caller (Coordinator dùng `c.clock.Now()`); test callsites đã update.
  - Phục vụ Sprint 2E: tính partition window (now - PartitionStartedAt) làm input cho Autonomous Replay khi chọn message của chính node để re-encrypt.
- **`scheduleHeal` scaffold + concurrency guard:**
  - `Coordinator.healing atomic.Bool` — CAS guard, `handleAnnounceLocked` không bị block bởi heal pipeline dài; **đảm bảo chỉ 1 heal goroutine in-flight tại 1 thời điểm** kể cả khi nhiều peer cùng announce trong cùng tick.
  - Khi `ProcessRemote` trả `NeedExternalJoin == true`, `handleAnnounceLocked` gọi `scheduleHeal(event)` (vẫn giữ `IncrPartitionsDetected` vì test cũ phụ thuộc + để rõ semantics "detected vs attempted").
  - `scheduleHeal` snapshot fields cần thiết trước khi spawn goroutine, log `fork_heal/scheduled` (kèm `trace_id`, `winner_peer`, `winner_epoch`, `partition_started_at_ms`, `partition_window_ms`, `scheduled_at_ms`), CAS-skip nếu đã đang heal (`fork_heal/skipped_already_running`).
  - `runHeal` goroutine emit per-step structured logs theo contract đã thống nhất:
    - `fork_heal/started` — `queued_ms` (delay từ scheduled → goroutine bắt đầu).
    - `fork_heal/deferred_to_2d` — placeholder; Sprint 2D sẽ thay bằng steps 1–7 (`groupinfo_request`, `groupinfo_received`, `external_join`, `state_swap`, `external_commit`, `replay_started`, `replay_completed`).
    - `fork_heal/aborted` (nếu ctx cancel giữa chừng) — kèm reason + duration.
    - `fork_heal/completed` + `fork_heal/aggregate` — `outcome`, `duration_ms` (heal pipeline), `total_ms` (since scheduled), `partition_window_ms`, `winner_*`. Aggregate là 1 line/heal cho Phase 9 evaluation script (filter theo `trace_id`).
  - Metrics wiring: `IncrForkHealingsAttempted` chỉ tăng khi CAS thành công (không inflate counter khi guard skip); `IncrForkHealingsSucceeded` ở cuối `runHeal` của scaffold (Sprint 2D sẽ chỉ inc khi external_commit broadcast OK).
  - `IsHealing()` exported cho diagnostics + tests.
- **Trace ID:** `newTraceID()` — 4 random bytes hex (8 chars). Đủ unique trong cùng node, ngắn để grep log.
- **Tests mới (`fork_heal_orchestrator_test.go` + `fork_healing_test.go`):**
  - `TestCoordinator_AnnounceLoop_FiresOnInterval` — Advance(`AnnounceInterval`) → có envelope `MsgAnnounce` trong network inbox.
  - `TestCoordinator_AnnounceLoop_DisabledWhenZero` — `AnnounceInterval = 0` → không announce; heartbeat vẫn chạy bình thường.
  - `TestCoordinator_HeartbeatAndAnnounceLoops_Independent` — HB=50ms, Announce=200ms → sau 4 ticks HB, có ≥4 heartbeats và ≥1 announce (chứng minh 2 timer độc lập).
  - `TestCoordinator_ScheduleHeal_RecordsMetrics` — gọi trực tiếp scheduleHeal → `ForkHealingsAttempted == 1 && ForkHealingsSucceeded == 1`, `IsHealing() == false` sau khi xong.
  - `TestCoordinator_ScheduleHeal_ConcurrencyGuard` — pre-set `healing = true` → scheduleHeal skip; `Attempted == 0`.
  - `TestCoordinator_HandleAnnounce_TriggersHealOnLosingBranch` — Bob announce winner branch → Alice detect partition + attempt heal end-to-end.
  - `TestForkDetector_PartitionStartedAt_StableAcrossObservations` — observations 2,3,...n cho cùng TreeHash giữ nguyên `firstSeenAt`.
- **FakeClock additions (test infra):** `WaitersCount()` accessor + `waitForWaiters` helper sync goroutines registered After-waiter trước khi `Advance` — fix race "test advance trước khi loop kịp register".
- **Validation:**
  - `go vet ./...` — clean.
  - `go test ./... -count=1` — toàn bộ PASS (coordination, service, store, p2p, config).
  - `go build ./...` — clean.
- **Phạm vi Sprint 2B:** chỉ scaffold. Sprint 2C wire GroupInfo exchange protocol, Sprint 2D plug pipeline body vào trong `runHeal` (giữa `started` và `completed`), Sprint 2E Autonomous Replay (consume `ReplayThrottleMs`), Sprint 2F persistence + `GetForkHealHistory` API, Sprint 2G integration tests.

### Latest Delta (2026-05-07) ✅ — Fork Healing Sprint 2A: Rust foundation

- **Real OpenMLS External Commit (replaces stub):**
  - `crypto-engine/src/mls.rs::external_join` đã được rewrite thật bằng `MlsGroup::external_commit_builder()` chain (`build_group → load_psks → build → finalize`). Không còn placeholder bytes.
  - Wire format: input là TLS-serialized `MlsMessageOut` containing `MlsMessageBodyIn::GroupInfo(VerifiableGroupInfo)`; output gồm `(group_state, commit_bytes, tree_hash)` — proto fields giữ nguyên, không breaking change.
- **New RPC `ExportGroupInfo`:**
  - Proto: `rpc ExportGroupInfo(ExportGroupInfoRequest) returns (ExportGroupInfoResponse)` — `proto/mls_service.proto`.
  - Rust: `mls::export_group_info(group_state, with_ratchet_tree)` wraps `MlsGroup::export_group_info` (openmls 0.8.0 native API).
  - Go: `coordination.MLSEngine.ExportGroupInfo(...)` interface + `GrpcMLSEngine` adapter + `MockMLSEngine` test impl.
- **Forward Secrecy auto-handled by OpenMLS (CRITICAL design note):**
  - Khi node B (nhánh thua) external-join vào nhánh thắng và B đã có leaf cũ trong nhánh thắng (cùng signature key), `ExternalCommitBuilder` **tự động chèn `Remove` proposal** cho leaf cũ trong cùng commit (xem `openmls-0.8.0/src/group/mls_group/commit_builder/external_commits.rs:249-255`).
  - Hệ quả: Forward Secrecy của leaf cũ được crypto-shredded nguyên tử trong external commit. Coordination Layer (Go) **KHÔNG cần** tách thành hai commits riêng (External Join + Remove).
- **Versioning:** `openmls = "0.8.0"` giữ nguyên (KHÔNG bump). Cả `external_commit_builder` và `export_group_info` đều có sẵn ở 0.8.0.
- **Tests:** `cd crypto-engine && cargo test` — 11/11 PASS, gồm:
  - `test_export_group_info_roundtrip`: export GroupInfo nhiều lần với cùng state → idempotent.
  - `test_external_join_fork_heal_happy_path`: A tạo nhóm → A export GroupInfo → B external join → A process commit → B encrypt → A decrypt thành công ở epoch mới.
  - `test_external_join_rejects_invalid_group_info`: malformed bytes bị reject ngay tại deserialize.
- **Validation:** `cd app && go vet ./...; go test ./...; go build ./...` — toàn bộ PASS.
- **Phạm vi Sprint 2A:** chỉ Rust + Proto + Go bridge + Mock. **Không** có coordination/service layer logic — đó là Sprint 2B–2G (auto-broadcast Announce, GroupInfo wire protocol, heal orchestrator, Autonomous Replay, persistence).

### Latest Delta (2026-05-02) ✅

- **Message length limits (backend source of truth):** `app/service/message_limits.go` defines DM (4000 runes) and channel title/body/comment caps; `SendGroupMessage` validates DM and channel outbound text before MLS encrypt. Sentinel `ErrTextExceedsLimit` / `TEXT_TOO_LONG` for over-limit; empty-after-trim returns `ERR_MESSAGE_EMPTY`-style error. Tests in `message_limits_test.go`.
- **Wails / UI:** `GetMessageLimits()` → `MessageLimitsDTO`; frontend `runtimeClient.getMessageLimits`, Zustand `useMessageLimitsStore`, rune counters on `MessageComposer`, `PostComposerCard`, `CommentComposer`; `formatSendError` + toast store for send/retry failures; helper copy points to future **encrypted file send** (Phase 8).
- **Docs:** README Section 3.2.1 documents limits, rune semantics, and relation to GossipSub wire cap vs Phase 8 file transfer.

### Latest Delta (2026-04-30) ✅

- **Roster/Profile/Presence pipeline is now canonicalized (backend-first):**
  - Added `group_members` table + repository methods (`UpsertGroupMember`, `ListGroupMembers`, `MarkGroupMemberLeft`).
  - `GetGroupMembers` now uses **roster active list from DB** + online presence overlay (no longer ActiveView-only membership source).
  - Display-name resolution order for member API: roster/profile cache -> verified token -> short peer id fallback.
- **Lifecycle hooks now maintain roster consistently:**
  - create group -> upsert local `creator`;
  - join by welcome -> upsert local member;
  - invite flow -> upsert invitee;
  - leave group -> mark local member `left`.
- **Data hygiene hardening for legacy sender IDs:**
  - Backfill from message history now validates `peer_id` (`peer.Decode`) before roster upsert.
  - `GetGroupMembers` filters invalid/non-decodable peer IDs to avoid corrupted/off-spec rows showing in UI.
- **Channel payload contract hardened:**
  - Canonical message id = `envelope_hash` (hex), strict validation for retry/delete.
  - Channel outbound payload validation added for `post/comment/mentions` (with tests).
- **Frontend mention system is now shared via hook (consistent across chat/post/comment):**
  - New hook: `frontend/src/features/chat/hooks/useMentions.tsx`.
  - Mention autocomplete enabled in DM composer + post composer + comment composer.
  - Mention highlighting unified in message/post/comment rendering; self-mentions are emphasized with orange style.

### Phase 1: System Architecture & Setup ✅
*   Monorepo, Sidecar lifecycle, gRPC IPC, CGO-free SQLite.

### Phase 2: P2P Networking Layer ✅
*   **Persistent PeerID:** Stored in SQLite `system_config` table.
*   **Resilient IP Detection (Hybrid):** UDP trick (8.8.8.8) → fallback interface scan (ignores Docker/WSL/VMWare).
*   **Libp2p Host:** Bound to the specific "best" IP found.
*   **Log Noise Suppression (2-Layer):** `go-log/v2` sets mdns to `error`; custom `LogFilterHandler` drops Windows virtual adapter warnings.
*   **Hybrid Discovery:** mDNS + Kademlia DHT + Dynamic Bootstrap.
*   **GossipSub:** Global chat topic `/org/chat/global`.

### Phase 3: Identity & Admin Onboarding ✅
*   Tất cả 7 bước đã implement và build clean (Go + Rust).

### Phase 3.5: Wails GUI Integration ✅
*   Thư mục `backend/` đã đổi tên thành `app/`, Go module name đổi từ `backend` → `app`.
*   Tích hợp Wails v2.11.0: composition root `app/main.go`, bindings trên `app/service.Runtime`, `app/adapter/wailsui` gọi `wails.Run` + `EventSink`.
*   Scaffold React + TypeScript + Tailwind frontend tại `app/frontend/`.
*   4 màn hình dev/test UI đã hoàn chỉnh.

### Hexagonal layout (Go) ✅
*   `app/domain`, `app/port`, `app/adapter/{store,p2p,sidecar,wailsui}`, `app/service`, `app/config`, `app/cli`, `app/pkg/log`; SQLite tại `adapter/store/`, libp2p tại `adapter/p2p/`, Rust sidecar tại `adapter/sidecar/`.

### Agent — Bản đồ mã nguồn & Wails (đọc trước khi sửa) 📌

| Vùng | Đường dẫn | Ghi chú |
|------|-----------|---------|
| Composition root | `app/main.go` | `config.Parse()`, nhánh `cli.Run` vs `wailsui.Run`; `//go:embed all:frontend/dist` |
| Cấu hình flag | `app/config` | `Config`, `Parse()`, `IsCommand()` — dùng chung cho `main` và `service` (**không** đặt `Parse` trong `cli` để tránh import cycle `service` ↔ `cli`) |
| CLI | `app/cli` | `runner.go`, `commands.go` — gọi `service.*` (headless node, backup, bundle, …) |
| Ứng dụng + Wails | `app/service` | `Runtime`; lifecycle export: `Startup`, `DomReady`, `BeforeClose`, `Shutdown`. File theo nghiệp vụ: `runtime.go`, `identity.go`, `admin.go`, `node_status.go`, `group.go`, `messaging.go`, `invite.go`, `session.go`, `app_state.go`, `identity_backup.go`, `bootstrap.go`, `cli_node.go`, `events.go` |
| SQLite | `app/adapter/store` | Thay cho `app/db` cũ |
| P2P | `app/adapter/p2p` | Thay cho `app/p2p` cũ |
| Crypto gRPC | `app/adapter/sidecar` | `StartCryptoEngine`, `NewMLSEngine` — thay cho `coordination/mls_adapter.go` + `crypto_engine.go` cũ |
| Wails vỏ | `app/adapter/wailsui` | `Run`, `EventSink` → `runtime.EventsEmit` |
| Log | `app/pkg/log` | `Setup(headless)` |
| Hex ports / domain | `app/port`, `app/domain` | Interfaces + kiểu thuần |
| Coordination protocol | `app/coordination` | Chỉ logic giao thức; **không** gắn binary Rust trực tiếp |

**Wails → TypeScript (quan trọng):** Bind target là `*service.Runtime`. Codegen tạo `frontend/wailsjs/go/service/Runtime.js|.d.ts` và `models.ts` với namespace **`service`** (không còn `main`). Import FE: `from '.../wailsjs/go/service/Runtime'`, `import { service } from '.../wailsjs/go/models'`. Sau đổi API Go: `cd app && wails generate module`, rồi `npm run build` trong `frontend/` nếu cần.

**Đường dẫn lịch sử (không dùng nữa):** `backend/`, `app/app.go`, `app/group_ops.go`, `app/node.go`, `app/commands.go`, `frontend/wailsjs/go/main/App`, `app/db`, `app/p2p` (root).

### Phase 4.1: Add member / KeyPackage (MLS) ✅
*   Proto: `GenerateKeyPackage`, `AddMembers`; `ProcessWelcome` extended with `epoch` and `key_package_bundle_private` (OpenMLS requires the invitee to retain the `KeyPackageBundle` private material until Welcome, not just the public KeyPackage).
*   Rust: `generate_key_package`, `add_members`; gRPC handlers; tests `test_generate_key_package`, `test_add_member_and_welcome`.
*   Go: `MLSEngine` + adapter + mock; `Coordinator.AddMember` added; `CommitMsg` broadcast only carries `CommitData` + `NewTreeHash` (Welcome is delivered out-of-band, not broadcast).
*   Wails: `KeyPackageResult`, `AddMemberToGroup`, `JoinGroupWithWelcome`, `GetGroupMembers`, `MemberInfo`; ChatPanel **Members & keys** panel.

---

## 3. Technical Decisions & Knowledge

### 3a. Hai loại Identity — KHÔNG nhầm lẫn

| Identity | Layer | Ai sinh | Ai quản lý | Mục đích |
|---|---|---|---|---|
| **Libp2p PeerID** | Mạng (P2P) | Go (`crypto.GenerateKeyPair`) | Go + SQLite | Định danh node, mã hóa kênh Noise |
| **MLS Identity** | Ứng dụng (E2EE) | **Rust** (OpenMLS) | Go (lưu SQLite) | Ký/mã hóa tin nhắn MLS Group |

### 3b. Luồng Onboarding đúng (CSR-like) — THIẾT KẾ CUỐI CÙNG

**NGUYÊN TẮC BẤT BIẾN:**
*   Admin là người cấp danh tính — **tên hiển thị do Admin đặt**, không phải user tự đặt.
*   MLS Private Key sinh ra trên máy user, KHÔNG BAO GIỜ rời máy.
*   Root Admin Private Key chỉ tồn tại trên máy Admin.

```
[Máy User - lần đầu]
  1. app --setup
     → GetOrCreateLibp2pIdentity() → PeerID (đã có từ Phase 2)
     → Rust GenerateIdentity() → MLS keypair (credential rỗng ban đầu)
     → Output: PeerID + MLS_PubKey_hex (KHÔNG có tên — Admin sẽ đặt)

[Máy Admin]
  2. Nhận PeerID_Alice + MLS_PubKey_Alice (Zalo/email)
  3. app --create-bundle \
       --bundle-name "Alice" \          ← Admin đặt tên cho user
       --bundle-peer-id <PeerID> \
       --bundle-pub-key <PubKeyHex> \
       --admin-passphrase "secret" \
       --bundle-output alice.bundle
  4. Gửi alice.bundle cho Alice (out-of-band)

[Máy User]
  5. app --import-bundle alice.bundle
     → Verify: chữ ký Admin + PeerID binding + PublicKey binding + expiry
     → SaveAuthBundle() vào SQLite
     → UpdateMLSDisplayName("Alice") ← tên từ token ghi đè vào mls_identity
     → App → StateAuthorized

  6. app (chạy bình thường — GUI mode)
     → Load bundle → BuildLocalToken() → NewP2PNode() với auth
     → Kết nối bootstrap_addr từ bundle
     → Auth handshake với mọi peer
```

**Admin Quick Setup (GUI shortcut):** Nếu máy đã có admin key (`--admin-setup` đã chạy) và đang ở trạng thái `AWAITING_BUNDLE`, UI hiện card "Admin Quick Setup" — nhập displayName + passphrase → tự tạo và import bundle cho chính mình trong 1 bước. Binding: `CreateAndImportSelfBundle(displayName, passphrase string) error`.

### 3c. Cấu trúc InvitationToken và InvitationBundle

```go
// app/admin/token.go
type InvitationToken struct {
    Version     int    `json:"version"`     // = 1
    DisplayName string `json:"display_name"` // do Admin đặt
    PeerID      string `json:"peer_id"`      // BẮTBUỘC — chống Token Replay Attack
    PublicKey   []byte `json:"public_key"`   // MLS public key (hex khi hiển thị)
    IssuedAt    int64  `json:"issued_at"`
    ExpiresAt   int64  `json:"expires_at"`   // = IssuedAt + 365 ngày
    Signature   []byte `json:"signature,omitempty"` // Ed25519 ký payload trên
}

type InvitationBundle struct {
    Token         *InvitationToken `json:"token"`
    BootstrapAddr string           `json:"bootstrap_addr"` // /ip4/IP/tcp/PORT/p2p/PEERID
    RootPublicKey []byte           `json:"root_public_key"` // TOFU khi import lần đầu
}
```

**bootstrap_addr PHẢI có `/p2p/PeerID`** — thiếu PeerID thì Noise Protocol không thể xác thực danh tính bootstrap node.

### 3d. Bảo vệ chống Token Replay / Spoofing Attack

```go
// app/adapter/p2p/auth_protocol.go — verifyPeerToken()
if token.PeerID != authenticatedPeerID.String() {
    reject() // Eve không có Libp2p private key của Alice → bị lộ ngay
}
```

### 3e. Auth Protocol — `/app/auth/1.0.0`

**Wire format (current):** `[4 bytes big-endian uint32: JSON length][JSON bytes of AuthHandshakeMsg]`

```go
type AuthHandshakeMsg struct {
  Token              *InvitationToken
  Session            SessionClaim // started_at + nonce + MLS-signed proof
  Error              string       // optional stale-session response
  SupersedingSession SessionClaim // newer same-identity proof for local lockout
}
```

Backward compatibility: parser still accepts legacy token-only payload.

**Quy tắc tránh deadlock:**
*   **Client (outbound, gọi `InitiateHandshake`):** SEND token trước → READ token peer
*   **Server (inbound, `handleIncoming` qua `SetStreamHandler`):** READ token peer trước → SEND token

**Auth state machine (per connection attempt):**
```
Connected → Handshaking → Verified       (crypto ok → lưu vào verifiedPeers)
                        → SecurityFail   (sai chữ ký / hết hạn / PeerID mismatch
                                          → rejectSecurity: blacklist TTL + close peer)
                        → TransientFail  (IO error / timeout / stream reset
                                          → rejectTransient: reset stream only, KHÔNG blacklist)
```

**Vòng đời verifiedPeers:**
*   Set khi handshake thành công (inbound hoặc outbound).
*   Xóa khi peer đóng **tất cả** connection (TCP + QUIC) — xử lý trong `Disconnected` notifee.
*   Đảm bảo peer reconnect/restart luôn phải handshake lại, không bị skip do stale state.

**AuthGater (TTL-based):**
*   `Blacklist(id, reason)` CHỈ được gọi từ `rejectSecurity` — khi `verifyPeerToken` thất bại.
*   KHÔNG gọi khi `NewStream` fail, IO error, hoặc timeout — đây là `rejectTransient`.
*   Blacklist entry tự hết hạn sau **30 phút** (peer được thử lại mà không cần restart app).
*   `isBlacklisted` evict entry hết hạn lazily khi check.

**Root cause bug đã fix (restart → "gater disallows connection"):**
Trước đây `handleIncoming` có `if IsVerified(peer) { return }`. Khi node A restart, node B
(còn chạy) vẫn giữ A trong verifiedPeers nên skip handshake mà không đọc/ghi token. A đang
chờ đọc token thì nhận EOF → gọi `reject()` (cũ) → blacklist B oan. Fix: bỏ early-return ở
inbound + thêm `Disconnected` handler xóa verifiedPeers + tách `rejectSecurity`/`rejectTransient`.

### 3f. App States — THIẾT KẾ CUỐI CÙNG

```
StateUninitialized  → Chưa có MLS keypair → GUI: SetupScreen
StateAwaitingBundle → Có keypair, chưa có bundle → GUI: AwaitingBundleScreen
StateAuthorized     → Có bundle hợp lệ → GUI: DashboardScreen
StateAdminReady     → StateAuthorized + có root admin key → GUI: DashboardScreen + AdminPanel
```

### 3g. display_name — Admin là người cấp, không phải user

*   `display_name` trong `InvitationToken` do **Admin đặt** — đây là tên chính thức.
*   Khi user chạy `--setup`, credential MLS ban đầu **rỗng**.
*   Khi user chạy `--import-bundle`, hàm `ImportInvitationBundle` tự động gọi `database.UpdateMLSDisplayName(token.DisplayName)`.
*   Định danh kỹ thuật thực sự: **PeerID** (mạng) và **MLS PublicKey** (crypto).

### 3h. CRITICAL — Device Migration phải export cả Libp2p Private Key

```
identity.backup (mã hóa AES-256-GCM + Argon2id)
├── libp2p_private_key   ← BẮT BUỘC — để PeerID giống hệt trên máy mới
├── mls_signing_key      ← MLS private key
├── mls_credential       ← display_name bytes (sau khi import bundle)
├── invitation_token     ← token Admin đã ký
├── mls_groups           ← serialized group_state snapshot
├── stored_messages      ← local decrypted chat history
├── kp_bundles           ← pending invite private material
├── pending_welcomes_out ← welcomes chưa giao
└── pending_invites      ← invitee-side pending invite UI state
```
Implementation note: backup/import is now fully handled in Go (no Rust `ExportIdentity/ImportIdentity` RPC required).

### 3i. Wails Integration — Kiến trúc GUI (cập nhật sau refactor)

**Nguyên tắc:** Wails binding — UI gọi exported methods trên `*service.Runtime`. Codegen: `frontend/wailsjs/go/service/Runtime.d.ts` + `Runtime.js`; DTO trong `frontend/wailsjs/go/models.ts` (namespace `service`).

**Phân nhánh CLI vs GUI trong `app/main.go`:**
```go
cfg := config.Parse()
if cfg.Headless || cfg.IsCommand() {
    cli.Run(cfg) // app/cli
} else {
    wailsui.Run(cfg, distFS) // app/adapter/wailsui — embed frontend/dist
}
```

**`IsCommand()`:** xem `app/config/config.go` (setup, admin, bundle, import/export identity, …).

**Wails lifecycle:** `Runtime.Startup` → DB + identity + sidecar + P2P nếu AUTHORIZED/ADMIN_READY; `Runtime.Shutdown` → teardown. `adapter/wailsui` gắn `EventSink` để `Runtime.emit` → `EventsEmit`.

**State chính (thread-safe, `mu sync.Mutex`):** `ctx`, `cfg`, `db`, `privKey`, `mlsClient`, `conn`, `stopEngine`, `node`, `transport`, `coordStorage`, `mlsEngine`, `coordinators`, … — định nghĩa trong `app/service/runtime.go`.

### 3j. Decentralized Coordination Protocol — Thiết kế mới (Phase 4)

**THAY ĐỔI QUAN TRỌNG so với thiết kế ban đầu:**
Phiên bản cũ dùng "Deterministic Conflict Resolution" — cho phép xung đột xảy ra rồi chọn commit có hash nhỏ nhất, commit thua bị rollback. **Phiên bản mới LOẠI BỎ HOÀN TOÀN xung đột** bằng Single-Writer Protocol.

**Bốn cơ chế cốt lõi:**

**1. Single-Writer Protocol (Giao thức Người ghi duy nhất):**
*   Tại mọi thời điểm, chỉ **một node duy nhất** — Epoch Token Holder — có quyền tạo Commit.
*   Election tất định (không cần giao tiếp): `TokenHolder = argmin_{node ∈ ActiveView} H(nodeID || epoch)`
*   Phân tách Proposal/Commit: mọi node có thể tạo Proposal, chỉ Token Holder đóng gói thành Commit.
*   Failover & Censorship Resistance: Token Holder không commit trong `TokenHolderTimeout` -> Đưa vào danh sách đình chỉ (`SuspendedOrExcluded`) -> Bầu lại Token Holder mới. Danh sách đình chỉ tự động bị xóa (clear) khi sang epoch mới (Epoch-bound Clearing).

**2. Epoch Consistency (Nhất quán nhân quả qua kiểm tra Epoch):**
*   `msg.epoch == local.epoch` → xử lý bình thường
*   `msg.epoch < local.epoch` → từ chối, gửi `CurrentEpochNotification`
*   `msg.epoch > local.epoch` → buffer, request state sync

**3. Group Fork Healing (Hàn gắn phân mảnh mạng):**
*   Phát hiện qua Gossip Heartbeat (khác TreeHash).
*   Hàm trọng số: `W = (C_members, E, H_commit)` — so sánh lexicographic.
*   Nhánh thua: Drop MlsGroup → External Join vào nhánh thắng → Autonomous Replay (chỉ gửi lại tin nhắn của chính mình).
*   Forward Secrecy bảo toàn (khóa nhánh thua bị hủy). PCS suy yếu tạm thời, khôi phục ngay sau External Join.

**4. Hybrid Logical Clock (HLC) — Thứ tự hiển thị tin nhắn:**
*   Epoch number chỉ ordering MLS state changes. Trong cùng 1 epoch, nhiều user gửi tin nhắn đồng thời → cần HLC để sắp xếp.
*   `HLCTimestamp = (L, C, NodeID)`: L = max(physical_time, received_L), C = logical counter, NodeID = tiebreaker.
*   Đảm bảo: causal consistency, total order, NTP-independent, human-readable (L là unix ms).
*   Mỗi application message mang HLC timestamp. UI sort bằng `Before()`.

**Hệ thống clock đầy đủ:**
| Clock | Mục đích |
|---|---|
| **Epoch Number** (logical counter) | MLS state ordering, Token Holder election, Fork Healing |
| **HLC** (hybrid logical) | Application message display ordering |
| **Local wall clock** | Liveness detection (heartbeat, T_timeout), feeds HLC |

**Package `app/coordination/` — ĐÃ IMPLEMENT (`go test ./coordination` — PASS; ~63 hàm `Test*` + subtests table-driven):**
*   `interfaces.go` — Contracts: Transport, Clock, MLSEngine (with treeHash returns), CoordinationStorage
*   `types.go` — Data types, wire messages, enums, sentinel errors
*   `config.go` — CoordinatorConfig + DefaultConfig + TestConfig + Validate
*   `clock_real.go` — RealClock (production); FakeClock (test-only, trong `clock_fake_test.go`)
*   `hlc.go` — Hybrid Logical Clock: Now(), Update(), thread-safe, injectable Clock
*   `metrics.go` — Thread-safe instrumentation for Phase 7 evaluation + Snapshot + Reset
*   `active_view.go` — ActiveView: heartbeat tracking, liveness check, eviction, sorted members, onChange
*   `single_writer.go` — ComputeTokenHolder (argmin SHA-256(nodeID||epoch)), BufferProposal, DrainProposals
*   `epoch.go` — ValidateEpoch, EpochTracker, future buffer with defensive copies, Advance returns buffered
*   `fork_healing.go` — CompareBranchWeight `(MemberCount > Epoch > CommitHash)`; ForkDetector
*   `coordinator.go` — Central orchestrator: ties ActiveView + SingleWriter + EpochTracker + ForkDetector + HLC into message processing pipeline. Public API: CreateGroup, Start, Stop, SendMessage, ProposeAdd/Remove/Update
*   **MLS ↔ gRPC:** `GrpcMLSEngine` nằm ở `app/adapter/sidecar/engine.go` (implements `coordination.MLSEngine`) — không còn file `app/coordination/mls_adapter.go`.
*   `testutil_test.go` — FakeNetwork (queue + DrainAll), FakeTransport, MockMLSEngine, MockStorage
*   `coordinator_test.go` — 10 integration tests: group creation, token holder election, message send/receive, proposal/commit, epoch consistency, heartbeats, HLC ordering, fork detection

### 3k. Threaded Workplace UX (Slack/Teams Paradigm)

Để tối ưu hóa trải nghiệm làm việc cho môi trường doanh nghiệp và loại bỏ việc dùng PeerID làm định danh chính:
* **Phân cấp Channels vs DMs:** SQLite table `mls_groups` bổ sung cột `group_type` (`'channel'` / `'dm'`).
* **Cú pháp tạo kênh:** Frontend chặn định dạng `#kênh-thảo-luận` để gắn nhãn `channel`, ngược lại gắn nhãn `dm`.
* **Cấu trúc dữ liệu Thread:** Tin nhắn trao đổi qua kênh được đóng gói dưới dạng JSON Envelope (`{type: 'post', title: '...', content: '...'}` và `{type: 'reply', parent_id: '...', content: '...'}`).
* **Xác thực lời mời thủ công:** Mã kết nối (Join Code) tương ứng với KeyPackageHex. Ứng dụng tích hợp luồng nhập liệu bù trừ DHT khi cần (AddMemberToGroup qua PeerID + KeyPackage).

---

## 4. Current Progress

### Phase 5.1 + 5.2 + 5.3 (Identity migration + session takeover + offline messaging) — COMPLETE ✅

#### Implemented in this update

- **MLS Atomic Apply (consistency hardening):**
  - Root cause fixed: `SQLITE_BUSY` during `SaveMessage` could previously leave MLS message-key consumption and DB persistence out of sync, causing replay-time `SecretReuseError`.
  - New atomic persistence API in coordination storage (`app/coordination/interfaces.go`):
    - `ApplyCommit(...)`
    - `ApplyApplication(...)`
  - SQLite implementation (`app/adapter/store/coordination_storage.go`) now persists group-state/message/applied-marker/envelope-log in one transaction for commit/application apply paths.
  - Coordinator refactor (`app/coordination/coordinator.go`):
    - in-memory state is updated only after atomic DB apply succeeds;
    - local send/commit paths persist first, then publish/emit;
    - inbound replay paths share the same idempotent apply boundary.
  - PeerID serialization cleanup:
    - canonical `peer.ID.String()` in envelope sender and storage write paths;
    - decode fallback for legacy/non-canonical values remains in read paths.
  - Regression coverage:
    - atomic apply/idempotency tests in `app/adapter/store/coordination_storage_test.go`;
    - coordinator guard test for persist-failure non-advancement in `app/coordination/coordinator_test.go`.

- **SQLite contention hardening:**
  - `app/adapter/store/db.go` now sets `WAL`, `busy_timeout`, and a single-writer pool (`SetMaxOpenConns(1)`) to reduce write contention bursts under concurrent goroutines.

- **Dev multi-instance scripts (Windows) now default to latest build:**
  - `scripts/dev-second-instance.ps1`, `scripts/dev-third-instance.ps1`, `scripts/dev-fourth-instance.ps1` support `-AutoBuild` (default true).
  - Default launch behavior: run `wails build` before starting node 2/3/4 exe; can disable via `-AutoBuild:$false`.

- **Identity backup/import (Phase 5.1, Go-side):**
  - Implementation: `app/service/identity_backup.go` (+ tests `identity_backup_test.go`)
    - `ExportIdentityBackup(...)` and `ImportIdentityBackup(...)`
    - Wire format: `[16B salt][12B nonce][AES-GCM ciphertext]`
    - Argon2id params aligned with admin key encryption.
  - Backup format v2: identity + snapshot (`mls_groups`, `stored_messages`, `kp_bundles`, `pending_welcomes_out`); import tương thích v1.
  - CLI: `app/cli/commands.go` + `runner.go` (`--export-identity`, `--import-identity`, …).
  - Wails: `Runtime.ExportIdentity`, `Runtime.ImportIdentityFromFile`
  - Side effects sau import: `service.ApplyIdentityImportSideEffects` (`session.go`)

- **Session takeover hardening (Phase 5.2, auth-handshake session claim model):**
  - `app/adapter/p2p/session_claim.go` + tests; auth wire trong `app/adapter/p2p/auth_protocol.go` (`AuthHandshakeMsg { token, session }`, tương thích token-only cũ).
  - Session keys / flags: `app/service/session.go` (`buildLocalAuthHandshake`, `resetSessionStartedAt`, `killSessionPendingConfigKey`, …).
  - Node startup: `app/service/runtime.go` (`launchP2PNode`) + `app/adapter/p2p/host.go`.

- **Offline messaging (Phase 5.3, store-and-forward):**
  - Offline sync stream + ACK cursor: `app/service/offline_sync.go`, `app/adapter/p2p/offline_wire.go`.
  - Envelope log + sync ack persistence: `app/adapter/store/coordination_storage.go`, schema `envelope_log` / `envelope_dedup` / `sync_acks` / `pending_delivery_acks` / `offline_sync_pull_state` trong `app/adapter/store/db.go`.
  - Runtime trigger: peer connect + manual trigger (`TriggerOfflineSync`) để pull missed envelopes từ connected verified peers.
  - `scheduleOfflineSyncPull` thêm retry/backoff ngắn để tránh race lúc peer vừa connect nhưng chưa advertise protocol.
  - Đã bỏ hoàn toàn DHT mailbox data-path (`app/adapter/p2p/offline_dht.go`, `offlineDHTPushLoop`, `offlineDHTCheckLoop`).

- **Invite offline store (KeyPackage / Welcome) — post-refactor:**
  - DHT application-data path đã loại bỏ (`app/adapter/p2p/kp_dht.go` removed).
  - Custom store protocols: `/app/kp-store/1.0.0`, `/app/kp-fetch/1.0.0`, `/app/welcome-store/1.0.0`, `/app/welcome-fetch/1.0.0` (wire: `app/adapter/p2p/invite_store_wire.go`).
  - `invite.go` vẫn replicate qua store streams (fanout mặc định 3), đồng thời publish blind-store object để tăng xác suất lưu hộ khi peer đích offline.
  - `CheckDHTWelcome` giữ tên để tương thích Wails UI cũ, nhưng implementation hiện fetch từ store peers (không còn gọi DHT).

- **Universal Blind-Store layer (new):**
  - Topic global: `/org/offline-store/v1` (`app/service/blind_store.go`).
  - Object types: `group-envelope`, `key-package`, `welcome`.
  - Runtime policy: regular nodes subscribe blind-store by default and only retain targeted replica objects; `--store-node` retains all objects; `--blind-store-participant=false` explicitly opts out (`app/config/config.go`).
  - Replica selection: ưu tiên Kademlia `GetClosestPeers(routingKey)` + fallback XOR-distance; chỉ nhận từ verified peers.
  - Coordinator hook: `OnEnvelopeBroadcast` publish `MsgCommit/MsgApplication` sang blind-store khi local node broadcast.

### Phase 6 P0 Backend Productization — COMPLETE ✅

- **Invite / pending invite lifecycle:**
  - Wails APIs: `GenerateJoinCode`, `ListPendingInvites`, `AcceptInvite`, `RejectInvite`.
  - SQLite `pending_invites` tracks invitee-side UI lifecycle; Welcome discovery now supports `/app/welcome-list/1.0.0`.
  - Pending invites are included in identity backup/import.
- **Group membership lifecycle:**
  - `LeaveGroup` implements soft leave: stop local participation, keep local history.
  - `RemoveMemberFromGroup` is end-to-end (2026-05-08): creator-only, verified-peer MLS identity, `coord.RemoveMember`, stable `ERR_REMOVE_MEMBER_*` wire codes; see delta §4 same file and `app/service/membership.go`.
- **Session takeover lifecycle:**
  - APIs: `GetSessionStatus`, `AcknowledgeSessionReplaced`.
  - Old sessions only enter `replaced` state after verifying a newer same-identity `SessionClaim` signed by the local MLS key; normal peer arbitration/disconnects do not trigger lockout.
  - Mutating/network actions are guarded by `ErrSessionReplaced`; read-only local history remains accessible.
- **Startup/runtime health:**
  - API: `GetRuntimeHealth`.
  - Events: `startup:progress`, `startup:error`, `p2p:status`, `offline_sync:status`, `runtime:health`.
- **Admin issuance readiness:**
  - APIs: `GetAdminStatus`, `ParseDeviceRequestJSON`, `CreateBundleFromRequest`.
  - P0 keeps passphrase-per-sign; no in-memory admin private-key cache.
  - Request validation checks JSON version, libp2p PeerID, and Ed25519 MLS public-key hex length.

#### Validation

- `go test ./...` PASS
- `go vet ./...` PASS
- `go build ./...` PASS

### Phase 6 P1 + Phase 7 (incremental productization) — IN PROGRESS ✅ (latest slice completed)

#### Backend P1 slice delivered in this update

- **Advanced Workplace Paradigm (Implemented):**
  - Expanded `mls_groups` schema in `app/adapter/store/db.go` with `group_type` to separate `channels` and `dms`.
  - Upgraded Go `CreateGroupChat` signatures to require structured type definitions.
  - Added full multi-select member registration modal triggers directly mapping `CreateGroupModal.tsx`.

- **Dynamic Workspace Identities Cache (Implemented):**
  - Added local `peer_directory` cache mapping PeerID ↔ Display Name.
  - Extended Go `MemberInfo` & `MessageInfo` with display name payload attributes.

- **Diagnostics snapshot/export (implemented):**
  - New Runtime APIs: `GetDiagnosticsSnapshot`, `ExportDiagnostics`, `OpenLogFolder`.
  - Added `app/service/network_diagnostics.go` with snapshot DTOs for app state, peer counts, offline sync status, coordinator group summary, runtime health.
  - Diagnostics export writes JSON artifact under `.local/diagnostics-<unix>.json`.

- **Message retry/delete contract (implemented):**
  - New Runtime APIs: `RetryMessage(groupID, messageID)`, `DeleteLocalMessage(groupID, messageID)`.
  - Added DB helpers in `app/adapter/store/db.go`: `GetStoredMessageByID`, `DeleteStoredMessageByID`.
  - Extended `coordination.StoredMessage` with stable `MessageID`; SQLite read path now maps `stored_messages.id` to `MessageID`.
  - `MessageInfo` now includes `message_id` + `status` for frontend-safe retry/delete actions.

- **Admin issuance history (implemented):**
  - Added SQLite table `admin_issuance_history`.
  - Added DB APIs: `SaveAdminIssuanceRecord`, `ListAdminIssuanceHistory`.
  - Runtime API `ListIssuanceHistory()` added.
  - Bundle issuance paths now record local audit rows after successful creation.

#### Frontend FE-4/5/6/7 slice delivered in this update

- **Runtime client aggregation expanded:**
  - `app/frontend/src/services/runtime/runtimeClient.ts` now exposes the full product-facing Runtime method set used by Group/Invite/Settings/Admin/Diagnostics flows.

- **Real feature screens replaced previous placeholders (`return null` removed):**
  - `features/invites/screens/InvitesScreen.tsx`:
    - Generate join code
    - Invite peer to active group
    - List/accept/reject pending invites
  - `features/settings/screens/SettingsScreen.tsx`:
    - Export identity backup
    - Validate/set bootstrap override + reconnect P2P
    - Export diagnostics + open log folder
  - `features/admin/screens/AdminPanelScreen.tsx`:
    - Init admin key
    - Parse request JSON + issue bundle
    - List issuance history

- **Main shell module navigation integrated:**
  - `PrimaryRail` now routes module view (`chat` / `invites` / `settings` / `admin`).
  - `MainChatModuleScreen` orchestrates module switching and renders feature screens.

- **Retry/delete wiring improved in chat flow:**
  - `useChatActions` now calls backend `retryMessage` / `deleteLocalMessage` for persisted messages.
  - `chatModel` maps backend `message_id` and `status` into local `ChatMessage`.

#### Validation for this slice

- `cd app && go test ./...` PASS
- `cd app && go vet ./...` PASS
- `cd app && wails generate module` PASS
- `cd app/frontend && npm run build` PASS

### Phase 4 Coordination Layer — COMPLETE ✅ (xác minh: `cd app && go test ./...`; `cd crypto-engine && cargo test`)

**Coordination:** `app/coordination/*.go` — Transport, Clock, MLSEngine (interface), Coordinator, HLC, fork healing. **Không** chứa binary Rust; bridge MLS: `app/adapter/sidecar/engine.go`.

**SQLite + transport:** `app/adapter/store` (`db.go`, `coordination_storage.go` — 8 tests store), `app/adapter/p2p/transport_adapter.go` (LibP2PTransport).

**Rust:** `crypto-engine/` — OpenMLS stateless qua gRPC (xem `crypto-engine/src/mls.rs`). `external_join` đã là implementation thật (Sprint 2A — 2026-05-07), `export_group_info` đã có (RPC mới).

**Wails + FE:** Methods trên `app/service.Runtime` (tách file: `group.go`, `messaging.go`, `invite.go`, …). Bindings TS: `frontend/wailsjs/go/service/Runtime.*`, models namespace `service`. UI: `frontend/src/**/*.tsx` (import Runtime, không dùng `go/main/App`).

**Kiểm tra nhanh:** `cd app && go vet ./... && go test ./...` ; `cd frontend && npm run build`.

### Wails bindings (receiver: `*service.Runtime`)

| Method | Mô tả |
|--------|-------|
| `GetAppState() string` | UNINITIALIZED / AWAITING_BUNDLE / AUTHORIZED / ADMIN_READY / ERROR |
| `GetOnboardingInfo() OnboardingInfo` | PeerID + PublicKeyHex |
| `GenerateKeys() OnboardingInfo` | Tạo MLS keypair qua Rust engine |
| `OpenAndImportBundle() error` | File dialog → import bundle → start P2P |
| `HasAdminKey() (bool, error)` | Kiểm tra có admin key không |
| `CreateAndImportSelfBundle(name, passphrase) error` | Admin tự cấp bundle cho mình |
| `InitAdminKey(passphrase) error` | Khởi tạo Root Admin key |
| `CreateBundle(req) (string, error)` | Tạo bundle cho user mới → save dialog |
| `GetAdminStatus`, `ParseDeviceRequestJSON`, `CreateBundleFromRequest` | Admin issuance UI: trạng thái key, parse request JSON, ký bundle passphrase-per-sign |
| `GetNodeStatus() NodeStatus` | State, PeerID, DisplayName, ConnectedPeers |
| `GetRuntimeHealth() RuntimeHealth` | Startup/app/P2P/crypto health cho loading/error screens |
| `GetSessionStatus`, `AcknowledgeSessionReplaced` | Single-active-device UX; route old device sang Session Replaced screen |
| **`CreateGroupChat(groupID) error`** | **Tạo MLS group + Coordinator + subscribe GossipSub** |
| **`SendGroupMessage(groupID, text) error`** | **Encrypt + broadcast qua Coordinator** |
| **`GetGroupMessages(groupID) []MessageInfo`** | **Lấy messages sorted by HLC** |
| **`GetGroups() []GroupInfo`** | **Danh sách groups đã tham gia** |
| **`GetGroupStatus(groupID) map[string]interface{}`** | **Epoch, token holder, member count, metrics** |
| `GetGroupMembers`, `AddMemberToGroup`, `JoinGroupWithWelcome`, `GenerateKeyPackage`, `LeaveGroup`, `RemoveMemberFromGroup` | MLS / group lifecycle; remove = creator-only + verified target + coordinator MLS remove (FE: `formatRemoveMemberError`) |
| `InvitePeerToGroup`, `CheckDHTWelcome`, `GetKPStatus`, `GenerateJoinCode`, `ListPendingInvites`, `AcceptInvite`, `RejectInvite` | Luồng invite offline-friendly (store-peer based; `CheckDHTWelcome` là tên legacy API) |
| `ExportIdentity`, `ImportIdentityFromFile` | Backup `.backup` (GUI) |

*(Danh sách đầy đủ: `wails generate module` → `Runtime.d.ts`.)*

### CLI Commands hiện tại (từ `app/`):

```powershell
# Lần đầu — User tạo key pair
go run . --setup

# Admin: khởi tạo root key (chỉ chạy 1 lần trên máy Admin)
go run . --admin-setup --admin-passphrase "MySecret"

# Admin: tạo bundle cho user mới
go run . --create-bundle `
  --bundle-name "Alice" `
  --bundle-peer-id "12D3KooW..." `
  --bundle-pub-key "a3f7c2..." `
  --admin-passphrase "MySecret" `
  --bundle-output alice.bundle

# User: import bundle từ Admin
go run . --import-bundle alice.bundle

# Headless mode (không GUI)
go run . --headless
go run . --headless --db mydb.db --p2p-port 4002

# GUI mode (mặc định khi không có flag)
wails dev        # development (hot-reload)
wails build      # production build
```

---

## 5. Lưu ý kỹ thuật quan trọng

*   **Module name:** `module app`. Import nội bộ: `"app/..."`.
*   **Wails embed:** `//go:embed all:frontend/dist` trong `app/main.go` (composition root). Source: `app/frontend/src/`; build Vite: `app/frontend/dist/`. Repo có thể có `dist/index.html` tối thiểu để `go build` không lỗi embed khi chưa `npm run build`.
*   **wails generate module:** Chạy từ `app/` sau khi thêm/sửa exported method trên `service.Runtime` → cập nhật `frontend/wailsjs/go/service/Runtime*` và `models.ts`. Đồng bộ import TS (`service/Runtime`, namespace `service`).
*   **protoc command đúng:** `protoc --go_out=. --go-grpc_out=. --proto_path=./proto mls_service.proto` (chạy từ project root)
*   **openmls_rust_crypto version:** phải dùng `0.5` (khớp với `openmls_traits 0.5`)
*   **bootstrap_addr format bắt buộc:** `/ip4/IP/tcp/PORT/p2p/PEERID`
*   **display_name trong MLS credential:** Hiện tại lưu raw UTF-8 bytes; có thể nâng cấp sau sang TLS-serialized `BasicCredential` nếu cần.
*   **Backup `.backup` (Phase 5.1):** Payload **phải** gồm `libp2p_private_key` (cùng các trường MLS/bundle) để khôi phục đúng PeerID — đã implement trong Go (`identity_backup.go`), không phụ thuộc RPC `ExportIdentity`/`ImportIdentity` của proto.
*   **Blacklisting policy:** `rejectSecurity` (có blacklist) chỉ gọi khi `verifyPeerToken` thất bại. `rejectTransient` (không blacklist) cho mọi lỗi IO/timeout. KHÔNG bao giờ blacklist khi `NewStream` fail.
*   **GetNodeStatus mutex:** Khi đã giữ lock `Runtime.mu`, dùng `getAppStateUnlocked()` thay vì gọi lại `GetAppState()` (tránh deadlock).

---

## 6. Frontend Progress Snapshot (latest)

### FE-2 status — COMPLETE (onboarding production baseline) ✅

- Routing chính đã chạy qua `RootRouter` với các nhánh:
  - `UNINITIALIZED` -> `WelcomeScreen`
  - `AWAITING_BUNDLE` -> `AwaitingBundleScreen`
  - import backup entry -> `ImportBackupScreen`
  - `AUTHORIZED/ADMIN_READY` -> main app screen
- Onboarding UX đã có:
  - tạo identity (`GenerateKeys`)
  - hiển thị PeerID + MLS pubkey
  - export `request.json` từ frontend helper
  - import `.bundle` với xử lý lỗi cơ bản
  - import `.backup` flow cơ bản
- Đã dọn phần dev/test onboarding khỏi đường đi chính.

### FE-3 status — COMPLETE (chat shell core) ✅

- Main app shell đã thay placeholder bằng `MainAppScreen`.
- Đã có 3 vùng layout:
  - sidebar groups/navigation (`MainSidebar`)
  - chat center (`ChatView`, `MessageList`, `MessageComposer`)
  - room panel phải (`RoomPanel`)
- Luồng core đã implement:
  - lấy groups (`GetGroups`), tạo group (`CreateGroupChat`), chọn group active
  - lấy lịch sử tin (`GetGroupMessages`)
  - gửi tin (`SendGroupMessage`)
  - realtime events qua `useWailsEvent`: `group:message`, `group:epoch`, `group:joined`
  - failed-send local recovery (`Retry` / `Remove`)
- Network status hiển thị liên tục từ `GetNodeStatus` qua `useNetworkStore`.

### UI polish status — COMPLETED ✅

- Đã hoàn tất các vòng thiết kế và đánh giá trực quan: giao diện dark shell hiện đại, phân cấp rõ ràng, tỷ lệ giãn cách tối ưu.
- Đồng bộ hóa toàn bộ hệ thống icon system, spacing scale, microcopy thân thiện và đảm bảo tính thống nhất hình ảnh.

---

## 7. Next Step — All Phases Completed! ✅

Phase 1 đến Phase 9 hoàn tất 100%. **Phase 5.1 (`.backup`), 5.2 (`SessionClaim` / single active device), 5.3 (offline store-and-forward)**, **Phase 6 backend productization**, **Phase 7 frontend UI**, **Phase 8 file transfer** và **Phase 9 evaluation** đã implement hoàn chỉnh.

Hệ thống đã có:
- OpenMLS (nhóm, tin nhắn, KeyPackage / AddMembers / Welcome, …) qua sidecar
- Coordination đầy đủ (Single-Writer, Epoch, Fork healing, HLC); **`go test ./...`** và **`cargo test`** để xác minh
- Offline sync + blind-store nền tảng cho envelope / KeyPackage / Welcome
- Identity migration `.backup` + session claim foundation
- P0 frontend-safe backend APIs cho invite lifecycle, membership lifecycle, session replaced lockout, runtime health, admin issuance readiness

**Roadmap tài liệu hiện tại:**
- `PROJECT_PLAN.md`: roadmap tổng thể: Phase 6 backend productization, Phase 7 frontend, Phase 8 file transfer, Phase 9 evaluation.
- `BACKEND_IMPLEMENTATION_PLAN.md`: kế hoạch backend chi tiết cần làm trước frontend.
- `FRONTEND_IMPLEMENTATION_PLAN.md`: đặc tả màn hình / luồng UI production-ready.
  - **Cập nhật kiến trúc bắt buộc (áp dụng từ FE-1):** Thin frontend layer, Zustand-first state management, Wails `EventsOn` cleanup bắt buộc để tránh memory leak, smart/dumb component boundary, desktop-safe routing (state-based hoặc `MemoryRouter`, không dùng `BrowserRouter`), ưu tiên stack Shadcn UI + Tailwind.

**Để manual test core hiện tại:**
1. `cd crypto-engine; cargo build --release`
2. `cd app; wails generate module; wails dev` (hoặc `go run . --headless` cho CLI)
3. Trong UI: nhập Group ID → Create / Join → gửi tin nhắn; kiểm tra HLC sort
4. Hai instance (hai DB / port) để thử P2P

**Tiếp theo (ưu tiên):**

1.  **Fork Healing Orchestration & Hardening (Sprint 2A-2G + Milestone 5 [COMPLETED ✅]):**
    - **2A-2G:** Implemented multi-node autonomous replay, weight comparisons, external joins, and step logs.
    - **Milestone 5 Hardening (2026-05-31):** Unified `job_id`, compound event indices, precise epoch+tree-hash branch matcher, explicit phase ordering helpers, and offline state recovery.
    - **Logging & Verification:** Verified all state transitions, branch mismatch handling, crash-safe outbox log ordering, and SQLite query filters under multi-fork scenarios. Fully validated through automated SQLite integration tests and multi-node scenarios.

2.  **Frontend FE-4 — Group & Invite Product Flows [COMPLETED ✅]:**
    - Group info panel + add member/join code + pending invites + leave/remove UX hoàn chỉnh.
    - Chuẩn hóa action policies theo backend và hiển thị an toàn trên giao diện.
    - Đảm bảo smart/dumb boundary và dọn dẹp các event listener.

3.  **Phase 6 P1 còn lại [COMPLETED ✅]:** developer diagnostics snapshot và log folder API đã hoàn tất và đóng cổng bảo mật.

4.  **Phase 8–9 [COMPLETED ✅]:** file transfer (MLS exporter / direct chunking `/app/file/1.0.0`) và chaos evaluation đa node hoàn thành 100%, xuất báo cáo đo lường liveness và đồ thị hội tụ cho luận văn.

**Lưu ý thiết kế quan trọng:**
*   **KHÔNG DÙNG "smallest hash" nữa** — phương pháp cũ đã bị thay thế bằng Single-Writer Protocol.
*   **GroupState trong Rust:** blob bytes chứa full persisted OpenMLS storage + metadata/signing key. Rust deserialize blob để load group và serialize lại sau mỗi operation (stateless giữa các RPC/process restart).
*   **Coordination Layer chạy hoàn toàn ở Go** — Rust không biết gì về Single-Writer hay Epoch.
*   **Real OpenMLS 0.8:** create_group dùng `MlsGroup::new_with_group_id`, encrypt dùng `group.create_message`, decrypt dùng `group.process_message`. Forward secrecy enforced (sender CANNOT decrypt own messages — own messages stored as plaintext directly).
*   **LibP2PTransport:** Wraps real GossipSub + direct streams qua protocol `/coordination/direct/1.0.0`. Auto-skips messages from self.
*   **Shared transport:** All Coordinators share a single `LibP2PTransport` instance.
