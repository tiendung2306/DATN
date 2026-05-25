# Giao thức Single-Writer: Cơ chế Token Holder Nhận thức Ngữ cảnh (Context-Aware)

Tài liệu này mô tả thiết kế và kiến trúc của giải pháp nâng cấp cơ chế bầu chọn Token Holder ở tầng giao thức điều phối (Coordination Layer), loại bỏ sự phụ thuộc vào logic nghiệp vụ (Business Logic) và áp dụng cơ chế bầu chọn tự tất định dựa trên tập hợp `Eligible(E, P)`.

---

## 1. Bản chất Vấn đề & Lý do Cần thay đổi (The Rationale)

### Vấn đề: Tự Xóa Nhóm và Deadlock (Self-Removal Deadlock)
Trong kiến trúc hiện tại, quyền phát hành một gói **Commit** tại một epoch được giới hạn cho một Node duy nhất gọi là **Epoch Token Holder**, được tính toán tất định ở local thông qua hàm băm:
$$TH(E) = \operatorname{argmin}_{n \in \text{ActiveView}(E)} \operatorname{SHA256}(n \parallel E)$$

Khi một Node bất kỳ muốn rời khỏi nhóm (hoặc bị trục xuất bởi một batch đề xuất), một **ProposalRemove** sẽ được phát tán trên GossipSub. Theo đặc tả MLS (RFC 9420):
* Một nút bị xóa **không thể tự ký Commit** chứa đề xuất xóa chính nó.
* Nếu Node đang là Token Holder hiện tại (`TH(E)`) bị đưa vào tầm ngắm xóa bỏ, node này sẽ không thể tạo Commit.
* Vì không node nào khác có quyền Commit tại epoch đó ngoài Token Holder (nguyên lý Single-Writer), nhóm rơi vào trạng thái nghẽn và bế tắc hoàn toàn (**Deadlock**).

### Giải pháp tình thế cũ (Leaky Abstraction)
Trước đây, hệ thống sử dụng một cơ chế "lách luật" ở tầng ứng dụng: nếu nút hiện tại là **Creator** (Người tạo nhóm) thì sẽ được phép bỏ qua cơ chế Single-Writer để commit ngay lập tức. Đây là một lỗi thiết kế nghiêm trọng (Leaky Abstraction) vì nó kéo logic nghiệp vụ tầng ứng dụng (App Layer) xuống tầng điều phối giao thức (Coordination Layer), vi phạm tính đóng gói.

---

## 2. Triết lý Thiết kế: Fast-path Lạc quan vs. Hồi phục Fork

Để giải quyết triệt để vấn đề này, chúng ta cần định nghĩa lại vai trò của Single-Writer trên mạng P2P bất đồng bộ (Asynchronous):

1. **Không phải là Đồng thuận tuyệt đối (Fast-path Heuristic):**
   Trong một mạng P2P phi tập trung và bất đồng bộ, việc bắt buộc mạng đạt đồng thuận tuyệt đối 100% không bao giờ có fork (Consensus) trước mỗi Commit đòi hỏi phải có cơ chế bỏ phiếu dạng Quorum (ví dụ Raft hoặc Paxos). Điều này cực kỳ nặng nề, làm chậm hệ thống và triệt tiêu khả năng hoạt động offline khi mạng bị chia cắt (đánh đổi theo định lý CAP).
   Do đó, **Single-Writer chỉ đóng vai trò là một heuristic lạc quan** để tối ưu hiệu năng: nó giảm tối đa xác suất concurrent commit trong điều kiện mạng bình thường bằng cách chỉ định một người viết ưu tiên.

2. **Lưới an toàn Fork Healing:**
   Nếu các nút có góc nhìn (view) khác nhau về tập Proposal hiện tại hoặc danh sách các nút online (`ActiveView`), chúng có thể bầu ra các Token Holder khác nhau và tạo ra các commit song song cạnh tranh nhau. Điều này hoàn toàn được chấp nhận. **Cơ chế Fork Healing** của hệ thống đã có sẵn chức năng phát hiện sự phân kỳ (divergence) qua epoch/tree hash và tự động kéo các nhánh về trạng thái hội tụ đồng nhất.
   
Vì giao thức đã có chốt chặn cuối cùng là Fork Healing, chúng ta không cần sự đồng thuận tuyệt đối trước khi tính Token Holder. Chúng ta có thể tự tin tính toán Token Holder dựa trên **nhận thức cục bộ** về tập Proposal hiện tại.

---

## 3. Giải pháp Giao thức: Công thức Eligible(E, P)

Chúng ta chuyển đổi việc tính toán Token Holder từ một biến tĩnh theo Epoch sang một hàm động nhận thức ngữ cảnh các đề xuất:
$$TH(E) \rightarrow TH(E, P)$$

Trong đó $P$ là tập các đề xuất (Proposals) đang chờ xử lý cục bộ tại thời điểm tính toán.

### Công thức tập hợp ứng viên hợp lệ:
$$\text{Eligible}(E, P) = \text{GroupMembers}(E) \cap \text{ActiveView}(E) - \text{RemovedBy}(P) - \text{SuspendedOrExcluded}(E)$$

*   $\text{GroupMembers}(E)$: Tập hợp thành viên trong cây MLS tại Epoch $E$.
*   $\text{ActiveView}(E)$: Tập hợp các nút đang online (dựa trên Heartbeat nhận được).
*   $\text{RemovedBy}(P)$: Tập hợp tất cả các Peer ID bị nhắm mục tiêu xóa bỏ bởi các gói `ProposalRemove` nằm trong tập $P$.
*   $\text{SuspendedOrExcluded}(E)$: Các nút bị đánh giá là mất kết nối/treo (được bao hàm và xử lý tự động thông qua việc loại khỏi `ActiveView` sau một khoảng thời gian trễ $T_{\text{timeout}}$ không có Heartbeat).

### Thuật toán bầu chọn:
$$TH(E, P) = \operatorname{argmin}_{n \in \text{Eligible}(E, P)} \operatorname{SHA256}(n \parallel E)$$

---

## 4. Kịch bản Vận hành & Ưu điểm vượt trội

### Kịch bản tự xóa mình (Self-Removal) của Token Holder:
Giả sử Node **A** đang là Token Holder mặc định ở Epoch $E$. Node **A** muốn rời nhóm nên phát đi `ProposalRemove(A)`.
1. **Node A nhận được ProposalRemove(A):**
   * A đưa proposal này vào bộ đệm $P$ local.
   * Khi tính toán `IsTokenHolder()`, A tính tập $\text{Eligible}(E, P)$ không còn chứa A (vì A nằm trong $\text{RemovedBy(P)}$).
   * A tính ra $TH(E, P) = \mathbf{B}$ (node fallback tiếp theo). A tự động nhường quyền, ngồi im và không tạo Commit.
2. **Node B nhận được ProposalRemove(A):**
   * B đưa proposal này vào bộ đệm cục bộ.
   * B tính toán tập $\text{Eligible}(E, P)$ không chứa A.
   * B tính ra $TH(E, P) = \mathbf{B}$. B nhận ra mình đã được thăng cấp làm Token Holder cho ngữ cảnh này.
   * B chủ động đóng gói `ProposalRemove(A)` thành Commit, ký và phát sóng Commit này lên mạng Gossip.
3. **Mạng đồng nhất:**
   * Khi Commit được áp dụng, mạng tiến lên Epoch $E+1$, loại bỏ hoàn toàn Node A một cách mượt mà mà không xảy ra bất kỳ deadlock hay xung đột quyền Creator nào.

---

## 5. Kế hoạch Triển khai (Implementation Details)

### Bước 1: Cập nhật hàm bầu chọn trong `single_writer.go`
* Thêm hàm xem trước buffer: `PeekNextBatch() []BufferedProposal` để đọc ra lô proposal chuẩn bị xử lý mà không làm rỗng hàng đợi.
* Thay đổi chữ ký hàm `ComputeTokenHolder`:
  ```go
  func ComputeTokenHolder(activeView []peer.ID, epoch uint64, pending []BufferedProposal) (peer.ID, error)
  ```
  Hàm sẽ lọc bỏ tất cả các `TargetPeerID` có trong các Proposal loại `ProposalRemove` khỏi `activeView` trước khi thực hiện vòng lặp tìm `argmin` băm.

### Bước 2: Tích hợp vào `Coordinator`
* Sửa hàm `handleActiveViewChange` trong `coordinator.go`:
  Lấy danh sách các proposal chuẩn bị gửi bằng `PeekNextBatch()` và truyền vào `ComputeTokenHolder` để đảm bảo cơ chế thăng cấp hoạt động ngay khi ActiveView thay đổi.
* Cập nhật các hàm kiểm tra trạng thái local `IsTokenHolder()` và `CurrentTokenHolder()` để nhận diện Token Holder động.

### Bước 3: Kiểm thử Độc lập
* Viết thêm các unit test đặc tả hành vi loại trừ trong `single_writer_test.go`.
* Chạy sweep test toàn bộ gói coordination:
  ```powershell
  go test -v ./coordination/...
  ```
