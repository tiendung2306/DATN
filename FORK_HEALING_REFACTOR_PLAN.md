# KẾ HOẠCH TÁI CẤU TRÚC KIẾN TRÚC FORK HEALING
**Chiến dịch: "Phoenix Protocol" - Hồi sinh từ đống tro tàn**

## 1. Đặt vấn đề (The Problem)
Trong kịch bản mạng P2P bị chia cắt (Network Partition), hệ thống có thể dẫn đến hiện tượng **Split-Brain (Divergent Evolution)**, trong đó nhánh A và nhánh B hoạt động độc lập và tự do biến đổi cây Ratchet. Vì chuẩn MLS là một DAG tuyến tính nghiêm ngặt, hai cây Ratchet này KHÔNG THỂ merge lại với nhau. Giao thức yêu cầu nhánh thua (ví dụ nhánh B) phải hủy trạng thái và sáp nhập lại vào nhánh thắng (A).

**Lỗ hổng của kiến trúc hiện tại (`ExternalJoin`):**
1. **Lỗi Duplicate Credential:** Những thành viên của nhánh B vốn dĩ vẫn còn tồn tại (dưới dạng một LeafNode đã cũ) trong cây MLS của nhánh A. Nếu họ sử dụng cơ chế `ExternalJoin` để xin vào lại nhánh A, chuẩn OpenMLS sẽ từ chối vì Identity của họ đã tồn tại trong nhóm.
2. **Nút thắt cổ chai $O(N^2)$ (Thundering Herd):** Nếu 64 người cùng dùng `ExternalJoin`, mỗi người sẽ tự tạo ra một Commit. Sự đụng độ Commit sẽ dẫn đến việc 63 người bị từ chối, phải retry liên tục, gây bão mạng và làm sập hệ thống.

## 2. Giải pháp Học thuật (The "Phoenix" Protocol)
Thay vì sử dụng `ExternalJoin` hay cố gắng duy trì một danh sách `external_senders` dư thừa, giải pháp tối ưu cho kiến trúc phân tán này là:
1. Nhóm thua vứt bỏ trạng thái cũ, tạo KeyPackage mới, và gửi **`JoinProposal`** (new_member_proposal) qua mạng GossipSub.
2. Tại nhánh thắng, Token Holder thu thập tất cả các `JoinProposal` này. 
3. Bằng quyền năng của Single-Writer, Token Holder phát hiện ra các Identity này là những "cái xác không hồn" (Zombie Leaf) trong cây Ratchet. Nó sẽ tự động sinh ra một `RemoveProposal` để dọn dẹp các xác này, rồi **gom chung (Batching)** cùng với `AddProposal` của thành viên đó vào đúng MỘT Commit duy nhất.

**Kết quả:** Giải quyết trọn vẹn cả lỗi Duplicate Credential lẫn bài toán hiệu năng Thundering Herd (từ $O(N^2)$ xuống $O(1)$).

---

## 3. Kế hoạch Triển khai (4 Bước)

### Bước 1: Cập nhật tài liệu học thuật
- **File tác động:** `README.md`, `PROJECT_PLAN.md`
- **Nhiệm vụ:** Viết lại toàn bộ phần lý thuyết của "Mechanism 3: Group Fork Healing". Loại bỏ khái niệm `ExternalJoin` trong kịch bản Split-Brain. Bổ sung cơ chế `new_member_proposal` và kỹ thuật "Token Holder tự động dọn rác (Batch Remove + Add)". Nhấn mạnh vào độ phức tạp thời gian $O(1)$ Batching để chứng minh tính khoa học.

### Bước 2: Tái cấu trúc luồng Nhánh Thua (Losing Branch)
- **File tác động:** `app/coordination/fork_healing.go`
- **Nhiệm vụ:** Thay đổi hành vi khi phát hiện bị rớt khỏi nhánh thắng:
  1. Hủy `MlsGroup` trong memory.
  2. Gọi Rust tạo một `KeyPackage` mới.
  3. Bọc `KeyPackage` vào thông điệp `JoinProposal` và phát qua GossipSub.
  4. Đưa node về trạng thái "Awaiting Welcome".

### Bước 3: Nâng cấp luồng Token Holder Nhánh Thắng (Winning Branch)
- **File tác động:** `app/coordination/coordinator.go`
- **Nhiệm vụ:** Can thiệp vào quá trình gom nhóm (Batching) các Proposal:
  1. Quét các `JoinProposal` đang chờ xử lý.
  2. Gọi hàm kiểm tra Cây Ratchet hiện tại xem TargetIdentity đã tồn tại hay chưa.
  3. Nếu tồn tại, tự động chèn thêm một `ProposalRemove` cho LeafIndex tương ứng.
  4. Đẩy danh sách Proposal đã bổ sung vào Rust `CreateCommit`.

### Bước 4: Viết lại Benchmark & Chứng minh Hiệu năng
- **File tác động:** `app/coordination/fork_heal_real_mls_bench_test.go`
- **Nhiệm vụ:**
  - Hủy bỏ kịch bản test $O(N^2)$ cũ (sử dụng ExternalJoin).
  - Khởi tạo kịch bản 64 nodes bắn `JoinProposal` cùng lúc.
  - Đo lường và chứng minh Token Holder xử lý toàn bộ 64 node trong một nhịp Commit duy nhất mà không xảy ra retry. Đảm bảo toàn bộ các bài test đều vượt qua (PASS).
