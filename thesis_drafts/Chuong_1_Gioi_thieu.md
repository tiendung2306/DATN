# CHƯƠNG 1: GIỚI THIỆU ĐỀ TÀI

## 1.1. Đặt vấn đề
Trong kỷ nguyên số hóa và toàn cầu hóa hiện nay, bảo mật thông tin liên lạc nội bộ đã trở thành một trong những ưu tiên hàng đầu của các tổ chức, doanh nghiệp và các cơ quan chính phủ. Sự rò rỉ thông tin nhạy cảm, dữ liệu chiến lược hay tài liệu mật không chỉ gây ra những thiệt hại kinh tế nghiêm trọng mà còn đe dọa đến vị thế và sự sống còn của tổ chức. Để giải quyết thách thức này, mật mã mã hóa đầu-cuối (End-to-End Encryption - E2EE) đã ra đời và trở thành tiêu chuẩn vàng, đảm bảo rằng chỉ có những người tham gia hội thoại hợp lệ mới có thể giải mã và đọc được nội dung thông điệp.

Tuy nhiên, hầu hết các hệ thống E2EE thương mại và phổ biến hiện nay (chẳng hạn như Signal, WhatsApp, Telegram, Microsoft Teams, Slack) đều được xây dựng dựa trên kiến trúc tập trung (Client-Server). Trong mô hình này, mọi thông tin liên lạc đều phải đi qua một hoặc một cụm máy chủ trung tâm do một bên thứ ba vận hành. Kiến trúc tập trung này bộc lộ những hạn chế bảo mật và vận hành sâu sắc:
1.  **Điểm sập duy nhất (Single Point of Failure - SPOF):** Khi máy chủ trung tâm gặp sự cố kỹ thuật, bị tấn công từ chối dịch vụ (DDoS) hoặc bị vô hiệu hóa bởi thiên tai, toàn bộ mạng lưới liên lạc của tổ chức sẽ bị tê liệt.
2.  **Mối đe dọa từ bên thứ ba (Third-party Trust):** Việc đặt niềm tin vào máy chủ trung tâm vi phạm nguyên tắc cơ bản của an ninh an toàn thông tin hiện đại — mô hình Zero-Trust. Ngay cả khi dữ liệu được mã hóa, nhà cung cấp máy chủ vẫn có thể thu thập siêu dữ liệu (metadata) cực kỳ nhạy cảm như tần suất liên lạc, cấu trúc sơ đồ tổ chức, thời gian hoạt động của các thành viên. Đồng thời, máy chủ trung tâm luôn là mục tiêu tấn công hàng đầu của tin tặc hoặc chịu sức ép pháp lý từ các cơ quan quản lý để cài đặt "cửa sau" (backdoor).
3.  **Khả năng hoạt động ngoại tuyến và nội bộ bị hạn chế:** Trong môi trường mạng cô lập (air-gapped network) của các tổ chức quân sự, chính phủ hoặc các doanh nghiệp có hạ tầng mạng nội bộ cách ly với Internet, các ứng dụng dựa trên máy chủ Internet hoàn toàn không khả thi.

Để khắc phục triệt để các rủi ro của kiến trúc tập trung, xu hướng phát triển ứng dụng **Local-First** và mạng ngang hàng phi tập trung (**Peer-to-Peer - P2P**) đã được đề xuất và phát triển mạnh mẽ. Trong mạng P2P, các nút mạng (peers) tự động khám phá và thiết lập kết nối trực tiếp với nhau (thông qua mạng cục bộ LAN, mạng diện rộng WAN hoặc mạng ảo riêng VPN) mà không phụ thuộc vào bất kỳ máy chủ trung tâm nào. Kiến trúc này mang lại khả năng chống chịu lỗi vượt trội, bảo toàn tuyệt đối quyền sở hữu dữ liệu của người dùng, và cho phép các nhóm cộng tác duy trì liên lạc ngoại tuyến ngay cả khi kết nối Internet bên ngoài bị cắt đứt.

Mặc dù mang lại nhiều lợi ích thiết thực, việc triển khai một giao thức mã hóa đầu-cuối an toàn, hiệu năng cao trên môi trường P2P phi tập trung và bất đồng bộ lại gặp phải những thách thức kỹ thuật vô cùng lớn về mặt mật mã học và lý thuyết hệ phân tán. Đây chính là động lực chính để nghiên cứu và thực hiện đề tài đồ án tốt nghiệp này.

---

## 1.2. Các giải pháp hiện tại và hạn chế
Để hiện thực hóa mã hóa đầu-cuối (E2EE) cho nhóm, các giải pháp công nghệ hiện tại chủ yếu dựa trên hai trường phái giao thức mật mã chính:

### 1. Giao thức Double Ratchet (Signal Protocol) và Megolm (Matrix Protocol)
*   **Giao thức Double Ratchet:** Được phát triển bởi Signal, giao thức này sử dụng cơ chế xoay vòng khóa liên tục dựa trên phép trao đổi khóa Diffie-Hellman và hàm băm một chiều để đạt được tính bảo mật xuôi (Forward Secrecy - FS) và bảo mật sau thỏa hiệp (Post-Compromise Security - PCS) ở mức độ rất cao. Tuy nhiên, Double Ratchet ban đầu chỉ được thiết kế cho các cuộc hội thoại 1-1. Để áp dụng cho nhóm chat đông người, Signal sử dụng giải pháp **Sender Keys (Pairwise Distribution)**: mỗi thành viên tự sinh một khóa gửi tin và phân phối an toàn khóa này cho $N-1$ thành viên còn lại thông qua các kênh 1-1 mã hóa riêng biệt.
    *   *Hạn chế về hiệu năng:* Độ phức tạp về băng thông và tính toán để thiết lập nhóm hoặc cập nhật thành viên (khi có người join hoặc leave) tăng tuyến tính theo quy mô nhóm, đạt mức $O(N)$ cho mỗi nút gửi và tổng băng thông mạng lưới là $O(N^2)$. Khi quy mô nhóm đạt đến hàng trăm hoặc hàng nghìn người, chi phí này trở thành một gánh nặng cực kỳ lớn, gây chậm trễ nghiêm trọng cho thiết bị di động hoặc máy tính cá nhân của người dùng.
*   **Giao thức Megolm:** Được Matrix sử dụng, Megolm tối ưu hóa bằng cách cho phép một khóa gửi được sử dụng tuần tuần để mã hóa nhiều tin nhắn gửi đến nhóm, giảm tần suất trao đổi khóa Diffie-Hellman.
    *   *Hạn chế về an toàn:* Việc tái sử dụng khóa làm giảm đáng kể khả năng bảo mật sau thỏa hiệp (PCS). Nếu thiết bị của một thành viên bị xâm nhập, kẻ tấn công có thể giải mã ngược các tin nhắn trong quá khứ hoặc tin nhắn tương lai trong cùng phiên khóa đó cho đến khi có một đợt cập nhật khóa thủ công diễn ra.

### 2. Giao thức Messaging Layer Security (MLS - RFC 9420)
Để giải quyết bài toán hiệu năng của các giao thức pairwise E2EE trong nhóm lớn, Hiệp hội Kỹ nghệ Internet (IETF) đã chuẩn hóa giao thức **MLS (Messaging Layer Security)** vào tháng 7 năm 2023. MLS giới thiệu cấu trúc mật mã cây logic gọi là **TreeKEM** (Cây mã hóa khóa bất đối xứng). 
Trong TreeKEM, các thành viên của nhóm được sắp xếp vào các lá (leaves) của một cây nhị phân đầy đủ. Mỗi nút trung gian của cây đại diện cho một khóa công khai nhóm con, và nút gốc của cây chứa khóa mật mã dùng để mã hóa thông điệp chung cho toàn bộ nhóm. Khi một nút thực hiện cập nhật khóa (để đạt PCS) hoặc khi thêm/xóa thành viên, hệ thống chỉ cần thay đổi đường đi từ lá của nút đó lên gốc cây.
*   *Lợi thế:* Độ phức tạp tính toán và băng thông cho các thao tác cập nhật nhóm được giảm từ tuyến tính $O(N)$ xuống logarithmic $O(\log N)$. Điều này cho phép MLS mở rộng quy mô nhóm an toàn lên tới hàng chục nghìn thành viên mà không làm suy giảm hiệu năng hệ thống.

### Hạn chế cốt lõi của MLS khi đưa vào môi trường P2P phi tập trung
Mặc dù MLS là một bước đột phá về mật mã học nhóm, đặc tả nguyên bản của MLS (RFC 9420) được xây dựng dựa trên một giả định kiến trúc quan trọng: **Sự tồn tại của một Máy chủ Phân phối trung tâm (Delivery Service - DS)** để serialize (tuần tự hóa) tất cả các thao tác thay đổi trạng thái nhóm.

Trong MLS, bất kỳ thay đổi nào đối với Ratchet Tree (chẳng hạn như một Commit thêm thành viên mới, trục xuất thành viên, hoặc xoay vòng khóa định kỳ) đều đưa nhóm dịch chuyển sang một kỷ nguyên mới (Epoch $E \to E+1$). 
*   Nếu không có máy chủ trung tâm DS làm trọng tài đứng ra sắp xếp thứ tự các bản tin Commit, hai nút mạng bất kỳ trong môi trường P2P bất đồng bộ có thể đồng thời phát sóng hai bản tin Commit cạnh tranh nhau tại cùng một Epoch $E$ (gọi là *concurrent commits*). 
*   Khi điều này xảy ra, cây mật mã TreeKEM tại mỗi nút sẽ rẽ nhánh (fork) sang các trạng thái khác nhau. Các nút ở nhánh rẽ này sẽ không thể giải mã được tin nhắn của các nút ở nhánh rẽ kia do bất đồng bộ về khóa mật mã gốc. Trạng thái mật mã của nhóm chat P2P lúc này sẽ bị sụp đổ hoàn toàn và không thể tự phục hồi nếu không có sự can thiệp thủ công từ bên ngoài.

Do đó, thách thức nghiên cứu lớn nhất hiện nay là: **Làm thế nào để duy trì tính nhất quán và tuần tự hóa các Commit của giao thức mật mã MLS trên mạng P2P hoàn toàn phi tập trung và bất đồng bộ mà không làm mất đi tính an toàn mật mã học nguyên bản của tiêu chuẩn RFC 9420?**

---

## 1.3. Mục tiêu và định hướng giải pháp

### Mục tiêu của đồ án
1.  **Về mặt nghiên cứu (Research):** Thiết kế và đặc tả một **Giao thức Điều phối Phi tập trung (Decentralized Coordination Protocol)** bao quanh MLS để thay thế vai trò tuần tự hóa của Delivery Service trung tâm trên mạng P2P bất đồng bộ. Giao thức này phải đảm bảo tính nhất quán của cây mật mã (TreeKEM), ngăn chặn tuyệt đối tình trạng phân rã trạng thái mật mã (concurrent commits), đồng thời hỗ trợ khả năng tự hàn gắn nhóm khi mạng bị chia cắt vật lý và kết nối lại.
2.  **Về mặt ứng dụng (Application):** Phát triển một ứng dụng cộng tác nội bộ (chat nhóm, tạo kênh tin tức dạng luồng, chia sẻ tệp tin an toàn) hoàn chỉnh theo mô hình Zero-Trust và Local-First. Ứng dụng phải tích hợp giao thức điều phối đề xuất, hoạt động mượt mà trên môi trường P2P thực tế (bao gồm cơ chế tự phát hiện thiết bị trong mạng LAN/VLAN và đồng bộ hóa offline).

### Định hướng giải pháp: Giao thức điều phối 4 cơ chế
Để đạt được các mục tiêu trên, đồ án đề xuất xây dựng Lớp điều phối (Coordination Layer) tại Go Host bao quanh Lớp mật mã (Crypto Layer) chạy trong Rust Sidecar, dựa trên 4 cơ chế cốt lõi:

```
+-----------------------------------------------------------------------+
|                         COORDINATION LAYER (Go)                       |
|                                                                       |
|  [Cơ chế 1: Single-Writer]      [Cơ chế 2: Epoch Checks]              |
|  - Bầu chọn động TH(E, P)       - Kiểm tra tính đơn điệu Epoch        |
|  - Ngăn ngừa concurrent commit  - Bộ đệm tương lai & Sync offline      |
|                                                                       |
|  [Cơ chế 3: Fork Healing]       [Cơ chế 4: HLC Message Ordering]       |
|  - Trọng số W(C, E, H)          - Sắp xếp tin nhắn trong Epoch        |
|  - External Join & Replay       - NTP-independent, Causal Consistency |
+-----------------------------------------------------------------------+
                                   |
                         gRPC over localhost (IPC)
                                   |
+-----------------------------------------------------------------------+
|                          CRYPTO LAYER (Rust)                          |
|                                                                       |
|  - Thư viện OpenMLS (RFC 9420)  - Xử lý mật mã thuần túy              |
|  - Quản lý TreeKEM mật mã       - Nhận/trả GroupState qua gRPC         |
+-----------------------------------------------------------------------+
```

1.  **Cơ chế Người ghi duy nhất (Single-Writer Protocol):**
    Thay vì giải quyết xung đột sau khi nó xảy ra, giao thức ngăn chặn triệt để concurrent commits bằng cách chỉ định duy nhất một nút mạng có quyền Commit tại mỗi epoch — nút này được gọi là **Epoch Token Holder**. Quyền Token Holder được bầu chọn tự tất định thông qua một hàm băm phụ thuộc vào PeerID và số Epoch hiện tại, đảm bảo tất cả các nút online đều đồng thuận về cùng một người viết mà không cần tốn chi phí đàm phán hay bỏ phiếu bầu. Giao thức tối ưu hóa bằng cách cho bầu chọn động phụ thuộc ngữ cảnh các đề xuất (`Eligible(E, P)`) để loại trừ các node sắp bị xóa, tránh bế tắc hệ thống.
2.  **Cơ chế Nhất quán Epoch (Epoch Consistency Checks):**
    Thiết lập lưới bảo vệ tại mỗi nút mạng để kiểm tra tính đơn điệu (monotonicity) của Epoch trên mọi thông điệp mật mã nhận được từ mạng GossipSub. Các tin nhắn cũ sẽ bị loại bỏ, các tin nhắn tương lai do lệch pha mạng sẽ được đưa vào hàng đợi tạm thời và kích hoạt quy trình kéo dữ liệu đồng bộ trực tiếp (direct stream sync) từ các peer khác.
3.  **Cơ chế Hàn gắn Phân mảnh Nhóm (Group Fork Healing):**
    Khi mạng vật lý bị chia cắt làm nhiều mảnh (network partition), mỗi phân mảnh sẽ tiến hóa độc lập theo các nhánh Epoch khác nhau. Khi kết nối lại, cơ chế Fork Healing sẽ tự động phát hiện sự phân kỳ của cây mật mã qua Gossip Heartbeat, tiến hành so sánh trọng số các nhánh dựa trên hàm đa biến $W = (C_{members}, E, H_{commit})$ để chọn ra nhánh thắng duy nhất. Các nút ở nhánh thua sẽ tiến hành hủy bỏ khóa cũ để bảo vệ Forward Secrecy và thực hiện **External Join** vào nhánh thắng, sau đó chạy lại các tin nhắn mồ côi của chính mình (Autonomous Replay).
4.  **Đồng bộ Thứ tự Hiển thị tin nhắn (Hybrid Logical Clock - HLC):**
    Vì Epoch chỉ thay đổi khi có Commit thay đổi trạng thái nhóm, hàng loạt tin nhắn ứng dụng thông thường có thể được gửi song song bởi nhiều người dùng trong cùng một Epoch. Để đảm bảo tính nhất quán nhân quả và hiển thị tin nhắn giống hệt nhau trên mọi thiết bị mà không tin cậy giờ hệ thống của máy (có thể bị clock skew trong mạng không có NTP), giao thức sử dụng HLC để gán nhãn thời gian và sắp xếp tin nhắn.

---

## 1.4. Đóng góp của đồ án
Đồ án tốt nghiệp đóng góp cả về mặt nghiên cứu lý thuyết lẫn kỹ thuật thực thi ứng dụng thực tiễn, cụ thể gồm:

### 1. Đóng góp về mặt Nghiên cứu & Giao thức (Theoretical Contributions)
*   **Xây dựng Giao thức Điều phối Phi tập trung bao quanh MLS:** Giải quyết thành công bài toán chạy MLS trong môi trường P2P không có Delivery Service trung tâm. Chứng minh được tính nhất quán và tính hội tụ trạng thái của cây mật mã dưới tác động của sự bất đồng bộ mạng.
*   **Thiết kế Giải thuật Dynamic Token Holder chống Deadlock:** Đưa ra công thức xác định tập hợp ứng viên hợp lệ $Eligible(E, P)$ để ngăn ngừa hiện tượng nghẽn mạng khi Token Holder tự rời nhóm hoặc bị trục xuất, loại bỏ các leaky abstractions của các nghiên cứu trước đây.
*   **Đặc tả giải thuật Fork Healing an toàn:** Đưa ra quy trình phục hồi phân mảnh mạng tự động, bền vững, an toàn mật mã học (đạt Forward Secrecy nhờ tiêu hủy khóa nhánh thua) và tôn trọng tính phi bác bỏ (Non-repudiation) thông qua Autonomous Replay.

### 2. Đóng góp về mặt Hiện thực Kỹ thuật (Engineering Contributions)
*   **Kiến trúc Sidecar Go-Rust qua gRPC:** Thiết lập giải pháp quản lý tiến trình tự động, phân phối cổng kết nối IPC an toàn. Go giữ vai trò điều phối, lưu trữ trạng thái nhóm trong SQLite và truyền `GroupState` cho Rust xử lý các thao tác OpenMLS. Cách tách lớp này giúp cô lập logic mật mã trong Rust, đồng thời giữ Go/SQLite là nguồn sự thật bền vững của ứng dụng.
*   **Ứng dụng Chat Bảo mật Serverless hoàn chỉnh:** Phát triển thành công phần mềm desktop sử dụng framework Wails (Go + React/TypeScript) tích hợp SQLite lưu trữ cục bộ, thư viện go-libp2p hỗ trợ khám phá kết nối tự động trong mạng LAN (qua mDNS) và diện rộng (qua Kademlia DHT).
*   **Tính năng nâng cao chuyên nghiệp:**
    *   *Offline Sync:* Hỗ trợ đồng bộ hóa thông điệp qua luồng trực tiếp khi thiết bị ngoại tuyến quay trở lại mạng.
    *   *Universal Blind-Store:* Cơ chế nhân bản tin nhắn mù trên các nút lưu trữ được chỉ định để tối ưu hóa khả năng phân phối tin nhắn ngoại tuyến.
    *   *Secure Identity Migration:* Cơ chế di chuyển danh tính thiết bị an toàn qua tệp backup Ed25519 được mã hóa mật khẩu, đi kèm cơ chế Session Takeover ngăn chặn nhân bản phiên hoạt động.
    *   *Secure File Transfer:* Cơ chế truyền tải tệp tin tốc độ cao cắt nhỏ trực tiếp qua P2P sử dụng khóa phái sinh một lần từ MLS Exporter (RFC 9420).

---

## 1.5. Bố cục đồ án
Quyển đồ án tốt nghiệp được cấu trúc thành 6 chương chính và danh mục tài liệu tham khảo:

*   **CHƯƠNG 1: GIỚI THIỆU ĐỀ TÀI**
    Đặt vấn đề nghiên cứu, phân tích các giải pháp bảo mật nhóm hiện nay cùng các hạn chế cốt lõi của tiêu chuẩn MLS trong môi trường phi tập trung. Xác định mục tiêu, định hướng giải pháp và các đóng góp khoa học, kỹ thuật của đồ án.
*   **CHƯƠNG 2: NỀN TẢNG LÝ THUYẾT**
    Trình bày chi tiết cơ sở khoa học của đề tài bao gồm ngữ cảnh mạng ngang hàng, các đe dọa Zero-Trust. Đi sâu vào tiêu chuẩn mật mã MLS (RFC 9420), cơ chế vận hành của cây TreeKEM, các thành phần của thư viện mạng go-libp2p (Noise, GossipSub, Kademlia DHT).
*   **CHƯƠNG 3: PHƯƠNG PHÁP ĐỀ XUẤT**
    Mô tả chi tiết kiến trúc giải pháp đề xuất. Chi tiết hóa thiết kế lớp điều phối Go với 4 cơ chế (Single-Writer, Epoch Checks, Fork Healing, HLC) và cách hiện thực hóa ứng dụng thông qua sự kết hợp Go - Rust sidecar.
*   **CHƯƠNG 4: PHÂN TÍCH LÝ THUYẾT**
    Phân tích và chứng minh toán học/mật mã học về độ an toàn của hệ thống (chứng minh tính FS/PCS được bảo toàn trong Fork Healing, an toàn PKI, Session Takeover). Phân tích độ phức tạp tính toán và băng thông của giao thức phối hợp khi triển khai với sidecar OpenMLS.
*   **CHƯƠNG 5: ĐÁNH GIÁ THỰC NGHIỆM**
    Mô tả môi trường, các thông số đánh giá và kịch bản thí nghiệm chaos. Trình bày và phân tích các biểu đồ thực nghiệm thu thập được về tính hội tụ trạng thái nhóm dưới phân mảnh mạng, độ trễ và khả năng mở rộng của giao thức so với pairwise E2EE và sự cải tiến của cơ chế caching.
*   **CHƯƠNG 6: KẾT LUẬN**
    Tổng kết các mục tiêu đã đạt được của đồ án, chỉ ra một số hạn chế còn tồn đọng của giao thức và đề xuất định hướng nghiên cứu phát triển tiếp theo để nâng cao độ tin cậy của mạng lưới liên lạc P2P.
*   **TÀI LIỆU THAM KHẢO**
    Liệt kê danh mục sách báo, các bài báo khoa học, đặc tả RFC và các nguồn tài liệu kỹ thuật được trích dẫn trong đồ án.
