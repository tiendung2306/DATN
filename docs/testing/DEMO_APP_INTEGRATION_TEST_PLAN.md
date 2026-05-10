# Demo App Integration Test Plan

Tài liệu này là kế hoạch triển khai integration test cho các tính năng **business/demo** của ứng dụng cộng tác. Mục tiêu là giảm tình trạng "làm tính năng mới thì hỏng tính năng cũ", đặc biệt các luồng demo như onboarding, tạo nhóm, mời người vào nhóm, nhắn tin, channel, invite approval và các trạng thái lỗi phổ biến.

Tham chiếu checklist kịch bản: `docs/testing/BUSINESS_INTEGRATION_TEST_SCENARIOS.md`.

## 1. Phạm Vi

### In Scope

- Test các API public trên `service.Runtime` mà frontend Wails đang dùng.
- Test persistence qua SQLite thật ở temp directory.
- Test DTO, runtime events, aggregate revisions, error code, side effects trong DB.
- Test một số luồng hai peer bằng fake/in-process harness khi cần chứng minh nghiệp vụ giữa hai node.
- Test smoke P2P thật chỉ cho luồng demo trọng yếu, chạy riêng vì dễ chậm/flaky.

### Out Of Scope

- Chứng minh tính đúng đắn sâu của MLS/OpenMLS.
- Chứng minh thuật toán Single-Writer, Epoch, Fork Healing, HLC.
- Benchmark hiệu năng Phase 9.
- E2E UI tự động bằng Playwright/Wails binary. Có thể làm sau, nhưng không phải lớp đầu tiên.

## 2. Mục Tiêu Chất Lượng

Integration suite này phải trả lời được các câu hỏi:

- App có khởi tạo đúng trạng thái demo không?
- Tạo nhóm, tạo channel, tạo DM có ổn không?
- Mời người khác vào nhóm có tạo welcome/pending invite/roster đúng không?
- Người được mời accept/join rồi gửi và nhận tin được không?
- Các lỗi thường gặp có được trả bằng error code ổn định thay vì fail im lặng không?
- Frontend có đủ event/DTO để refresh UI sau action không?
- Refactor invite/channel/message có làm gãy dữ liệu cũ hoặc API runtime không?

## 3. Test Pyramid Cho App Demo

### Lớp 1: Service Integration Fast Suite

Đây là lớp chính, chạy nhiều nhất.

- Package: `app/service`.
- DB: SQLite temp file thật.
- Crypto: mock `coordination.MLSEngine` hoặc real sidecar tùy scenario.
- P2P: fake transport/fake node hoặc direct method call.
- Event: fake `EventSink` thu event vào memory.
- Tốc độ mục tiêu: dưới 30-60 giây cho P0.

Lệnh đề xuất:

```powershell
cd app
go test -tags=business_integration ./service -count=1 -run TestBusinessP0
go test -tags=business_integration ./service -count=1 -run TestBusinessP1_Sprint2
```

### Lớp 2: In-Process Multi-Node Business Suite

Dùng cho các luồng cần hai node nhưng không nhất thiết cần libp2p thật.

- Mỗi node có temp DB riêng.
- Mỗi node có `Runtime` riêng.
- Dùng fake network/in-memory bridge để mô phỏng peer verified, delivery welcome, offline envelope hoặc invite request RPC.
- Dùng real SQLite để bắt lỗi persistence.
- Có thể dùng mock MLS deterministic để tạo welcome/message state giả nhưng ổn định.

Lệnh đề xuất:

```powershell
cd app
go test -tags=business_integration ./service -count=1 -run TestBusinessMultiNode
```

### Lớp 3: Real P2P Smoke Suite

Dùng rất ít, chỉ trước demo hoặc nightly.

- Hai hoặc ba runtime thật.
- DB riêng, port riêng.
- Có thể chạy Rust sidecar thật.
- Kiểm tra "happy path" end-to-end: admin tạo bundle, hai node verified, tạo group, invite, accept, gửi tin.
- Không nên chạy trên mỗi lần save vì dễ flaky do network/timing.

Lệnh đề xuất sau khi có harness:

```powershell
cd app
go test -tags=business_smoke ./service -count=1 -run TestSmokeDemo
```

### Lớp 4: Manual Demo Checklist

Vẫn cần checklist tay trước buổi bảo vệ/demo:

- Chạy app node admin.
- Chạy app node user 1 và user 2 với DB riêng.
- Tạo group/channel.
- Mời user 2.
- User 2 accept.
- Gửi tin hai chiều.
- Restart một node, kiểm tra history và tiếp tục gửi.

Manual checklist không thay thế automated integration test, nhưng giúp bắt lỗi UI/window/dialog mà Go test không thấy.

## 4. Cấu Trúc Thư Mục Đề Xuất

```text
app/service/
  business_test_harness_test.go
  business_identity_integration_test.go
  business_admin_integration_test.go
  business_group_membership_integration_test.go
  business_invite_integration_test.go
  business_invite_request_integration_test.go
  business_messaging_integration_test.go
  business_channel_categories_integration_test.go
  business_runtime_events_integration_test.go
  business_offline_sync_integration_test.go
  business_file_transfer_integration_test.go
```

Tất cả file integration nên dùng build tag để không làm chậm test thường:

```go
//go:build business_integration
```

Smoke P2P thật nên dùng tag riêng:

```go
//go:build business_smoke
```

## 5. Test Harness Cần Xây

### 5.1. `businessTestNode`

Một node test nên gom các thành phần:

```go
type businessTestNode struct {
    Name       string
    Runtime    *Runtime
    DBPath     string
    TempDir    string
    Events     *fakeEventSink
    PeerID     string
    PublicKey  string
    Cleanup    func()
}
```

Nhiệm vụ:

- Tạo temp dir và temp SQLite DB.
- Tạo config test với port/network tắt hoặc port random.
- Tạo runtime.
- Gắn fake event sink.
- Seed identity/auth bundle nếu scenario cần authorized state.
- Cleanup runtime, DB, temp files.

### 5.2. `fakeEventSink`

Event sink cần capture:

- Tên event.
- Payload map.
- Thứ tự emit.
- Helper `WaitForEvent(name)` chỉ dùng timeout ngắn khi thật sự cần async.

Assertion thường gặp:

- `app:state_changed`
- `node:status`
- `runtime:health`
- `group:joined`
- `group:message`
- `group:epoch`
- `group:left`
- `group:members_changed`
- `runtime:event`

### 5.3. Identity/Admin Fixtures

Fixture cần hỗ trợ:

- `newUninitializedNode(t, name)`
- `newNodeWithKeys(t, name)`
- `newAuthorizedNode(t, name)`
- `authorizeNodeWithAdmin(t, admin, user)`
- `makeValidBundle(t, admin, user)`
- `makeTamperedBundle(t, admin, user)`

Không nên copy-paste JSON bundle trong test. Hãy tạo bằng helper để luôn bám schema hiện tại.

### 5.4. Mock MLS Engine

Vì đây là test nghiệp vụ, mock engine cần deterministic:

- `GenerateKeyPackage` trả public/private bundle giả hợp lệ theo format test.
- `CreateGroup` trả group state có groupID/epoch.
- `AddMembers` trả welcome bytes deterministic.
- `ProcessWelcome` trả group state mới.
- `EncryptMessage`/`DecryptMessage` có thể wrap/unwrap plaintext đơn giản.
- `HasMember` trả theo roster fixture khi test remove/revoked.

Không kiểm tra bảo mật của mock. Chỉ dùng để làm business flow ổn định.

Các test muốn bắt lỗi integration thật với sidecar có thể nằm trong smoke riêng.

### 5.5. Fake P2P / Verified Peer Harness

Các luồng invite cần peer identity và trạng thái verified. Harness nên có:

- `connectVerified(a, b)` để đánh dấu hai runtime nhìn thấy nhau như verified peers.
- `advertiseKeyPackage(node)` để seed KP public.
- `deliverWelcome(sender, receiver, groupID)` hoặc fake store peer.
- `submitInviteRequest(requester, creator, request)` để gọi handler/RPC in-memory.

Mục tiêu không phải mô phỏng libp2p đầy đủ, mà là mô phỏng đúng **điều kiện nghiệp vụ** mà Runtime yêu cầu: peer verified, có KP, có welcome, có creator, có group.

### 5.6. DB Assertions

Nên có helper đọc qua public API trước, chỉ đọc DB trực tiếp khi API không expose đủ:

- `assertGroupExists`
- `assertGroupType`
- `assertMemberStatus`
- `assertPendingInviteStatus`
- `assertInviteRequestStatus`
- `assertMessageCount`
- `assertRuntimeEvent`
- `assertAggregateRevisionIncreased`

Ưu tiên assert bằng Runtime API:

- `GetGroups`
- `GetGroupMembers`
- `ListPendingInvites`
- `ListGroupInviteRequests`
- `GetGroupMessages`
- `GetRuntimeEventsSince`

## 6. Setup Test Từng Loại

### 6.1. Service Integration Single Node

Áp dụng cho:

- App state.
- Admin.
- Create group.
- Category.
- Message validation.
- Runtime events.
- Diagnostics.

Setup:

1. Tạo `businessTestNode`.
2. Nếu cần authorized, seed identity + auth bundle.
3. Gọi Runtime API.
4. Assert DTO/event/DB.
5. Cleanup.

Ví dụ test intent:

```text
TestBusinessP0_CreateChannel_PersistsGroupAndCreatorRoster
  arrange: authorized node Alice
  act: CreateGroupChat("team-general", "channel", "")
  assert: GetGroups has channel, GetGroupMembers has Alice as creator
```

### 6.2. Service Integration Two Nodes In-Process

Áp dụng cho:

- Add member/join by welcome.
- Invite peer to group.
- Pending invite accept/reject.
- Invite request approve/reject/cancel.
- Offline sync cơ bản.

Setup:

1. Tạo admin node hoặc admin fixture.
2. Tạo Alice authorized.
3. Tạo Bob authorized.
4. `connectVerified(Alice, Bob)`.
5. Alice tạo group.
6. Bob advertise KP hoặc generate join code.
7. Thực hiện flow invite/join/message.
8. Assert cả hai node.

Ví dụ test intent:

```text
TestBusinessP0_InvitePeerToGroup_DeliversWelcomeAndBobCanJoin
  arrange: Alice and Bob authorized + verified, Alice has group
  act: Bob advertises KP; Alice InvitePeerToGroup(Bob, group)
  assert: Bob has pending invite or joined group depending path; accept -> Bob GetGroups contains group
```

### 6.3. Real P2P Smoke

Áp dụng cho đúng 1-3 happy paths:

- Hai node connect/auth.
- Invite peer to group.
- Gửi tin sau join.

Setup:

1. Build Rust sidecar nếu cần.
2. Tạo temp root dir cho mỗi node.
3. Chạy runtime với port random/explicit.
4. Dùng bootstrap addr của Alice cho Bob.
5. Wait verified với timeout rõ.
6. Chạy flow.
7. Shutdown cả hai.

Quy tắc:

- Mỗi wait phải có timeout.
- Log diagnostics khi fail.
- Không chạy trong suite P0 mặc định nếu flaky.

## 7. Thứ Tự Triển Khai Đề Xuất

### Sprint 1: Nền Harness Và App State

Mục tiêu: có khung chạy ổn định.

Implement:

- `businessTestNode`.
- `fakeEventSink`.
- Temp DB/config helper.
- Authorized node fixture.
- Tests: `BI-001` đến `BI-006`, `BI-017` đến `BI-021`.

Exit criteria:

- Chạy được `go test -tags=business_integration ./service -run TestBusinessP0_AppState`.
- Không cần P2P thật.

### Sprint 2: Group, Members, Messaging Core

Mục tiêu: bảo vệ luồng tạo nhóm và chat cơ bản.

Implement:

- Mock MLS engine deterministic.
- Group/member assertion helpers.
- Message assertion helpers.
- Tests: `BI-026` đến `BI-043`, `BI-070` đến `BI-080`, `BI-108` đến `BI-112`.

Exit criteria:

- Demo local single-node không bị gãy.
- Tạo group, list member, gửi tin, history, post/comment có test.

### Sprint 3: Invite Legacy Và Pending Invite

Mục tiêu: chặn regression "không mời được người khác vào nhóm".

Implement:

- Two-node in-process harness.
- Fake verified peer.
- Fake KP advertisement/store.
- Fake welcome delivery/store.
- Tests: `BI-044` đến `BI-054`.

Exit criteria:

- Test chứng minh Alice mời Bob và Bob accept/join được.
- Test chứng minh peer chưa verified hoặc thiếu KP trả lỗi rõ.

### Sprint 4: Invite Approval Policy

Mục tiêu: bảo vệ luồng mới `group_invite_requests`.

Implement:

- Request record assertion helper.
- Creator/requester role fixtures.
- Optional fake wire RPC.
- Tests: `BI-055` đến `BI-069`.

Exit criteria:

- Request/approve/reject/cancel/list/pagination có coverage.
- Policy creator-only/any-member có coverage.

### Sprint 5: Channel Category, Runtime Events, Diagnostics

Mục tiêu: bảo vệ UI shell và sidebar/workspace behavior.

Implement:

- Category tests: `BI-081` đến `BI-087`.
- Runtime events: `BI-099` đến `BI-101`.
- Diagnostics/network settings: `BI-102` đến `BI-106`.

Exit criteria:

- Frontend có đủ event/revision để refresh UI sau action.

### Sprint 6: Offline Sync, Blind Store, File Transfer Smoke

Mục tiêu: bảo vệ các tính năng demo nâng cao.

Implement:

- Offline/blind-store harness vừa đủ.
- File transfer temp file tests.
- Tests: `BI-088` đến `BI-098`.

Exit criteria:

- Offline sync không phá message path.
- File transfer prepare/download/open basic hoạt động.

### Sprint 7: Real P2P Demo Smoke

Mục tiêu: xác nhận hệ thống ghép thật hoạt động trước demo.

Implement:

- `business_smoke` tag.
- Two real runtime nodes.
- Happy path: authorized, verified, create group, invite, accept, send message.

Exit criteria:

- Có lệnh smoke chạy trước demo.
- Khi fail, export diagnostics/log rõ.

## 8. P0 Demo Suite Cụ Thể

Đây là danh sách test nên có trước tiên:

```text
TestBusinessP0_AppState_GenerateKeysAndImportBundle
TestBusinessP0_Runtime_StartupShutdownAuthorized
TestBusinessP0_Group_CreateDMAndChannel
TestBusinessP0_Group_DuplicateGroupDoesNotCorruptState
TestBusinessP0_Members_CreatorRosterAfterCreate
TestBusinessP0_Members_AddMemberAndJoinWithWelcome
TestBusinessP0_Members_LeaveGroupBlocksMutationButKeepsHistory
TestBusinessP0_Members_RemoveMember_ErrorMatrix
TestBusinessP0_Invite_GenerateJoinCodeAndListPending
TestBusinessP0_Invite_AcceptAndRejectPendingInvite
TestBusinessP0_Invite_InvitePeerToGroupRequiresVerifiedPeerAndKP
TestBusinessP0_InviteRequest_RequestApproveRejectCancel
TestBusinessP0_InviteRequest_ListPaginationAndStatusFilter
TestBusinessP0_Messaging_SendGetRetryDelete
TestBusinessP0_Messaging_TextValidationMatrix
TestBusinessP0_Channel_PostCommentPagination
TestBusinessP0_Categories_CreateAssignDelete
TestBusinessP0_CrossCutting_GroupNotFoundRuntimeNotInitialized
TestBusinessP0_CrossCutting_ConcurrentAcceptInvite
```

## 9. Data Fixture Quy Ước

Dùng tên cố định để đọc test dễ:

- Admin: `Root Admin`
- Alice: creator/admin node
- Bob: invited member
- Charlie: target/requested member
- Mallory: invalid/unverified peer

Group IDs:

- DM: `dm-alice-bob`
- Channel: `team-general`
- Private channel: `team-security`

Category IDs/names:

- `cat-general` / `General`
- `cat-projects` / `Projects`

Text:

- Normal message: `hello from alice`
- Channel post title: `Sprint update`
- Channel post body: `Today we finished invite flow validation.`

## 10. Error Contract Cần Assert

Không nên assert toàn bộ chuỗi lỗi dài. Chỉ assert prefix/code:

- `ERR_RUNTIME_NOT_INITIALIZED`
- `ERR_GROUP_NOT_FOUND`
- `ERR_MESSAGE_EMPTY`
- `TEXT_TOO_LONG` hoặc code đang dùng trong `formatSendError`
- `ERR_REMOVE_MEMBER_FORBIDDEN`
- `ERR_REMOVE_MEMBER_SELF`
- `ERR_REMOVE_MEMBER_PEER_NOT_VERIFIED`
- `ERR_REMOVE_MEMBER_ACCESS_REVOKED`
- `ERR_FILE_NOT_DOWNLOADED`
- `ERR_FILE_MISSING_LOCAL`
- `ERR_FILE_OPEN_FAILED`

Nếu một API chưa có error code ổn định, nên thêm issue/TODO trong test plan thay vì assert chuỗi tiếng Anh không ổn định.

## 11. Event Contract Cần Assert

Các event quan trọng cho frontend:

- `app:state_changed`
- `startup:progress`
- `startup:error`
- `runtime:health`
- `node:status`
- `group:joined`
- `group:message`
- `group:epoch`
- `group:left`
- `group:members_changed`
- `runtime:event`
- `offline_sync:status`
- `file:prepare`
- `file:sent`
- `file:received`

Với event payload, assert field tối thiểu:

- Có `group_id` với event group.
- Có `reason` với `group:left` và `group:members_changed` nếu contract yêu cầu.
- Có sequence/revision với runtime event nếu có.
- Không assert toàn bộ payload nếu frontend không phụ thuộc.

## 12. CI Và Lệnh Chạy

### Test nhanh khi đang phát triển

```powershell
cd app
go test ./service -count=1
go test -tags=business_integration ./service -count=1 -run TestBusinessP0
```

### Trước khi merge feature chạm business flow

```powershell
cd app
go test ./... -count=1
go vet ./...
go test -tags=business_integration ./service -count=1
```

Nếu có đổi Wails exported API:

```powershell
cd app
wails generate module
cd frontend
npm run build
```

Nếu có đổi Rust/proto/MLS path:

```powershell
cd crypto-engine
cargo test
```

### Trước demo

```powershell
cd app
go test -tags=business_integration ./service -count=1
go test -tags=business_smoke ./service -count=1 -run TestSmokeDemo
cd frontend
npm run build
```

Sau đó chạy manual checklist hai node.

## 13. Quy Tắc Chống Flaky

- Không dùng `time.Sleep` dài trong test. Dùng condition wait với timeout ngắn và log rõ.
- Mỗi test dùng temp DB riêng.
- Không share global runtime/node giữa tests.
- Cleanup bằng `t.Cleanup`.
- Nếu test dùng port, lấy port random hoặc port từ config test.
- Với async event, wait theo event name/condition, không wait theo số milliseconds cố định.
- Không phụ thuộc thứ tự map/list nếu API không cam kết sort; test nên sort trước khi assert hoặc assert theo ID.
- Không gọi network thật trong `business_integration`; network thật chỉ trong `business_smoke`.

## 14. Khi Một Test Business Fail Thì Debug Như Nào

1. Xác định fail ở lớp nào: DTO, DB, event, error code hay async delivery.
2. In diagnostics của node test: app state, node status, groups, members, pending invites, invite requests.
3. Nếu fail do async, kiểm tra event đã emit chưa trước khi tăng timeout.
4. Nếu fail sau thay đổi API Go, chạy lại `wails generate module` và `npm run build`.
5. Nếu fail invite, kiểm tra theo thứ tự:
   - Hai peer có identity/auth bundle không?
   - Peer target có verified không?
   - Target có key package public không?
   - Welcome có được lưu/deliver không?
   - Pending invite có được tạo không?
   - Accept có gọi `JoinGroupWithWelcome` không?
6. Nếu fail message, kiểm tra:
   - Group active và local còn member không?
   - Session có replaced/access revoked không?
   - Message validation có chặn text không?
   - Stored message có ID/status/HLC không?

## 15. Definition Of Done Cho Kế Hoạch Test Demo

Kế hoạch này được xem là hoàn thành khi:

- Có P0 business integration suite chạy ổn định bằng một lệnh.
- Luồng "Alice tạo nhóm, mời Bob, Bob join, hai bên nhắn tin" có ít nhất một test automated.
- Các lỗi thường gặp trong invite/member/message có test error-code.
- Runtime event quan trọng cho UI có test.
- CI hoặc checklist dev có lệnh chạy suite.
- Có real P2P smoke trước demo hoặc manual checklist thay thế nếu chưa đủ thời gian.

## 16. Mapping Nhanh Từ Kế Hoạch Sang Checklist

- Sprint 1 map `BI-001` đến `BI-025`.
- Sprint 2 map `BI-026` đến `BI-043`, `BI-070` đến `BI-080`, `BI-108` đến `BI-112`.
- Sprint 3 map `BI-044` đến `BI-054`.
- Sprint 4 map `BI-055` đến `BI-069`.
- Sprint 5 map `BI-081` đến `BI-087`, `BI-099` đến `BI-106`.
- Sprint 6 map `BI-088` đến `BI-098`.
- Sprint 7 map một số P0 thành smoke thật: onboarding, verified peer, invite, accept, message.
