# CHƯƠNG 4. PHÂN TÍCH LÝ THUYẾT

Chương này phân tích các bất biến cốt lõi của giao thức phối hợp phi tập trung được xây dựng bao quanh MLS trong môi trường P2P. MLS cung cấp lớp mật mã nhóm, nhưng tiêu chuẩn này giả định tồn tại một Delivery Service có khả năng tuần tự hóa các thao tác thay đổi trạng thái nhóm. Khi loại bỏ máy chủ trung tâm, hệ thống cần một lớp phối hợp riêng để tránh commit đồng thời, kiểm soát epoch, phát hiện phân nhánh và khôi phục hội tụ sau phân mảnh mạng.

Phạm vi của chương này là phân tích ở mức thiết kế và bất biến giao thức, không phải chứng minh hình thức đầy đủ theo mô hình toán học. Các kết luận lý thuyết quan trọng được kiểm chứng bổ sung bằng thực nghiệm ở Chương 5.

## 4.1. Bất biến an toàn của giao thức phối hợp

### 4.1.1. Bất biến Single-Writer

Tại mỗi epoch, hệ thống chỉ cho phép một nút giữ quyền phát hành MLS Commit. Nút này được gọi là Token Holder và được tính quyết định từ tập thành viên đang hoạt động:

$$
TokenHolder = \arg\min_{node \in ActiveView} H(groupID \parallel epoch \parallel nodeID)
$$

Trong đó $H$ là SHA-256. Nếu hai nút có cùng `ActiveView`, cùng `groupID` và cùng epoch, kết quả bầu chọn là giống nhau mà không cần thêm vòng bỏ phiếu. Các nút không phải Token Holder chỉ được tạo Proposal và phát tán Proposal qua GossipSub; quyền gom Proposal thành Commit thuộc về Token Holder.

Từ đó suy ra bất biến quan trọng: trong một nhánh mạng không bị phân mảnh và có cùng ActiveView, tối đa một nút hợp lệ có thể tạo Commit cho epoch hiện tại. Các Commit đến từ nút không phải Token Holder bị từ chối ở lớp phối hợp. Bất biến này không loại bỏ hoàn toàn phân nhánh khi mạng vật lý bị chia cắt, vì mỗi phân vùng có thể hình thành ActiveView riêng; phần phân nhánh đó được xử lý bởi cơ chế Fork Healing.

### 4.1.2. Bất biến đơn điệu epoch

Mỗi thông điệp MLS được bọc trong envelope chứa epoch của người gửi. Lớp phối hợp áp dụng quy tắc:

| Điều kiện | Xử lý |
|---|---|
| `msg.epoch == local.epoch` | Xử lý bình thường |
| `msg.epoch < local.epoch` | Từ chối như thông điệp cũ |
| `msg.epoch > local.epoch` | Đệm lại và yêu cầu đồng bộ trạng thái |

Quy tắc này ngăn việc áp dụng ciphertext hoặc Commit lên sai trạng thái Ratchet Tree. Về mặt lý thuyết, nếu một nút chỉ cập nhật `GroupState` sau khi Commit hợp lệ được xử lý thành công, thì epoch cục bộ là một đại lượng đơn điệu không giảm. Điều này là điều kiện cần để tránh rollback trạng thái mật mã.

### 4.1.3. Sắp xếp thông điệp ứng dụng bằng HLC

Epoch chỉ sắp xếp các thay đổi trạng thái MLS, không sắp xếp toàn bộ tin nhắn ứng dụng trong cùng một epoch. Vì GossipSub không đảm bảo thứ tự nhận giống nhau trên mọi nút, hệ thống sử dụng Hybrid Logical Clock:

$$
HLC = (L, C, NodeID)
$$

Trong đó $L$ là thời gian vật lý theo mili-giây, $C$ là bộ đếm logic, và `NodeID` là khóa phá hòa quyết định. Khi nhận một timestamp từ nút khác, đồng hồ cục bộ cập nhật theo quy tắc lấy cực đại giữa thời gian cục bộ, thời gian nhận được và thời gian vật lý hiện tại. Do đó, nếu thông điệp $a$ gây ra thông điệp $b$, thì $HLC(a) < HLC(b)$. Với các thông điệp đồng thời, thứ tự từ điển trên $(L,C,NodeID)$ tạo ra cùng một thứ tự hiển thị trên mọi nút.

## 4.2. Phân tích Fork Healing và bảo toàn thuộc tính bảo mật

### 4.2.1. Lựa chọn nhánh thắng

Khi các nút phát hiện `TreeHash` hoặc `CommitHash` khác nhau trong cùng nhóm, hệ thống xem đó là dấu hiệu phân nhánh. Hàm trọng số cơ bản dùng để so sánh hai nhánh là:

$$
W = (C_{support}, C_{members}, E, H_{commit}, H_{tree})
$$

Trong đó `C_support` là số peer quan sát được trên cùng nhánh, `C_members` là số thành viên trực tuyến trong nhánh, `E` là epoch, `H_commit` là hash của Commit cuối, và `H_tree` là hash cây dùng làm khóa phá hòa cuối cùng. Cách trình bày rút gọn trong thiết kế ban đầu là $W=(C_{members},E,H_{commit})$; triển khai hiện tại bổ sung `C_support`, kiểm tra commit đã bị đánh dấu không hợp lệ, và `TreeHash` phá hòa để tăng tính an toàn trong các tình huống nhiều nhánh liên tiếp.

So sánh được thực hiện quyết định, không cần leader tập trung. Nếu nút cục bộ thua, nó không cố gắng gộp hai cây MLS, mà chuyển sang quy trình External Join vào nhánh thắng.

### 4.2.2. External Join và Autonomous Replay

Việc gộp trực tiếp khóa của hai nhánh sau phân mảnh sẽ nguy hiểm vì khóa của nhánh thua có thể bị kéo dài vòng đời ngoài ý muốn. Thiết kế của hệ thống tránh điều này bằng ba bước:

1. Đóng băng xử lý live message trong nhóm đang heal.
2. Thay thế trạng thái nhánh thua bằng trạng thái thu được từ External Join vào nhánh thắng.
3. Chỉ phát lại các thông điệp do chính nút đó tạo ra trong cửa sổ phân mảnh.

Autonomous Replay không phát lại thông điệp của người khác, nhờ đó giữ bất biến không chối bỏ: một nút không thể nhân danh nút khác để tái mã hóa và phát tán lại nội dung. Các thông điệp replay được mã hóa lại bằng khóa epoch mới của nhánh thắng, thay vì tái sử dụng ciphertext hoặc khóa cũ của nhánh thua.

Kết luận lý thuyết là: Fork Healing không dựa trên việc hợp nhất khóa cũ; nó dựa trên gia nhập lại nhánh thắng và mã hóa lại nội dung do chính tác giả sở hữu. Điều này giúp giữ forward secrecy theo mô hình MLS tốt hơn so với cách merge trạng thái thủ công. Tuy nhiên, trong phần thực nghiệm cần phân biệt rõ: test real sidecar hiện có chứng minh thành viên bị xóa không giải mã được thông điệp sau khi bị xóa; còn bằng chứng end-to-end cho toàn bộ quy trình fork-heal với OpenMLS thật cần được chạy và ghi log riêng nếu muốn khẳng định ở mức hệ thống đầy đủ.

### 4.2.3. Token Binding trong cơ chế định danh

Trong hệ thống zero-trust, một nút không được chấp nhận chỉ vì tự khai báo danh tính. Admin ký một `InvitationToken` ràng buộc đồng thời danh tính mạng và danh tính MLS:

$$
Token = Sign_{Admin}(DisplayName \parallel PeerID_{libp2p} \parallel PublicKey_{MLS} \parallel Expiry)
$$

Ràng buộc `PeerID` buộc nút phải sở hữu khóa riêng libp2p tương ứng, vì Noise handshake chứng minh quyền sở hữu PeerID ở lớp mạng. Ràng buộc `PublicKey_MLS` ngăn kẻ tấn công thay thế khóa mật mã ứng dụng sau khi đánh cắp hoặc phát lại token. Do đó, một token hợp lệ chỉ có giá trị với đúng thiết bị đã tạo cả danh tính mạng và danh tính MLS tương ứng.

## 4.3. Phân tích độ phức tạp

### 4.3.1. Gửi thông điệp ứng dụng

Với nhóm có $N$ thành viên, baseline pairwise E2EE yêu cầu người gửi mã hóa hoặc bọc khóa riêng cho từng người nhận. Vì vậy chi phí người gửi và kích thước dữ liệu điều khiển tăng theo $O(N)$.

Với MLS, tin nhắn ứng dụng được mã hóa bằng khóa ứng dụng của nhóm tại epoch hiện tại. Ở tầng thuật toán, thao tác tạo ciphertext ứng dụng gần với $O(1)$ theo số thành viên, vì người gửi không phải tạo $N$ ciphertext riêng biệt. Tuy nhiên, triển khai production của đồ án dùng sidecar stateless: Go truyền `group_state` sang Rust, Rust xử lý rồi trả `new_group_state` để Go lưu SQLite. Vì kích thước snapshot trạng thái tăng theo quy mô nhóm, độ trễ phần mềm end-to-end có thêm chi phí tuần tự hóa/giải tuần tự hóa phụ thuộc kích thước state.

Do đó cần phân biệt hai mức:

- Mức thuật toán MLS: gửi tin nhắn ứng dụng không cần lặp qua từng người nhận.
- Mức triển khai production stateless: có thêm overhead theo kích thước `group_state`.

### 4.3.2. Cập nhật thành viên và Key Rotation

TreeKEM của MLS cập nhật các nút trên đường từ lá lên gốc cây, nên mô hình lý thuyết thường được diễn giải là tăng theo $O(\log N)$ cho phần cấu trúc cây. Tuy nhiên, thực thi cụ thể còn phụ thuộc vào thư viện, serialization, storage provider và cách benchmark tách hay gộp các lớp chi phí triển khai.

Trong triển khai hiện tại của dự án, `GrpcMLSEngine` dùng full-blob stateless RPC: Go đọc `GroupState` từ SQLite, gửi sang Rust, Rust xử lý OpenMLS rồi trả lại trạng thái mới để Go lưu bền vững. Đây là kiến trúc chính vì đảm bảo Rust không giữ trạng thái lâu dài và Go/SQLite là nguồn sự thật sau crash hoặc restart.

Vì vậy, kết luận học thuật nên viết thận trọng: MLS loại bỏ chi phí pairwise $O(N)$ của người gửi ở mức thuật toán, nhưng đường triển khai sidecar full-blob vẫn làm phát sinh overhead theo kích thước `GroupState`. Các kết quả thực nghiệm cần phản ánh đúng đường production này thay vì khẳng định toàn bộ hệ thống đạt $O(\log N)$.

## 4.4. Liên hệ giữa phân tích lý thuyết và triển khai

Các bất biến ở chương này được ánh xạ trực tiếp vào các thành phần triển khai như sau:

| Bất biến hoặc thuộc tính | Cơ chế triển khai | Kiểm chứng thực nghiệm tương ứng |
|---|---|---|
| Single-Writer | Bầu Token Holder quyết định theo `groupID`, epoch và `ActiveView`; chỉ Token Holder được phát hành Commit | Test proposal đồng thời và batching; các test Single-Writer trong `app/coordination` |
| Epoch monotonicity | Envelope mang epoch; `EpochTracker` từ chối stale epoch và đệm future epoch | Chaos test hội tụ epoch; test epoch convergence sweep |
| TreeHash convergence | Mỗi Commit cập nhật `TreeHash`; sau heal các node phải hội tụ về cùng `TreeHash` | `TestIntegration_Chaos_Convergence` đã kiểm tra cả epoch và `TreeHash` cuối |
| Fork Healing | So sánh trọng số nhánh, nhánh thua External Join vào nhánh thắng | Test fork-heal integration và crash-safety |
| Non-repudiation trong replay | Autonomous Replay chỉ phát lại thông điệp do chính node tạo ra | Các test replay/fork-heal kiểm tra không replay hộ người khác |
| Forward secrecy sau remove member | Rust sidecar xử lý MLS state thật; member bị xóa không có khóa epoch tương lai | `TestBusinessP1_E2E_RealSidecar_ForwardSecrecy` |
| Tách lớp phối hợp và mật mã | Go xử lý ordering/election/fork-heal; Rust chỉ xử lý MLS stateless | Test service/coordination và benchmark MLS sidecar |

Từ bảng trên có thể thấy đóng góp của đồ án không nằm ở việc thay đổi thuật toán mật mã MLS, mà ở lớp phối hợp phi tập trung bao quanh MLS để thay thế vai trò tuần tự hóa của Delivery Service. Chương 5 tiếp tục đánh giá các thuộc tính này bằng test tự động và benchmark thực nghiệm.
