# Business Integration Test Scenarios

Tài liệu này liệt kê các kịch bản integration test cho phần **ứng dụng cộng tác** của dự án. Mục tiêu là bảo vệ các luồng demo và tính năng nghiệp vụ chạy qua `service.Runtime`, SQLite, Wails-facing DTO/event contract, và phần P2P ở mức cần thiết.

Tài liệu này **không** thay thế test chuyên sâu cho MLS, Single-Writer, Epoch Consistency, Fork Healing, HLC hoặc Rust OpenMLS. Các tầng nghiên cứu đó có suite riêng. Ở đây, crypto/coordination được xem như dependency/hộp đen và chỉ kiểm tra tác động nghiệp vụ quan sát được.

## Nguyên Tắc Bao Phủ

- Test gọi qua API public của `service.Runtime` càng nhiều càng tốt, vì đây là ranh giới mà frontend Wails sử dụng.
- Test xác minh output theo hợp đồng nghiệp vụ: DTO trả về, record SQLite, runtime event, aggregate revision, error code ổn định.
- Không assert chi tiết ratchet tree, MLS ciphertext, token-holder election hoặc fork-heal algorithm trừ khi nó hiện ra như hành vi nghiệp vụ.
- Các test cần tránh phụ thuộc thời gian thật khi có thể. Dùng fake clock, temp DB, mock engine, fake event sink hoặc fake transport.
- Với luồng P2P thật hoặc hai node thật, chỉ đưa vào nhóm smoke/integration cấp cao vì dễ flaky hơn test service-level.

## Priority Legend

- **P0:** Phải có trước demo. Nếu fail thì demo core có nguy cơ gãy.
- **P1:** Nên có để ổn định demo nhiều máy và các tính năng phụ trợ.
- **P2:** Quan trọng cho admin/diagnostics/file transfer nhưng không phải luồng chat tối thiểu.
- **P3:** Bổ sung sau, phục vụ độ tin cậy dài hạn.

## Matrix Kịch Bản

| ID | Priority | Nhóm | Kịch bản | Setup tối thiểu | Hành động | Kỳ vọng |
|---|---|---|---|---|---|---|
| BI-001 | P0 | App state | DB trống trả trạng thái khởi tạo đúng | Temp DB mới, runtime chưa có identity | `GetAppState()` | Trả `UNINITIALIZED`, không crash |
| BI-002 | P0 | App state | Sinh MLS identity lần đầu | Temp DB, runtime có crypto mock/engine | `GenerateKeys()` | Có `OnboardingInfo`, DB có identity, state chuyển `AWAITING_BUNDLE` |
| BI-003 | P0 | App state | Lấy onboarding info sau khi sinh khóa | Sau BI-002 | `GetOnboardingInfo()` | PeerID và MLS public key không rỗng, khớp DB |
| BI-004 | P0 | App state | Import bundle hợp lệ | Admin fixture tạo bundle cho local PeerID/public key | Import bundle qua helper/runtime | State chuyển `AUTHORIZED` hoặc `ADMIN_READY` nếu có admin key |
| BI-005 | P0 | App state | Import bundle sai chữ ký bị từ chối | Bundle bị sửa payload/signature | Import bundle | Có lỗi ổn định, DB không lưu auth bundle nửa vời |
| BI-006 | P0 | App state | Import bundle không khớp PeerID bị từ chối | Bundle bind PeerID khác | Import bundle | Có lỗi, state vẫn `AWAITING_BUNDLE` |
| BI-007 | P1 | App state | Export device request JSON | Có MLS identity | `ExportDeviceRequestJSON()` | JSON parse được, có version, peer ID, public key |
| BI-008 | P1 | Backup | Export/import identity backup happy path | Runtime authorized có một group và message fixture | Export backup, import vào DB mới | Identity, auth bundle, groups, messages khôi phục được |
| BI-009 | P1 | Backup | Import backup sai passphrase | Backup hợp lệ, passphrase sai | Import | Lỗi rõ, DB đích không bị ghi một phần |
| BI-010 | P1 | Backup | Import backup không force khi DB đã có identity | DB đích đã có identity | Import `force=false` | Bị chặn; `force=true` mới ghi đè theo contract |
| BI-011 | P2 | Admin | Khởi tạo admin key lần đầu | Temp DB | `InitAdminKey(passphrase)` | `HasAdminKey()` true, `GetAdminStatus()` phản ánh có key |
| BI-012 | P2 | Admin | Verify passphrase đúng/sai | Có admin key | `VerifyAdminPassphrase()` | Đúng pass; sai pass trả lỗi |
| BI-013 | P2 | Admin | Parse device request JSON hợp lệ | JSON từ BI-007 | `ParseDeviceRequestJSON()` | Trả DTO đúng peer/public key |
| BI-014 | P2 | Admin | Parse device request JSON lỗi schema | JSON thiếu trường/version sai | `ParseDeviceRequestJSON()` | Lỗi validation rõ |
| BI-015 | P2 | Admin | Create bundle từ request | Có admin key, request hợp lệ | `CreateBundleFromRequest()` | Tạo bundle import được, ghi issuance history |
| BI-016 | P2 | Admin | List issuance history | Sau BI-015 | `ListIssuanceHistory()` | Có record mới, metadata đúng |
| BI-017 | P1 | Runtime lifecycle | Startup khi chưa authorized không start P2P | DB chỉ có identity hoặc DB trống | `Startup()` | Không start node, health/app state đúng |
| BI-018 | P0 | Runtime lifecycle | Startup khi authorized start runtime core | DB authorized, crypto engine/mock sẵn | `Startup()` | Health crypto ready, app state authorized, node status hợp lệ nếu P2P enabled |
| BI-019 | P1 | Runtime lifecycle | Shutdown idempotent | Runtime đã startup | Gọi `Shutdown()` hai lần | Không panic, node/coordinator dừng |
| BI-020 | P1 | Runtime health | Health cập nhật startup error | Inject lỗi crypto/P2P | `Startup()` | `GetRuntimeHealth()` có code/message/fatal đúng |
| BI-021 | P1 | Node status | Node status khi chưa chạy mạng | Runtime chưa start P2P | `GetNodeStatus()` | `IsRunning=false`, có PeerID/display name nếu DB có |
| BI-022 | P1 | Node status | Known peers merge DB và connected peers | Seed peer profile + fake connected peer | `GetKnownPeers()` | Không duplicate, display name ưu tiên verified token khi có |
| BI-023 | P1 | Session | Session active mặc định | Runtime authorized | `GetSessionStatus()` | Trạng thái active/not replaced |
| BI-024 | P1 | Session | Acknowledge replaced session | Seed state replaced hoặc gọi helper | `AcknowledgeSessionReplaced()` | UX flag được clear theo contract |
| BI-025 | P1 | Session | Mutation bị chặn khi session replaced | Runtime marked replaced | Gọi gửi tin/invite | Trả lỗi session replaced, đọc lịch sử vẫn được |
| BI-026 | P0 | Group | Tạo DM group | Runtime authorized, coordination mock/engine | `CreateGroupChat(groupID, "dm", "")` | Group được lưu, creator/member local có trong roster |
| BI-027 | P0 | Group | Tạo channel group | Runtime authorized | `CreateGroupChat(groupID, "channel", categoryID)` | Group type/channel metadata đúng trong `GetGroups()` |
| BI-028 | P0 | Group | Duplicate group ID | Group đã tồn tại | Tạo lại cùng ID | Lỗi rõ hoặc idempotent theo contract, không tạo duplicate |
| BI-029 | P0 | Group | GetGroups sau nhiều group | Có DM + channel + left group | `GetGroups()` | Danh sách đầy đủ, field DTO đúng, left group xử lý đúng theo UI contract |
| BI-030 | P1 | Group | GetGroupStatus group tồn tại | Có coordinator/group | `GetGroupStatus()` | Có epoch/member_count/token_holder/metrics hoặc field tương đương |
| BI-031 | P1 | Group | GetGroupStatus group không tồn tại | Không có group | `GetGroupStatus("missing")` | Không panic; lỗi/empty theo contract hiện tại |
| BI-032 | P0 | Members | Creator xuất hiện trong roster sau create | Sau BI-026 | `GetGroupMembers(groupID)` | Có local member role creator/active |
| BI-033 | P0 | Members | Generate key package | Runtime authorized | `GenerateKeyPackage()` | Public hex và bundle private hex không rỗng |
| BI-034 | P0 | Members | Add member bằng key package hợp lệ | Node A group, Node B KP | `AddMemberToGroup()` | Trả welcome hex, pending welcome/roster state hợp lệ |
| BI-035 | P0 | Members | Join group bằng welcome hợp lệ | Node B có welcome + private KP | `JoinGroupWithWelcome()` | Group xuất hiện ở B, roster có B |
| BI-036 | P0 | Members | Add member với KP sai | Group tồn tại, KP malformed | `AddMemberToGroup()` | Lỗi crypto/business rõ, state group không đổi |
| BI-037 | P0 | Members | GetGroupMembers sau join | A và B cùng group | `GetGroupMembers()` | A/B đều có trong roster, display name resolve đúng |
| BI-038 | P0 | Members | Leave group soft leave | Member trong group | `LeaveGroup()` | Group còn history local, member local marked left, mutation bị chặn |
| BI-039 | P0 | Members | Remove member happy path | Creator, target member active | `RemoveMemberFromGroup()` | Target marked removed/left, event members changed |
| BI-040 | P0 | Members | Remove member forbidden | Non-creator hoặc policy không cho phép | `RemoveMemberFromGroup()` | Trả `ERR_REMOVE_MEMBER_FORBIDDEN` hoặc mã tương đương |
| BI-041 | P0 | Members | Remove self bị chặn | Creator tự remove chính mình | `RemoveMemberFromGroup(localPeer)` | Trả `ERR_REMOVE_MEMBER_SELF` |
| BI-042 | P0 | Members | Remove peer chưa verified bị chặn | Target không verified | `RemoveMemberFromGroup()` | Trả `ERR_REMOVE_MEMBER_PEER_NOT_VERIFIED` |
| BI-043 | P0 | Members | Access revoked chặn mutation | Local bị marked removed | Send/invite/remove | Trả access revoked, không ghi side effect mới |
| BI-044 | P0 | Invite legacy | Generate join code | Runtime authorized | `GenerateJoinCode()` | Code chứa KP/public material, DB KP state cập nhật |
| BI-045 | P0 | Invite legacy | Pending invites trống ban đầu | Authorized node chưa có welcome | `ListPendingInvites()` | Trả mảng rỗng |
| BI-046 | P0 | Invite legacy | Save pending invite từ welcome (manual seed) | Seed welcome row trực tiếp qua `SavePendingInvite` | `ListPendingInvites()` | Có pending invite với group_id/group_type/source — kiểm tra storage layer, không trigger auto-join |
| BI-047 | P0 | Invite legacy | Accept invite happy path (manual recovery) | Có pending invite valid | `AcceptInvite(inviteID)` | Join group thành công, invite marked accepted — vẫn giữ làm idempotent recovery API dù UI bỏ |
| BI-048 | P0 | Invite legacy | Reject invite (chỉ áp dụng cho row còn pending) | Có pending invite chưa auto-joined | `RejectInvite(inviteID)` | Invite marked rejected, group không join |
| BI-049 | P0 | Invite legacy | Reopen rejected invite khi welcome mới | Invite rejected, reopen flag/flow | Save/fetch welcome mới | Invite trở lại pending nếu contract cho phép |
| BI-046b | P0 | Invite auto-join | Auto-join khi Welcome đến online | Runtime authorized, KP bundle đã persist | Gọi `savePendingInviteFromWelcome(...)` (proxy cho stream/replication/blind-store) | `HasGroup=true` ngay; row `pending_invites` ở `status=accepted`; emit `invite:auto_joined` |
| BI-046c | P0 | Invite auto-join | Auto-process pending Welcome lúc startup | Welcome đã ở `status=pending` từ session trước, KP có sẵn | `processPendingWelcomesOnStartup()` | Mỗi row pending hợp lệ chuyển sang `accepted`, group được join |
| BI-046d | P0 | Invite auto-join | Defer khi sidecar/KP chưa sẵn sàng | Runtime không có KP bundle | Gọi `savePendingInviteFromWelcome(...)` rồi sau đó persist KP và gọi `processPendingWelcomesOnStartup()` | Lần đầu giữ row `pending`, không join; lần sau join thành công, row chuyển `accepted` |
| BI-046e | P0 | Invite auto-join wire-path | End-of-pipe regression guard cho user journey "Alice mời Bob → Bob có trong nhóm" | Hai runtime mock; Bob persist KP, Alice cache KP của Bob qua `stored_keypackages` | Alice gọi `InvitePeerToGroup`; lấy welcome bytes từ `pending_welcomes_out`; chuyển vào `bob.savePendingInviteFromWelcome` (proxy wire delivery) | `bob.HasGroup=true` ngay (không Accept), row `pending_invites` ở `status=accepted` — fail test này = auto-join đã regress, KHÔNG dựa vào `JoinGroupWithWelcome` thủ công |
| BI-050 | P0 | Invite legacy | CheckDHTWelcome tên legacy nhưng dùng store | Store peers có welcome | `CheckDHTWelcome(groupID)` | Fetch/apply hoặc tạo pending invite đúng |
| BI-051 | P0 | Invite legacy | InvitePeerToGroup peer verified có KP | A/B verified, B advertised KP | `InvitePeerToGroup(B, groupID)` | Welcome delivered/replicated, không lỗi |
| BI-052 | P0 | Invite legacy | InvitePeerToGroup peer chưa verified | Target không verified | `InvitePeerToGroup()` | Lỗi rõ, không tạo welcome |
| BI-053 | P1 | Invite legacy | GetKPStatus sau advertise/refresh | Runtime authorized | `GetKPStatus()` | Trạng thái KP có public key/latest time/peer counts nếu có |
| BI-054 | P1 | Invite legacy | Resend pending welcome khi peer reconnect | Có pending welcome outbound | Simulate peer connected | Welcome resent hoặc marked delivered |
| BI-055 | P0 | Invite request | Get default invite policy | Group mới | `GetGroupInvitePolicy()` | Trả policy mặc định đúng |
| BI-056 | P0 | Invite request | Set invite policy creator-only | Creator local | `SetGroupInvitePolicy()` | Policy persist, list/get khớp |
| BI-057 | P0 | Invite request | Non-creator set policy bị chặn | Local không creator | `SetGroupInvitePolicy()` | Lỗi quyền, policy không đổi |
| BI-058 | P0 | Invite request | `any_member` policy: wire submit từ member auto-approve tại creator | Alice creator any_member, Bob member, Charlie KP cached | `alice.handleGroupInviteWireRPC(bob, submit Charlie)` | Trả record `status=approved` (creator là Token Holder, tự execute MLS Commit). Nếu fail → kiến trúc forward-to-creator đã regress |
| BI-059 | P0 | Invite request | Wire submit duplicate bị chặn/idempotent | Pending request cùng target seed sẵn ở creator's DB | Submit lại qua wire | Reject với `errInviteDuplicateActive` |
| BI-060 | P0 | Invite request | Wire submit cho target đã là member bị chặn | Target = Alice (creator) — đã ở roster | `alice.handleGroupInviteWireRPC(bob, submit alice)` | Reject "target is already a member" |
| BI-061 | P0 | Invite request | Approve request | Creator có pending request | `ApproveGroupInviteRequest()` | Status approved/processing/delivered đúng, welcome path được kích hoạt |
| BI-062 | P0 | Invite request | Reject request với reason | Creator có pending request | `RejectGroupInviteRequest()` | Status rejected, reason lưu |
| BI-063 | — | (removed) | Cancel request bởi requester — gỡ bỏ 2026-05-10 | Không còn API `CancelGroupInviteRequest`; muốn rút lại request thì requester chờ creator quyết, hoặc nếu đã auto-join thì dùng `LeaveGroup` |  |  |
| BI-063b | P0 | Invite request | `creator_approval` policy: wire submit để pending chờ creator | Creator policy creator_approval, member submit qua wire | `creator.handleGroupInviteWireRPC(member, submit)` | Trả record `status=pending` (không auto-execute), creator UI quyết Duyệt/Từ chối |
| BI-064 | P0 | Invite request | List requests filter status | Seed nhiều status | `ListGroupInviteRequests(statuses)` | Chỉ trả status yêu cầu |
| BI-065 | P0 | Invite request | List requests pagination | Seed nhiều request | `ListGroupInviteRequests(cursor, limit)` | Không trùng, cursor tiếp tục đúng |
| BI-066 | P1 | Invite request | Sync request from creator | Creator có bản mới hơn | `SyncInviteRequestFromCreator()` | Local requester cập nhật status |
| BI-067 | P1 | Invite request | Retry failed request | Request failed/transient | `RetryGroupInviteRequest()` | Status/attempt cập nhật, không duplicate |
| BI-068 | P1 | Invite request P2P | Submit request qua stream verified | Hai node verified, handler registered | Client submit RPC | Creator lưu record, trả response OK |
| BI-069 | P1 | Invite request P2P | Submit request từ peer chưa verified bị reject | Stream remote unverified | RPC submit | Response error, không lưu record |
| BI-070 | P0 | Messaging | Send DM text happy path | Group DM active | `SendGroupMessage()` | Message persisted, event `group:message`, `GetGroupMessages()` thấy |
| BI-071 | P0 | Messaging | Send empty/whitespace bị chặn | Group active | `SendGroupMessage("   ")` | Lỗi message empty, không ghi DB |
| BI-072 | P0 | Messaging | Send quá giới hạn | Lấy limit rồi tạo text vượt | `SendGroupMessage()` | Trả mã text too long |
| BI-073 | P0 | Messaging | GetGroupMessages pagination | Seed nhiều messages | `GetGroupMessages(limit, offset)` | Đúng số lượng, thứ tự ổn định |
| BI-074 | P0 | Messaging | Send sau leave/access revoked bị chặn | Group left/revoked | `SendGroupMessage()` | Lỗi đúng, không ghi message |
| BI-075 | P0 | Messaging | Retry failed message | Seed failed local message | `RetryMessage()` | Status chuyển theo contract hoặc gửi lại thành công |
| BI-076 | P0 | Messaging | Delete local message | Seed message | `DeleteLocalMessage()` | Message không còn trong list local |
| BI-077 | P0 | Channel | Validate channel post payload | Group channel active | Send payload post hợp lệ | `GetGroupPosts()` thấy post |
| BI-078 | P0 | Channel | Reject invalid post/comment payload | Payload thiếu type/content/parent | Send | Lỗi validation, không persist |
| BI-079 | P0 | Channel | Add comment vào post | Có post | Send reply/comment payload | `GetPostComments()` trả comment đúng parent |
| BI-080 | P0 | Channel | Get posts pagination | Seed nhiều post/comment | `GetGroupPosts(limit, offset)` | Chỉ post, không lẫn comment |
| BI-081 | P0 | Categories | Baseline categories | Runtime startup | `ListChannelCategories()` | Có danh mục mặc định nếu baseline expected |
| BI-082 | P0 | Categories | Create category | Runtime authorized | `CreateChannelCategory(name)` | DTO có ID/name/revision, list thấy |
| BI-083 | P0 | Categories | Reject duplicate/blank category | Có category hoặc blank name | `CreateChannelCategory()` | Lỗi validation, không duplicate |
| BI-084 | P0 | Categories | Assign category to channel | Có channel + category | `AssignChannelCategory()` | `GetGroups()` hoặc metadata cho channel khớp |
| BI-085 | P0 | Categories | Delete unused category | Category không gắn channel | `DeleteChannelCategory()` | List không còn |
| BI-086 | P1 | Categories | Delete category đang được dùng | Category gắn channel | `DeleteChannelCategory()` | Hoặc bị chặn, hoặc unassign đúng contract |
| BI-087 | P1 | Categories P2P | Sync category snapshot | Hai node verified, category changed | Trigger sync/broadcast | Node còn lại list category khớp |
| BI-088 | P1 | Offline sync | Trigger offline sync không peer | Runtime authorized, không peer | `TriggerOfflineSync()` | Không crash, status hợp lệ |
| BI-089 | P1 | Offline sync | Pull missed envelope từ peer | Hai node, one missed message | `TriggerOfflineSync()` hoặc peer connect | Missing message được apply/persist |
| BI-090 | P1 | Offline sync | Delivery ACK cập nhật cursor | Sau sync thành công | Query status/DB | Ack cursor tăng, không gửi lại quá mức |
| BI-091 | P1 | Blind store | Publish group envelope object | Local send message | Inspect fake blind-store publish | Object type `group-envelope`, routing key đúng |
| BI-092 | P1 | Blind store | Fetch key package/welcome from store peers | Store peer giữ object | Fetch flow | Object apply được, không cần target online |
| BI-093 | P2 | File transfer | Prepare outgoing file | Temp file nhỏ, group active | `PrepareOutgoingFileTransfer()` | DTO có file_id/name/size/chunks |
| BI-094 | P2 | File transfer | Reject missing/oversized file | Path sai hoặc vượt limit | Prepare | Lỗi rõ |
| BI-095 | P2 | File transfer | SendGroupFile announces transfer | Mock file dialog hoặc direct prepare+announce | `SendGroupFile()`/announce | Message/file event được tạo |
| BI-096 | P2 | File transfer | Download file happy path | Sender has prepared file | `DownloadGroupFile()` | File lưu local, DB state completed |
| BI-097 | P2 | File transfer | Download from wrong sender/missing file | FileID không tồn tại | `DownloadGroupFile()` | Lỗi rõ, state failed hoặc không đổi |
| BI-098 | P2 | File transfer | Open downloaded file | File completed local | `OpenDownloadedFile()` | Trả/open path, missing local trả `ERR_FILE_MISSING_LOCAL` |
| BI-099 | P1 | Runtime events | Runtime event cursor tăng | Emit vài event qua actions | `GetRuntimeEventCursor()` | Seq tăng monotonic |
| BI-100 | P1 | Runtime events | Get events since cursor | Có event seq 1..N | `GetRuntimeEventsSince(lastSeq, limit)` | Không trả event cũ, respect limit |
| BI-101 | P1 | Runtime events | Aggregate revisions cập nhật | Groups/messages/categories changed | `GetAggregateRevisions()` | Revision liên quan tăng |
| BI-102 | P2 | Diagnostics | Validate multiaddr hợp lệ | Runtime any | `ValidateMultiaddr(valid)` | Không lỗi |
| BI-103 | P2 | Diagnostics | Validate multiaddr sai | Runtime any | `ValidateMultiaddr(invalid)` | Lỗi validation |
| BI-104 | P2 | Diagnostics | Get/set network settings | Runtime with DB | `GetNetworkSettings()`, `SetBootstrapAddress()` | Persist config đúng |
| BI-105 | P2 | Diagnostics | Reconnect P2P | Authorized runtime | `ReconnectP2P()` | Node restart sạch hoặc trả lỗi rõ nếu chưa ready |
| BI-106 | P2 | Diagnostics | Export diagnostics | Runtime authorized | `ExportDiagnostics()` | File JSON tồn tại, parse được |
| BI-107 | P3 | Fork-heal history | List history từ DB seeded | Seed fork_heal_events/audit | `GetForkHealHistory()` | Trả summary + audit theo limit |
| BI-108 | P0 | Cross-cutting | API theo group missing | Không có group | Gọi send/members/invite/category assign | Trả `ERR_GROUP_NOT_FOUND` hoặc contract tương đương |
| BI-109 | P0 | Cross-cutting | Runtime chưa initialized | Runtime thiếu DB/crypto | Gọi API mutation | Trả `ERR_RUNTIME_NOT_INITIALIZED` hoặc lỗi rõ |
| BI-110 | P0 | Cross-cutting | Concurrent accept invite | Một pending invite | Hai goroutine `AcceptInvite()` | Chỉ một thành công/idempotent, state cuối đúng |
| BI-111 | P0 | Cross-cutting | Concurrent create same group | Runtime authorized | Hai goroutine `CreateGroupChat()` cùng ID | Không duplicate, state cuối nhất quán |
| BI-112 | P1 | Cross-cutting | Event sink nhận đúng event quan trọng | Fake event sink | Create group/send/leave/invite | Có event đúng tên/payload tối thiểu |
| BI-113 | P0 | E2E group integrity | Three-node end-to-end: Bob (member) invites Charlie under `any_member`, welcome carries inline `category_id` | Alice creates category + channel, sets policy `any_member`, Bob auto-joins via wire path; Charlie KP cached on Alice | `bob.handleGroupInviteWireRPC(submit Charlie)` → wire-deliver welcome to Charlie | Charlie `HasGroup=true`, `GroupRecord.CategoryID = catX`, `GroupType = "channel"`, members table chứa Alice (welcome-source); Alice's `GroupInvitePolicy = any_member`. Fail = bug "Node 3 mất danh mục" đã regress |
| BI-114 | P1 | E2E group integrity | Fallback path: welcome đến với `category_id=""` (legacy / replication chưa upgrade) | Same setup BI-113 | Charlie `savePendingInviteFromWelcome(..., categoryID="", ...)` | Charlie vẫn `HasGroup=true` (auto-join không bị block bởi thiếu metadata). Category sẽ phục hồi sau peer-connect qua `scheduleChannelCategorySync` |
| BI-115 | P0 | E2E creator-hint invariant | Defensive guard: caller writes `localID` as `sourcePeerID` into `savePendingInviteFromWelcome` | Same as BI-113 (Bob already auto-joined) | Replay welcome bytes with `sourcePeerID = bobInfo.PeerID` | `GetGroupInviteCreatorHint(group)` không trả về self; vẫn trỏ về Alice. Fail = bug "non-creator can't invite anymore" đã regress |
| BI-116 | P0 | E2E creator-hint invariant | `resolveGroupCreatorPeerID` end-to-end trên non-creator member | Bob đã auto-join | Gọi `bob.resolveGroupCreatorPeerID(group)` (1) qua members table, (2) sau khi xoá members → fallback hint | Cả 2 path đều trả Alice — đúng peer Bob phải forward `RequestGroupInvite` đến |
| BI-117 | P0 | E2E creator-hint invariant | Restart-replay: receiver reload welcome từ local `stored_welcomes` | Bob đã auto-join | Gọi `bob.fetchWelcomeFromStorePeers(group)` (kích hoạt local-row branch) | `stored_welcomes.source_peer_id` vẫn là Alice; `pending_invites.source_peer_id` không bị overwrite bằng self; creator hint resolve về Alice |
| BI-118 | P1 | Wire contract invariant | `WelcomeFetchResponseV1.SourcePeerID` được fill bởi responder | Bob đã có row stored welcome | Build response trực tiếp + verify `resp.SourcePeerID == inviter` | Đảm bảo refactor sau không drop trường này (silent JSON omit) khiến fetch chain mất creator hint |
| BI-119 | P0 | E2E creator-fetch fallback | Creator KHÔNG có target's KeyPackage cached, requester (Bob) đính kèm KP vào wire submit | `e2eAliceBobCharlie`, xoá Alice's `stored_keypackages` cho Charlie, seed KP vào Bob | Bob gửi `handleGroupInviteWireRPC(submit, TargetKeyPackage=charlieKP)` | Wire response `OK=true`, `Status=approved`; Alice's `stored_keypackages` có KP của Charlie sau khi xử lý; `pending_welcomes_out` có row cho Charlie. Fail = bug "ERR_INVITE_ADD_MEMBER_FAILED khi creator không kết nối target" đã regress |
| BI-120 | P1 | E2E backward compat | Wire submit không đính kèm `TargetKeyPackage` (legacy peer) | Same setup BI-119 nhưng giữ Alice's KP cache | Bob gửi `submit` với KP rỗng | Wire vẫn `OK=true`, `Status=approved` — fallback path giữ tương thích peer cũ |

## Nhóm P0 Tối Thiểu Cho Demo

Nếu cần chốt nhanh trước một buổi demo, chạy tối thiểu các scenario sau:

- `BI-001` đến `BI-006`: onboarding và authorized state.
- `BI-018`, `BI-019`: runtime start/stop.
- `BI-026` đến `BI-029`: tạo và đọc group.
- `BI-032` đến `BI-043`: thành viên, add/join/remove/leave.
- `BI-044` đến `BI-052`: join code, pending invite, invite peer cơ bản.
- `BI-055` đến `BI-065`: request/approve/reject/cancel invite theo policy.
- `BI-070` đến `BI-080`: gửi tin, history, post/comment.
- `BI-081` đến `BI-085`: category cơ bản nếu UI demo có channel sidebar.
- `BI-108` đến `BI-112`: lỗi chéo và race dễ phá demo.

## Gợi Ý Tổ Chức Test File

- `app/service/business_identity_integration_test.go`
- `app/service/business_admin_integration_test.go`
- `app/service/business_group_membership_integration_test.go`
- `app/service/business_invite_integration_test.go`
- `app/service/business_invite_request_integration_test.go`
- `app/service/business_messaging_integration_test.go`
- `app/service/business_channel_categories_integration_test.go`
- `app/service/business_offline_sync_integration_test.go`
- `app/service/business_file_transfer_integration_test.go`
- `app/service/business_runtime_events_integration_test.go`

Các file có thể dùng build tag riêng:

```go
//go:build business_integration
```

Lệnh chạy đề xuất:

```powershell
cd app
go test -tags=business_integration ./service -count=1 -run TestBusiness
```

## Definition Of Done Cho Một Scenario

Một scenario được xem là đủ khi có:

- Setup rõ fixture/harness.
- Hành động gọi API public hoặc protocol boundary rõ ràng.
- Assertion trên ít nhất một output nghiệp vụ: DTO, DB state, runtime event hoặc error code.
- Cleanup temp DB/file/goroutine.
- Không phụ thuộc thứ tự test khác.
- Không sleep thời gian thật nếu có cách trigger trực tiếp hoặc fake clock.
