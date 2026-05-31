# BÁO CÁO PHÂN TÍCH KIẾN TRÚC: CÁC VẤN ĐỀ TỒN ĐỌNG TRONG HỆ THỐNG PHÂN TÁN MLS P2P

> [!NOTE]
> Tài liệu này được biên soạn bởi **Senior Architect** nhằm phân tích chuyên sâu các dị thường (anomalies) và rủi ro kỹ thuật còn tồn đọng trong giao thức điều phối phi tập trung MLS trên mạng ngang hàng (P2P). Đây là những điểm nghẽn kiến trúc sâu sắc mà các giải pháp sửa lỗi tình thế (workarounds) chưa thể xử lý triệt để.

---

## 1. Dị thường "Đuổi hình bắt bóng" trong Recovery Replay (Stale Replay Starvation)

### Hoàn cảnh xuất hiện (Context)
Hiện tượng này xảy ra khi một nút (Node) bị mất kết nối mạng vật lý tạm thời (hoặc tắt ứng dụng trong thời gian dài) rồi kết nối trở lại vào mạng GossipSub, kích hoạt luồng tự động phục hồi và đồng bộ dữ liệu lịch sử (`Recovery & Catch-up`).

### Nguyên nhân kỹ thuật (Reason)
Trong một mạng P2P bất đồng bộ, các Commit (thay đổi thành viên, xoay vòng khóa) thường được truyền đi qua luồng Gossip trực tiếp rất nhanh, hoặc được gom nhóm đồng bộ trước.
1. Khi một nút vừa online trở lại, nó nhận được các thông báo Commit mới nhất và nhanh chóng nâng trạng thái `c.epoch` cục bộ từ $e \to e+5$.
2. Cùng lúc đó, luồng đồng bộ offline chậm (`GetPendingEnvelopes`) mới bắt đầu kéo các tin nhắn ứng dụng (`MsgApplication`) được gửi ở các epoch cũ $e+1$ và $e+2$ về database để chạy lại (`ReplayEnvelopes`).
3. Khi chạy hàm `ReplayEnvelopesDetailed` cho các tin nhắn ứng dụng này, hệ thống sẽ thực hiện kiểm tra biểu thức Grace Window:
   $$\text{Epoch}_{\text{msg}} + 3 \ge \text{Epoch}_{\text{cục bộ}}$$
   Vì tin nhắn ở epoch $e+1$, mà trạng thái cục bộ đã bị kéo vọt lên $e+5$ từ trước do Commit, biểu thức trở thành:
   $$(e+1) + 3 = e+4 < e+5 \quad (\text{Thất bại!})$$
4. Hệ thống phân loại tin nhắn này là **Stale** (`ActionRejectStale`) và trả về mã trạng thái `ReplayStateStaleEpoch`.
5. Trong luồng `replayPendingEnvelopesForGroup` (`recovery_replay.go`), do chúng ta bỏ qua lỗi `ReplayStateStaleEpoch` để tránh làm nghẽn hàng đợi (Livelock), hệ thống sẽ **bỏ qua tin nhắn này và vĩnh viễn đánh dấu là đã xử lý thành công (Applied)**.

### Vấn đề hiện tại (Current Issue)
* **Mất dữ liệu âm thầm:** Nút phục hồi sẽ bị **mất vĩnh viễn toàn bộ các tin nhắn ứng dụng** được gửi trong các epoch trung gian ($e+1, e+2$) mặc dù về mặt mật mã học, các stale keys của OpenMLS hoàn toàn có thể giải mã được chúng nếu chúng được chạy đúng thứ tự.
* **Mất tính nhất quán dữ liệu:** Giao diện người dùng sẽ bị lệch pha dữ liệu lịch sử giữa nút bị offline và các nút online liên tục.

---

## 2. Xung đột Mật mã học giữa Forward Secrecy và Stale Decryption Window

### Hoàn cảnh xuất hiện (Context)
Đây là mâu thuẫn trực tiếp giữa **Tính khả dụng của ứng dụng (User Experience/Availability)** và **Tính an toàn tuyệt đối của mật mã học Zero-Trust (Security/Forward Secrecy)** trong thư viện OpenMLS (lớp Rust sidecar) và lớp điều phối (lớp Go).

### Nguyên nhân kỹ thuật (Reason)
* **Yêu cầu khả dụng:** Để hỗ trợ giải mã các tin nhắn đến muộn (do trễ mạng hoặc lệch epoch nhẹ), thư viện mật mã MLS bắt buộc phải duy trì một bộ nhớ đệm lưu trữ các khóa giải mã của các epoch cũ (gọi là **Stale Epoch Keys Cache**, ví dụ lưu tối đa 3 epoch gần nhất).
* **Nguyên nhân mất an toàn:** MLS (RFC 9420) yêu cầu tính năng **Forward Secrecy** cực kỳ nghiêm ngặt. Khi một nút tiến lên epoch mới, toàn bộ khóa của epoch cũ phải bị hủy ngay lập tức (Crypto-shredding) để ngăn chặn kẻ tấn công giải mã ngược lịch sử nếu thiết bị vật lý bị xâm nhập (compromised) trong tương lai.

### Vấn đề hiện tại (Current Issue)
* **Thất bại giải mã ngầm:** Nếu chúng ta cấu hình OpenMLS bảo mật cao (hủy khóa ngay khi đổi epoch), cơ chế nới lỏng Grace Window (+3 epoch) ở tầng Go hoàn toàn vô dụng. Tầng Go cho phép tin nhắn đi qua, nhưng tầng Rust sidecar sẽ trả về lỗi `DecryptFailed` do không còn khóa.
* **Rò rỉ khóa (Key Exposure):** Nếu chúng ta ép Rust giữ lại stale keys để phục vụ Go, chúng ta sẽ mở ra một cửa sổ thời gian (window of vulnerability) dài. Kẻ tấn công chiếm được thiết bị ở epoch $e$ sẽ đọc ngược được tin nhắn ở epoch $e-3$.

---

## 3. Mất mát Đề xuất Im lặng (Silent Proposal Dropping)

### Hoàn cảnh xuất hiện (Context)
Lỗi này xảy ra khi mạng có tần suất tương tác cao (Key rotation, Join, Leave diễn ra liên tục) hoặc mạng có độ trễ lớn khiến các đề xuất (Proposals) của các nút bị cạnh tranh lẫn nhau.

### Nguyên nhân kỹ thuật (Reason)
Giả sử tại Epoch $e$, cả Alice và Charlie đồng thời gửi đề xuất cập nhật trạng thái nhóm (ví dụ `ProposeUpdate` xoay vòng khóa):
1. Đề xuất của Alice đến Token Holder trước. Token Holder lập tức tạo Commit, đẩy epoch của cả nhóm lên $e+1$.
2. Đề xuất của Charlie mang nhãn epoch $e$ truyền trễ đến sau. Token Holder kiểm tra thấy epoch $e$ đã cũ so với epoch $e+1$ hiện tại, liền áp dụng luật `ActionRejectStale` và **lẳng lặng loại bỏ đề xuất này** (`handleProposalLocked`).
3. Về phía Charlie, khi nhận được bản tin Commit của Alice, Charlie tự động nâng epoch cục bộ lên $e+1$ và **tự động xóa sạch hàng đợi đề xuất của chính mình** vì nghĩ rằng "epoch đã thay đổi, đề xuất cũ không còn giá trị".

### Vấn đề hiện tại (Current Issue)
* **Hành động người dùng bị "bỏ rơi":** Đề xuất của Charlie bị hủy im lặng mà không hề có bất kỳ cơ chế phản hồi lỗi (Error Callback) nào báo về cho ứng dụng hoặc người dùng.
* **Trải nghiệm kém:** Người dùng thực hiện thao tác (nhêu đổi tên nhóm hoặc thêm thành viên) nhưng hệ thống không có phản hồi, trạng thái bị đóng băng vĩnh viễn cho đến khi họ nhận ra và bấm thực hiện lại thủ công.

---

## 4. Mất mát dữ liệu nhánh thua khi Phục hồi Phân mảnh (Fork Healing Data Loss) [ĐÃ GIẢI QUYẾT TRONG MILESTONE 5 ✅]

### Hoàn cảnh xuất hiện (Context)
Hiện tượng Split-brain xảy ra khi mạng bị chia cắt làm hai phân mảnh (Partition 1 và Partition 2) hoạt động độc lập, mỗi bên tự bầu ra Token Holder riêng và tự tiến hóa trạng thái epoch của mình.

### Nguyên nhân kỹ thuật (Reason)
Khi mạng kết nối lại, cơ chế **Fork Healing** sẽ chọn phân mảnh có epoch cao hơn (hoặc nhiều thành viên hơn) làm nhánh thắng (Winning Branch). Giả sử Nhánh 2 thắng, các nút ở Nhánh 1 bắt buộc phải từ bỏ nhánh của mình để nhập vào Nhánh 2.
1. Để đảm bảo tính nhất quán mật mã và an toàn Forward Secrecy, luật số 13 bắt buộc các nút ở Nhánh 1 phải thực hiện **External Join** vào Nhánh 2, đồng thời hủy bỏ toàn bộ dữ liệu khóa của Nhánh 1 cũ (Crypto-shredding).
2. Toàn bộ các tin nhắn ứng dụng đã được gửi và giải mã thành công ở Nhánh 1 trước đó sẽ bị **mồ côi** (orphaned) vì chúng không tồn tại trong lịch sử của Nhánh 2.
3. Theo luật số 12 (Non-repudiation), một nút chỉ được phép mã hóa lại và resend tin nhắn của **chính mình**, không được phép resend tin nhắn hộ nút khác để tránh giả mạo danh tính (spoofing).

### Giải pháp và Kết quả kiểm thử (Milestone 5 Hardening)
Trong **Milestone 5 (2026-05-31)**, hệ thống đã được nâng cấp lên kiến trúc **Senior-Grade Crash-Safe Fork Healing State Machine**:
* **Bảo vệ chống Crash/Restart đột ngột:** Trạng thái fork healing đã được chuyển thành mô hình bền vững (Durable State Machine) lưu trong SQLite. Job ID được xác định duy nhất (`job_id PRIMARY KEY`), phân tách các lần fork liên tiếp thông qua index duy nhất có điều kiện (`idx_fork_healing_active_group`), loại bỏ tình trạng stuck job do trùng khóa chính nhóm.
* **Outbox Replay Queue bền vững:** Tin nhắn ứng dụng mồ côi (Orphaned) của chính node được thu thập và lưu vào bảng replay ngoại tuyến trước khi broadcast. Đảo thứ tự lưu log ngoại tuyến trước khi phát tin nhắn qua GossipSub, đồng thời propagate lỗi ghi DB thay vì nuốt lỗi, ngăn chặn hoàn toàn việc mất tin nhắn replayed khi crash trước khi broadcast.
* **Cách ly sự kiện đa luồng:** Thêm `job_id` vào `application_event` để cách ly các sự kiện giải mã và replay giữa các job fork khác nhau, tránh xung đột hoặc ghi đè dữ liệu.
* **Đồng bộ hóa an toàn:** Khi restart trong trạng thái `EXTERNAL_JOINED`, node có thể khôi phục trực tiếp từ `PendingGroupState` lưu trữ trên đĩa để tự thực hiện state swap hoàn tất vòng đời mà không phụ thuộc vào winner peer online.

---

# ĐỀ XUẤT CẢI TIẾN KIẾN TRÚC CHO PHA TIẾP THEO

> [!TIP]
> Để nâng tầm dự án DATN này đạt mức xuất sắc tối đa, chúng ta nên đề xuất hoặc thiết kế các giải pháp khắc phục sau:

1. **Strict Replay Sequencer:**
   Thay đổi logic trong `recovery_replay.go` để không replay một cách mù quáng theo HLC vật lý thuần túy. Hàng đợi replay offline phải được sắp xếp theo **Epoch trước, HLC sau**. Xử lý toàn bộ tin nhắn ứng dụng thuộc epoch $e$ hoàn chỉnh trước khi áp dụng bản tin Commit nâng nhóm lên epoch $e+1$.
2. **Dynamic Stale Window Coordination:**
   Đồng bộ hóa tham số `max_stale_epochs` của OpenMLS (Rust) khớp chính xác với Grace Period của Go. Đồng thời bổ sung tính năng dọn dẹp khóa cũ có kiểm soát (graceful key shredding) để cân bằng tính bảo mật FS.
3. **Automated Proposal Retry Loop:**
   Thêm logic theo dõi đề xuất (Proposal Tracking): Nếu nút phát hiện epoch nhóm đã tiến lên $e+1$ mà đề xuất có `OperationID` của mình ở epoch $e$ bị bỏ lại, Go sẽ tự động đóng gói lại (re-package) đề xuất đó với nhãn epoch mới $e+1$ và tự động phát lại (re-propose).
