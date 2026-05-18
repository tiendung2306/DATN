# KẾ HOẠCH TỐI ƯU HÓA HIỆU NĂNG BỘ CỐT LÕI MLS (O(1) VÀ O(log N))

## 1. Chẩn đoán vấn đề hiện tại

Hệ thống hiện tại đang bị thắt cổ chai ở mức $O(N)$ (tuyến tính) cho cả việc nhắn tin và thêm thành viên, mặc dù thuật toán bên trong OpenMLS là $O(1)$ và $O(\log N)$. Nguyên nhân gốc rễ là kiến trúc **Stateless Wrapper (Vỏ bọc không trạng thái)**.

### a. Tại sao Nhắn tin (Messaging) đang bị O(N)?
Trong lý thuyết, mã hóa 1 tin nhắn chỉ tốn 1 phép toán AES-GCM ($O(1)$). Nhưng thực tế:
1. **Go:** Đọc 2MB dữ liệu (Ratchet Tree của 1000 người) từ SQLite -> Đóng gói gRPC -> Gửi sang Rust ($O(N)$).
2. **Rust:** Giải mã gRPC -> Tạo lại cấu trúc cây MlsGroup trong RAM -> **Verify lại chữ ký của toàn bộ 1000 người** để đảm bảo an toàn ($O(N)$).
3. **Rust:** Mã hóa tin nhắn ($O(1)$).
4. **Rust:** Gom lại toàn bộ 1000 người, nén thành file JSON 2MB ($O(N)$) -> Gửi về Go.
5. **Go:** Nhận 2MB -> Ghi đè vào bảng `mls_groups` trong SQLite và fsync xuống ổ cứng ($O(N)$).

👉 **Hậu quả:** Thao tác nhắn tin phải "cõng" 4 lần $O(N)$, khiến tốc độ bị kéo lùi từ `0.1ms` thành `40-50ms`.

### b. Tại sao Thêm người (Add Member) đang bị O(N)?
Thao tác thêm người bản chất là cập nhật cấu trúc cây Ratchet Tree. Mặc dù OpenMLS chỉ cập nhật $\log_2(N)$ nút trên cây (rất nhanh), nhưng nó vẫn chịu chung hình phạt của kiến trúc Stateless:
1. Cũng phải truyền 2MB dữ liệu qua gRPC ($O(N)$).
2. Cũng phải verify 1000 chữ ký ban đầu ($O(N)$).
3. Sau khi tính toán xong ($O(\log N)$), lại phải nén toàn bộ cây mới thành JSON 2MB ($O(N)$) và ghi xuống DB ($O(N)$).

---

## 2. Giải pháp: In-place State Mutation (Trí nhớ thường trực)

Để giải quyết, chúng ta phải chuyển Rust từ một "người làm thuê mất trí nhớ" (chỉ xử lý xong rồi quên) thành một **"Quản gia có trí nhớ dài hạn" (Stateful Sidecar)**, kết hợp với việc **"Tách biệt Dữ liệu Động/Tĩnh"**.

### Kế hoạch sửa đổi chi tiết:

### Bước 1: Sửa Rust Sidecar (Stateful Memory)
Thay vì load và drop `MlsGroup` liên tục, Rust sẽ giữ nó sống trong RAM.
*   Tạo một Registry: `static ref ACTIVE_GROUPS: DashMap<String, Mutex<MlsGroup>>`.
*   **Khi có Request (Encrypt/AddMember):** 
    *   Rust kiểm tra xem GroupID đã có trong `ACTIVE_GROUPS` chưa.
    *   Nếu có: Dùng luôn Object đó để tính toán. (Bỏ qua hoàn toàn bước giải nén JSON và verify chữ ký).

### Bước 2: Tối ưu hóa luồng Nhắn tin (Đạt O(1) tuyệt đối)
Khi nhắn tin, cấu trúc cây (Ratchet Tree) **KHÔNG HỀ THAY ĐỔI**.
*   **Ở Rust:** Hàm `encrypt_message` sẽ chỉ mã hóa ciphertext, không gọi hàm `export_state` (không duyệt cây). Trả về `new_group_state` là một mảng byte RỖNG.
*   **Ở Go:** Hàm lưu DB sẽ kiểm tra: Nếu `new_group_state` rỗng -> **Bỏ qua lệnh UPDATE vào bảng `mls_groups`**. Chỉ insert tin nhắn vào bảng `messages`. (Ghi log rất nhỏ, $O(1)$).

### Bước 3: Tối ưu hóa luồng Thêm người (Đạt O(log N) thuật toán)
Khi thêm người, cây Ratchet Tree có thay đổi (Epoch tăng).
*   **Ở Rust:** Hàm `add_members` chạy với tốc độ $O(\log N)$ trên cây đang nằm trong RAM. 
*   **Lưu ý Đánh đổi (The Trade-off):** Mặc dù thuật toán chạy $O(\log N)$, Rust **vẫn phải** nén cây mới thành JSON ($O(N)$) và gửi về Go để Go cập nhật SQLite ($O(N)$). 
*   **Tại sao chấp nhận điều này?**
    1. Để đảm bảo hệ thống có thể phục hồi (Fault-tolerant) nếu Rust bị sập, Go BẮT BUỘC phải giữ bản copy mới nhất của cây trên ổ cứng. 
    2. Thao tác Add Member (thay đổi Epoch) là thao tác hiếm gặp (Control Plane) so với việc gửi hàng ngàn tin nhắn (Data Plane). Việc tốn $O(N)$ cho thao tác quản trị là tiêu chuẩn chấp nhận được trong hệ thống phân tán, miễn là quá trình tính toán mật mã không bị lặp lại vô ích.

### Bước 4: Điều chỉnh gRPC Protocol
*   Mọi Request gọi xuống Rust sẽ ưu tiên dùng `group_id`. Go chỉ gửi `group_state` (cái cây 2MB) cho Rust trong **duy nhất lần gọi đầu tiên** sau khi khởi động app. 
*   Các lần sau, Go gửi `group_state = null`. Rust dùng ID để móc trong RAM ra.

---

## 3. Tóm tắt biểu đồ sau khi tối ưu

Nếu chúng ta thực hiện thành công kế hoạch này, biểu đồ 1.000 nodes cuối cùng sẽ như sau:

*   **Đường Encrypt (Màu xanh):** Trở thành một đường thẳng nằm ngang ở mức **~0.1ms** -> Chứng minh **E2E O(1)** cho nhắn tin.
*   **Đường Add Member (Màu đỏ):** Sẽ có 2 thành phần thời gian:
    *   *Thời gian nội bộ Rust (Core Crypto):* Cong vút theo chuẩn **$O(\log N)$** (Chỉ tốn ~1-2ms).
    *   *Thời gian tổng (Total Latency):* Vẫn có xu hướng $O(N)$ (tốn ~20-30ms) do bị chi phí Ghi ổ cứng và Nén JSON kéo lại. (Chúng ta sẽ ghi chú rõ sự đánh đổi này trong báo cáo đồ án).

## 4. Thứ tự thực thi
1. Sửa file `proto/mls_service.proto` (Thêm trường `group_id`).
2. Viết lại lõi `crypto-engine/src/mls.rs` (Dùng Mutex giữ State).
3. Sửa hàm gRPC trong `crypto-engine/src/main.rs`.
4. Sửa `engine.go` và `coordination_storage.go` trong Go.
5. Chạy lại bài test 1000 nodes.
