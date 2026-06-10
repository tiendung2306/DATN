# CHƯƠNG 2: NỀN TẢNG LÝ THUYẾT

## Mở đầu chương
Chương này sẽ xây dựng hệ thống cơ sở lý thuyết khoa học và phân tích các nghiên cứu liên quan để giải quyết bài toán cốt lõi của đồ án: **Hiện thực hóa tiêu chuẩn mật mã học Messaging Layer Security (MLS) trên môi trường mạng ngang hàng (P2P) phi tập trung**. 

Nội dung chương được cấu trúc như sau: 
*   **Mục 2.1 (Ngữ cảnh bài toán)** phân tích thách thức khoa học trong việc duy trì tính nhất quán mật mã của cấu trúc cây logic TreeKEM trên môi trường phân tán, bất đồng bộ và có độ trễ lớn.
*   **Mục 2.2 (Các nghiên cứu liên quan)** đánh giá chuyên sâu các mô hình mã hóa nhóm hiện tại (như Pairwise Double Ratchet, Megolm, và các nỗ lực phi tập trung hóa MLS bằng Blockchain/BFT), chỉ ra các điểm nghẽn kiến trúc và động lực khoa học của đồ án.
*   **Mục 2.3 (Messaging Layer Security & TreeKEM)** đi sâu vào các cơ chế mật mã học của RFC 9420 và lý do tại sao MLS bắt buộc phải có một Delivery Service để tuần tự hóa trạng thái.
*   **Mục 2.4 (Mạng ngang hàng phi tập trung & Thư viện go-libp2p)** trình bày các công nghệ nền tảng truyền dẫn mạng, làm rõ tính bất đồng bộ và phi tập trung của hạ tầng mạng P2P tạo ra các thách thức trực tiếp cho lớp mật mã MLS.

---

## 2.1. Ngữ cảnh của bài toán: Thách thức nhất quán mật mã học trong hệ phân tán

Trong lý thuyết hệ phân tán, việc duy trì một trạng thái đồng thuận và nhất quán giữa các nút mạng luôn là một bài toán kinh điển. Đối với truyền thông bảo mật nhóm sử dụng mật mã mã hóa đầu-cuối (E2EE), bài toán này nâng lên một cấp độ phức tạp mới: **Nhất quán mật mã học (Cryptographic State Consistency)**.

```
       Mạng phân tán P2P bất đồng bộ (Chaotic, Latency, Churn)
                                │
       ┌────────────────────────┴────────────────────────┐
       ▼                                                 ▼
[Nút A: Commit E -> E+1]                        [Nút B: Commit E -> E+1]
       │                                                 │
       ▼                                                 ▼
[Cây TreeKEM của A rẽ nhánh A]                  [Cây TreeKEM của B rẽ nhánh B]
       └────────────────────────┬────────────────────────┘
                                ▼
         Mất đồng bộ mật mã học (Cryptographic State Fork)
             --> Các nút không thể giải mã tin nhắn của nhau!
```

### 2.1.1. Sự dịch chuyển mô hình: Từ Tập trung sang Ngang hàng
Truyền thông bảo mật nhóm truyền thống (Client-Server) dựa vào máy chủ trung tâm để đóng vai trò làm **Thực thể Tuần tự hóa (Sequencer)**. Máy chủ này nhận các yêu cầu cập nhật trạng thái nhóm, xếp chúng vào một hàng đợi duy nhất và phát đi theo thứ tự tuyến tính. 

Khi chuyển dịch sang kiến trúc ngang hàng (P2P) không máy chủ (Serverless), chúng ta đối mặt với một môi trường mạng hoàn toàn hỗn loạn:
1.  **Tính bất đồng bộ (Asynchrony):** Các thông điệp truyền trên mạng có độ trễ ngẫu nhiên và không dự đoán trước. Không có một đồng hồ vật lý chung (global clock) đáng tin cậy để làm căn cứ sắp xếp thứ tự tin nhắn.
2.  **Sự biến động nút mạng (Peer Churn):** Các nút mạng tự do gia nhập, rời mạng hoặc mất kết nối đột ngột mà không báo trước.
3.  **Sự phân mảnh mạng (Network Partition):** Mạng có thể bị chia cắt vật lý hoặc logic thành nhiều phân mảnh độc lập hoạt động song song.

### 2.1.2. Mâu thuẫn cốt lõi của MLS trên P2P
Tiêu chuẩn mật mã nhóm thế hệ mới MLS (RFC 9420) đại diện cho nhóm bằng một cây mật mã nhị phân TreeKEM. Trạng thái của cây tiến hóa qua các kỷ nguyên (Epochs) thông qua các bản tin mật mã gọi là `Commit` (chứa các cập nhật thành viên hoặc cập nhật khóa).

Mâu thuẫn kỹ thuật lớn nhất khi đưa MLS vào môi trường P2P là:
*   **Đặc tả MLS (RFC 9420):** Giả định sự tồn tại của một **Delivery Service (DS)** làm nhiệm vụ sắp xếp thứ tự các Commit. Nếu có hai Commit cùng gửi lên tại Epoch $E$, DS sẽ chấp nhận Commit đến trước, tăng Epoch lên $E+1$, và từ chối Commit đến sau.
*   **Môi trường P2P:** Không có DS. Hai nút mạng ở cách xa nhau có thể đồng thời phát ra hai bản tin Commit cạnh tranh nhau tại cùng một Epoch $E$ (concurrent commits). 
*   **Hậu quả:** Nếu không có lớp điều phối, các nút nhận được cả hai Commit này sẽ tiến hành cập nhật cây TreeKEM theo các hướng khác nhau, dẫn đến hiện tượng **Rẽ nhánh mật mã (Cryptographic State Fork)**. Khi đã bị rẽ nhánh, các nút ở nhánh này không thể giải mã tin nhắn của các nút ở nhánh kia do mất đồng bộ về khóa mật mã gốc ($K_{root}$). Trạng thái mật mã của toàn nhóm bị sụp đổ hoàn toàn.

Do đó, ngữ cảnh khoa học của đồ án này là giải quyết bài toán: **Làm thế nào để thiết lập một giao thức điều phối phi tập trung, đóng vai trò như một lớp trung gian ảo hóa chức năng tuần tự hóa của Delivery Service, đảm bảo tính nhất quán đơn điệu của cây mật mã MLS trên mạng P2P bất đồng bộ.**

---

## 2.2. Các kết quả nghiên cứu tương tự và Động lực khoa học

Để bảo mật thông tin nhóm trong môi trường mạng, các nghiên cứu và giải pháp công nghệ trước đây đã tiếp cận theo nhiều hướng khác nhau. Dưới đây là phân tích ưu nhược điểm của từng hướng tiếp cận nhằm làm rõ khoảng trống khoa học mà đồ án này hướng tới.

### 2.2.1. Nhóm giải pháp dựa trên Mật mã học đa cặp (Pairwise E2EE)
Tiêu biểu là giao thức **Double Ratchet (Signal Protocol)** và cơ chế **Sender Keys** dùng trong các ứng dụng như Signal, WhatsApp.
*   **Cơ chế:** Để mã hóa nhóm $N$ người, hệ thống phân rã thành $N(N-1)/2$ kênh truyền 1-1 riêng biệt sử dụng thuật toán xoay khóa Double Ratchet. Mỗi khi cấu trúc nhóm thay đổi, thông tin khóa mới phải được mã hóa và phân phối riêng lẻ cho từng thành viên.
*   **Nhược điểm về độ mở rộng (Scalability):** Độ phức tạp tính toán tăng tuyến tính $O(N)$ tại nút gửi và độ phức tạp băng thông tăng theo hàm bình phương $O(N^2)$ trên toàn mạng. Với các thiết bị di động hoặc nút P2P có tài nguyên hạn chế, việc xử lý nhóm chat lớn (hàng trăm đến hàng nghìn người) là không khả thi.
*   **Đóng góp của đồ án so với hướng này:** Đồ án kế thừa cấu trúc cây TreeKEM của MLS để đưa độ phức tạp về mức Logarithmic $O(\log N)$, giải quyết triệt để bài toán hiệu năng nhóm lớn.

### 2.2.2. Nhóm giải pháp nới lỏng thuộc tính bảo mật (Megolm Protocol)
Được phát triển bởi Matrix để khắc phục điểm yếu hiệu năng của Signal trong nhóm lớn.
*   **Cơ chế:** Sử dụng cơ chế khóa gửi tuần hoàn (key stream). Một nút sử dụng cùng một khóa để mã hóa nhiều tin nhắn liên tiếp cho nhóm, giảm thiểu tần suất thực hiện các phép tính Diffie-Hellman đắt đỏ.
*   **Nhược điểm về an toàn:** Vi phạm nghiêm trọng thuộc tính bảo mật sau thỏa hiệp (Post-Compromise Security - PCS). Kẻ tấn công nếu chiếm được thiết bị tại một thời điểm có thể giải mã ngược các tin nhắn trong quá khứ và các tin nhắn trong tương lai thuộc cùng một chu kỳ khóa.
*   **Đóng góp của đồ án so với hướng này:** Đồ án kiên quyết giữ vững các bảo chứng an toàn mật mã học cao nhất của MLS (đạt FS và PCS trên từng kỷ nguyên) mà không chấp nhận hạ thấp tiêu chuẩn bảo mật để đổi lấy hiệu năng.

### 2.2.3. Nhóm nghiên cứu phi tập trung hóa MLS (Decentralized MLS)
Đây là hướng nghiên cứu học thuật đang được cộng đồng quốc tế quan tâm. Đã có một số công trình đề xuất giải pháp chạy MLS trên P2P, chia làm hai nhánh chính:

1.  **Nhánh sử dụng giải thuật đồng thuận phân tán nặng (Blockchain/BFT Consensus):**
    *   *Cơ chế:* Sử dụng một sổ cái chia sẻ (Shared Ledger), Blockchain hoặc giải thuật PBFT/Raft chạy giữa các nút để đồng thuận thứ tự của các Commit.
    *   *Ưu điểm:* Giải quyết được bài toán tuần tự hóa Commit một cách tuyệt đối.
    *   *Nhược điểm:* Độ trễ giao dịch cực kỳ lớn (phải đợi block time hoặc nhiều vòng biểu quyết mạng). Chi phí băng thông và năng lượng tính toán cao, hoàn toàn không phù hợp với các nút mạng P2P di động thường xuyên bị mất kết nối (high churn rate).
2.  **Nhánh sử dụng mô hình Token tĩnh (Static Token):**
    *   *Cơ chế:* Chỉ định cố định một nút duy nhất (thường là người tạo nhóm - Creator) nắm giữ quyền ghi Commit.
    *   *Ưu điểm:* Thiết kế đơn giản, loại bỏ hoàn toàn concurrent commits.
    *   *Nhược điểm:* Lỗi điểm sập duy nhất (SPOF). Nếu Creator bị ngắt kết nối vật lý hoặc thoát ứng dụng, toàn bộ nhóm sẽ rơi vào trạng thái bế tắc (deadlock), không thành viên nào có thể cập nhật khóa hoặc mời thêm người mới.

**Bảng so sánh tổng hợp các hướng nghiên cứu liên quan:**

| Tiêu chí so sánh | Pairwise (Signal Sender Keys) | Megolm (Matrix) | Decentralized MLS (BFT/Raft) | Giao thức điều phối đề xuất |
| :--- | :---: | :---: | :---: | :---: |
| **Kiến trúc mạng** | Tập trung / Phân phối | Liên bang (Federated) | Ngang hàng (P2P) | **Ngang hàng (P2P) phi tập trung** |
| **Băng thông phân phối khóa (khi thay đổi nhóm $N$ người)** | $O(N)$ thông điệp mã hóa đơn lẻ | $O(N)$ thông điệp khi xoay khóa | $O(N^2)$ hoặc $O(N)$ thông điệp đồng thuận + $O(\log N)$ kích thước Commit | **$O(\log N)$ kích thước Commit** (Phát qua GossipSub, $O(0)$ tin nhắn bầu chọn) |
| **Độ phức tạp tính toán mật mã (mỗi nút khi cập nhật nhóm)** | $O(N)$ đối với nút gửi, $O(1)$ đối với nút nhận | $O(1)$ mật mã đối xứng, $O(N)$ khi xoay khóa | $O(\log N)$ phép tính trên cây TreeKEM | **$O(\log N)$ phép tính trên cây TreeKEM** |
| **Độ trễ trước khi gửi tin (Epoch Transition Delay)** | Thấp (Không trễ) | Thấp (Không trễ) | Cao (Phụ thuộc số vòng đồng thuận của PBFT/Raft) | **Thấp** (Xác định Token Holder offline bằng hàm băm local, trễ GossipSub) |
| **Bảo mật FS & PCS trên mỗi tin nhắn** | Đầy đủ | FS đầy đủ, PCS yếu (do tái sử dụng session key) | Đầy đủ | **Đầy đủ** (Do kế thừa từ đặc tả MLS) |
| **Hoạt động khi phân tách mạng (Network Partition)** | Không thể (Do phụ thuộc máy chủ trung tâm) | Không thể (Do phụ thuộc máy chủ trung tâm) | Chỉ phân mảnh đa số (Quorum $\ge 50\%$) hoạt động; phân mảnh thiểu số bị khóa | **Tất cả các phân mảnh hoạt động độc lập** (Tự bầu Token Holder nội bộ) và **tự phục hồi khi sáp nhập** |

**Động lực khoa học của đồ án:**
Khoảng trống nghiên cứu lớn nhất hiện nay là: **Chưa có một giải pháp điều phối MLS phi tập trung nào vừa giữ nguyên vẹn thuộc tính mật mã mạnh mẽ của RFC 9420, vừa có hiệu năng nhẹ nhàng để chạy trên nút P2P thông thường, đồng thời có khả năng phục hồi tự động khi mạng bị chia cắt.** 

Đồ án này đề xuất một giải pháp mới: **Giao thức điều phối 4 cơ chế (Single-Writer, Epoch Checks, Fork Healing, HLC)** chạy hoàn toàn bất đồng bộ ở lớp Go Host bao quanh lõi OpenMLS chạy trong Rust Sidecar, mang lại khả năng hội tụ trạng thái mật mã học nhanh chóng và an toàn tuyệt đối.

---

## 2.3. Tiêu chuẩn mật mã học Messaging Layer Security (MLS) & TreeKEM

Để hiểu rõ cách lớp điều phối vận hành, việc nghiên cứu sâu cấu trúc toán học và mật mã học của tiêu chuẩn MLS (RFC 9420) là bắt buộc.

### 2.3.1. Cấu trúc cây TreeKEM và cơ chế chia sẻ bí mật nhóm
TreeKEM sử dụng một cây nhị phân để quản lý khóa nhóm. Cấu trúc toán học này dựa trên nguyên lý: **Mỗi thành viên chỉ nắm giữ khóa bí mật của các nút nằm trên đường dẫn trực tiếp từ lá của mình đến gốc cây.**

Giả sử nhóm có 4 thành viên ($N_1, N_2, N_3, N_4$ tương ứng với các lá $L_1, L_2, L_3, L_4$):
*   Nút gốc (Root) đại diện cho khóa chung của toàn nhóm. Khóa bí mật của Root ($sk_{root}$) được dùng để phái sinh ra khóa ứng dụng (Application Secrets) mã hóa dữ liệu.
*   Nút trung gian $P_{12}$ là cha của $L_1$ và $L_2$. Khóa công khai của nó ($pk_{12}$) được biết bởi cả nhóm, nhưng khóa bí mật ($sk_{12}$) chỉ được biết bởi $N_1$ và $N_2$.
*   Nút trung gian $P_{34}$ là cha của $L_3$ và $L_4$. Khóa bí mật ($sk_{34}$) chỉ được biết bởi $N_3$ và $N_4$.

Khi $N_1$ muốn gửi tin nhắn, $N_1$ sử dụng khóa ứng dụng được tạo ra từ $sk_{root}$. Vì $N_1$ nằm trên nhánh của $P_{12}$, $N_1$ biết $sk_{12}$ và từ đó biết $sk_{root}$.

### 2.3.2. Tiến trình chuyển dịch Epoch thông qua Commit (Path Update)
Khi có bất kỳ thay đổi nào về cấu trúc nhóm (ví dụ: $N_1$ muốn trục xuất $N_4$), trạng thái mật mã của cây phải thay đổi để đảm bảo tính an toàn. Quá trình này được gọi là **Path Update**:

```
Bước 1: N_1 sinh cặp khóa mới cho chính mình và các nút cha: L_1, P_12, Root
Bước 2: N_1 mã hóa khóa bí mật mới của cha cho các nhánh con liền kề (copath):
  - Mã hóa sk_{12} mới bằng pk_{L2} (gửi cho N_2)
  - Mã hóa sk_{root} mới bằng pk_{P34} (gửi cho N_3 và N_4 - lúc này chưa bị trục xuất)
Bước 3: N_1 đóng gói các khóa đã mã hóa này vào bản tin Commit và phát lên mạng.
```

Khi các thành viên khác nhận được bản tin Commit này, họ sẽ sử dụng khóa bí mật cao nhất mà họ biết trên đường dẫn trực tiếp cũ để giải mã và lấy ra khóa bí mật mới của cha chung, từ đó nâng trạng thái cây lên Epoch mới $E+1$.

### 2.3.3. Giả định bắt buộc về thực thể Delivery Service (DS)
Mô hình toán học của TreeKEM yêu cầu sự chuyển dịch trạng thái của cây phải tuân theo thứ tự tuyến tính nghiêm ngặt. Nếu hai nút cùng phát hành Commit tại Epoch $E$:
*   Commit của $N_1$ tạo ra cây $T_1$ ở Epoch $E+1$.
*   Commit của $N_2$ tạo ra cây $T_2$ ở Epoch $E+1$.
*   Vì $T_1 \neq T_2$, nếu nút $N_3$ áp dụng Commit của $N_1$ trước, nó sẽ chuyển sang trạng thái $T_1(E+1)$. Lúc này, bản tin Commit của $N_2$ (vốn được xây dựng dựa trên trạng thái cây cũ ở Epoch $E$) khi gửi đến $N_3$ sẽ không thể giải mã được nữa vì cấu trúc khóa đã thay đổi.

Đặc tả RFC 9420 giải quyết vấn đề này bằng cách quy định **Delivery Service (DS)** là thực thể duy nhất quyết định Commit nào được chấp nhận. DS hoạt động như một chốt chặn tuần tự (Global Sequencer). Trên mạng ngang hàng phi tập trung, việc thiếu vắng DS chính là rào cản lớn nhất khiến MLS chưa thể tự vận hành.

---

## 2.4. Mạng ngang hàng phi tập trung & Thư viện go-libp2p

Hạ tầng truyền dẫn của đồ án dựa trên mạng ngang hàng (P2P) bất đồng bộ. Để hiện thực hóa lớp mạng này một cách khoa học, thư viện `go-libp2p` được sử dụng làm nền tảng.

### 2.4.1. Đặc tính bất đồng bộ của mạng P2P
Mạng P2P khác biệt hoàn toàn với mạng Client-Server truyền thống ở các điểm sau:
1.  **Không có địa chỉ IP cố định:** Các nút thường nằm sau các lớp NAT (Network Address Translation) hoặc Firewall khác nhau. Việc thiết lập kết nối trực tiếp (NAT Traversal) đòi hỏi các kỹ thuật như STUN/TURN hoặc UPnP.
2.  **Độ trễ truyền tin không đồng đều (Network Latency Skew):** Bản tin GossipSub có thể mất vài mili-giây để đến nút lân cận nhưng mất hàng giây để lan truyền đến các nút ở xa trong mạng mesh.
3.  **Tần suất mất kết nối cao (High Churn Rate):** Các nút mạng phân tán thường là thiết bị cá nhân, có thể bị ngắt kết nối do mất sóng Wi-Fi, chuyển vùng mạng hoặc chuyển sang trạng thái ngủ (sleep mode).

### 2.4.2. Vai trò của go-libp2p trong việc giải quyết bài toán P2P
Thư viện `go-libp2p` cung cấp các giải pháp mô-đun hóa để giải quyết các thách thức trên:
*   **Định danh nút bằng mã hóa (Cryptographic PeerID):** Mỗi nút trong mạng được định danh duy nhất bằng một `PeerID` (phái sinh từ mã băm SHA-256 khóa công khai Ed25519 của thiết bị). Định danh này không phụ thuộc vào địa chỉ IP, cho phép duy trì định danh nhất quán ngay cả khi thiết bị di chuyển giữa các mạng khác nhau.
*   **Noise Protocol Handshake:** Lớp bảo mật Noise thực hiện trao đổi khóa Diffie-Hellman ngay khi thiết lập kết nối TCP/QUIC, tạo ra một đường truyền mật mã hóa đối xứng an toàn, đồng thời xác thực danh tính PeerID của nút đối diện.
*   **GossipSub Mesh Router:** GossipSub tự động xây dựng một đồ thị liên kết (mesh topology) giữa các nút đang hoạt động. Khi một nút phát bản tin (Proposal hoặc Commit), GossipSub sẽ đẩy nhanh tin nhắn qua mạng lưới mesh này bằng thuật toán chuyển tiếp có giới hạn (flood publishing) kết hợp với các bản tin siêu dữ liệu (IHAVE/IWANT) để kéo các gói tin bị thiếu.
*   **Kademlia DHT & Discovery:** DHT giải quyết bài toán tìm kiếm địa chỉ của các nút trong mạng diện rộng bằng cách lưu trữ các bản ghi ánh xạ PeerID $\to$ Multiaddress trên toàn mạng lưới một cách phân tán.

Sự kết hợp giữa tính phi tập trung của `go-libp2p` và tính bảo mật của MLS tạo ra một hệ thống truyền thông có độ tự chủ cao, nhưng cũng đặt ra thách thức cực kỳ lớn cho lớp điều phối (Go) trong việc bảo vệ tính đơn điệu và nhất quán của trạng thái mật mã học.

---

## Kết luận chương
Chương 2 đã thiết lập một nền tảng khoa học vững chắc cho đồ án. Bằng việc phân tích sâu ngữ cảnh bài toán "Nhất quán mật mã học" trong hệ phân tán bất đồng bộ và so sánh các nghiên cứu tương tự, đồ án đã định vị rõ ràng khoảng trống khoa học cần giải quyết: **Sự thiếu vắng một giao thức điều phối nhẹ nhàng để chạy MLS trên P2P không máy chủ**. 

Các kiến thức nền tảng về TreeKEM (MLS - RFC 9420) và hạ tầng mạng P2P (`go-libp2p`) đã làm sáng tỏ nguyên nhân kỹ thuật của vấn đề rẽ nhánh mật mã (cryptographic state fork). Đây là tiền đề trực tiếp để Chương 3 đi vào trình bày phương pháp đề xuất: Giao thức điều phối 4 cơ chế giải quyết triệt để bài toán này.
