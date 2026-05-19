# KẾ HOẠCH TỐI ƯU HÓA HIỆU NĂNG BỘ CỐT LÕI MLS (GIẢM O(N) OVERHEAD, TIẾN GẦN O(log N))

## 1. Chẩn đoán vấn đề hiện tại

Hệ thống hiện tại đang bị thắt cổ chai ở mức $O(N)$ (tuyến tính) cho cả việc nhắn tin và thêm thành viên, mặc dù phần group-keying của MLS/TreeKEM có thiết kế tăng chậm hơn nhiều so với pairwise E2EE. Nguyên nhân gốc rễ là kiến trúc **Stateless Wrapper (Vỏ bọc không trạng thái)**.

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

### Bước 2: Tối ưu hóa luồng Nhắn tin (giảm hệ số tuyến tính b trong benchmark)
Khi nhắn tin, hot path không được import/export full `GroupState`.
*   **Ở Rust:** Hàm `encrypt_message` chạy trên `MlsGroup` resident trong cache theo `group_id`; benchmark chính không gọi `export_state()` trong timed section.
*   **Ở Go:** Hot benchmark không ghi full `mls_groups.group_state` sau mỗi message. Nếu cần checkpoint để phục hồi, checkpoint được đo thành metric riêng hoặc chạy ngoài timed section.
*   **Lưu ý an toàn:** Không được hiểu "không trả `new_group_state`" là bỏ qua hoàn toàn persistence trong production. MLS application encrypt/decrypt vẫn có thể mutate ratchet/generation state. Production path phải có version fencing, actor queue và persistence/delta/checkpoint rõ ràng trước khi ACK ra network.

### Bước 3: Tối ưu hóa luồng Commit/Update (đường chính cho biểu đồ gần O(log N))
Khi thêm người, cây Ratchet Tree có thay đổi (Epoch tăng).
*   **Ở Rust:** Ưu tiên benchmark `CreateUpdateCommit` / `ProcessUpdateCommit` trên cây đang nằm trong RAM. Đây là operation phù hợp nhất để thể hiện hành vi TreeKEM tăng chậm theo độ sâu cây.
*   **AddMember/RemoveMember:** Không dùng làm biểu đồ chính vì dễ dính Welcome generation, credential validation, leaf insertion và storage update. Có thể dùng làm biểu đồ phụ.
*   **Lưu ý Đánh đổi (The Trade-off):** Nếu Rust vẫn nén cây mới thành JSON ($O(N)$) và gửi về Go để Go cập nhật SQLite ($O(N)$), total latency vẫn có phần tuyến tính. Phần này phải đo riêng là "checkpoint/persistence overhead", không trộn vào biểu đồ local crypto chính.
*   **Tại sao chấp nhận điều này?**
    1. Để đảm bảo hệ thống có thể phục hồi (Fault-tolerant) nếu Rust bị sập, Go BẮT BUỘC phải giữ bản copy mới nhất của cây trên ổ cứng. 
    2. Thao tác Add Member (thay đổi Epoch) là thao tác hiếm gặp (Control Plane) so với việc gửi hàng ngàn tin nhắn (Data Plane). Việc tốn $O(N)$ cho thao tác quản trị là tiêu chuẩn chấp nhận được trong hệ thống phân tán, miễn là quá trình tính toán mật mã không bị lặp lại vô ích.

### Bước 4: Điều chỉnh gRPC Protocol cho hot path
*   Mọi Request gọi xuống Rust sẽ ưu tiên dùng `group_id`, `expected_epoch`, `expected_state_version`, `operation_id`.
*   Thêm RPC benchmark/phục hồi kiểu `LoadGroup(group_id, group_state)` để preload state trước measurement. RPC này không được tính vào latency chính.
*   Các hot RPC như `EncryptMessage`, `DecryptMessage`, `CreateUpdateCommit`, `ProcessCommit`, `ExportSecret` không truyền `group_state` / `new_group_state` trong benchmark tối ưu.
*   Rust từ chối request nếu epoch/version không khớp để tránh request cũ mutate state.

---

## 3. Tóm tắt biểu đồ sau khi tối ưu

Nếu chúng ta thực hiện thành công kế hoạch này, biểu đồ trong phạm vi local operation (không network, không setup group, không full checkpoint trong timed section) sẽ như sau:

*   **Pairwise E2EE baseline:** tăng tuyến tính rõ vì sender phải xử lý từng recipient.
*   **Current full-blob MLS:** có thể vẫn tăng gần tuyến tính do gRPC/JSON/SQLite full blob. Đây là đường ablation để chứng minh bottleneck implementation.
*   **Rust MLS core only:** tăng chậm hơn rõ rệt; dùng để chứng minh lợi thế MLS/TreeKEM khi tách khỏi I/O.
*   **Hot-cache sidecar:** thấp hơn current full-blob, có đường cong nhẹ/sublinear hơn. Nếu fit `a log2(N)+b` tốt hơn fit tuyến tính, dùng làm bằng chứng chính.
*   **Application Encrypt:** có thể gần flat/sublinear; không gọi là "O(log N)" nếu dữ liệu không thể hiện log, mà gọi là "near-constant/sublinear sender-side encryption".

Mô hình phân tích:

```text
T_MLS(N) = a log2(N) + bN + c
T_pairwise(N) = pN + q
```

Mục tiêu thực dụng là làm `b << p`, để trong dải benchmark luận văn đường MLS thấp và tăng chậm hơn rõ rệt so với baseline O(N). Không claim toàn bộ app end-to-end là O(log N).

## 4. Claim học thuật và phạm vi benchmark

Claim được phép dùng:

> Khi loại bỏ tầng mạng và giảm full-state serialization khỏi hot path, MLS/TreeKEM cho local group-key operations có latency tăng chậm hơn rõ rệt so với baseline P2P E2EE pairwise O(N). Kết quả được giải thích tốt hơn bởi mô hình logarithmic/sublinear trong dải benchmark đã kiểm soát.

Claim không được dùng:

> Toàn bộ hệ thống P2P end-to-end đạt O(log N).

Nguyên tắc benchmark chính:

1. Không đo network, GossipSub, direct stream, UI event, Wails bridge.
2. Không tính setup group, add tuần tự N members, generate identities/keypackages, cold-load hoặc cache warmup vào measurement.
3. Không import/export full GroupState trong timed section.
4. Không ghi full SQLite blob trong timed section.
5. Không scan toàn bộ member list hoặc validate toàn bộ credentials trong timed section.
6. Dùng median latency làm đường chính; p95/p99 dùng làm phụ để giải thích jitter.
7. Chọn dải N vừa phải trước: `16, 32, 64, 128, 256, 512, 1024, 2048, 4096`. Chỉ mở rộng lên 10k/100k cho synthetic hoặc core benchmark đã loại overhead.

Biểu đồ nên có:

1. Pairwise E2EE O(N) vs MLS Update Commit / Process Commit.
2. Pairwise E2EE O(N) vs MLS Application Encrypt, gọi là near-constant/sublinear nếu dữ liệu gần flat.
3. Current full-blob MLS vs Rust core vs hot-cache sidecar.
4. Fit curve: so sánh R² của `T_log(N)=a log2(N)+b` với `T_linear(N)=cN+d`.

Lưu ý học thuật: Nếu OpenMLS nội bộ vẫn materialize hoặc duyệt một số cấu trúc theo N, kết quả có thể không là log tuyệt đối. Khi đó báo cáo phải dùng ngôn ngữ "near-logarithmic", "sublinear trend", hoặc "better explained by logarithmic fit than linear fit" thay vì khẳng định Big-O tuyệt đối.

## 5. Thứ tự thực thi
1. Thêm Rust benchmark core trong `crypto-engine` để đo MLS operation không qua Go/gRPC/SQLite.
2. Thêm pairwise E2EE baseline trong cùng môi trường benchmark.
3. Thêm benchmark current full-blob sidecar để làm đường ablation xấu.
4. Sửa proto theo hướng hot-cache thử nghiệm: thêm `OperationContext`, `LoadGroup`, `GetGroupMetadata`; hot RPC dùng `group_id` thay vì full `group_state`.
5. Viết lại lõi `crypto-engine/src/mls.rs` theo hướng `DashMap<GroupID, GroupRuntime>` + per-group serialized actor queue.
6. Sửa gRPC server trong `crypto-engine/src/main.rs`.
7. Sửa `app/adapter/sidecar/engine.go` và các điểm gọi benchmark trong Go để không đọc/ghi full blob trong timed section.
8. Chạy benchmark dải vừa phải trước: `16, 32, 64, 128, 256, 512, 1024, 2048, 4096`.
9. Fit curve `log2(N)` và tuyến tính, báo cáo R². Chỉ mở rộng lên 10k/100k khi core/hot-cache đã ổn.
10. Nếu còn thời gian, triển khai Rust-owned StorageProvider/delta persistence để đưa persistence tối ưu vào benchmark production-grade.
