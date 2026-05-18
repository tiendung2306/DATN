# BÁO CÁO NGHIÊN CỨU VÀ ĐỀ XUẤT TỐI ƯU HÓA HỆ THỐNG MLS P2P

Tài liệu này ghi lại các khám phá quan trọng trong quá trình đánh giá thực nghiệm hệ thống và lộ trình tối ưu hóa để đạt được hiệu năng lý thuyết $O(\log N)$.

## 1. Khám phá quan trọng (Key Findings)

Trong quá trình chạy thực nghiệm với 1.000 nodes, chúng tôi đã phát hiện ra các nút thắt cổ chai (bottlenecks) khiến hệ thống bị kéo lùi về độ phức tạp $O(N)$ thay vì $O(\log N)$:

### a. Chi phí truyền tải trạng thái (State Transport Overhead)
*   **Hiện tượng:** Thời gian nhắn tin (Messaging) tăng tuyến tính theo số lượng thành viên ($O(N)$).
*   **Nguyên nhân:** Kiến trúc Sidecar Stateless yêu cầu Go gửi toàn bộ `GroupState` (chứa Ratchet Tree 1MB-2MB cho 1.000 người) qua gRPC ở mỗi tin nhắn. Việc sao chép bộ nhớ qua socket tốn $O(N)$.
*   **Giải pháp đã thử nghiệm:** Cơ chế "State-Passing by ID". Go chỉ gửi ID/Epoch, Rust dùng cache trong RAM.

### b. Chi phí giải nén và xác minh (Serialization & Verification)
*   **Hiện tượng:** Rust xử lý `AddMember` rất chậm (>500ms cho 1.000 người).
*   **Nguyên nhân:** Thư viện `OpenMLS` khi nhận mảng byte từ Go sẽ thực hiện quét toàn bộ cây để verify chữ ký của 1.000 thành viên ($O(N)$).
*   **Giải pháp đã thử nghiệm:** "Hot-State Caching". Giữ instance `MlsGroup` sống trong RAM để bỏ qua bước verify này.

### c. Chi phí ghi dữ liệu (Database Write Amplification)
*   **Hiện tượng:** SQLite ghi đè toàn bộ blob 2MB sau mỗi tin nhắn.
*   **Nguyên nhân:** Hệ thống coi `GroupState` là một khối duy nhất, không tách biệt phần Tree (Tĩnh) và Secrets (Động).
*   **Giải pháp đề xuất:** Tách cấu trúc lưu trữ thành 2 bảng: `mls_group_tree` ($O(N)$, hiếm khi cập nhật) và `mls_group_secrets` ($O(1)$, cập nhật mỗi tin nhắn).

## 2. Kết quả thực nghiệm (Benchmarking Results)

Chúng tôi đã thu được 4 biểu đồ quan trọng (lưu tại `evaluation/plots/`):

1.  **`Evaluation_MLS_Scalability_O_logN.png`**: Chứng minh khả năng mở rộng của thuật toán MLS. Đường cong Logarithm đã được bóc tách khỏi nhiễu hệ thống.
2.  **`Evaluation_EndToEnd_Latency_Breakdown.png`**: Phân tích chi tiết các thành phần gây trễ (Mật mã vs Hệ thống).
3.  **`Evaluation_SingleWriter_Commit_Latency_CDF.png`**: Đánh giá hiệu năng của giao thức điều phối Single-Writer.
4.  **`Evaluation_MLS_Epoch_Convergence_Network_Chaos.png`**: Khẳng định tính hội tụ của giao thức trong điều kiện mạng không ổn định.

## 3. Lộ trình tối ưu hóa tương lai (Future Roadmap)

Sau buổi demo, hệ thống cần được tái cấu trúc theo các bước sau để đạt $O(\log N)$ dứt điểm:

1.  **Chuyển đổi sang Stateful Sidecar:** Rust sẽ giữ `DashMap` các nhóm đang hoạt động.
2.  **Giao thức gRPC "Pointer-based":** Cập nhật API để Go chỉ truyền handle/ID thay vì toàn bộ state blob.
3.  **Refactor Storage Layer:** Triển khai cơ chế lưu trữ phân lớp (Layered Storage) trong Go để triệt tiêu Write Amplification.
4.  **Delta State Sync:** Chỉ đồng bộ những thay đổi nhỏ trên cây (Update Path) thay vì toàn bộ Ratchet Tree qua mạng.

---
*Ghi chú: Toàn bộ mã nguồn đã được rollback về trạng thái ổn định nhất để phục vụ Demo. Các dữ liệu thực nghiệm và script vẽ biểu đồ đã được giữ lại để trích dẫn vào Luận văn.*
