# Kế Hoạch Triển Khai Benchmark Fork Healing (Coordination Layer)

> **Mục tiêu:** Đo lường chi phí phục hồi sau phân mảnh (Fork Healing) ở góc nhìn hệ thống mạng thực tế (Go Layer) và engine mật mã (Rust). Số liệu từ đây sẽ được dùng để cập nhật trực tiếp vào Chương 5 (Đánh giá Thực nghiệm) và Chương 6 (Kết luận & Giới hạn) của Đồ án Tốt nghiệp.

Tài liệu này phác thảo chi tiết 2 bài test benchmark cần viết thêm vào hệ thống. Các bài benchmark này sẽ sử dụng **Real MLS Engine** (tiến trình Rust thật thông qua `ProcessManager`) thay vì Mock, đảm bảo số liệu phản ánh chính xác chi phí tính toán.

---

## 1. Benchmark 1: Độ Trễ Phục Hồi End-to-End (Losing Branch Perspective)

**Câu hỏi nghiên cứu:** Một thiết bị rớt mạng hoàn toàn (rơi vào nhánh thua) sẽ mất bao nhiêu thời gian để đồng bộ lại trạng thái từ nhánh thắng và xử lý lại các tin nhắn ứng dụng bị lỡ (Autonomous Replay)?

### 1.1. Thiết kế kịch bản (`BenchmarkForkHeal_EndToEndLatency`)
- **Vị trí file:** Tạo file `app/coordination/fork_heal_real_mls_bench_test.go`.
- **Tham số thay đổi (Biến độc lập):** Kích thước của state gốc (số lượng thành viên $N \in \{10, 50, 100\}$).
- **Quy trình:**
  1. Khởi tạo một cluster $N$ nodes (dùng chung 1 tiến trình Rust Sidecar). Bầu Node 0 làm Token Holder.
  2. **Gây đứt mạng:** Cách ly hoàn toàn Node $X$ khỏi mạng.
  3. **Tiến hóa nhánh thắng:** Node 0 tiếp tục cho phép các node còn lại thêm/xóa thành viên $\to$ Nhánh chính tăng Epoch.
  4. **Tích lũy Offline Messages:** Cho Node $X$ gửi 10 application messages. Các tin này nằm ở Local Outbox.
  5. **Kích hoạt khôi phục (Start Timer):** Nối lại mạng cho Node $X$. Node $X$ nhận được `GroupStateAnnouncement` khác biệt, tiến trình `runHeal` được kích hoạt.
  6. **Đo độ trễ (Stop Timer):** Tính toán mốc thời gian từ lúc Fetch `GroupInfo` (Stage: `EXTERNAL_JOINED`) cho đến khi `job.Status == "STATE_SWAPPED"` và hoàn thành quét Outbox (Autonomous Replay). Tracking thông qua `Metrics` được cắm vào hàm `runHeal`.

### 1.2. Giá trị đem lại cho Đồ án (Chương 5 & 6)
- Biểu đồ Stacked Bar Chart phân rã thời gian phục hồi: *(a) Fetch GroupInfo* $\to$ *(b) External Join* $\to$ *(c) State Swap* $\to$ *(d) Replay tin nhắn.*
- Làm nổi bật giá trị của tính năng *Autonomous Replay* trong việc duy trì tính nhất quán.

---

## 2. Benchmark 2: Thử Thách Chịu Tải Thundering Herd (Winning Branch Perspective)

**Câu hỏi nghiên cứu:** Nếu mạng bị chia cắt đôi (VD: 1000 nodes bị chia thành 500-500). Khi nối cáp lại, 500 nodes ở nhánh thua sẽ đồng loạt xin gia nhập lại nhánh thắng. Dựa trên thiết kế hiện tại và **Rule 13**, các node nhánh thua sẽ sử dụng `ExternalJoin` (tạo Commit trực tiếp). Khi hàng loạt node cùng bung Commit ra mạng lưới, "cơn bão Commit rác" và hiện tượng kẹt xe Epoch sẽ diễn ra như thế nào?

### 2.1. Thiết kế kịch bản (`BenchmarkForkHeal_ThunderingHerd`)
- **Vị trí file:** Cùng nằm trong `app/coordination/fork_heal_real_mls_bench_test.go`.
- **Tham số thay đổi (Biến độc lập):** Số lượng nút ở nhánh thua ùa vào ($K \in \{5, 20, 50\}$). Giữ quy mô tổng nhóm cố định $N = 100$.
- **Quy trình:**
  1. Khởi tạo cluster $N=100$ (Real MLS Engine). Node 0 là Token Holder.
  2. Phân mảnh mạng: Nhóm A (Nhánh thắng - 50 nodes), Nhóm B (Nhánh thua - 50 nodes).
  3. Nhóm A tăng Epoch lên $E+1$.
  4. **Thundering Herd Trigger (Start Timer):** Xóa rào cản mạng. $K$ nodes từ Nhóm B nhận ra mình bị tụt hậu. TẤT CẢ cùng chạy hàm `c.mls.ExternalJoin` và phát hành $K$ cái `Commit` lên P2P/gRPC.
  5. **Nút thắt (Bottleneck):** Quan sát hệ thống reject các Epoch trùng lặp (chỉ 1 Commit được chấp nhận mỗi Epoch, $K-1$ Commit còn lại bị `ActionRejectStale`). Các node bị reject sẽ phải backoff và làm lại ở các Epoch sau.
  6. **Kết thúc (Stop Timer):** Đo thời gian từ lúc "cơn bão" bắt đầu đến khi toàn bộ $K$ nodes đều ExternalJoin thành công vào nhánh chính. Đếm số lượng tin nhắn bị reject làm Overhead.

### 2.2. Giá trị đem lại cho Đồ án (Chương 5 & 6)
- Cung cấp số liệu Stress Test chứng minh sức cản mạng (Starvation/Epoch Collision) khi sử dụng cơ chế `ExternalJoin` đồng loạt.
- Cung cấp dẫn chứng thực nghiệm cho **"Giới hạn của thiết kế"** ở Chương 6.
- Đề xuất Hướng Phát triển: *"Bổ sung cơ chế Rate-Limit/Backoff cho ExternalJoin"* hoặc thay thế bằng cơ chế *"Giao thức cử ra 1 node đại diện nhánh thua thực hiện ExternalJoin, sau đó Token Holder dùng Bidirectional Batching gửi Proposal để Add các node còn lại"*.

---

## Kế hoạch hành động tiếp theo
Nếu kế hoạch này đã chuẩn xác với thiết kế hệ thống, hãy gõ:
> *"Bắt đầu code file benchmark đi"*
