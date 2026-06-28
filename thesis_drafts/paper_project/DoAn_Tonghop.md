# ĐẠI HỌC BÁCH KHOA HÀ NỘI

## ĐỒ ÁN TỐT NGHIỆP

**Nghiên cứu và ứng dụng giao thức điều phối phi tập trung cho MLS trên mạng ngang hàng**

**LÊ TIẾN DŨNG**

dung.lt224837@sis.hust.edu.vn

**Chương trình đào tạo:** Khoa học máy tính

| Thông tin | Chi tiết |
|---|---|
| **Giảng viên hướng dẫn:** | TS. Đỗ Bá Lâm |
| **Khoa:** | Khoa học máy tính |
| **Trường:** | Công nghệ Thông tin và Truyền thông |

**HÀ NỘI, 06/2026**

---

# LỜI CẢM ƠN

Trước hết, em xin bày tỏ lòng biết ơn chân thành nhất tới TS. Đỗ Bá Lâm. Trong suốt quá trình thực hiện đề tài, thầy đã luôn dành thời gian định hướng khoa học, tận tình hướng dẫn và hỗ trợ em vượt qua những khó khăn từ giai đoạn bắt đầu cho đến khi hoàn thành đồ án tốt nghiệp này.

Đồng thời, em xin trân trọng cảm ơn tập thể giảng viên Đại học Bách khoa Hà Nội nói chung và các thầy cô giáo tại Trường Công nghệ Thông tin và Truyền thông nói riêng. Những kiến thức chuyên môn sâu sắc cùng phương pháp luận nghiên cứu khoa học mà các thầy cô truyền đạt trong suốt quá trình học tập là hành trang vô cùng quý giá cho em.

Cuối cùng, em xin gửi lời cảm ơn đến gia đình và bạn bè. Đây là nguồn động viên tinh thần to lớn, luôn chia sẻ, đồng hành và tạo mọi điều kiện thuận lợi nhất để em yên tâm tập trung học tập và hoàn thành chặng đường nghiên cứu này.

---

# TÓM TẮT NỘI DUNG ĐỒ ÁN

Trong bối cảnh rủi ro an ninh mạng ngày càng phức tạp, việc bảo vệ các hệ thống liên lạc nội bộ đang trở thành ưu tiên hàng đầu, thúc đẩy sự phổ biến của tiêu chuẩn mã hóa đầu cuối (E2EE) như một giải pháp bảo mật tất yếu. Hiện nay, hầu hết các ứng dụng nhắn tin E2EE hàng đầu vận hành dựa trên giao thức Double Ratchet, một tiêu chuẩn bộc lộ nhiều hạn chế khi mở rộng quy mô nhóm. Nhằm giải quyết bài toán này, Messaging Layer Security (MLS) ra đời như một giao thức tiên phong, tái định nghĩa toàn diện kiến trúc mã hóa nhóm. Tuy nhiên, tiêu chuẩn MLS hiện tại được thiết kế độc quyền cho kiến trúc tập trung và bắt buộc phụ thuộc vào một máy chủ điều phối để đảm bảo thứ tự. Việc triển khai MLS trên môi trường mạng ngang hàng (P2P) phi tập trung vẫn là một thách thức chưa được giải quyết.

Từ thực tiễn đó, em quyết định theo đuổi hướng nghiên cứu tích hợp trực tiếp giao thức MLS lên kiến trúc mạng ngang hàng, tước bỏ hoàn toàn máy chủ trung tâm. Hướng đi này được lựa chọn nhằm chứng minh khả năng dung hợp giữa tiêu chuẩn mã hóa nhóm tiên tiến nhất hiện nay và triết lý mạng phi tập trung, từ đó vạch ra mô hình lý thuyết cho một hệ thống truyền thông không tồn tại điểm lỗi duy nhất.

Tổng quan giải pháp của nghiên cứu là đề xuất một lớp điều phối phi tập trung. Kiến trúc này thay thế hoàn toàn vai trò của máy chủ trong việc quản lý cập nhật trạng thái nhóm, đồng thời cung cấp khả năng tự động duy trì trạng thái mã hóa nhóm, nhận diện và Fork Healing khi mạng xảy ra đứt gãy. Bên cạnh phần thiết kế giao thức, đồ án còn xây dựng một ứng dụng desktop nhằm hiện thực các luồng khởi tạo định danh, cấp bundle, tham gia tổ chức, trao đổi thông điệp mã hóa đầu cuối và quản trị thành viên. Sự hiện diện của ứng dụng này cho phép đối chiếu trực tiếp giữa mô hình giao thức và hành vi vận hành ở mức ứng dụng, qua đó củng cố tính khả thi của hướng tiếp cận được đề xuất.

---

# ABSTRACT

In the context of increasingly complex cybersecurity risks, protecting internal communication systems has become a top priority, driving the adoption of end-to-end encryption (E2EE) as an essential security solution. Currently, most leading E2EE messaging applications operate based on the Double Ratchet protocol, a standard that exhibits many limitations when scaling to group communications. To address this issue, Messaging Layer Security (MLS) emerged as a pioneering protocol, comprehensively redefining the group encryption architecture. However, the current MLS specification has a major academic gap: it is designed exclusively for centralized architectures and is forced to rely on a delivery service server to ensure ordering. Implementing MLS in decentralized peer-to-peer (P2P) network environments remains an unsolved challenge. From this practical reality, I decided to pursue a research direction that directly integrates the MLS protocol onto P2P network architectures, completely removing the central server. This direction was chosen to demonstrate the fusion between the most advanced group encryption standard today and the philosophy of decentralized networks, thereby outlining a theoretical model for a communication system without a single point of failure. The overview of the proposed solution is to introduce a decentralized delivery service layer. This architecture completely replaces the role of the server in managing group state updates, while providing the ability to automatically maintain consensus, detect, and heal branches when network partitioning occurs. The core contribution of this thesis is the successful establishment of a complete solution for the MLS application model in P2P networks, culminating in a stably operating prototype application that directly validates the research's correctness and opens a solid reference foundation for future security systems.

---

# DANH MỤC THUẬT NGỮ VÀ TỪ VIẾT TẮT

| Thuật ngữ | Ý nghĩa |
|---|---|
| E2EE | Mã hóa đầu cuối (End-to-End Encryption) |
| MLS | Bảo mật tầng thông điệp (Messaging Layer Security) |
| P2P | Mạng ngang hàng (Peer-to-Peer) |
| DS | Dịch vụ phân phối (Delivery Service) |
| DMLS | Bảo mật tầng thông điệp phi tập trung (Decentralized Messaging Layer Security) |
| HLC | Đồng hồ logic hỗn hợp (Hybrid Logical Clock) |
| FS | Bảo mật xuôi (Forward Secrecy) |
| PCS | Bảo mật sau thỏa hiệp (Post-Compromise Security) |
| DHT | Bảng băm phân tán (Distributed Hash Table) |
| mDNS | Hệ thống phân giải tên miền multicast (multicast Domain Name System) |
| NAT | Biến đổi địa chỉ mạng (Network Address Translation) |
| BFT | Chịu lỗi Byzantine (Byzantine Fault Tolerance) |
| FREEK | Thỏa thuận khóa nhóm liên tục chịu rẽ nhánh (Fork-Resilient Continuous Group Key Agreement) |
| LAN | Mạng cục bộ (Local Area Network) |
| WAN | Mạng diện rộng (Wide Area Network) |
| IP | Giao thức Internet (Internet Protocol) |
| TCP | Giao thức điều khiển truyền vận (Transmission Control Protocol) |
| QUIC | Kết nối Internet UDP nhanh (Quick UDP Internet Connections) |
| RFC | Tài liệu đặc tả chuẩn / Yêu cầu bình luận (Request for Comments) |
| SMR | Máy trạng thái nhân bản (State Machine Replication) |
| CAP | Định lý CAP (Consistency, Availability, Partition Tolerance) |
| AP | Khả dụng và chịu đựng phân vùng mạng (Availability and Partition Tolerance) |
| AEAD | Mã hóa có xác thực kèm dữ liệu (Authenticated Encryption with Associated Data) |
| API | Giao diện lập trình ứng dụng (Application Programming Interface) |

---

# CHƯƠNG 1. GIỚI THIỆU ĐỀ TÀI

## 1.1. Đặt vấn đề

Trong bối cảnh hoạt động trao đổi thông tin ngày càng phụ thuộc vào hạ tầng số, yêu cầu bảo vệ nội dung liên lạc nội bộ đang trở nên cấp thiết đối với cơ quan, doanh nghiệp và các tổ chức có dữ liệu nhạy cảm. Rò rỉ thông tin không chỉ gây thiệt hại về kinh tế, mà còn ảnh hưởng trực tiếp tới uy tín và an toàn vận hành. Vì vậy, mã hóa đầu cuối đã trở thành một hướng tiếp cận quan trọng, trong đó chỉ các thiết bị đầu cuối hợp lệ mới có khả năng giải mã nội dung thông điệp, còn các thành phần trung gian không được phép truy cập bản rõ.

Tuy nhiên, trong nhiều hệ thống nhắn tin bảo mật hiện nay, máy chủ trung tâm vẫn giữ vai trò rất lớn trong quá trình vận hành. Máy chủ thường đảm nhiệm việc định tuyến thông điệp, hỗ trợ thiết bị ngoại tuyến, lưu trữ vật liệu khóa ban đầu và đồng bộ trạng thái giữa các thành viên. Cách tổ chức này giúp hệ thống dễ quản lý hơn, nhưng đồng thời cũng tạo ra sự phụ thuộc đáng kể vào hạ tầng tập trung. Với những môi trường như mạng nội bộ biệt lập, mạng tạm thời không có Internet, hoặc các tổ chức muốn kiểm soát chặt chẽ dữ liệu, sự phụ thuộc đó trở thành một hạn chế thực tế. Những bối cảnh này đòi hỏi một mô hình trong đó dữ liệu được ưu tiên lưu tại thiết bị, các nút có thể trao đổi trực tiếp với nhau, và hệ thống vẫn duy trì được hoạt động ngay cả khi không có máy chủ trung tâm luôn sẵn sàng. Định hướng đó gần với tinh thần ưu tiên dữ liệu cục bộ và mạng ngang hàng.

Đối với bài toán liên lạc hai bên, nhiều giao thức đã chứng minh được hiệu quả trong thực tế, tiêu biểu là Double Ratchet. Tuy nhiên, khi mở rộng sang liên lạc nhóm, bài toán không còn dừng ở việc mã hóa từng thông điệp, mà chuyển thành bài toán duy trì một trạng thái mật mã chung giữa nhiều thiết bị có thể gửi, nhận, mất kết nối và quay trở lại ở những thời điểm khác nhau. Nói cách khác, khó khăn cốt lõi của truyền thông nhóm bảo mật không chỉ nằm ở việc giữ bí mật nội dung, mà còn nằm ở việc bảo đảm tất cả thành viên hợp lệ cùng nhìn thấy một trạng thái khóa nhóm nhất quán.

Messaging Layer Security (MLS) đã được chuẩn hóa trong RFC 9420 để giải quyết bài toán thiết lập khóa nhóm bất đồng bộ với các bảo đảm quan trọng như Forward Secrecy và Post-Compromise Security. Nhờ cấu trúc TreeKEM, MLS cho phép cập nhật trạng thái khóa nhóm hiệu quả hơn nhiều so với các cơ chế mã hóa nhóm dựa hoàn toàn trên các kênh liên lạc hai bên. Dù vậy, MLS không phải là một hệ thống nhắn tin hoàn chỉnh. Theo kiến trúc của RFC 9750, giao thức này vẫn cần được đặt trong một môi trường triển khai cụ thể, nơi ứng dụng phải tự giải quyết các vấn đề như xác thực danh tính, phân phối thông điệp và đặc biệt là xử lý thứ tự của các bản tin làm thay đổi trạng thái nhóm.

Chính ở điểm này dẫn đến xuất hiện mâu thuẫn trung tâm của đề tài. Trong mô hình có một Delivery Service nhất quán mạnh, thành phần phân phối này có thể đóng vai trò áp đặt thứ tự tuyến tính cho các Commit và loại bỏ xung đột khi nhiều thành viên cùng cập nhật nhóm tại một epoch. Ngược lại, trong môi trường P2P không có một thực thể trung tâm làm công việc tuần tự hóa, các Commit có thể đến các thiết bị khác nhau theo những thứ tự khác nhau. Hệ quả là các nút có thể áp dụng những chuỗi thay đổi không giống nhau và làm nhóm rơi vào trạng thái phân nhánh mật mã, tức là các thành viên tuy vẫn mang cùng một định danh nhóm nhưng không còn chia sẻ cùng trạng thái khóa để có thể tiếp tục liên lạc.

Từ thực tế đó, đồ án lựa chọn nghiên cứu bài toán vận hành MLS trên mạng ngang hàng theo hướng bổ sung một lớp điều phối phi tập trung ở tầng ứng dụng. Mục tiêu của hướng tiếp cận này không phải là sửa đổi lõi mật mã của MLS, mà là xây dựng các cơ chế hỗ trợ để nhóm vẫn có thể duy trì tính nhất quán trạng thái, phát hiện phân nhánh khi cần thiết và hội tụ trở lại sau khi mạng được khôi phục.

## 1.2. Các giải pháp hiện tại và hạn chế

Các hướng tiếp cận hiện nay đối với truyền thông nhóm bảo mật có thể chia thành hai nhóm lớn. Nhóm thứ nhất mở rộng từ các cơ chế bảo mật hai bên sang môi trường nhóm; nhóm thứ hai sử dụng giao thức thiết lập khóa nhóm chuyên biệt, trong đó MLS là đại diện tiêu biểu. Mỗi hướng đều có giá trị thực tiễn nhất định, nhưng khi đặt trong yêu cầu vận hành P2P hoàn toàn phi tập trung, các giới hạn của chúng bộc lộ rõ.

Với nhóm thứ nhất, những cơ chế như Sender Keys hay Megolm có ưu điểm là triển khai tương đối thực dụng và có chi phí gửi tin thấp khi nhóm đông thành viên. Tuy nhiên, chúng chủ yếu tối ưu cho việc phát tán thông điệp trong nhóm, chứ không trực tiếp giải quyết bài toán duy trì một trạng thái khóa nhóm nhất quán khi thành viên thay đổi thường xuyên hoặc khi mạng bị chia cắt. Trong các trường hợp khóa gửi tin bị lộ hoặc membership biến động liên tục, chi phí rekey và khả năng phục hồi bảo mật của các cơ chế này vẫn còn những hạn chế đáng kể.

Với nhóm thứ hai, MLS cung cấp một mô hình chặt chẽ hơn cho quản lý khóa nhóm và đạt được các bảo đảm bảo mật mạnh hơn cho truyền thông quy mô lớn. Dù vậy, MLS chuẩn vẫn giả định hệ thống triển khai có khả năng bảo đảm việc phân phối và tuần tự hóa các bản tin thay đổi trạng thái. Khi đưa MLS sang môi trường P2P, bài toán khó nhất không nằm ở bản thân thao tác mật mã, mà nằm ở việc thay thế vai trò điều phối vốn thường do Delivery Service đảm nhiệm.

Một số nghiên cứu gần đây, chẳng hạn hướng Decentralized MLS, đã thử giải quyết vấn đề bằng cách mở rộng trực tiếp giao thức MLS để hỗ trợ xử lý Commit ngoài thứ tự trong môi trường phi tập trung. Đây là một hướng nghiên cứu đáng chú ý, nhưng cũng đòi hỏi client duy trì thêm trạng thái mật mã cũ hoặc trạng thái tạm thời, từ đó làm tăng độ phức tạp triển khai và đặt ra thêm đánh đổi đối với Forward Secrecy. Hơn nữa, hướng này hiện vẫn dừng ở mức Internet-Draft, chưa trở thành một chuẩn hoàn chỉnh.

Từ các phân tích trên có thể thấy, rào cản lớn nhất khi đưa MLS lên mạng ngang hàng không xuất phát từ bản thân các thuật toán mật mã. Vấn đề bảo mật dữ liệu nhạy cảm đã được giải quyết trọn vẹn bởi cấu trúc của chuẩn MLS. Khó khăn thực sự nằm ở góc độ hệ phân tán (distributed systems): làm thế nào để duy trì sự nhất quán của trạng thái nhóm khi hệ thống không còn máy chủ trung tâm (Delivery Service) làm nhiệm vụ tuần tự hóa thông điệp. Việc thiếu vắng một cơ chế điều phối đủ thực dụng để giải quyết bài toán đồng thuận mạng này chính là khoảng trống mà đồ án hướng tới giải quyết.

## 1.3. Mục tiêu và định hướng giải pháp

Mục tiêu của đồ án là nghiên cứu, thiết kế và hiện thực một cơ chế điều phối phi tập trung nhằm hỗ trợ MLS vận hành trên mạng ngang hàng. Về mặt nghiên cứu, đồ án tập trung vào ba vấn đề chính: giảm khả năng xuất hiện concurrent commits khi các nút còn cùng ActiveView, bảo vệ tính nhất quán epoch của trạng thái cục bộ trong môi trường bất đồng bộ, và xây dựng cơ chế phục hồi khi phân nhánh trạng thái vẫn xảy ra do phân vùng mạng. Về mặt ứng dụng, đồ án hướng tới một hệ thống liên lạc nhóm bảo mật ưu tiên dữ liệu cục bộ, có khả năng vận hành trong các điều kiện mạng không ổn định và giảm phụ thuộc vào hạ tầng tập trung. Ứng dụng desktop được xây dựng trong đồ án không chỉ đóng vai trò minh họa giao diện, mà còn là môi trường tích hợp để kiểm chứng trọn vẹn chuỗi thao tác từ khởi tạo định danh, cấp bundle, tham gia tổ chức, tạo nhóm cho tới trao đổi thông điệp.

Trên cơ sở mục tiêu đó, đồ án lựa chọn cách tiếp cận giữ nguyên lõi mật mã của MLS/OpenMLS, đồng thời bổ sung một lớp điều phối ở phía ngoài để xử lý các vấn đề mà môi trường P2P đặt ra. Thay vì mở rộng trực tiếp giao thức MLS hoặc đưa vào một cơ chế đồng thuận nặng cho mọi Commit, giải pháp đề xuất tập trung vào bốn nội dung: giới hạn quyền tạo Commit theo từng epoch, kiểm tra tính phù hợp của thông điệp trước khi đưa xuống lớp mật mã, phát hiện và Fork Healing trạng thái khi mạng bị chia cắt, và sử dụng đồng hồ logic lai để ổn định thứ tự hiển thị thông điệp ứng dụng. Cách tiếp cận này cho phép hệ thống tận dụng các bảo đảm bảo mật của MLS, đồng thời thích nghi tốt hơn với đặc tính bất đồng bộ và phân tán của mạng ngang hàng.

Trong lựa chọn thiết kế này, đồ án chấp nhận một đánh đổi có chủ đích. Hệ thống không theo đuổi nhất quán mạnh tuyệt đối tại mọi thời điểm, bởi điều đó thường đòi hỏi chi phí đồng thuận cao và làm giảm tính sẵn sàng của các phân vùng mạng không đủ điều kiện liên lạc. Thay vào đó, đồ án ưu tiên khả năng hoạt động cục bộ trong điều kiện mạng chia cắt, nhưng vẫn đặt ra cơ chế để các nút có thể hội tụ trở lại một trạng thái MLS hợp lệ sau khi kết nối được khôi phục. Đây là hướng đi phù hợp hơn với mục tiêu P2P và định hướng ưu tiên dữ liệu cục bộ của hệ thống.

## 1.4. Đóng góp của đồ án

Trên cơ sở định hướng nêu trên, đồ án có bốn đóng góp chính:

1. Đồ án xác định bài toán trung tâm của MLS trên mạng ngang hàng dưới góc nhìn nhất quán trạng thái mật mã, qua đó chỉ ra rằng khó khăn cốt lõi không nằm ở việc truyền thông điệp, mà nằm ở việc duy trì một chuỗi trạng thái nhóm hợp lệ khi không còn Delivery Service trung tâm.
2. Đồ án đề xuất một lớp điều phối phi tập trung bao quanh MLS, trong đó các cơ chế Single-Writer (người ghi duy nhất), kiểm tra epoch, phát hiện phân nhánh và phục hồi sau phân mảnh được phối hợp để giảm nguy cơ fork và hỗ trợ hệ thống hội tụ trở lại.
3. Đồ án xây dựng một ứng dụng desktop kết hợp mạng P2P, lưu trữ cục bộ và lõi mật mã MLS/OpenMLS, trong đó các luồng khởi tạo, cấp bundle, quản trị tổ chức và trò chuyện nhóm được dùng để kiểm tra khả năng triển khai của hướng tiếp cận đã đề xuất.
4. Đồ án cung cấp các kết quả đánh giá ban đầu về tính hội tụ, khả năng phục hồi sau phân vùng mạng và tác động của lớp điều phối đối với quá trình vận hành MLS trong môi trường mạng ngang hàng.

## 1.5. Bố cục đồ án

Phần còn lại của báo cáo được tổ chức như sau. Chương 2 trình bày cơ sở lý thuyết của đề tài, bao gồm nền tảng MLS, TreeKEM, vai trò của Delivery Service, các hướng tiếp cận liên quan và những đặc tính của môi trường mạng ngang hàng có ảnh hưởng trực tiếp tới bài toán nhất quán trạng thái. Trên nền tảng đó, Chương 3 trình bày chi tiết giao thức điều phối phi tập trung được đề xuất, làm rõ các thành phần, nguyên tắc hoạt động, cách phối hợp giữa lớp điều phối với lõi mật mã MLS, đồng thời giới thiệu ứng dụng desktop minh họa như môi trường hiện thực của các quyết định thiết kế.

Tiếp theo, Chương 4 phân tích cơ sở lý thuyết của giải pháp theo hướng các bất biến quan trọng, điều kiện hội tụ và giới hạn của phương pháp trong môi trường phân tán bất đồng bộ. Chương 5 trình bày quá trình đánh giá thực nghiệm trên ứng dụng thử nghiệm của đồ án, kết hợp giữa dữ liệu đo đạc tự động và các kịch bản thao tác trực tiếp trên ứng dụng để làm rõ khả năng hội tụ, phục hồi sau phân vùng mạng và chi phí vận hành của hệ thống. Cuối cùng, Chương 6 tổng kết các kết quả đạt được, chỉ ra những hạn chế còn tồn tại và đề xuất các hướng phát triển tiếp theo của đồ án; phần phụ lục tập hợp bộ ảnh giao diện và các kịch bản minh họa chi tiết của ứng dụng này.

---

# CHƯƠNG 2. NỀN TẢNG LÝ THUYẾT

Chương 1 đã trình bày bối cảnh, mục tiêu và định hướng chung của đề tài. Tiếp nối phần mở đầu đó, Chương 2 tập trung làm rõ nền tảng lý thuyết cần thiết cho bài toán vận hành MLS trên mạng ngang hàng. Trọng tâm của chương không phải là liệt kê thật nhiều kiến thức rời rạc, mà là chỉ ra mối liên hệ giữa ba lớp vấn đề: lõi mật mã của MLS, đặc tính bất đồng bộ của mạng P2P và yêu cầu duy trì một trạng thái nhóm nhất quán.

## 2.1. Ngữ cảnh của bài toán: nhất quán trạng thái mật mã trong hệ phân tán

Trong hệ phân tán nói chung, bài toán nhất quán trạng thái nhằm bảo đảm nhiều nút cùng hiểu và cùng cập nhật dữ liệu theo một quy tắc thống nhất. Đối với hệ thống nhắn tin nhóm được mã hóa đầu cuối, trạng thái cần được giữ nhất quán không chỉ là dữ liệu ứng dụng, mà còn là trạng thái mật mã dùng để sinh khóa, xác thực thành viên và ràng buộc lịch sử hội thoại. Vì vậy, nếu các nút không còn cùng nhìn thấy một trạng thái mật mã chung, khả năng giao tiếp của nhóm sẽ bị ảnh hưởng trực tiếp.

Trong MLS, trạng thái nhóm tiến hóa theo các kỷ nguyên, gọi là epoch. Có thể hiểu mỗi epoch là một phiên bản cụ thể của nhóm, trong đó tập thành viên hợp lệ cùng chia sẻ một bộ bí mật tương ứng. Khi nhóm thêm thành viên, loại bỏ thành viên hoặc cập nhật khóa, trạng thái này phải chuyển sang epoch mới. Nếu tất cả thành viên cùng áp dụng các thay đổi theo cùng một thứ tự, nhóm tiếp tục hoạt động bình thường. Ngược lại, nếu các thay đổi được áp dụng theo những thứ tự khác nhau, các thành viên có thể dần tách khỏi nhau về mặt mật mã.

Hiện tượng đó được gọi là rẽ nhánh trạng thái mật mã (Cryptographic State Fork). Đây là tình huống trong đó các nút vẫn nghĩ rằng mình thuộc cùng một nhóm, thậm chí có thể cùng nhìn thấy cùng số epoch, nhưng thực tế lại đang nắm giữ các bí mật nhóm, hàm băm cây (tree hash) hoặc lịch sử bắt tay (transcript hash) khác nhau. Khi đó, thông điệp được tạo trên một nhánh không còn được xử lý hợp lệ trên nhánh còn lại. Khác với xung đột dữ liệu thông thường, trạng thái mật mã đã rẽ nhánh không thể được ghép lại bằng cách trộn hai phiên bản dữ liệu, vì mỗi nhánh đã sinh ra một chuỗi khóa và một lịch sử xác thực riêng.

Trong các triển khai thông thường của MLS, việc hạn chế rẽ nhánh được hỗ trợ bởi Delivery Service (DS), có thể hiểu là lớp dịch vụ tiếp nhận và phân phối các thông điệp MLS giữa các thiết bị. Trong môi trường có DS nhất quán mạnh, khi nhiều thành viên cùng tạo Commit ở cùng một epoch, hệ thống có một điểm tự nhiên để quyết định Commit nào được xử lý trước. Nhờ đó, các thay đổi trạng thái dễ được quan sát theo một thứ tự thống nhất hơn.

Khó khăn xuất hiện khi đưa MLS sang mạng ngang hàng. Trong môi trường P2P, thông điệp có thể đến chậm, đến sai thứ tự, bị lặp lại hoặc tạm thời không đến được do thiết bị ngoại tuyến hay do mạng bị chia cắt. Khi đó, không còn một thực thể trung tâm nào tự nhiên đứng ra sắp xếp thứ tự cho các Commit. Nói cách khác, MLS đòi hỏi chuỗi epoch tiến hóa gần như tuyến tính, trong khi mạng ngang hàng bất đồng bộ lại không tự cung cấp thứ tự tuyến tính toàn cục cho các thông điệp thay đổi trạng thái. Đây chính là mâu thuẫn cốt lõi của bài toán mà đồ án cần giải quyết.

Dưới góc nhìn của hệ phân tán, quá trình tiến hóa trạng thái của một nhóm MLS mang bản chất của một Máy trạng thái nhân bản (SMR). Trong mô hình này, mỗi thiết bị của người dùng duy trì một bản sao của trạng thái mật mã (như Ratchet Tree, KeySchedule). Các thao tác Commit chính là các phép chuyển trạng thái. Đặc điểm làm cho bài toán đồng thuận trong MLS trở nên khắt khe là tính chất hoàn toàn không giao hoán của thao tác Commit. Nếu hai thành viên cùng tạo ra Commit tại cùng một epoch, việc áp dụng Commit thứ nhất rồi đến Commit thứ hai sẽ sinh ra một bộ khóa hoàn toàn khác biệt so với việc áp dụng theo thứ tự ngược lại. Nó không thể được tự động dung hòa bằng các thuật toán gộp dữ liệu bình thường, bởi mỗi bước chuyển trạng thái mật mã đều dùng hàm băm và dẫn xuất ra khóa một chiều dựa trên chính trạng thái liền trước đó. Do tính chất không giao hoán này, hệ thống bắt buộc phải duy trì một thứ tự toàn cục để tránh hiện tượng rẽ nhánh.

Trong phạm vi đồ án, hệ thống được xem như một mạng gồm nhiều thiết bị ngang hàng, mỗi thiết bị lưu cục bộ trạng thái MLS của các nhóm mà nó tham gia. Các tình huống được quan tâm gồm độ trễ mạng không đồng đều, thiết bị ngoại tuyến, sự biến động nút mạng và phân vùng mạng. Mục tiêu của phần nghiên cứu không phải thay đổi các bảo đảm mật mã nền tảng của MLS, mà là tìm cách tổ chức việc điều phối ở tầng ứng dụng để các thiết bị vẫn có thể hội tụ trở lại một trạng thái nhóm hợp lệ sau những bất ổn của mạng.

## 2.2. Các kết quả nghiên cứu tương tự

Các nghiên cứu liên quan có thể được nhìn từ ba hướng chính: các cơ chế nhắn tin nhóm xuất hiện trước MLS, MLS trong mô hình có Delivery Service và các nỗ lực đưa MLS sang môi trường phi tập trung.

### 2.2.1. Các cơ chế mã hóa nhóm trước MLS

Trước khi MLS được chuẩn hóa, nhiều hệ thống nhắn tin nhóm đã mở rộng từ cơ chế bảo mật hai bên sang môi trường nhiều người tham gia. Một nền tảng quan trọng là Double Ratchet, vốn rất hiệu quả cho liên lạc hai bên. Khi đưa sang nhóm, một hướng phổ biến là để mỗi người gửi tự tạo một khóa gửi tin riêng, rồi phân phối khóa đó cho các thành viên còn lại qua các kênh bảo mật hai bên. Cách làm này thường được gọi là Sender Keys. Sau khi khóa gửi tin đã được chia sẻ, người gửi chỉ cần mã hóa một lần cho mỗi thông điệp, còn hạ tầng bên dưới chịu trách nhiệm phát tán bản mã tới cả nhóm. Ưu điểm của hướng này là chi phí gửi tin thấp và triển khai tương đối thực dụng. Tuy nhiên, khi nhóm đông hoặc thay đổi thành viên thường xuyên, chi phí cập nhật khóa và phục hồi sau thỏa hiệp vẫn tăng đáng kể.

Megolm trong hệ sinh thái Matrix cũng theo tinh thần tối ưu việc gửi tin nhóm. Có thể hiểu đơn giản rằng mỗi người gửi duy trì một phiên gửi ra, tức một trạng thái khóa được dùng liên tiếp để mã hóa nhiều tin nhắn trong cùng một phòng trò chuyện. Bên trong phiên đó, trạng thái khóa được cập nhật dần theo một cơ chế xoay khóa nhóm. Thiết kế này giúp việc gửi tin trong nhóm nhẹ hơn, nhưng điểm đổi lại là khả năng phục hồi sau thỏa hiệp không mạnh bằng những mô hình thay khóa nhóm chặt chẽ hơn. Nếu trạng thái của một phiên bị lộ, phạm vi thông điệp bị ảnh hưởng có thể kéo dài cho đến khi hệ thống tạo phiên mới và phân phối lại khóa.

Nhìn chung, Sender Keys và Megolm đều có giá trị thực tiễn cao đối với bài toán phát tán tin nhắn trong nhóm. Tuy nhiên, chúng không trực tiếp giải quyết bài toán mà đồ án quan tâm nhất, đó là duy trì một trạng thái nhóm nhất quán khi nhiều thiết bị có thể cùng thay đổi trạng thái trong môi trường P2P bất đồng bộ. Đây là lý do MLS trở thành nền tảng phù hợp hơn cho đề tài.

### 2.2.2. MLS với Delivery Service

MLS được chuẩn hóa để cung cấp cơ chế thiết lập khóa nhóm bất đồng bộ với các bảo đảm bảo mật mạnh hơn cho truyền thông nhóm quy mô lớn. Điểm quan trọng của MLS là nó không chỉ tối ưu việc gửi thông điệp, mà còn mô hình hóa rõ quá trình thay đổi thành viên và cập nhật khóa của cả nhóm thông qua các khái niệm như proposal, commit và epoch.

Trong kiến trúc chuẩn của MLS, Delivery Service thường đảm nhiệm việc lưu trữ một số vật liệu khóa công khai ban đầu, phân phối thông điệp MLS và hỗ trợ quá trình truyền thông giữa các client. Nhờ có một đầu mối như vậy, hệ thống triển khai theo mô hình Client-Server có điều kiện thuận lợi hơn để quan sát các Commit theo một thứ tự đủ nhất quán. Vì thế, MLS hoạt động tự nhiên hơn trong môi trường có DS, nhưng khi bỏ đi điểm điều phối này thì bài toán ordering của Commit lập tức trở nên khó hơn nhiều.

### 2.2.3. Các hướng phi tập trung hóa MLS

Vì quá trình tiến hóa trạng thái của MLS mang bản chất SMR, một hướng tiếp cận trực tiếp là bổ sung một lớp đồng thuận phân tán truyền thống để thống nhất thứ tự Commit. Các cơ chế đồng thuận này thường rơi vào hai nhóm chính: đồng thuận dựa trên số đông (quorum-based) hoặc đồng thuận chuỗi khối (blockchain-based). Tuy nhiên, cả hai hướng đi này đều gặp phải những rào cản lớn khi áp dụng vào môi trường nhắn tin P2P.

**(i) Đối với đồng thuận dựa trên Quorum** (điển hình như BFT, Paxos hay Raft), các thuật toán này đòi hỏi một tỷ lệ nhất định các nút mạng (thường là lớn hơn một nửa hoặc hai phần ba) phải trực tuyến và duy trì kết nối liên tục để bầu chọn nhóm trưởng và xác nhận thứ tự thông điệp. Dưới góc độ của định lý CAP, các cơ chế này ưu tiên tính Nhất quán (Consistency) và hy sinh tính Khả dụng khi mạng bị chia cắt (Partition). Trong ứng dụng chat P2P, thiết bị của người dùng thường xuyên ngoại tuyến hoặc mạng chập chờn. Việc yêu cầu một quorum trực tuyến sẽ khiến hệ thống thường xuyên bị "đóng băng", người dùng không thể gửi Commit hay thay đổi thành viên nhóm. Thêm vào đó, đối với các thuật toán chịu lỗi Byzantine (BFT), độ phức tạp truyền thông có thể lên tới O(N²) cho các vòng xác nhận chéo, tạo ra gánh nặng quá lớn đối với băng thông của thiết bị di động.

**(ii) Đối với đồng thuận chuỗi khối** (blockchain-based), việc sử dụng blockchain để tạo ra một nhật ký tuyến tính chung giải quyết được bài toán thiếu máy chủ trung tâm. Tuy nhiên, kiến trúc này sinh ra độ trễ (latency) tạo khối rất cao, hoàn toàn không đáp ứng được yêu cầu thời gian thực của một ứng dụng trò chuyện. Ngoài ra, việc thiết lập và duy trì một mạng lưới blockchain cũng như cơ chế đồng thuận riêng cho từng nhóm chat độc lập là một sự lãng phí tài nguyên không cần thiết.

Một hướng khác là Decentralized MLS (DMLS) của Kohbrok, trong đó chính bản thân MLS được mở rộng để xử lý Commit đến ngoài thứ tự trong môi trường phi tập trung. Về ý tưởng, client giữ lại thêm một phần vật liệu khóa hoặc trạng thái tạm thời để có thể xử lý những nhánh Commit khác nhau khi mạng phi tập trung không cung cấp ordering rõ ràng. Cách làm này cho thấy một hướng nghiên cứu đáng chú ý, nhưng đồng thời cũng làm tăng độ phức tạp triển khai và đặt ra thêm đánh đổi đối với Forward Secrecy. Hơn nữa, hướng này hiện vẫn ở mức Internet-Draft, chưa trở thành chuẩn hoàn chỉnh.

Từ các hướng trên có thể thấy khoảng trống còn lại nằm ở chỗ sau: cần một cơ chế đủ nhẹ để phù hợp với P2P, nhưng vẫn đủ chặt để giảm nguy cơ rẽ nhánh và hỗ trợ hệ thống hội tụ trở lại sau khi mạng bị chia cắt. Đó cũng là động lực trực tiếp cho hướng tiếp cận mà đồ án theo đuổi ở các chương sau.

## 2.3. Nền tảng MLS và TreeKEM

### 2.3.1. Các khái niệm cốt lõi: client, group, member và epoch

Trong MLS, client là thực thể nắm giữ khóa mật mã và tham gia vào tiến trình thiết lập trạng thái nhóm. Group là một nhóm logic gồm các client cùng chia sẻ một trạng thái mật mã tại một thời điểm. Member là client đang thực sự nằm trong trạng thái chia sẻ đó và có quyền truy cập các bí mật của nhóm. Epoch, như đã trình bày ở trên, là một phiên bản cụ thể của trạng thái nhóm.

Điểm quan trọng cần nhấn mạnh là trạng thái nhóm trong MLS không phải một tập dữ liệu rời rạc có thể ghép lại tùy ý. Nó là một chuỗi epoch nối tiếp nhau, trong đó mỗi epoch mới được sinh ra từ epoch trước đó. Vì vậy, nếu các thành viên không cùng nhìn nhận một chuỗi epoch tương thích, nhóm sẽ mất khả năng duy trì một trạng thái mật mã chung.

### 2.3.2. Ratchet Tree và TreeKEM

TreeKEM là cơ chế trung tâm giúp MLS cập nhật khóa nhóm hiệu quả. Thay vì để mỗi thành viên phải trao đổi khóa riêng với tất cả các thành viên còn lại, MLS tổ chức các thành viên thành các lá của một cây nhị phân, gọi là Ratchet Tree. Có thể hiểu trực quan rằng mỗi lần nhóm thay đổi, hệ thống chỉ cần cập nhật một số bí mật dọc theo một đường trong cây, thay vì phát lại toàn bộ khóa cho cả nhóm.

Khi một thành viên cập nhật khóa, thành viên đó tạo ra một đường dẫn cập nhật từ lá của mình lên gốc cây. Các bí mật mới trên đường đi này được mã hóa cho các nhánh đồng cấp tương ứng, thường được gọi là copath. Nhờ cấu trúc cây, số lượng phép tính và lượng dữ liệu cập nhật chỉ tăng gần theo logarit của số thành viên trong nhóm. Nói ngắn gọn, TreeKEM là cơ chế giúp MLS thay khóa nhóm theo cách có cấu trúc và tiết kiệm hơn so với việc phân phối lại khóa theo kiểu tuyến tính cho toàn bộ thành viên.

Cần lưu ý rằng bí mật ở gốc cây không được dùng trực tiếp để mã hóa thông điệp ứng dụng. Sau mỗi Commit, MLS còn đưa các bí mật mới qua một cơ chế sinh khóa để tạo ra các khóa cụ thể cho từng mục đích như xác thực, mã hóa thông điệp hay xuất vật liệu khóa. Vì vậy, TreeKEM nên được hiểu là cơ chế cập nhật trạng thái khóa nhóm, còn việc sinh các khóa sử dụng trực tiếp cho thông điệp được xử lý ở bước sau.

### 2.3.3. Proposal, Commit và tiến trình chuyển epoch

MLS phân biệt rõ proposal và commit. Proposal là thông điệp đề xuất một thay đổi cho nhóm, chẳng hạn thêm thành viên, loại bỏ thành viên hoặc cập nhật khóa. Bản thân proposal chưa làm thay đổi epoch. Commit mới là thông điệp thực sự áp dụng một tập proposal và đưa nhóm sang epoch mới.

Nhìn dưới góc độ hệ phân tán, có thể xem Commit như thao tác ghi vào trạng thái mật mã của nhóm. Mỗi Commit đều làm xuất hiện một phiên bản trạng thái mới. Vì thế, nếu tại cùng một epoch mà có nhiều Commit cạnh tranh, nguy cơ rẽ nhánh trạng thái sẽ xuất hiện. Đây là lý do bài toán ordering của Commit luôn giữ vai trò trung tâm khi vận hành MLS trên mạng ngang hàng.

### 2.3.4. Các dấu hiệu nhận biết trạng thái: tree hash, transcript hash và epoch authenticator

Để kiểm tra các thành viên có còn cùng một trạng thái hay không, MLS sử dụng một số giá trị ràng buộc trạng thái. Trong đó, hàm băm cây (tree hash) phản ánh cấu trúc Ratchet Tree hiện tại. Hàm băm lịch sử bắt tay (transcript hash) tóm tắt lịch sử các bản tin điều khiển của MLS đã được nhóm chấp nhận, chẳng hạn như Proposal và Commit, nhờ đó cho biết các thành viên có thực sự đi qua cùng một chuỗi thay đổi trạng thái hay không. Còn bộ xác thực epoch (epoch authenticator) có thể được dùng để đối chiếu việc các thành viên có thực sự đang chia sẻ cùng trạng thái epoch hay không.

Ý nghĩa của các giá trị này đối với đồ án là rất trực tiếp. Trong môi trường P2P, hai thiết bị có thể cùng báo cùng mã nhóm và cùng số epoch, nhưng điều đó chưa đủ để kết luận rằng chúng còn ở cùng trạng thái MLS. Nếu tree hash hoặc transcript hash khác nhau, về bản chất chúng đã tách thành hai nhánh. Vì vậy, khi nghiên cứu bài toán phát hiện phân nhánh và đồng bộ lại trạng thái, các dấu hiệu nhận biết này đóng vai trò đặc biệt quan trọng.

### 2.3.5. Application messages, delayed messages và External Commit

MLS phân biệt thông điệp bắt tay với thông điệp ứng dụng. Proposal và Commit thuộc nhóm thứ nhất vì chúng làm thay đổi hoặc chuẩn bị thay đổi trạng thái nhóm. Ngược lại, application messages là các tin nhắn ứng dụng được mã hóa bằng khóa sinh ra từ trạng thái của epoch hiện tại.

Trong mạng thực tế, thông điệp ứng dụng có thể đến muộn hoặc đến sai thứ tự. MLS có cơ chế hỗ trợ một mức độ nhất định cho các thông điệp bị trễ, chẳng hạn thông qua bộ khóa của người gửi và bộ đếm thế hệ. Tuy nhiên, để bảo vệ Forward Secrecy, thiết bị không nên giữ các khóa cũ vô thời hạn. Điều này tạo ra một đánh đổi quen thuộc: nếu xóa khóa cũ quá sớm thì thông điệp đến muộn có thể không giải mã được; nếu giữ khóa cũ quá lâu thì phạm vi thiệt hại khi thiết bị bị thỏa hiệp sẽ tăng.

MLS cũng hỗ trợ cơ chế gia nhập từ bên ngoài, thường được gọi là External Commit hoặc External Join. Có thể hiểu đây là cách để một client chưa ở trong trạng thái hiện tại của nhóm gia nhập lại nhóm khi có đủ thông tin công khai cần thiết. Cơ chế này quan trọng đối với đồ án vì khi trạng thái đã rẽ nhánh, một thiết bị có thể cần từ bỏ nhánh cũ và tham gia lại nhánh đang được chọn.

Bên cạnh External Commit, RFC 9420 còn định nghĩa một cơ chế khác cho một đối tượng ngoài nhóm tác động lên trạng thái nhóm, gọi là External Proposal. Hai cơ chế này có đặc tính khác nhau về ai giữ quyền tạo Commit. External Commit là thao tác mà chính node ngoài tự tạo Commit để gia nhập, nên node đó tự chịu trách nhiệm sinh ra epoch mới. External Proposal thì khác: node ngoài chỉ gửi một đề nghị thay đổi, thường là Add, vào nhóm, còn việc đưa đề nghị đó vào Commit thuộc về một thành viên đang ở trong nhóm. Cả hai đều là cơ chế hợp lệ của RFC 9420, nhưng cách phân chia trách nhiệm khác nhau dẫn đến hệ quả khác nhau khi áp dụng vào môi trường P2P.

Một vấn đề khác cần lưu ý là định danh trùng khi gia nhập lại. Khi một node cũ gia nhập lại nhóm, định danh cũ của node đó vẫn còn nằm trong cây MLS dưới dạng một lá không còn hoạt động. Nếu node đó dùng External Commit với cùng định danh, lõi MLS sẽ từ chối vì phát hiện trùng định danh. Cách giải quyết là kết hợp Remove, loại bỏ lá cũ, và Add, thêm lại với KeyPackage mới, trong cùng một Commit. Khả năng gom nhiều Proposal vào một Commit là một cơ chế được cung cấp sẵn của MLS, cho phép nhiều thay đổi thành viên được xử lý trong cùng một epoch.

## 2.4. Nền tảng mạng ngang hàng và thứ tự trong hệ phân tán

### 2.4.1. Đặc tính của mạng ngang hàng

Mạng ngang hàng khác với mô hình Client-Server ở chỗ không có một máy chủ trung tâm duy nhất điều phối toàn bộ luồng truyền thông. Các thiết bị có thể tự phát hiện nhau, thiết lập kết nối trực tiếp hoặc gián tiếp và trao đổi dữ liệu qua nhiều đường truyền khác nhau. Cách tổ chức này giúp hệ thống giảm phụ thuộc vào hạ tầng trung tâm, nhưng đồng thời làm cho việc điều phối trở nên khó hơn.

Trong môi trường P2P, các hiện tượng như thiết bị ngoại tuyến, độ trễ không đồng đều, trùng lặp thông điệp, biến động nút mạng hoặc phân vùng mạng là điều bình thường. Với ứng dụng thông thường, chúng chủ yếu gây chậm hoặc mất dữ liệu tạm thời. Với MLS, các hiện tượng đó còn có thể kéo theo hệ quả nghiêm trọng hơn: một số thiết bị đã chuyển sang epoch mới trong khi thiết bị khác vẫn ở epoch cũ, hoặc hai phân vùng mạng cùng tạo ra những Commit khác nhau. Điều này cho thấy lớp mạng P2P chỉ giải quyết việc truyền thông, chứ không tự giải quyết bài toán nhất quán trạng thái mật mã.

### 2.4.2. go-libp2p và GossipSub

Trong ứng dụng thử nghiệm của đồ án, tầng mạng được xây dựng trên go-libp2p. Đây là thư viện P2P mô-đun hóa, cho phép mỗi thiết bị có một định danh mạng riêng, thiết lập kết nối qua nhiều cơ chế truyền dẫn và trao đổi dữ liệu trên nhiều luồng logic. Những khả năng này phù hợp với yêu cầu của một ứng dụng nhắn tin không phụ thuộc hoàn toàn vào máy chủ trung tâm.

Một thành phần quan trọng của libp2p là GossipSub, tức cơ chế phát tán thông điệp theo mô hình publish/subscribe. Có thể hiểu đơn giản rằng khi một thiết bị gửi thông điệp vào một chủ đề chung, thông điệp đó sẽ được lan truyền dần qua các thiết bị lân cận trong mạng. Cơ chế này phù hợp để phát tán proposal, commit hoặc các thông báo trạng thái trong môi trường P2P.

Tuy nhiên, GossipSub chỉ là cơ chế phát tán thông điệp, không phải cơ chế đồng thuận. Nó không bảo đảm mọi thiết bị nhận thông điệp theo cùng một thứ tự, không tự chọn Commit thắng cuộc khi có xung đột, và cũng không bảo đảm một thông điệp chỉ được nhận đúng một lần. Vì vậy, chỉ đưa MLS lên trên GossipSub là chưa đủ để bảo vệ chuỗi trạng thái của nhóm.

### 2.4.3. Đồng hồ logic và thứ tự hiển thị thông điệp ứng dụng

Trong cùng một epoch, nhiều thành viên có thể gửi thông điệp ứng dụng song song. Những thông điệp này không nhất thiết cần một thứ tự mật mã toàn cục như Commit, nhưng giao diện chat vẫn cần một thứ tự hiển thị đủ ổn định giữa các thiết bị. Nếu chỉ dựa vào đồng hồ vật lý, hệ thống dễ gặp sai lệch do lệch giờ hoặc do môi trường mạng không ổn định.

Vì vậy, hệ phân tán thường sử dụng các dạng đồng hồ logic để biểu diễn quan hệ thứ tự giữa các sự kiện. Lamport Clock cho một thứ tự logic đơn giản. Hybrid Logical Clock (HLC) kết hợp thời gian vật lý và bộ đếm logic để tạo ra dấu thời gian gần với thời gian thực hơn, nhưng vẫn ổn định hơn khi đồng hồ giữa các thiết bị không hoàn toàn khớp nhau. Trong đồ án, loại dấu thời gian này chỉ có ý nghĩa đối với việc sắp xếp hiển thị các thông điệp ứng dụng; nó không thay thế epoch, chữ ký hay các cơ chế bảo vệ trạng thái của MLS.

### 2.4.4. Khoảng trống cần giải quyết

Từ các phân tích trên có thể thấy bài toán của đề tài nằm ở giao điểm giữa mật mã học nhóm và hệ phân tán. MLS cung cấp lõi mật mã mạnh nhưng giả định hệ thống triển khai phải giải quyết việc phân phối và ordering của thông điệp. Libp2p và GossipSub cung cấp hạ tầng truyền thông ngang hàng nhưng không cung cấp thứ tự tuyến tính cho Commit. HLC hỗ trợ sắp xếp thông điệp ứng dụng nhưng không bảo vệ trạng thái mật mã của nhóm.

Khoảng trống nghiên cứu vì vậy không nằm ở chỗ thiếu một giao thức mã hóa nhóm, mà nằm ở chỗ thiếu một cơ chế điều phối đủ nhẹ để phù hợp với P2P, nhưng vẫn đủ chặt để giảm nguy cơ rẽ nhánh và hỗ trợ hội tụ trở lại sau khi mạng bất ổn. Khoảng trống đó là cơ sở trực tiếp cho giải pháp sẽ được trình bày ở Chương 3.

---

# CHƯƠNG 3. PHƯƠNG PHÁP ĐỀ XUẤT

Chương 2 đã trình bày nền tảng của MLS, TreeKEM, mạng ngang hàng và nguyên nhân dẫn tới phân nhánh trạng thái mật mã khi nhiều Commit cùng xuất hiện trong một Epoch. Trên cơ sở đó, Chương 3 tập trung vào lớp điều phối phi tập trung mà đồ án đề xuất để MLS có thể vận hành trong môi trường P2P mà không cần một Delivery Service trung tâm sắp thứ tự Commit.

## 3.1. Tổng quan bài toán và hướng tiếp cận

### 3.1.1. Bài toán cần giải quyết

Trong mô hình MLS thông thường, Delivery Service không chỉ chuyển tiếp thông điệp mà còn giúp các thành viên nhìn thấy cùng một thứ tự Commit. Khi chuyển sang môi trường P2P và bỏ điểm tuần tự hóa trung tâm này, hệ thống phải đối mặt với một rủi ro trực tiếp: hai hoặc nhiều thành viên có thể cùng phát hành Commit cho cùng một Epoch. Khi đó, nhóm không còn tiến hóa theo một chuỗi trạng thái duy nhất, mà có thể tách thành nhiều nhánh trạng thái mật mã khác nhau.

Giả sử nhóm đang ở Epoch E. Nếu thành viên A tạo Commit C_A và thành viên B đồng thời tạo Commit C_B, một phần nút có thể áp dụng C_A trước, trong khi phần còn lại áp dụng C_B trước. Kết quả là nhóm có thể sinh ra hai trạng thái S_A(E+1) và S_B(E+1) khác nhau dù cùng mang nhãn Epoch E+1. Khi đó, các nút ở hai nhánh không còn cùng tree hash, cùng lịch sử Commit hay cùng khóa ứng dụng, nên cũng không còn xử lý thông điệp của nhau một cách hợp lệ.

Vì vậy, phương pháp đề xuất trong đồ án phải đồng thời đáp ứng bốn yêu cầu:
1. Giảm mạnh khả năng xuất hiện concurrent commits khi mạng còn liên thông.
2. Buộc mỗi nút chỉ xử lý thông điệp trên trạng thái MLS tương thích với epoch cục bộ của nó.
3. Phát hiện được sự phân kỳ trạng thái khi phân vùng mạng xảy ra và hỗ trợ hội tụ trở lại một nhánh hợp lệ.
4. Giữ được một thứ tự hiển thị ổn định cho các thông điệp ứng dụng trong cùng một Epoch mà không nhầm lẫn vai trò đó với thứ tự Commit.

### 3.1.2. Kiến trúc của giải pháp

Về kiến trúc, phương pháp chia thành hai tầng. Tầng điều phối quyết định khi nào một nút được quyền tạo Commit, cách phân loại thông điệp theo epoch, cách đối chiếu trạng thái nhánh và cách khôi phục sau phân vùng mạng. Tầng MLS lo việc xử lý Proposal, Commit, Welcome, External Proposal, External Commit, mã hóa và giải mã thông điệp theo đúng quy tắc của MLS.

Lớp điều phối không lập trình lại MLS, còn lõi MLS không tự quyết định thứ tự điều phối trong môi trường phân tán. Nhờ cách tách này, đồ án giữ nguyên các bảo đảm mật mã của MLS và chỉ bổ sung lớp điều phối cho bối cảnh P2P.

### 3.1.3. Các cơ chế chính của phương pháp

Phương pháp đề xuất gồm bốn cơ chế liên kết với nhau:

1. **Single-Writer theo epoch** để giới hạn quyền phát hành Commit.
2. **Kiểm tra nhất quán epoch** để ngăn nút xử lý thông điệp trên trạng thái MLS không tương thích.
3. **Phát hiện và Fork Healing** để đưa các nút hội tụ lại sau phân vùng mạng.
4. **Đồng hồ logic lai HLC** để sắp xếp thông điệp ứng dụng sau khi đã qua lớp kiểm tra của MLS.

Bốn cơ chế này không thay thế MLS mà chỉ bao quanh nó, bù vào vai trò mà Delivery Service thường đảm nhiệm trong mô hình tập trung. Đóng góp của đồ án là cách tổ chức lớp điều phối chứ không phải sửa đổi thuật toán mật mã cốt lõi.

### 3.1.4. Phạm vi bảo mật và Mô hình mối đe dọa (Threat Model)

Vì đồ án sử dụng mạng P2P thay cho máy chủ trung tâm, rủi ro bảo mật thực chất lại mở rộng hơn do mọi nút trong mạng đều có thể tham gia định tuyến tin nhắn và tiếp cận siêu dữ liệu (metadata). Để làm rõ ranh giới giữa an toàn mật mã và đồng thuận phân tán, đồ án xác định mô hình mối đe dọa gồm ba đối tượng chính:

- **Kẻ nghe lén (Passive Eavesdropper):** Bất kỳ nút P2P trung gian nào cố tình theo dõi luồng dữ liệu truyền qua. Rủi ro này được ủy thác hoàn toàn cho lớp mật mã: hệ thống chống lại bằng cơ chế mã hóa có xác thực (AEAD) của MLS, đảm bảo trung gian tuyệt đối không thể truy cập bản rõ.
- **Thành viên bị chiếm quyền (Compromised Member):** Một thành viên hợp lệ trong nhóm bị kẻ tấn công đánh cắp thiết bị hoặc lộ vật liệu khóa. Hệ thống kế thừa và chống lại rủi ro này bằng các thuộc tính Forward Secrecy (FS) và Post-Compromise Security (PCS) mặc định của cơ chế TreeKEM trong MLS.
- **Kẻ phá hoại mạng (Active Attacker / Malicious Node):** Nút mạng cố tình giữ lại tin nhắn, phát tán thông điệp sai thứ tự, hoặc cố ý tạo phân nhánh nhằm chia rẽ sự đồng thuận của nhóm. Đây chính là trọng tâm mà lớp điều phối phi tập trung của đồ án trực tiếp giải quyết thông qua cơ chế bầu Token Holder (Single-Writer) và quá trình hàn gắn phân nhánh (Fork Healing).

Đồ án giả định các cấu trúc mật mã của MLS là an toàn. Bài toán bảo mật được giải quyết bằng việc tích hợp chuẩn này; phần việc của đồ án là giữ cho hệ thống không bị sụp đổ hay phân nhánh khi mạng ngang hàng hoạt động bất đồng bộ.

## 3.2. Giao thức Single-Writer theo epoch

### 3.2.1. Mục tiêu thiết kế

Tiến hóa trạng thái nhóm về bản chất là một bài toán Máy trạng thái nhân bản (SMR), nên thiết kế giao thức bị chi phối bởi các đánh đổi của định lý CAP. Nếu cố đạt tính Nhất quán mạnh như các thuật toán đồng thuận truyền thống, hệ thống nhắn tin P2P sẽ mất tính Khả dụng mỗi khi mạng bị chia cắt.

Đồ án chọn hướng ưu tiên tính Khả dụng và chịu đựng phân vùng mạng (AP), kết hợp với mô hình Nhất quán cuối cùng. Nguyên nhân trực tiếp của phân nhánh là nhiều Commit hợp lệ cùng xuất hiện trong một epoch, nên đồ án kiểm soát bằng cách chỉ cho một thành viên duy nhất phát hành Commit. Nếu mạng bị chia cắt và thông tin về Token bị phân mảnh, sinh ra nhiều nhánh Commit độc lập, hệ thống cho phép các nhóm con tiếp tục hoạt động, sau đó dùng cơ chế hàn gắn để đưa nhóm về một trạng thái chung khi mạng ổn định lại.

Ý tưởng trung tâm của cơ chế này là tách vai trò Proposal và Commit. Thành viên hợp lệ nào cũng có thể phát biểu mong muốn thay đổi nhóm dưới dạng Proposal, nhưng chỉ một thành viên duy nhất trong tập ứng viên hợp lệ mới được quyền gom các Proposal đó để tạo Commit. Thành viên này được gọi là Token Holder của Epoch đang xét.

### 3.2.2. Tập ứng viên hợp lệ

Token Holder không được chọn từ toàn bộ các nút mà hệ thống nhìn thấy trên mạng, mà chỉ được chọn từ tập ứng viên hợp lệ của nhóm. Tập này được xác định bởi ba loại điều kiện: tư cách thành viên trong trạng thái MLS, tình trạng còn hoạt động trên mạng và chính sách quyền của ứng dụng. Ở mức khái niệm, tập ứng viên tại Epoch E được mô tả như sau:

> **Eligible(E) = M(E) ∩ A(E) ∩ C(E) \ S(E)**

Trong đó:
- **M(E)** là tập thành viên mà trạng thái MLS còn xem là hợp lệ ở Epoch E.
- **A(E)** là tập thành viên đang được quan sát là còn hoạt động.
- **C(E)** là tập thành viên mà chính sách ứng dụng cho phép tham gia tạo Commit.
- **S(E)** là tập thành viên tạm thời bị loại khỏi vai trò điều phối vì lỗi, ngoại tuyến quá lâu hoặc bị chính sách ứng dụng đình chỉ.

Cách mô hình hóa này giúp phân biệt rõ hai chuyện thường bị trộn lẫn. Một thành viên có thể vẫn còn trong nhóm ở tầng MLS nhưng đang ngoại tuyến, nên không thích hợp giữ vai trò Token Holder. Ngược lại, một nút đang trực tuyến nhưng không thuộc trạng thái nhóm cũng không có quyền tham gia quyết định Commit. Do đó, chỉ phần giao của các điều kiện trên mới có ý nghĩa đối với bài toán điều phối.

### 3.2.3. Hàm bầu chọn Token Holder

Sau khi xác định được tập ứng viên hợp lệ, mỗi nút tự tính Token Holder bằng cùng một quy tắc tất định cục bộ. Thành viên có giá trị băm nhỏ nhất khi ghép định danh nhóm, số epoch và định danh nút sẽ được chọn:

> **TH(E) = argmin_{n ∈ Eligible(E)} H(group_id ∥ E ∥ n)**

Ở đây, H là hàm băm mật mã như SHA-256. Việc đưa group_id vào đầu vào giúp một nút không mặc nhiên giữ ưu thế ở mọi nhóm; việc đưa E vào đầu vào giúp quyền giữ Token thay đổi theo từng epoch; còn định danh nút bảo đảm mọi nút đang có cùng ActiveView sẽ đi tới cùng kết quả.

**Thuật toán bầu chọn Token Holder (ComputeTokenHolder):**
- Đầu vào: groupID, epoch, members, activeView, authorized, suspended
- Đầu ra: holderID
1. eligible ← members ∩ activeView ∩ authorized \ suspended
2. Nếu eligible là tập rỗng → Trả về lỗi NoEligibleCommitter
3. bestID ← null, bestHash ← MAX_HASH
4. Với mỗi peerID trong eligible:
   - data ← Encode(groupID, epoch, peerID)
   - candidateHash ← SHA256(data)
   - Nếu candidateHash < bestHash: bestHash ← candidateHash, bestID ← peerID
5. Trả về bestID

Ưu điểm chính của cách chọn này là không cần thêm một vòng bỏ phiếu mạng cho mỗi Commit. Nếu các nút có cùng trạng thái nhóm và cùng ActiveView, chúng sẽ tự tính ra cùng một Token Holder. Chi phí của bước bầu chọn vì thế chủ yếu là chi phí cục bộ, tăng tuyến tính theo số lượng ứng viên hợp lệ.

### 3.2.4. Tạo Proposal và Commit

Trong cơ chế đề xuất, Proposal là yêu cầu thay đổi nhóm, còn Commit là thao tác thực sự làm nhóm chuyển sang Epoch mới. Tất cả thành viên hợp lệ đều có thể tạo Proposal. Tuy nhiên, chỉ Token Holder của Epoch hiện tại mới có quyền gom các Proposal hợp lệ để phát hành Commit.

Quy tắc này cho phép hệ thống chấp nhận nhiều ý định thay đổi cùng lúc nhưng vẫn duy trì một nút duy nhất cho mỗi epoch được quyền Commit. Khi mạng còn liên thông, các nút sẽ có cùng Token Holder và vì thế giảm mạnh khả năng sinh concurrent commits.

**Quy trình tạo Commit của Token Holder (CommitByTokenHolder):**
- Đầu vào: localState, proposalQueue, holderID, selfID
- Đầu ra: Bản tin Commit được phát tán nếu nút cục bộ là Token Holder
1. Nếu selfID ≠ holderID → Kết thúc
2. Chọn tập Proposal hợp lệ P từ proposalQueue
3. Nếu P rỗng và không có nhu cầu cập nhật cục bộ → Kết thúc
4. Tạo Commit từ localState và P bằng lớp MLS
5. Nếu Commit hợp lệ → Phát tán Commit cho toàn nhóm
6. Ngược lại → Loại bỏ các Proposal không còn hợp lệ và ghi nhận lỗi

MLS chịu trách nhiệm kiểm tra tính hợp lệ mật mã của Commit, còn lớp điều phối chịu trách nhiệm quyết định ai được quyền phát hành Commit ở Epoch đó.

### 3.2.5. Chuyển quyền và failover

Sau mỗi Commit hợp lệ, nhóm chuyển sang Epoch E+1 và mọi nút tự tính lại Token Holder từ trạng thái mới. Vì quy tắc chọn phụ thuộc vào epoch, quyền phát hành Commit có thể luân chuyển giữa các thành viên thay vì gắn cố định vào một nút.

Trong mạng P2P, Token Holder có thể ngoại tuyến hoặc không kịp phát hành Commit. Để tránh bế tắc, các nút duy trì một danh sách các thành viên được ghi nhận là đang hoạt động trong nhóm, gọi là ActiveView. Nếu Token Holder hiện tại không còn được quan sát là đang hoạt động hoặc vượt quá thời gian chờ hợp lý, các nút sẽ loại nó khỏi tập ứng viên của Epoch đó và tính lại Token Holder.

Failover này không phải là cơ chế đồng thuận Byzantine đầy đủ. Khi mạng bị chia cắt, các phân vùng có thể có ActiveView khác nhau và tính ra các Token Holder khác nhau. Vai trò của Single-Writer là giảm nguy cơ concurrent commits khi các nút còn chung ActiveView, chứ không loại bỏ hoàn toàn khả năng fork trong mọi điều kiện mạng.

## 3.3. Kiểm tra nhất quán Epoch

### 3.3.1. Mục tiêu thiết kế

MLS yêu cầu mỗi Commit và thông điệp liên quan phải được xử lý trên đúng trạng thái nhóm tương ứng. Nếu nút áp dụng bản tin lên trạng thái không tương thích, kết quả có thể là lỗi giải mã, lỗi xác thực, hoặc làm hỏng tiến trình tiến hóa trạng thái cục bộ. Vì vậy, đồ án đặt một lớp kiểm tra ngoài MLS để phân loại thông điệp trước khi chuyển xuống tầng mật mã.

### 3.3.2. Cấu trúc bản tin điều phối

Để phân loại thông điệp, đồ án dùng một cấu trúc bản tin điều phối (CoordinationEnvelope) gồm các trường: GroupID, SenderID, Type (Proposal/Commit/application message), Epoch, Timestamp (HLC), Payload. Trường timestamp chỉ dùng sắp xếp thông điệp ứng dụng, không dùng để quyết định Commit nào hợp lệ.

### 3.3.3. Phân loại thông điệp theo Epoch

Khi nhận một bản tin, nút cục bộ không xử lý ngay mà trước hết so sánh env.epoch với epoch cục bộ của nhóm tương ứng. Có ba tình huống chính:

- **env.epoch = localEpoch:** Thông điệp thuộc về trạng thái hiện tại và được chuyển tiếp cho lớp MLS xử lý theo luồng bình thường.
- **env.epoch < localEpoch:** Thông điệp thuộc về quá khứ, không được phép kéo trạng thái nhóm lùi lại. Từ chối áp dụng.
- **env.epoch > localEpoch:** Nút cục bộ được xem là đang đi sau. Đưa bản tin vào bộ đệm thông điệp tương lai, kích hoạt quá trình đồng bộ trạng thái.

Cách phân loại này giữ cho epoch cục bộ của mỗi nút là một đại lượng đơn điệu không giảm. Nút không tự ý tăng epoch chỉ vì nhìn thấy bản tin từ tương lai, mà chỉ tăng epoch sau khi đã xử lý thành công các bước chuyển trạng thái cần thiết.

### 3.3.4. Đồng bộ trạng thái khi gặp thông điệp tương lai

Một thông điệp đến từ tương lai không tự nó là bằng chứng của phân nhánh. Trong nhiều trường hợp, điều đó chỉ cho thấy nút cục bộ đã bỏ lỡ một hoặc nhiều Commit trung gian. Phản ứng đúng của lớp điều phối là trước hết kiểm tra xem lịch sử Commit của nút cục bộ có còn là tiền tố hợp lệ của lịch sử trên nhánh xa hay không.

Nếu kiểm tra tiền tố cho thấy lịch sử cục bộ vẫn nằm trên cùng một chuỗi với nhánh còn lại kia, nút chỉ cần áp dụng tuần tự các Commit còn thiếu để bắt kịp. Nếu lịch sử đã phân kỳ, nút mới chuyển sang quy trình Fork Healing.

### 3.3.5. Xử lý thao tác bị bỏ lỡ

Trong mạng bất đồng bộ, một Proposal do người dùng tạo ra có thể không được đưa vào Commit của Epoch mà nó hướng tới. Đồ án xử lý tình huống này theo nguyên tắc đề xuất lại có điều kiện. Nếu sau khi nhóm chuyển sang Epoch mới mà thao tác của người dùng vẫn còn hợp lệ về mặt ngữ nghĩa, nút cục bộ có thể tạo lại Proposal trên trạng thái mới rồi phát tán lại. Nếu thao tác không còn hợp lệ nữa, ứng dụng phải báo rõ cho người dùng.

## 3.4. Cơ chế Fork Healing trạng thái

### 3.4.1. Bối cảnh phân vùng mạng

Single-Writer hoạt động tốt nhất khi các nút còn có cùng ActiveView. Tuy nhiên, trong mạng P2P, phân vùng mạng có thể chia nhóm thành nhiều phân vùng không liên lạc được với nhau. Khi đó, mỗi phân vùng chỉ quan sát được một phần thành viên đang hoạt động và có thể tính ra các Token Holder khác nhau. Nếu các phân vùng tiếp tục tiến hóa độc lập, hệ thống có thể xuất hiện nhiều nhánh trạng thái MLS khác nhau.

Đồ án xem phân vùng mạng là trường hợp phải xử lý ngay từ thiết kế chứ không phải ngoại lệ hiếm. Vấn đề không phải là ngăn hoàn toàn phân nhánh trong môi trường bất đồng bộ -- điều đó không khả thi -- mà là nhận ra sự phân kỳ và đưa hệ thống hội tụ lại khi các phân vùng nối thông.

### 3.4.2. Phát hiện nghi vấn phân nhánh và đối chiếu trạng thái

Để nhận biết tình trạng phân kỳ, mỗi nút định kỳ công bố một bản tóm tắt trạng thái nhánh đang giữ (GroupStateAnnouncement), gồm: GroupID, Epoch, TreeHash, HistoryHash (R(E) = H(R(E-1) ∥ CommitHash(E))), ActiveMemberCount.

Để tính history hash, mỗi nút duy trì một giá trị theo công thức:

> R(0) = r₀, R(e) = H(R(e-1) ∥ C(e))

trong đó r₀ là hằng số khởi tạo, C(e) là giá trị băm của Commit tại epoch e, còn H là hàm băm mật mã. Giá trị R(E) tại epoch hiện tại phản ánh toàn bộ lịch sử Commit từ epoch 1 đến epoch E. Nếu hai nút có cùng R(e) tại cùng một mốc e, thì có thể xem như chúng đã đi qua cùng một chuỗi các thao tác lịch sử để cùng đến mốc đó.

Dựa vào cơ chế này, việc phân biệt giữa chậm đồng bộ và phân nhánh chỉ cần đưa về một phép kiểm tra tiền tố. Xét hai nút A và B với E_A ≤ E_B:
- Nếu R_B(E_A) = R_A(E_A): A chỉ đang đi sau trên cùng một nhánh, không có phân nhánh.
- Nếu R_B(E_A) ≠ R_A(E_A): Hai nút đã đi qua những thao tác lịch sử khác nhau và phân nhánh thực sự đã xảy ra.

### 3.4.3. Hàm trọng số nhánh

Khi hệ thống đã xác định rằng đang tồn tại nhiều nhánh khác nhau của cùng một nhóm, nó cần một quy tắc tất định để chọn nhánh tiếp tục. Đồ án dùng một hàm trọng số nhánh:

> **W(B) = (C_active(B), E(B), H_commit(B))**

Các nhánh được so sánh theo thứ tự từ điển:
1. Nhánh có nhiều thành viên hoạt động hơn được ưu tiên trước.
2. Nếu bằng nhau, nhánh có epoch cao hơn được ưu tiên.
3. Nếu vẫn bằng nhau, giá trị băm Commit đóng vai trò phá hòa tất định.

Quy tắc này không nói rằng nhánh được chọn là nhánh "đúng" theo nghĩa tuyệt đối. Nó chỉ bảo đảm rằng nếu các nút cùng quan sát một tập nhánh và áp dụng cùng quy tắc tất định, chúng sẽ đi đến cùng một quyết định.

### 3.4.4. Quy trình Fork Healing

Sau khi chọn được nhánh thắng, hệ thống không cố hợp nhất trực tiếp hai trạng thái MLS đã phân kỳ. Trộn khóa, trộn transcript hay ghép thủ công hai trạng thái như vậy không thể duy trì trạng thái hợp lệ của MLS. Thay vào đó, nút ở nhánh thua lấy thông tin của nhánh thắng rồi gia nhập lại bằng External Proposal.

**Quy trình Fork Healing (ForkHealing):**
1. winningBranch ← SelectBranch(Branches)
2. Với mỗi nút đang ở nhánh thua:
   - Hủy trạng thái MLS cũ
   - Tạo KeyPackage mới
   - Gửi External Proposal (Add) qua GossipSub
3. **Token Holder:**
   - Nhận External Proposal
   - Nếu định danh cũ còn trong cây → Tạo Remove Proposal cho lá cũ
   - Tạo Add Proposal và buffer vào hàng đợi
   - Khi hết hạn gom: tạo Commit chứa tất cả proposals
   - Sinh Welcome và chuyển tiếp cho nút nhánh thua
4. **Nút nhánh thua:**
   - ProcessWelcome → khôi phục trạng thái
   - Tự phát lại tin nhắn do chính mình tạo (Autonomous Replay)

Việc dùng External Proposal kết hợp với Token Holder gom tất cả đề nghị vào một Commit duy nhất giúp tránh tình huống N Commits cạnh tranh. Dù có K nút cùng cần heal, hệ thống chỉ tạo đúng một Commit.

Sau khi quá trình Fork Healing hoàn tất, các nút nhánh thua không sở hữu khóa bí mật của các epoch đã qua trong thời gian phân mảnh mạng. Để lấp đầy khoảng trống dữ liệu này mà không phá vỡ mô hình mã hóa đầu cuối, mỗi thiết bị tự bọc lại các thông điệp do chính nó tạo ra trong thời gian đứt mạng bằng khóa nhóm của epoch mới nhất, sau đó phát lại vào mạng. Quy tắc này đảm bảo dữ liệu được phục hồi mà không vi phạm ranh giới thẩm quyền của MLS, bởi vì thao tác mã hóa được thực hiện bởi chính tác giả gốc của thông điệp.

Để tránh bão mạng khi phát lại hàng nghìn thông điệp, hệ thống gom hàng loạt tin nhắn lại và gộp chung vào các gói tin lớn. Cách làm này không làm giảm tổng dung lượng, nhưng sẽ giúp triệt tiêu hoàn toàn lượng metadata định tuyến khổng lồ không cần thiết.

## 3.5. Thứ tự thông điệp ứng dụng bằng Hybrid Logical Clock

### 3.5.1. Lý do cần HLC

Epoch trong MLS chỉ quyết định thứ tự thay đổi trạng thái mật mã. Nó không tạo ra một thứ tự toàn phần cho các thông điệp ứng dụng được gửi gần đồng thời trong cùng một Epoch. Trong môi trường P2P, hai thiết bị có thể nhận các thông điệp đó theo thứ tự khác nhau do độ trễ mạng.

HLC kết hợp thành phần thời gian gần với đồng hồ vật lý với một bộ đếm logic, từ đó tạo ra một khóa sắp thứ tự ổn định hơn:

> **HLC = (L, C, N)**

Trong đó, L là thành phần thời gian logic bám theo thời gian vật lý, C là bộ đếm logic dùng khi nhiều sự kiện có cùng L, còn N là định danh nút dùng để phá hòa một cách tất định.

### 3.5.2. Tạo timestamp khi gửi tin (HLC_Now)

1. pt ← thời gian vật lý hiện tại
2. Nếu pt > hlc.L: hlc.L ← pt, hlc.C ← 0
3. Ngược lại: hlc.C ← hlc.C + 1
4. Trả về (hlc.L, hlc.C, nodeID)

### 3.5.3. Cập nhật HLC khi nhận tin (HLC_Update)

1. pt ← thời gian vật lý hiện tại
2. newL ← max(hlc.L, msg.L, pt)
3. Nếu newL == hlc.L và newL == msg.L: hlc.C ← max(hlc.C, msg.C) + 1
4. Ngược lại nếu newL == hlc.L: hlc.C ← hlc.C + 1
5. Ngược lại nếu newL == msg.L: hlc.C ← msg.C + 1
6. Ngược lại: hlc.C ← 0
7. hlc.L ← newL
8. Trả về hlc

### 3.5.4. Vai trò của HLC trong hệ thống

HLC chỉ được dùng để sắp thứ tự hiển thị các thông điệp ứng dụng đã vượt qua bước kiểm tra epoch và đã được MLS xử lý hợp lệ. Nó không quyết định Commit nào đúng, không thay thế lớp điều phối ở tầng trạng thái và không được phép ảnh hưởng tới key schedule của MLS.

## 3.6. Hiện thực ứng dụng thử nghiệm phục vụ kiểm chứng

### 3.6.1. Ranh giới trách nhiệm trong ứng dụng thử nghiệm

Ở mức hiện thực ứng dụng thử nghiệm, các nguyên tắc trên được ánh xạ thành một kiến trúc hai tầng gồm một tiến trình chủ chịu trách nhiệm điều phối và một thành phần MLS chịu trách nhiệm xử lý mật mã. Trạng thái bền vững của nhóm được lưu ở phía host; khi cần xử lý Proposal, Commit, External Proposal, External Commit hoặc application message, host lấy trạng thái hiện tại, chuyển nó cho tầng MLS xử lý rồi ghi lại trạng thái mới sau khi thao tác hoàn tất.

### 3.6.2. Vai trò của ứng dụng desktop

Ứng dụng desktop không được xem là phần đóng góp thuật toán mới. Vai trò của nó là hiện thực hóa các luồng thao tác quan trọng nhất để kiểm chứng phương pháp: tham gia hệ thống bằng bundle, tạo và quản trị nhóm, và trao đổi thông điệp trên nhiều nút.

## 3.7. Tính đúng đắn trực giác của phương pháp

Khi các nút còn chung ActiveView, Single-Writer giảm khả năng concurrent commits vì mỗi epoch chỉ có một Token Holder được quyền Commit. Kết hợp với kiểm tra epoch, cơ chế này buộc mọi bước tiến hóa trạng thái cục bộ phải đi qua chuỗi Commit hợp lệ.

Khi phân vùng mạng xảy ra, phương pháp không giả định hệ thống giữ được một trạng thái duy nhất mọi lúc. Nó chấp nhận phân kỳ tạm thời, nhưng yêu cầu lớp điều phối nhận ra khác biệt giữa các nhánh, thử bắt kịp khi cần, rồi chọn một nhánh để hội tụ bằng cơ chế hợp lệ của MLS.

HLC giữ thứ tự hiển thị ổn định cho thông điệp ứng dụng sau khi đã xử lý hợp lệ. Phương pháp đề xuất quan tâm cả đến nhất quán trạng thái mật mã lẫn tính quan sát được ở tầng ứng dụng, nhưng vẫn tách rõ hai loại ordering này.

---

# CHƯƠNG 4. PHÂN TÍCH LÝ THUYẾT

Chương 3 đã trình bày lớp điều phối phi tập trung được đề xuất để đưa MLS vào môi trường mạng ngang hàng. Tuy nhiên, việc nêu cơ chế thôi chưa đủ để khẳng định hướng tiếp cận là hợp lý. Vì vậy, chương này tiếp tục phân tích cơ sở lý thuyết của các cơ chế đã nêu, làm rõ chúng bảo toàn được những bất biến nào, trong phạm vi giả định nào và giới hạn của chúng nằm ở đâu.

## 4.1. Mô hình phân tích và giả định

Xét một nhóm MLS có định danh groupID và trạng thái hiện hành ở epoch E. Mỗi nút giữ một trạng thái cục bộ, trong đó phần lõi là trạng thái MLS dùng để xử lý Proposal, Commit và application message; phần còn lại là metadata điều phối phục vụ bầu chọn Token Holder, đối chiếu trạng thái và phát hiện phân nhánh.

Mạng ngang hàng được xem là môi trường bất đồng bộ. Thông điệp có thể đến trễ, đến sai thứ tự, bị lặp hoặc tạm thời không đến được. Khi mạng còn liên thông, các nút cuối cùng có thể trao đổi lại thông tin với nhau, nhưng không giả định rằng mọi nút quan sát thông điệp theo cùng một thứ tự. Khi xảy ra phân vùng mạng, hai tập nút khác nhau có thể hình thành hai ActiveView khác nhau.

Các lập luận dưới đây được xây dựng trong phạm vi giả định của đồ án: các nút nhìn chung tuân thủ giao thức, các thao tác MLS cốt lõi được OpenMLS xử lý đúng, và mục tiêu chính là phân tích hành vi của lớp điều phối khi thiếu Delivery Service tập trung.

## 4.2. Bất biến 1: Epoch cục bộ chỉ được giữ nguyên hoặc tăng

**Phát biểu bất biến:** Tại mỗi nút, epoch cục bộ chỉ được giữ nguyên hoặc tăng lên sau khi nút xử lý thành công một Commit hợp lệ.

> E_i(t + 1) ≥ E_i(t)

**Cơ sở từ MLS:** Trong MLS, mỗi Commit hợp lệ đưa nhóm sang một epoch mới và làm thay đổi trạng thái cây, lịch sử bắt tay cùng lịch trình khóa của nhóm. Một thông điệp được tạo ở epoch cũ không thể được xem là tương thích một cách mặc nhiên với trạng thái hiện tại.

**Cơ chế bảo toàn:** Lớp điều phối kiểm tra epoch trước khi chuyển thông điệp xuống MLS:

| Quan hệ epoch | Hành động của lớp điều phối |
|---|---|
| E_msg = E_local | Thông điệp được chuyển tiếp cho lớp MLS xử lý theo luồng bình thường |
| E_msg < E_local | Thông điệp không được áp dụng lên trạng thái hiện tại |
| E_msg > E_local | Thông điệp chưa được xử lý ngay; nút tạm giữ lại và kích hoạt đồng bộ trạng thái |

**Mệnh đề 1.** Nếu một nút chỉ tăng epoch sau khi xử lý thành công Commit hợp lệ và không áp dụng thông điệp thuộc epoch cũ lên trạng thái hiện tại, thì epoch cục bộ của nút là đơn điệu không giảm.

*Lập luận.* Mọi thông điệp cũ đều bị chặn ở lớp điều phối, nên trạng thái MLS hiện tại không bị đè ngược bởi dữ liệu quá khứ. Đối với thông điệp đi trước trạng thái cục bộ, nút không áp dụng ngay mà phải đồng bộ lại chuỗi trạng thái. Bởi trạng thái mới chỉ xuất hiện sau khi xử lý Commit hợp lệ, chuỗi giá trị epoch tại một nút không thể tự giảm xuống.

## 4.3. Bất biến 2: Với cùng một ActiveView, chỉ có một Token Holder

**Phát biểu bất biến:** Ở epoch E, nếu hai nút có cùng groupID, cùng số epoch và cùng tập ứng viên hợp lệ, chúng phải tính ra cùng một Token Holder.

Tập ứng viên hợp lệ:

> Eligible(E) = GroupMembers(E) ∩ ActiveView(E) ∩ AuthorizedCommitters(E) \ RemovedOrSuspended(E)

Token Holder được tính bằng quy tắc tất định:

> TokenHolder(E) = argmin_{n ∈ Eligible(E)} H(groupID ∥ E ∥ n)

**Mệnh đề 2.** Nếu hai nút có cùng groupID, cùng epoch E và cùng tập Eligible(E), thì chúng chọn cùng một Token Holder.

*Lập luận.* Khi đầu vào của công thức là như nhau, tập giá trị băm sinh ra trên hai nút cũng như nhau. Do phép chọn argmin là tất định, kết quả cuối cùng bắt buộc trùng nhau. Hệ thống không cần thêm một vòng bỏ phiếu mạng chỉ để quyết định ai là người tạo Commit trong cùng một ActiveView.

**Ý nghĩa đối với concurrent commits:** Bất biến trên chỉ đúng khi các nút đang có cùng ActiveView. Nếu hai phân vùng giữ hai ActiveView khác nhau, chúng có thể hình thành hai tập Eligible(E) khác nhau và từ đó chọn ra hai Token Holder khác nhau. Single-Writer không phải là lời hứa rằng hệ thống không bao giờ phân nhánh; vai trò đúng của nó là giảm mạnh khả năng sinh concurrent commits khi mạng còn đủ liên thông.

## 4.4. Bất biến 3: Phân nhánh phải được nhận biết sau bước đối chiếu lịch sử

### Phân biệt chậm đồng bộ và phân nhánh thực sự

Trong mạng ngang hàng, khác biệt quan sát được giữa hai nút chưa đủ để kết luận rằng nhóm đã phân nhánh. Bài toán không đơn thuần là so sánh hai trạng thái hiện tại, mà là đối chiếu lịch sử Commit mà mỗi nút đã chấp nhận.

Ký hiệu C_i(e) là giá trị băm của Commit mà nút i đã chấp nhận tại epoch e. Từ dãy các Commit này, mỗi nút tính một giá trị tóm tắt:

> R_i(0) = r₀  
> R_i(e) = H(R_i(e - 1) ∥ C_i(e))

Mỗi nút công bố định kỳ một bản tóm tắt trạng thái: StateSummary = (groupID, E, treeHash, historyHash).

**Mệnh đề 3.** Giả sử hai nút A và B thuộc cùng một nhóm và E_A ≤ E_B. Sau khi A nhận được StateSummary của B và, nếu cần, lấy thêm giá trị R_B(E_A):

- Nếu R_B(E_A) ≠ R_A(E_A): A và B không còn đứng trên cùng một chuỗi Commit → phân nhánh thực sự.
- Nếu R_B(E_A) = R_A(E_A): Khác biệt quan sát được có thể giải thích bằng việc A đang chậm đồng bộ, lịch sử của A chỉ là tiền tố của lịch sử ở B.

*Lập luận.* Do R_i(e) được xây dựng bằng cách băm lũy tiến trên toàn bộ dãy Commit từ 1 đến e, điều kiện R_B(E_A) = R_A(E_A) có nghĩa là hai nút đã chấp nhận cùng một đoạn lịch sử đến epoch E_A. Nếu R_B(E_A) ≠ R_A(E_A), thì phải tồn tại ít nhất một epoch mà hai bên đã chấp nhận hai Commit khác nhau.

## 4.5. Bất biến 4: Không hợp nhất trực tiếp hai trạng thái MLS đã phân nhánh

### Vì sao không thể gộp trực tiếp hai nhánh

Sau khi phân nhánh, mỗi nhánh MLS có lịch sử Commit, trạng thái cây, lịch sử bắt tay và lịch trình khóa riêng. Nếu lớp điều phối tự ý trộn hai trạng thái đó để tạo ra một trạng thái mới, trạng thái mới sẽ không còn là kết quả của một chuỗi thao tác MLS hợp lệ. Phương pháp của đồ án không tìm cách hợp nhất trực tiếp hai nhánh đã tách, mà chọn một nhánh tiếp tục và đưa các nút ở nhánh còn lại gia nhập lại nhóm theo con đường MLS hợp lệ.

### Quy tắc chọn nhánh

Mỗi nhánh B được gắn với trọng số:

> W(B) = (C_active(B), E(B), H_commit(B))

So sánh theo thứ tự từ điển: ưu tiên nhánh có nhiều thành viên hoạt động hơn, rồi nhánh có epoch cao hơn, cuối cùng dùng giá trị băm Commit để phá hòa.

**Mệnh đề 4.** Nếu các nút quan sát cùng một tập nhánh và cùng áp dụng quy tắc chọn nhánh, thì chúng sẽ có cùng cơ sở để chọn nhánh tiếp tục; việc đưa các nút của nhánh thua gia nhập lại qua External Proposal được Token Holder gom vào Commit giúp hệ thống hội tụ, tuân thủ theo tiêu chuẩn MLS mà không cần phải tự chế ra một cơ chế mật mã mới.

## 4.6. Xử lý thông điệp phát sinh ở nhánh thua

Trong thời gian phân vùng mạng, người dùng ở mỗi nhánh vẫn có thể gửi application message. Khi hệ thống hội tụ trở lại, mỗi nút chỉ được tự mã hóa lại các thông điệp do chính nó tạo ra, rồi gửi chúng như các application message mới dưới trạng thái hợp lệ của nhánh thắng. Cách làm này giữ ba ranh giới quan trọng:

1. Hệ thống không kéo dài vòng đời sử dụng của các khóa thuộc nhánh thua.
2. Một nút không được tự ý phát lại nội dung thay cho nút khác.
3. Thông điệp sau khi quay lại hệ thống sẽ là thông điệp mới của trạng thái hiện hành.

## 4.7. Bất biến 5: HLC chỉ sắp xếp thông điệp ứng dụng, không quyết định Commit

**Phát biểu:** HLC giúp tạo thứ tự hiển thị tất định cho cùng một tập application message đã được giải mã hợp lệ, nhưng không thay thế cơ chế ordering của Commit.

*Lập luận.* Nếu message A là nguyên nhân dẫn đến message B, thì timestamp HLC của B sẽ lớn hơn timestamp của A theo thứ tự từ điển. Với các message đồng thời, hệ thống dùng bộ ba (L, C, NodeID) để phá hòa.

**Giới hạn của HLC:** HLC không được dùng để xác định Commit nào hợp lệ, cũng không được dùng để sửa phân nhánh trạng thái MLS.

## 4.8. Phân tích chi phí lý thuyết

### Chi phí gửi application message

Với MLS, application message được bảo vệ bằng khóa dẫn xuất từ trạng thái nhóm tại epoch hiện tại. Người gửi không cần tạo một bản mã độc lập cho từng thành viên. Chi phí phần mã hóa nội dung không tăng theo cùng cách như mô hình mã hóa từng cặp.

### Chi phí cập nhật thành viên và rekey

Các thao tác thêm thành viên, loại bỏ thành viên hoặc cập nhật khóa đều đi qua Proposal và Commit. Ở mức TreeKEM, chi phí thay đổi thành viên chỉ tăng theo độ phức tạp logarit O(log N), thay vì tăng tuyến tính O(N).

### Chi phí của Single-Writer

Single-Writer không yêu cầu một vòng đồng thuận mạng riêng. Mỗi nút chỉ cần duyệt qua tập Eligible(E) và tính hàm băm cho từng ứng viên. Chi phí tính toán cục bộ là O(N) theo cách cài đặt trực tiếp, nhưng đây là chi phí cục bộ, không phải chi phí nhiều vòng mạng.

### Chi phí của kiểm tra epoch và đối chiếu trạng thái

Kiểm tra epoch chỉ yêu cầu so sánh metadata, chi phí O(1) trên mỗi thông điệp. Đối chiếu trạng thái đòi hỏi trao đổi bản tóm tắt ngắn, kích thước hằng số theo kích thước nhóm. Chi phí phụ thêm chỉ phát sinh khi cần phân biệt giữa chậm đồng bộ và phân nhánh.

### Chi phí của Fork Healing

Fork Healing chỉ phát sinh khi đã có phân nhánh. Khi dùng External Proposal, chi phí tạo Commit cho fork healing không tăng theo số nút cần heal. Token Holder gom tất cả External Proposals vào một Commit duy nhất, nên dù có K nút cùng cần gia nhập lại, hệ thống chỉ tạo đúng một Commit. Điều này khác với việc mỗi nút tự tạo Commit riêng, khi đó K Commits cạnh tranh sẽ dẫn đến chi phí O(K²).

### Phân tích cơ chế phát lại thông điệp ứng dụng

Về mặt bảo mật, cách làm này không vi phạm Forward Secrecy hay Backward Secrecy của giao thức MLS. Về khía cạnh hiệu năng mạng lưới, đồ án sử dụng kỹ thuật gom tin nhắn để hạn chế tối đa bão mạng. Tổng dung lượng dữ liệu cần truyền đi là không đổi, nhưng việc gom nhiều tin nhắn vào chung các gói tin giúp giảm đáng kể số lần CPU phải mã hóa MLS và giảm lượng lớn metadata định vị.

## 4.9. Tổng hợp các bất biến lý thuyết và cơ chế bảo toàn

| Bất biến / Thuộc tính | Cơ chế bảo toàn | Ý nghĩa |
|---|---|---|
| Epoch cục bộ chỉ giữ nguyên hoặc tăng | Kiểm tra epoch | Ngăn nút áp dụng thông điệp lên trạng thái MLS cũ hoặc không tương thích |
| Một Token Holder trong cùng ActiveView | Single-Writer theo epoch | Giảm concurrent commits khi các nút có cùng ActiveView |
| Dấu hiệu phân nhánh có thể quan sát được | Bản tóm tắt trạng thái, treeHash, giá trị tóm tắt lịch sử Commit | Cho phép phân biệt giữa chậm đồng bộ và phân nhánh thực sự |
| Không gộp trực tiếp trạng thái MLS đã tách nhánh | Chọn nhánh và gia nhập lại theo luồng MLS hợp lệ | Tránh tự tạo ra trạng thái mật mã ngoài mô hình MLS |
| Thứ tự hiển thị không quyết định Commit | HLC | Tách application ordering khỏi crypto ordering |

## 4.10. So sánh đặc tính với các thuật toán đồng thuận truyền thống

| Tiêu chí đánh giá | Đồng thuận Quorum (Raft/BFT) | Blockchain | Giải pháp đề xuất (Single-Writer Token) |
|---|---|---|---|
| Mô hình nhất quán | Nhất quán mạnh | Nhất quán mạnh | **Nhất quán cuối cùng** |
| Sức chịu đựng phân vùng mạng | Kém (hệ thống đóng băng nếu không đạt Quorum >N/2) | Tốt | **Rất tốt (các phân vùng nhỏ vẫn hoạt động độc lập và hàn gắn sau)** |
| Độ phức tạp truyền thông tạo Commit | Nặng nề (từ O(N) đến O(N²)) | Nặng nề (phát sóng khối) | **Siêu nhẹ (O(1) - Token Holder phát sóng một lần)** |
| Độ trễ xác nhận | Phụ thuộc RTT mạng (chờ xác nhận đa số) | Rất cao (phụ thuộc thời gian sinh khối) | **Ngay lập tức và cục bộ (không cần biểu quyết)** |
| Sự phù hợp cho P2P Chat | Kém (dễ bị treo mạng, tốn kém tài nguyên) | Không phù hợp (cấu trúc quá cồng kềnh) | **Rất cao (linh hoạt, tiết kiệm băng thông)** |

## 4.11. Giới hạn lý thuyết của phương pháp

Phương pháp đề xuất không nhằm thay thế hoàn toàn các giao thức đồng thuận mạnh trong mọi bài toán phân tán. Nó được xây dựng cho bối cảnh mạng ngang hàng nơi các nút cần tiếp tục hoạt động cục bộ trong khi vẫn cố gắng giữ trạng thái nhóm không phân kỳ quá xa.

Single-Writer phát huy hiệu quả rõ nhất khi các nút còn chia sẻ cùng một ActiveView. Khi ActiveView giữa các phân vùng đã lệch nhau, chính các cơ chế đối chiếu trạng thái và Fork Healing mới trở thành phần quyết định khả năng hội tụ.

HLC chỉ giải quyết thứ tự hiển thị thông điệp ứng dụng. Nó không tham gia vào việc xác nhận Commit, không thay thế kiểm tra epoch và cũng không sửa được các phân nhánh trạng thái.

---

# CHƯƠNG 5. ĐÁNH GIÁ THỰC NGHIỆM

Sau phần phân tích lý thuyết ở Chương 4, đồ án cần một bước kiểm chứng để trả lời câu hỏi liệu các cơ chế đã đề xuất có vận hành được trên hệ thống thực nghiệm hay không. Chương này trình bày phần đánh giá thực nghiệm, trong đó ứng dụng desktop thử nghiệm được dùng như một môi trường kiểm chứng tích hợp.

## 5.1. Mục tiêu và phạm vi đánh giá

Các mục tiêu đánh giá được chia thành ba hướng:
1. Xem xét hiệu quả vận hành của cơ chế Single-Writer và cơ chế gom đề xuất.
2. Kiểm tra tính đúng đắn của tiến trình điều phối (epoch cuối cùng, mã băm cây, số Commit hợp lệ, khả năng phát hiện phân nhánh).
3. Kiểm tra phần mật mã và an toàn, đặc biệt là thuộc tính Forward Secrecy sau khi một thành viên bị loại khỏi nhóm.

| Tiêu chí | Ý nghĩa đánh giá |
|---|---|
| Hội tụ epoch | Các nút hợp lệ phải kết thúc ở cùng một epoch sau khi mạng ổn định trở lại |
| Hội tụ mã băm cây | Các nút hợp lệ phải có cùng mã băm cây MLS |
| Single-Writer trong cùng epoch | Không xuất hiện hai Commit hợp lệ cạnh tranh trong cùng một epoch khi các nút có cùng ActiveView |
| Hiệu quả gom đề xuất | Nhiều Proposal gần nhau có thể được gom vào một Commit thay vì sinh nhiều epoch liên tiếp |
| Thời gian phục hồi phân vùng | Khoảng thời gian từ khi mạng được nối lại đến khi các nút hội tụ |
| Độ trễ thao tác MLS | Chi phí thực thi mã hóa, cập nhật thành viên và xử lý Commit theo quy mô nhóm |
| Forward Secrecy | Thành viên đã bị loại khỏi nhóm không thể giải mã thông điệp ở epoch mới |

## 5.2. Môi trường và phương pháp thí nghiệm

Các thí nghiệm được thực hiện trực tiếp trên mã nguồn của hệ thống. Phần điều phối được viết bằng Go; phần MLS được hiện thực bằng Rust/OpenMLS và được Go khởi chạy dưới dạng tiến trình đi kèm qua gRPC trên localhost. Với những kiểm thử cần cô lập logic điều phối, hệ thống sử dụng các thành phần giả lập như FakeNetwork, FakeClock và MockMLSEngine. Với những kiểm thử cần xác nhận thuộc tính mật mã, hệ thống dùng thành phần MLS thật.

## 5.3. Đánh giá cơ chế Single-Writer và gom đề xuất

Thí nghiệm so sánh hai chiến lược: xử lý ngay khi nhận Proposal và gom theo lô (chờ 1 giây để Token Holder đưa nhiều Proposal vào cùng một Commit).

| Mức đồng thời | Xử lý ngay - Proposal | Xử lý ngay - Commit | Tỷ lệ thành công | Gom theo lô - Proposal | Gom theo lô - Commit | Tỷ lệ thành công |
|---|---|---|---|---|---|---|
| 1 | 1 | 1 | 1,00 | 1 | 1 | 1,00 |
| 2 | 3 | 2 | 0,67 | 2 | 1 | 1,00 |
| 3 | 6 | 3 | 0,50 | 3 | 1 | 1,00 |
| 4 | 10 | 4 | 0,40 | 4 | 1 | 1,00 |
| 5 | 15 | 5 | 0,33 | 5 | 1 | 1,00 |

Khi mức đồng thời tăng, chiến lược xử lý ngay phải tạo thêm Proposal do các nút thường rơi vào tình trạng gửi muộn so với epoch hiện hành. Trong khi đó, chiến lược gom theo lô giữ số Proposal đúng bằng số thao tác thực và vẫn chỉ cần một Commit. Kết quả này củng cố lập luận rằng vai trò của cơ chế Single-Writer không chỉ nằm ở tính đúng đắn, mà còn ở hiệu quả vận hành.

## 5.4. Đánh giá hội tụ trong điều kiện mạng hỗn loạn

Bài kiểm thử TestIntegration_Chaos_Convergence tạo một cụm gồm năm nút cùng tham gia một nhóm MLS. Trong quá trình chạy, tiến trình thử nghiệm liên tục chia mạng thành hai phân vùng, giữ trạng thái phân vùng trong khoảng 600 ms rồi nối mạng lại. Đồng thời, các nút vẫn tiếp tục gửi thông điệp và phát sinh thao tác thay đổi thành viên.

Kết quả cuối cùng cho thấy các nút hợp lệ hội tụ về cùng epoch và cùng mã băm cây. Điều này phù hợp với lập luận ở Chương 4: khác biệt trạng thái được nhận diện trước, sau đó các nút đối chiếu nhánh, chọn nhánh tiếp tục và đưa nhánh còn lại quay về một trạng thái thống nhất. Quan trọng hơn, quá trình này diễn ra mà không cần một Delivery Service trung tâm áp đặt thứ tự từ bên ngoài.

## 5.5. Ảnh hưởng độ sâu phân kỳ đến thời gian phục hồi

Thí nghiệm đo thời gian healing khi độ sâu phân kỳ (số epoch mà hai nhánh đã lệch nhau) tăng từ 5 đến 50. Cụm thử nghiệm gồm 32 nút, chia thành hai phân vùng: nhánh thắng có 30 nút và nhánh thua có 2 nút.

| D (epoch lệch) | Healing (ms) | Group State (KB) | ms/KB |
|---|---|---|---|
| 5 | 728 | 449 | 1,62 |
| 10 | 1039 | 762 | 1,36 |
| 20 | 1155 | 1381 | 0,84 |
| 50 | 2098 | 3240 | 0,65 |

Thời gian healing tăng từ 728 ms ở D=5 lên 2098 ms ở D=50. Tuy nhiên, số bước giao thức không thay đổi theo D — luôn là một ProposalJoin, một Commit và một Welcome. Tỷ lệ ms/KB giảm từ 1,62 ở D=5 xuống 0,65 ở D=50, cho phép tách thành hai thành phần:

1. **Chi phí cố định của giao thức** (O(1)): bầu Token Holder, phát tán ProposalJoin, chờ Commit và Welcome, hoán đổi trạng thái, phát lại tin nhắn.
2. **Chi phí xử lý MLS**: serialize/deserialize group state trong các lệnh gọi gRPC. Phần này tỉ lệ với kích thước group state, chứ không tỉ lệ với D. Nguyên nhân: OpenMLS tuần tự hóa toàn bộ ratchet tree khi xuất group state, và kiến trúc stateless của lõi Rust nhận nguyên group state qua mỗi lệnh gọi gRPC.

Giao thức fork healing đạt độ phức tạp O(1) về số bước. Thời gian tăng theo kích thước group state là hệ quả của cách triển khai hiện tại, không phải chi phí của giao thức điều phối.

## 5.6. Đánh giá khả năng mở rộng của thao tác thành viên

| Quy mô nhóm | Thêm thành viên (ms) | Xóa thành viên (ms) | Cập nhật (ms) |
|---|---|---|---|
| 5 | 0,00 | 0,52 | 0,00 |
| 10 | 0,53 | 1,05 | 0,52 |
| 50 | 6,19 | 7,26 | 6,59 |
| 100 | 24,53 | 23,53 | 24,67 |
| 250 | 130,47 | 129,70 | 128,83 |
| 500 | 527,36 | 531,18 | 517,97 |
| 750 | 1265,50 | 1169,21 | 1188,41 |
| 1000 | 2237,29 | 2194,79 | 2146,94 |

Chi phí tăng rõ khi số lượng thành viên tăng, phù hợp với phân tích lý thuyết. Thao tác thay đổi thành viên luôn đắt hơn gửi application message thông thường vì kéo theo Proposal, Commit và cập nhật trạng thái nhóm.

## 5.7. Đánh giá chi phí mật mã MLS

Benchmark tạo sẵn các trạng thái nhóm với kích thước từ 16 đến 4096 thành viên, lặp lại mỗi phép đo 100 lần với payload 1024 byte.

| Số nút | Mã hóa từng cặp (ms) | MLS (ms) |
|---|---|---|
| 16 | 3,62 | 0,55 |
| 256 | 59,62 | 5,41 |
| 1024 | 238,75 | 19,86 |
| 4096 | 955,18 | 78,95 |

| Mốc đo | Từng cặp (ms) | MLS (ms) |
|---|---|---|
| p95 | 1023,66 | 95,04 |
| p99 | 1081,72 | 110,85 |

Đường mã hóa từng cặp tăng gần tuyến tính theo số thành viên. Đường MLS hiện tại cũng tăng theo quy mô nhóm nhưng chậm hơn nhiều. Ngay cả ở p95 và p99 ở nhóm 4096 thành viên, chênh lệch giữa hai hướng vẫn còn rất lớn.

Tuy nhiên, benchmark cũng cho thấy điểm nghẽn: hàm encrypt_message phải nạp lại toàn bộ trạng thái nhóm và xuất lại trạng thái mới sau mỗi lần gửi, nên đường MLS vẫn tăng theo kích thước group_state.

## 5.8. Chi phí phụ trợ của lớp điều phối

| Số thành viên | Điều phối (ms) | Tổng (ms) | MLS (ms) | Tỷ trọng (%) |
|---|---|---|---|---|
| 16 | 1,58 | 39,73 | 38,15 | 3,97 |
| 32 | 3,08 | 115,43 | 112,35 | 2,67 |
| 64 | 11,30 | 650,70 | 639,40 | 1,74 |
| 128 | 35,55 | --- | --- | --- |
| 256 | 134,86 | --- | --- | --- |
| 512 | 539,69 | --- | --- | --- |
| 1000 | 2204,85 | --- | --- | --- |

Tỷ trọng điều phối giảm dần khi quy mô nhóm tăng: từ 3,97% ở nhóm 16 xuống 1,74% ở nhóm 64. Phần điều phối chỉ chiếm một tỷ lệ nhỏ của tổng chi phí cục bộ. Nút thắt lớn hơn vẫn nằm ở việc xử lý và trao đổi trạng thái nhóm có kích thước ngày càng lớn.

## 5.9. Kiểm chứng Forward Secrecy bằng thành phần MLS thật

Bài kiểm thử TestBusinessP1_E2E_RealSidecar_ForwardSecrecy sử dụng trực tiếp thành phần MLS thật. Kịch bản: Alice tạo nhóm, Bob tham gia nhóm, Alice loại Bob khỏi nhóm, sau đó Alice gửi một thông điệp mới ở epoch sau khi Bob đã bị loại.

Kết quả: Bob không giải mã được thông điệp đó và không có plaintext nào được lưu lại. Việc đặt lớp điều phối bên ngoài MLS không làm suy giảm thuộc tính Forward Secrecy trong kịch bản được kiểm thử. Lớp điều phối chỉ quyết định tiến trình trạng thái và thứ tự xử lý; quyền giải mã cuối cùng vẫn do trạng thái mật mã MLS kiểm soát.

## 5.10. Đối chiếu trên ứng dụng desktop

### Tạo tổ chức và cấp bundle

Ở máy quản trị, người dùng tạo tổ chức và mở phần quản trị để cấp bundle cho một thiết bị mới. Ở máy thành viên, người dùng tạo định danh cục bộ rồi dừng ở màn hình chờ bundle. Một thiết bị không thể tự ý vào mạng của tổ chức; nó phải có bundle hợp lệ do phía quản trị cấp.

### Thiết bị mới vào hệ thống

Sau khi nhận được bundle, người dùng nhập bundle vào ứng dụng. Nếu bundle hợp lệ, ứng dụng chuyển từ màn hình chờ sang màn hình làm việc chính. Thiết bị chỉ thực sự vào hệ thống sau khi bundle được kiểm tra xong, dữ liệu tổ chức được nạp vào máy cục bộ và các quan hệ nhóm cần thiết được khởi tạo.

### Trao đổi tin nhắn trong nhóm

Các bên cùng gửi tin nhắn vào một cuộc trò chuyện chung. Trước đó, các thay đổi về thành viên phải đi qua Proposal và Commit để mọi phía cùng ổn định về một trạng thái nhóm. Chỉ sau bước đó, các tin nhắn mới được mã hóa bằng MLS và hiển thị theo cùng một logic sắp xếp.

## 5.11. Nhận xét và giới hạn đánh giá

Các kết quả thực nghiệm cho thấy hướng tiếp cận của đồ án đạt được ba điểm quan trọng:
1. Cơ chế Single-Writer và cơ chế gom đề xuất giúp giảm xung đột Commit.
2. Tiến trình đối chiếu trạng thái và Fork Healing dựa trên External Proposal cho phép các nút quay về cùng epoch và cùng mã băm cây sau phân vùng mạng.
3. Khi kết hợp với thành phần MLS thật, hệ thống vẫn giữ được thuộc tính Forward Secrecy.

Giới hạn: các chaos test chủ yếu dùng mạng giả lập; các benchmark chưa phải là đánh giá đầu cuối của toàn bộ ứng dụng desktop trên nhiều máy độc lập; mô hình trao đổi toàn trạng thái giữa các thành phần còn gây chi phí lớn khi nhóm rất đông thành viên.

---

# CHƯƠNG 6. KẾT LUẬN

## 6.1. Kết luận chung

Đồ án tốt nghiệp này được thực hiện với hai mục tiêu gắn bó với nhau. Mục tiêu thứ nhất là nghiên cứu một lớp điều phối phi tập trung để đưa MLS vào môi trường mạng ngang hàng, nơi không còn Delivery Service tập trung làm nhiệm vụ áp đặt thứ tự Commit. Mục tiêu thứ hai là xây dựng một ứng dụng desktop thử nghiệm để hiện thực các cơ chế đó và kiểm chứng rằng chúng có thể đi vào một luồng sử dụng thực tế.

Về mặt nghiên cứu, đóng góp chính của đồ án nằm ở lớp điều phối được đặt quanh MLS. Lớp này gồm cơ chế Single-Writer theo epoch để giảm khả năng xuất hiện concurrent commits, cơ chế kiểm tra epoch để tránh xử lý thông điệp trên trạng thái không tương thích, cơ chế đối chiếu và Fork Healing để đưa các nút hội tụ trở lại sau phân vùng mạng, cùng cơ chế HLC để ổn định thứ tự hiển thị application message ở tầng ứng dụng. Điểm quan trọng là các cơ chế này không sửa đổi lõi mật mã của MLS, mà bổ sung lớp điều phối cần thiết để MLS có thể vận hành trong bối cảnh mạng ngang hàng bất đồng bộ.

Một đóng góp cụ thể của đồ án trong Fork Healing là việc thay External Commit bằng External Proposal kết hợp Token Holder. Khi nhiều nút cùng cần gia nhập lại sau phân vùng, External Proposal cho phép Token Holder gom tất cả đề nghị vào một Commit duy nhất thay vì để mỗi nút tự tạo Commit cạnh tranh. Kết quả thực nghiệm cho thấy chiến lược này giữ số Commits luôn bằng 1 và tỷ lệ thành công luôn đạt 1,00. Điều này làm cho chi phí tạo Commit cho fork healing không tăng theo số nút cần heal, giữ ở mức O(1).

Về mặt hiện thực, đồ án đã xây dựng được một ứng dụng desktop thử nghiệm đủ để kiểm chứng các luồng chính của hệ thống. Ứng dụng này cho phép quan sát trực tiếp các thao tác như cấp bundle, gia nhập thiết bị mới, tạo nhóm, thêm thành viên và trao đổi tin nhắn.

Về mặt đánh giá, các kết quả thực nghiệm bước đầu phù hợp với mục tiêu đã đặt ra. Các chaos test và thí nghiệm phân vùng mạng cho thấy hệ thống có thể đưa các nút quay về cùng epoch và cùng mã băm cây sau khi mạng được nối lại. Các kiểm thử với thành phần MLS thật cho thấy thuộc tính Forward Secrecy vẫn được giữ lại.

## 6.2. Giới hạn của đồ án

1. Phần phân tích lý thuyết mới dừng ở mức lập luận theo bất biến và theo giả định vận hành chính, chưa đi tới kiểm chứng hình thức đầy đủ cho toàn bộ giao thức.
2. Phần đánh giá thực nghiệm hiện nay vẫn chủ yếu dựa trên mạng giả lập và các bài kiểm thử có kiểm soát, chưa phản ánh hết độ phức tạp của mạng libp2p thực tế.
3. Trong nhiều thao tác, hệ thống vẫn trao đổi toàn bộ trạng thái nhóm giữa phần điều phối và phần MLS, tạo thêm chi phí khi quy mô nhóm tăng.
4. Cơ chế tự mã hóa và phát lại tin nhắn tồn tại điểm yếu: nếu người gửi ngoại tuyến trước khi mạng hồi phục, hệ thống tạm thời mất thông điệp đó cho đến khi người gửi trực tuyến trở lại.
5. Thời gian healing tăng nhanh khi hai nhánh đã lệch nhiều epoch, phần chi phí nằm ở phía nút nhánh thua khi phải xử lý Welcome và khôi phục trạng thái.
6. Ứng dụng desktop chỉ đóng vai trò ứng dụng thử nghiệm, chưa đủ để xem như một sản phẩm hoàn thiện.

## 6.3. Hướng phát triển

1. **Làm chặt hơn phần cơ sở lý thuyết:** Các bất biến như tính an toàn của cơ chế Single-Writer, tính đơn điệu của epoch và điều kiện hội tụ sau Fork Healing cần được mô hình hóa và kiểm chứng hình thức đầy đủ hơn.
2. **Mở rộng thực nghiệm trên môi trường gần với triển khai thật hơn:** Chạy trên nhiều tiến trình, nhiều máy và trong các điều kiện mạng đa dạng hơn (độ trễ cao, mất gói, thay đổi thành viên liên tục, phân vùng mạng kéo dài).
3. **Tối ưu hóa đường hiện thực giữa phần điều phối và phần MLS:** Giảm chi phí trao đổi trạng thái, tổ chức lại cách lưu trữ hoặc áp dụng các kỹ thuật snapshot hiệu quả hơn. Tối ưu phần khôi phục trạng thái khi độ sâu phân kỳ lớn (tăng tốc xử lý Welcome, áp dụng cơ chế checkpoint định kỳ).
4. **Hoàn thiện ứng dụng thử nghiệm:** Các công cụ quan sát trạng thái, kiểm soát thay đổi thành viên và theo dõi quá trình hội tụ nếu được làm rõ hơn sẽ giúp việc đánh giá hệ thống thuận lợi hơn ở các nghiên cứu tiếp theo.

---

# PHỤ LỤC A. ỨNG DỤNG DEMO

## A.1. Kiến trúc phần mềm

Ứng dụng desktop được xây dựng trên nền tảng Wails, kết hợp Go ở phần backend và React/TypeScript ở phần frontend. Giao diện sử dụng Shadcn UI và Tailwind CSS. Quản lý trạng thái frontend dùng Zustand. Tầng mạng P2P dựa trên libp2p. Lõi mật mã MLS được hiện thực bằng Rust/OpenMLS và giao tiếp với Go qua gRPC. Dữ liệu được lưu cục bộ bằng SQLite.

## A.2. Các công nghệ sử dụng

| Thành phần | Công nghệ |
|---|---|
| Backend | Go, Wails |
| Frontend | React, TypeScript, Vite |
| UI | Shadcn UI, Tailwind CSS |
| State management | Zustand |
| Mạng P2P | libp2p, GossipSub |
| MLS engine | Rust, OpenMLS |
| Giao tiếp Go-Rust | gRPC, protobuf |
| Lưu trữ | SQLite |

## A.3. Các use case chính

### Khởi tạo định danh và onboarding

Người dùng tạo định danh cục bộ trên thiết bị. Quản trị viên tổ chức cấp bundle cho thiết bị mới. Thiết bị nhập bundle để tham gia hệ thống. Quá trình này đảm bảo không thiết bị nào tự ý gia nhập mạng tổ chức mà không có bundle hợp lệ.

### Quản lý nhóm và thành viên

Người dùng tạo nhóm mới. Quản trị viên nhóm có thể thêm thành viên, loại thành viên. Mỗi thao tác thay đổi thành viên đi qua Proposal và Commit, đảm bảo tất cả thành viên cùng cập nhật trạng thái nhóm.

### Trao đổi tin nhắn

Các thành viên trong nhóm có thể gửi tin nhắn văn bản. Tin nhắn được mã hóa bằng MLS và hiển thị theo thứ tự HLC. Giao diện hiển thị trạng thái gửi/nhận và số epoch hiện tại của nhóm.

### Chia sẻ file mã hóa

Người dùng có thể chia sẻ file trong nhóm. File được mã hóa bằng khóa nhóm MLS trước khi phát tán qua mạng P2P.

---

# PHỤ LỤC B. QUY ĐỊNH VIẾT ĐỒ ÁN

Phụ lục này tổng hợp các quy định về trình bày đồ án tốt nghiệp, bao gồm:

- **Định dạng trang:** Khổ A4, lề trái 3cm, lề phải 2cm, lề trên 2cm, lề dưới 2cm.
- **Font chữ:** Times New Roman 13pt cho nội dung, 16pt in đậm cho tiêu đề chương.
- **Đánh số trang:** Số trang ở góc dưới bên phải.
- **Trích dẫn:** Theo chuẩn IEEE.
- **Bảng biểu và hình vẽ:** Đánh số theo chương (Ví dụ: Hình 3.1, Bảng 5.2).
- **Công thức toán:** Sử dụng môi trường equation trong LaTeX, đánh số theo chương.
- **Chống đạo văn:** Mọi nội dung trích dẫn phải ghi rõ nguồn. Đồ án phải là công trình nghiên cứu của sinh viên.

---

# TÀI LIỆU THAM KHẢO

1. J. Alwen, M. Mularczyk, and Y. Tselekounis, "Fork-Resilient Continuous Group Key Agreement," in *Advances in Cryptology -- CRYPTO 2023*, Springer, 2023, pp. 3--34. [Online]. Available: https://eprint.iacr.org/2023/394
2. T. Perrin, M. Marlinspike, and R. Schmidt, "The Double Ratchet Algorithm," Signal Specifications, Revision 4, 2025. [Online]. Available: https://signal.org/docs/specifications/doubleratchet/
3. D. Balbás, D. Collins, and P. Gajland, "Analysis and Improvements of the Sender Keys Protocol for Group Messaging," arXiv preprint arXiv:2301.07045, 2023. [Online]. Available: https://arxiv.org/abs/2301.07045
4. J. Ginesin and C. Nita-Rotaru, "The Matrix Reloaded: A Mechanized Formal Analysis of the Matrix Cryptographic Suite," arXiv preprint arXiv:2408.12743, 2024. [Online]. Available: https://arxiv.org/abs/2408.12743
5. libp2p, "libp2p Concepts Documentation," Official documentation. [Online]. Available: https://docs.libp2p.io/concepts/
6. R. Barnes, B. Beurdouche, R. Robert, J. Millican, E. Omara, and K. Cohn-Gordon, "The Messaging Layer Security (MLS) Protocol," RFC 9420, Jul. 2023. [Online]. Available: https://www.rfc-editor.org/rfc/rfc9420.html
7. S. Turner, R. Barnes, B. Beurdouche, R. Robert, and E. Omara, "The Messaging Layer Security (MLS) Architecture," RFC 9750, Feb. 2025. [Online]. Available: https://www.rfc-editor.org/rfc/rfc9750.html
8. S. S. Kulkarni, M. Demirbas, D. Madeppa, B. Avva, and M. Leone, "Logical Physical Clocks and Consistent Snapshots in Globally Distributed Databases," University at Buffalo, 2014. [Online]. Available: https://cse.buffalo.edu/tech-reports/2014-04.pdf
9. M. Kleppmann, A. Wiggins, P. van Hardenberg, and M. McGranaghan, "Local-first software: You own your data, in spite of the cloud," in *Proc. ACM SIGPLAN Int. Symp. New Ideas, New Paradigms, and Reflections on Programming and Software (Onward! '19)*, 2019. [Online]. Available: https://martin.kleppmann.com/2019/10/23/local-first-at-onward.html
10. L. Lamport, "Time, Clocks, and the Ordering of Events in a Distributed System," *Communications of the ACM*, vol. 21, no. 7, pp. 558--565, 1978.
11. S. Gilbert and N. Lynch, "Brewer's Conjecture and the Feasibility of Consistent, Available, Partition-Tolerant Web Services," *ACM SIGACT News*, vol. 33, no. 2, pp. 51--59, 2002.
12. K. Kohbrok, "Decentralized Messaging Layer Security," Internet-Draft draft-kohbrok-mls-dmls-03, Oct. 2025. [Online]. Available: https://datatracker.ietf.org/doc/draft-kohbrok-mls-dmls/
13. D. Ongaro and J. Ousterhout, "In Search of an Understandable Consensus Algorithm," in *2014 USENIX Annual Technical Conference (USENIX ATC 14)*, Philadelphia, PA, 2014, pp. 305--319.
14. L. Lamport, "Paxos Made Simple," *ACM SIGACT News*, vol. 32, no. 4, pp. 51--58, Dec. 2001.
15. M. Castro and B. Liskov, "Practical Byzantine Fault Tolerance," in *3rd Symp. Operating Systems Design and Implementation (OSDI 99)*, New Orleans, LA, 1999, pp. 173--186.
16. F. B. Schneider, "Implementing Fault-Tolerant Services Using the State Machine Approach: A Tutorial," *ACM Computing Surveys*, vol. 22, no. 4, pp. 299--319, Dec. 1990.
17. S. Nakamoto, "Bitcoin: A Peer-to-Peer Electronic Cash System," Whitepaper, 2008. [Online]. Available: https://bitcoin.org/en/bitcoin-paper
18. W. Vogels, "Eventually Consistent," *ACM Queue*, vol. 6, no. 6, pp. 14--19, Oct. 2008.
19. D. Vyzovitis, Y. Napora, D. McCormick, D. Dias, and Y. Psaras, "GossipSub: Attack-Resilient Message Propagation in the Filecoin Network," Protocol Labs, 2020. [Online]. Available: https://research.protocol.ai/publications/gossipsub-attack-resilient-message-propagation-in-the-filecoin-network/
20. OpenMLS Project, "OpenMLS -- A Rust Implementation of the Messaging Layer Security Protocol," 2025. [Online]. Available: https://openmls.tech/
21. Wails, "Wails -- Build Desktop Applications using Go & Web Technologies," 2025. [Online]. Available: https://wails.io/
22. Protocol Labs, "libp2p Documentation," 2025. [Online]. Available: https://docs.libp2p.io/
23. Rust gRPC, "tonic -- A Rust gRPC Framework," 2025. [Online]. Available: https://github.com/hyperium/tonic
24. SQLite, "SQLite Documentation," 2025. [Online]. Available: https://www.sqlite.org/docs.html
25. Shadcn, "shadcn/ui -- Build Your Component Library," 2025. [Online]. Available: https://ui.shadcn.com/
26. Tailwind CSS, "Tailwind CSS Documentation," 2025. [Online]. Available: https://tailwindcss.com/docs
27. Zustand, "Zustand -- Bear Necessities for State Management in React," 2025. [Online]. Available: https://github.com/pmndrs/zustand
28. Google, "Protocol Buffers -- Google's Data Interchange Format," 2025. [Online]. Available: https://protobuf.dev/
29. Vite, "Vite -- Next Generation Frontend Tooling," 2025. [Online]. Available: https://vitejs.dev/
30. React, "React -- The Library for Web and Native User Interfaces," 2025. [Online]. Available: https://react.dev/
