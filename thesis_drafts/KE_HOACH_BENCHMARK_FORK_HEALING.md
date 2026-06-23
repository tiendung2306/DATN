# Kế Hoạch Triển Khai Benchmark Fork Healing (Coordination Layer)

> **Mục tiêu:** Đo lường chi phí phục hồi sau phân mảnh (Fork Healing) ở góc nhìn hệ thống mạng thực tế (Go Layer), thay vì chỉ đo thuật toán mật mã ở lõi Rust. Số liệu từ đây sẽ được dùng để cập nhật trực tiếp vào Chương 5 (Đánh giá Thực nghiệm) và Chương 6 (Kết luận & Giới hạn) của Đồ án Tốt nghiệp.

Tài liệu này phác thảo chi tiết 2 bài test benchmark cần viết thêm vào hệ thống. Cả hai đều tận dụng lại các tiện ích trong `chaos_e2e_test.go` (như `setupCluster`, `startAll`, v.v.).

---

## 1. Benchmark 1: Độ Trễ Phục Hồi End-to-End (Losing Branch Perspective)

**Câu hỏi nghiên cứu:** Một thiết bị rớt mạng hoàn toàn (rơi vào nhánh thua) sẽ mất bao nhiêu thời gian để đồng bộ lại trạng thái từ nhánh thắng và xử lý lại các tin nhắn ứng dụng bị lỡ (Autonomous Replay)?

### 1.1. Thiết kế kịch bản (`BenchmarkForkHeal_EndToEndLatency`)
- **Vị trí file:** Tạo file `app/coordination/fork_heal_latency_bench_test.go`.
- **Tham số thay đổi (Biến độc lập):** Kích thước của state gốc (số lượng thành viên $N \in \{10, 50, 100\}$).
- **Quy trình:**
  1. Khởi tạo một cluster $N$ nodes. Bầu Node 0 làm Token Holder.
  2. **Gây đứt mạng:** Cách ly hoàn toàn Node $X$ (bằng cách chặn luồng gRPC/P2P đến và đi của $X$).
  3. **Tiến hóa nhánh thắng:** Node 0 tiếp tục cho phép các node còn lại thêm/xóa thành viên $\to$ Nhánh chính tăng Epoch.
  4. **Tích lũy Offline Messages:** Cho Node $X$ gửi 10 application messages. Do đứt mạng, các tin này nằm lại ở Local Outbox của $X$.
  5. **Kích hoạt khôi phục (Start Timer):** Nối lại mạng cho Node $X$. Node $X$ sẽ nhận được `GroupStateAnnouncement` khác biệt, chạy cơ chế Fork Detector $\to$ bắt đầu chu trình `HealFork`.
  6. **Đo độ trễ (Stop Timer):** Tính toán mốc thời gian từ lúc $X$ gửi yêu cầu fetch `GroupInfo` cho đến khi `job.Status == "STATE_SWAPPED"` và quét xong Outbox.

### 1.2. Giá trị đem lại cho Đồ án (Chương 5 & 6)
- Bạn sẽ có một biểu đồ Stacked Bar Chart phân rã thời gian phục hồi của $X$ thành các chặng: 
  *(a) Fetch GroupInfo* $\to$ *(b) Tính toán ExternalJoin* $\to$ *(c) Gom Commit (nếu có)* $\to$ *(d) Replay tin nhắn.*
- **Lập luận bổ sung:** Làm nổi bật giá trị của tính năng *Autonomous Replay* (Sprint 2E) trong việc duy trì tính nhất quán của dữ liệu dù mạng bị cắt đứt.

---

## 2. Benchmark 2: Thử Thách Chịu Tải Thundering Herd (Winning Branch Perspective)

**Câu hỏi nghiên cứu:** Nếu mạng bị chia cắt đôi (VD: 1000 nodes bị chia thành 500-500). Khi nối cáp lại, 500 nodes ở nhánh thua sẽ đồng loạt xin "External Join" vào nhánh thắng trong cùng một tích tắc. Token Holder và mạng lưới sẽ chịu đựng "cơn bão" này như thế nào?

### 2.1. Thiết kế kịch bản (`BenchmarkForkHeal_ThunderingHerd`)
- **Vị trí file:** Tạo file `app/coordination/thundering_herd_bench_test.go`.
- **Tham số thay đổi (Biến độc lập):** Số lượng nút ở nhánh thua ùa vào ($K \in \{5, 20, 50\}$). Giữ quy mô tổng nhóm cố định $N = 100$.
- **Quy trình:**
  1. Khởi tạo cluster $N=100$. Node 0 là Token Holder.
  2. Phân mảnh mạng: Chia làm 2 nhóm, Nhóm A (Nhánh thắng - 50 nodes), Nhóm B (Nhánh thua - 50 nodes).
  3. Nhóm A tăng Epoch lên $E+1$.
  4. **Thundering Herd Trigger (Start Timer):** Xóa rào cản mạng. $K$ nodes từ Nhóm B sẽ lập tức phát hiện lệch Epoch và TẤT CẢ cùng chạy `ExternalJoin`, sinh ra $K$ cái `Commit` rác đẩy lên P2P/gRPC.
  5. **Nút thắt (Bottleneck):** Quan sát cách Token Holder đối phó. Liệu Token Holder có bác bỏ (reject) hàng loạt do lệch Epoch không? Các node nhánh thua có cơ chế Backoff-Retry hợp lý không? 
  6. **Kết thúc (Stop Timer):** Đo thời gian để toàn bộ $K$ nodes đều báo cáo đã hội tụ về nhánh chính. Đếm số lượng tin nhắn gRPC/P2P thực tế phát sinh (Message Overhead).

### 2.2. Giá trị đem lại cho Đồ án (Chương 5 & 6)
- Cung cấp số liệu "Thử tải khắc nghiệt nhất" (Stress Test).
- Nếu thời gian hội tụ bị kéo dài theo cấp số nhân khi $K$ lớn, đây chính là **"Giới hạn của thiết kế"** tuyệt vời để đưa vào Chương 6 (điểm cộng rất lớn cho tư duy phản biện hệ thống).
- Bạn có thể đề xuất Hướng Phát triển ở Chương 6: *"Bổ sung cơ chế Rate-Limit cho ExternalJoin Request tại Token Holder"* hoặc *"Thay vì 1000 nút cùng External Join, bầu ra 1 nút đại diện của nhánh thua làm External Join, sau đó mời (Add) 999 nút kia vào"*.

---

## Kế hoạch hành động tiếp theo
Nếu bạn hài lòng với thiết kế này, bạn chỉ cần gõ:
> *"Tuyệt vời, hãy viết code cho file `fork_heal_latency_bench_test.go` trước đi"*
Tôi sẽ trực tiếp sử dụng công cụ để lập trình và chạy thử bài benchmark đó cho bạn!
