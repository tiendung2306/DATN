# Kịch bản thuyết trình chi tiết — Đồ án tốt nghiệp
**Tổng thời lượng mục tiêu: ~14 phút nói + 1 phút dự phòng (giới hạn 15 phút)**

Ghi chú cách dùng: mỗi slide có (1) thời lượng gợi ý, (2) lời nói đầy đủ để học thuộc, (3) lưu ý về giọng điệu/nhịp độ. Chỗ có `//` là ghi chú riêng cho bạn, không đọc ra.

---

## Slide 1 — Trang bìa (0:00 – 0:30)

> Kính thưa quý thầy cô trong hội đồng, em tên là Lê Tiến Dũng. Em xin trình bày đồ án tốt nghiệp với đề tài: **"Nghiên cứu và ứng dụng giao thức điều phối phi tập trung cho MLS trên mạng ngang hàng"**, dưới sự hướng dẫn của thầy Đỗ Bá Lâm.

// Nói chậm, rõ, nhìn hội đồng. Đây là câu duy nhất cần thuộc nguyên văn vì mở đầu tạo ấn tượng.

---

## Slide 2 — Nội dung trình bày (0:30 – 0:40)

> Bài trình bày gồm 4 phần chính: (1) giới thiệu bài toán và đóng góp; (2) giao thức điều phối đề xuất gồm 4 cơ chế; (3) triển khai ứng dụng và đánh giá thực nghiệm RQ1–RQ5; (4) kết luận và hướng phát triển.

---

## Slide 3 — 1.1. Nền tảng MLS (0:40 – 1:20)

> Em xin bắt đầu bằng một câu hỏi: **làm sao mã hóa tin nhắn cho nhóm hàng nghìn người mà vẫn đảm bảo khi một thành viên rời nhóm, anh ta không đọc được tin sau đó?**
>
> Câu trả lời là **MLS — Messaging Layer Security**, chuẩn hóa trong RFC 9420, đang được Signal, WhatsApp và Cisco Webex triển khai — tiếp cận hàng tỷ người dùng. MLS là bước tiến lớn nhất trong mã hóa nhóm thập kỷ qua vì nó giải quyết được điều mà mã hóa từng cặp không thể: **cập nhật khóa với độ phức tạp O(log N)** nhờ cấu trúc TreeKEM, thay vì tuyến tính O(N).
>
> MLS quản lý nhóm bằng **Epoch** — mỗi lần thay đổi thành viên là một epoch mới. Ba loại thông điệp: **Proposal** đề nghị thay đổi, **Commit** gom Proposal và tạo epoch mới, **Welcome** cho thành viên mới. Đặc biệt, MLS đảm bảo hai thuộc tính bảo mật mạnh: **Forward Secrecy** — thành viên bị loại không đọc được tin tương lai, và **Post-Compromise Security** — hệ thống tự phục hồi sau khi khóa bị lộ.
>
> Nhưng MLS có **một giả định nền tảng** mà em muốn nhấn mạnh: Commit **không giao hoán** — thứ tự xử lý quyết định kết quả. Để đảm bảo thứ tự đó, MLS **giả định luôn có một Delivery Service trung tâm** đứng ra tuần tự hóa. Và chính giả định này là nguồn gốc của bài toán mà đồ án em giải quyết.

// Slide này phải tạo hai cảm xúc: (1) MLS rất đột phá, (2) nhưng có một "nhưng" rất lớn. Câu hỏi mở đầu thu hút chú ý ngay từ giây đầu. Nhấn mạnh cụm "giả định luôn có Delivery Service" — câu bản lề dẫn sang slide 4.

---

## Slide 4 — 1.2. Khoảng trống nghiên cứu (1:20 – 2:25)

> Giả định về Delivery Service đó hoàn toàn hợp lý cho Signal hay WhatsApp — họ có máy chủ. Nhưng có một lớp ứng dụng đang ngày càng quan trọng mà **không thể có máy chủ trung tâm**: các ứng dụng nhắn tin **P2P và local-first** — nơi người dùng sở hữu dữ liệu, không phụ thuộc đám mây.
>
> Khi bỏ Delivery Service ra khỏi MLS, vấn đề xảy ra. Ở mô hình truyền thống, DS nhận Commit của Alice, sắp thứ tự, phát cho Bob, Carol, Dave theo một trình tự duy nhất — tất cả đồng bộ. Nhưng trên P2P, nếu Alice và Carol cùng lúc tạo Commit ở cùng epoch E, phát qua GossipSub, Bob nhận Commit của Alice trước, Dave nhận Commit của Carol trước — hai nút cùng chuyển sang epoch E+1 nhưng **cây khóa hoàn toàn khác nhau**. Họ **không thể giải mã tin nhắn của nhau** — đây là hiện tượng **phân nhánh trạng thái mật mã**.
>
> Khoảng trống nghiên cứu ở đây là: **làm sao vận hành MLS trên P2P mà không cần Delivery Service, nhưng vẫn tránh và phục hồi được phân nhánh — và quan trọng nhất, vẫn giữ nguyên toàn bộ Forward Secrecy và PCS?**
>
> Tại sao đây là khoảng trống thật sự? Vì hướng tiếp cận duy nhất hiện có — **DMLS**, một Internet-Draft mở rộng MLS cho phi tập trung — lại **phải đánh đổi Forward Secrecy**, nghĩa là bỏ đi chính lý do tồn tại của MLS. Còn các thuật toán đồng thuận như Raft hay BFT thì **đóng băng hoàn toàn** khi mất quá nửa nút — không phù hợp môi trường P2P vốn phân vùng thường xuyên.

// Đây là slide định hình toàn bộ bài — hội đồng cần hiểu: có một khoảng trống nghiên cứu thật sự, chưa ai lấp được mà không đánh đổi. Nói chậm, nhấn từng ý. Có thể dừng 1 giây sau từ "FORK!" để nhấn.

---

## Slide 5 — 1.3. Mục tiêu nghiên cứu (2:25 – 3:00)

> Để lấp khoảng trống đó, em xác định **4 mục tiêu cụ thể**, mỗi mục tiêu giải quyết một khía cạnh của bài toán: thứ nhất, **giảm commits đồng thời** khi mạng còn liên thông — để tránh phân nhánh ngay từ đầu; thứ hai, **buộc mỗi nút chỉ xử lý trên epoch tương thích** — để ngăn trạng thái lùi; thứ ba, **phát hiện và hàn gắn phân nhánh** khi phân vùng mạng đã xảy ra — để hội tụ trở lại; và thứ tư, **giữ thứ tự hiển thị ổn định** cho tin nhắn — vì trên P2P không có đồng hồ trung tâm.
>
> Ràng buộc xuyên suốt: **không sửa RFC 9420**, chỉ đóng gói bên ngoài.

// Nói nhanh — mỗi mục tiêu nối trực tiếp với một khía cạnh của khoảng trống ở slide 4. Phần so sánh chi tiết với Raft/BFT/Blockchain/DMLS sẽ ở slide tiếp theo.

---

## Slide 6 — 1.4. Tại sao không dùng giải pháp có sẵn? (3:00 – 3:45)

> Hội đồng có thể hỏi: tại sao không dùng luôn các giải pháp đồng thuận hiện có?
>
> **Raft hay BFT** — đảm bảo nhất quán mạnh, nhưng **đóng băng hoàn toàn** khi mất quá nửa nút, và chi phí truyền thông O(N bình phương). Trong môi trường P2P, phân vùng là hiện tượng thường xuyên, không phải ngoại lệ — đóng băng mỗi khi phân vùng là không chấp nhận được.
>
> **Blockchain** chịu phân vùng tốt hơn nhưng độ trễ tạo khối tính bằng giây — quá chậm cho chat thời gian thực.
>
> **DMLS** — Internet-Draft gần đây — là hướng tiếp cận gần nhất, nhưng **tăng độ phức tạp và phải đánh đổi Forward Secrecy**. Đây là điểm mấu chốt: MLS tồn tại chính vì Forward Secrecy và PCS — nếu bỏ đi thì dùng MLS làm gì?
>
> Giải pháp đề xuất chọn **Eventual Consistency** — chấp nhận phân vùng nhỏ vẫn hoạt động, chi phí mạng tạo Commit chỉ **O(1)** — một Token Holder phát sóng một lần, không cần N² message như Raft. Chi phí xử lý cục bộ ở mỗi nút vẫn là O(N) nhưng không cần truyền thông thêm. Và đặc biệt **giữ nguyên toàn bộ FS/PCS** — đây là điểm khác biệt cốt lõi so với DMLS.

// Giọng tự tin — đây là slide chứng minh bạn đã khảo sát đầy đủ và có lý do rõ ràng để không dùng giải pháp có sẵn.

---

## Slide 7 — 1.5. Đóng góp chính (3:45 – 4:10)

> Từ khoảng trống nghiên cứu đó, đồ án có **2 nhóm đóng góp**. Về **nghiên cứu**, em đề xuất 4 cơ chế điều phối — Single-Writer, kiểm tra epoch, Fork Detection/Healing và HLC — hoàn toàn **không sửa lõi MLS**, chỉ đóng gói bên ngoài RFC 9420. Đây là điểm khác biệt so với DMLS: DMLS sửa bên trong MLS và đánh đổi FS, còn em giữ nguyên MLS và thêm lớp điều phối bên ngoài. Phần nghiên cứu cũng có cơ sở lý thuyết từ CAP/SMR/FLP để chọn AP + Eventual Consistency, cùng phân tích chi phí O(1)/O(log N).
>
> Về **ứng dụng**, em xây dựng desktop thử nghiệm bằng Wails + Go + Rust, triển khai 4 use case thực tế và chạy thực nghiệm RQ1–RQ5 trên mạng giả lập.
>
> **Kết quả cốt lõi: giữ nguyên FS/PCS của MLS trong môi trường P2P không có Delivery Service — điều mà DMLS chưa làm được.**

// Nói dứt khoát — câu cuối là câu quan trọng nhất, nhấn mạnh "điều mà DMLS chưa làm được".

---

## Slide 8 — 2.1. Cơ chế 1: Single-Writer (4:10 – 5:25)

> Em xin đi vào phần đóng góp nghiên cứu. Cơ chế đầu tiên — và là nền tảng cho cả 3 cơ chế còn lại — là **Single-Writer theo epoch**.
>
> Về mặt lý thuyết, tiến hóa trạng thái nhóm bản chất là bài toán **State Machine Replication**, chịu ràng buộc của **định lý CAP**. Nếu chọn Nhất quán mạnh như Raft hay BFT, hệ thống sẽ mất Khả dụng mỗi khi phân vùng. Vì vậy em chọn hướng **AP kết hợp Eventual Consistency** — ưu tiên khả dụng, chấp nhận phân kỳ tạm thời, miễn là có cơ chế hội tụ trở lại.
>
> Ý tưởng cốt lõi: tách vai trò Proposal và Commit. **Mọi thành viên hợp lệ đều được tạo Proposal**, nhưng **chỉ một Token Holder duy nhất** trong mỗi epoch được quyền gom Proposal thành Commit.
>
> Token Holder được bầu bằng công thức xác định trước: lấy SHA-256 của group ID ghép với epoch và peer ID, ai có giá trị hash nhỏ nhất thì được chọn. Vì tất cả các nút cùng đầu vào sẽ luôn ra cùng kết quả, hệ thống **không cần thêm bất kỳ vòng bỏ phiếu mạng nào** — chi phí chỉ là O(N) tính toán cục bộ.
>
> Nếu Token Holder rơi mạng, các nút sẽ loại nó khỏi ActiveView và tự động tính lại — đây là cơ chế failover.

// Đây là slide có yếu tố lý thuyết (CAP) — nói với giọng tự tin, đây là điểm cộng lớn về chiều sâu.

---

## Slide 9 — 2.2. Cơ chế 2: Kiểm tra epoch (5:10 – 6:00)

> Cơ chế thứ hai bảo vệ tính đúng đắn ở tầng cục bộ: **kiểm tra epoch trước khi đưa thông điệp xuống lớp mật mã**.
>
> Khi một bản tin đến, nút so sánh epoch của bản tin với epoch cục bộ. Nếu **nhỏ hơn** — bản tin thuộc quá khứ — **từ chối ngay**, không cho phép kéo trạng thái lùi lại. Nếu **bằng** — xử lý bình thường. Nếu **lớn hơn** — bản tin đến từ tương lai — hệ thống **đệm lại** và kiểm tra xem đó chỉ là do mình đang chậm đồng bộ, hay đã thực sự phân nhánh.
>
> Bất biến giữ được ở đây là: epoch cục bộ của một nút là đại lượng **đơn điệu không giảm** — E tại thời điểm t+1 luôn lớn hơn hoặc bằng E tại thời điểm t.

---

## Slide 10 — 2.3. Cơ chế 3: Fork Detection (6:00 – 7:00)

> Vấn đề đặt ra: khi thấy một bản tin từ tương lai, làm sao phân biệt được đó là **do mình chậm** hay **đã thực sự phân nhánh**? Đồ án giải quyết bằng **History Hash**.
>
> Mỗi nút duy trì một giá trị R(E), tính bằng cách băm lũy tiến: R(e) bằng hàm băm của R(e trừ 1) ghép với hash của Commit tại epoch e. Giá trị này tóm tắt **toàn bộ chuỗi Commit** từ epoch 1 cho tới hiện tại — không chỉ Commit gần nhất.
>
> Khi đối chiếu hai nút A và B: nếu R của B tại epoch của A **trùng khớp** với R của A, thì A chỉ đang đi chậm trên cùng một nhánh — chỉ cần bắt kịp. Nếu **khác nhau**, thì đã có phân nhánh thật sự xảy ra, và hệ thống chuyển sang quy trình Fork Healing.
>
> Ở đây em cũng phát biểu một **định nghĩa hình thức về hội tụ**: một nhóm được xem là hội tụ tại epoch E khi và chỉ khi mọi nút cùng thỏa **3 điều kiện đồng thời** — cùng epoch, cùng tree hash, **và** cùng history hash. Chỉ cùng epoch thôi là chưa đủ, vì hai nút vẫn có thể đang ở hai nhánh khác nhau dù cùng nhãn epoch.

// Nhấn mạnh phần "định nghĩa hình thức" — đây là chi tiết thể hiện tính học thuật/rigor.

---

## Slide 11 — 2.4. Fork Healing (7:00 – 8:25)

> Đây là phần đóng góp em cho là quan trọng nhất về mặt kỹ thuật. Slide chia làm hai phần: **bên trái** là 5 bước hàn gắn, **bên phải** là lý do tại sao quy trình này hiệu quả, và **dưới cùng** là sơ đồ luồng từ nhánh thua sang nhánh thống nhất.
>
> **5 bước hàn gắn**. **Bước 1**: chọn nhánh thắng theo hàm trọng số — ưu tiên số thành viên hoạt động, rồi epoch cao hơn, rồi hash Commit để phá vỡ hòa; chi tiết công thức ở Phụ lục B. **Bước 2**: nút ở nhánh thua **hủy MLS cũ** và tạo KeyPackage mới. **Bước 3**: gửi một **External Proposal** dạng Add qua GossipSub. **Bước 4**: Token Holder của nhánh thắng gom tất cả các External Proposal vào **đúng một Commit duy nhất**, kèm Welcome. **Bước 5**: các nút replay tin nhắn hợp lệ trên nhánh mới — Autonomous Replay.
>
> Tại sao hiệu quả? Vì **1 Commit hàn gắn toàn bộ K nút** lệch nhánh. Nếu để mỗi nút tự commit riêng, K nút sẽ tạo ra nhiều Commit cạnh tranh, chi phí tăng lên **O(K bình phương)**. Cách tiếp cận này chỉ cần **O(1)** — đúng một Commit bất kể K lớn cỡ nào. Đồng thời, nút thua phải **crypto-shredding** — hủy khóa nhánh thua để giữ Forward Secrecy/PCS — và Autonomous Replay chỉ cho phép mỗi nút phát lại tin của chính mình, không đại diện cho người khác.

// Slide dài nhất, quan trọng nhất — luyện nói mạch lạc, không vấp. Chỉ vào slide 11 khi đã thuộc mạch 5 bước.

---

## Slide 12 — 2.5. Cơ chế 4: HLC (8:25 – 9:25)

> Cơ chế cuối cùng giải quyết một vấn đề khác: **thứ tự hiển thị**.
>
> MLS có hai loại thông điệp: **Control message** — Proposal, Commit, làm thay đổi khóa nhóm; và **Application message** — tin nhắn chat thực sự. Khi có Delivery Service, DS gắn timestamp quyết định thứ tự hiển thị. Nhưng trên P2P, không có DS, và đồng hồ vật lý giữa các máy **không đồng bộ** — không thể tin cậy physical clock thuần túy.
>
> Giải pháp là **Hybrid Logical Clock** — HLC, biểu diễn bằng bộ ba (L, C, NodeID): L là thời gian logic bám theo thời gian vật lý, C là bộ đếm phân biệt các sự kiện cùng L, NodeID phá vỡ hòa. Thuật toán HLC_Now lấy giá trị lớn hơn giữa L cục bộ và thời gian vật lý; nếu bằng L cũ thì tăng counter, nếu không thì reset counter về 0.
>
> Điểm mấu chốt: HLC **chỉ áp dụng cho application message**, hoàn toàn **không can thiệp vào Control message** — Commit vẫn dùng epoch ordering như bình thường. Hai loại thứ tự này — thứ tự mật mã và thứ tự hiển thị — được **tách biệt hoàn toàn**, để tránh nhầm lẫn giữa đồng hồ ứng dụng với cơ chế điều phối trạng thái mật mã.

---

## Slide 13 — 3.1. Kiến trúc phần mềm (9:25 – 10:25)

> Chuyển sang phần triển khai. Về kỹ nghệ phần mềm, hệ thống có kiến trúc **ba tầng**.
>
> **Frontend** bằng React, TypeScript, Shadcn UI, Tailwind, Zustand, giao tiếp qua Wails Bindings.
>
> **Tầng điều phối** viết bằng Go theo **kiến trúc Hexagonal**, chứa toàn bộ 4 cơ chế vừa trình bày, với các adapter cho P2P dùng libp2p và GossipSub, và adapter lưu trữ SQLite mã hóa.
>
> **Tầng MLS** viết bằng Rust dùng thư viện OpenMLS, chạy như một **stateless sidecar** giao tiếp qua gRPC. Điểm quan trọng nhất ở đây: **Rust hoàn toàn không biết gì về Single-Writer hay epoch check** — nó chỉ thực hiện các phép biến đổi mật mã hợp lệ theo đúng RFC 9420. Go lấy trạng thái từ SQLite, gửi qua gRPC, Rust tính toán, trả kết quả, Go lưu lại.
>
> Nguyên tắc ranh giới này đảm bảo: lớp điều phối quyết định **ai** được Commit và **khi nào**, còn MLS chỉ lo phần **biến đổi mật mã** — tuyệt đối không sửa đổi chuẩn RFC 9420.

---

## Slide 14 — 3.2. Ứng dụng desktop: Onboarding & Quản trị nhóm (10:25 – 10:50)

> Hai use case đầu tiên của ứng dụng desktop. **Onboarding & Bundle**: thiết bị tự sinh khóa định danh, gửi yêu cầu lên quản trị viên; QTV duyệt và cấp Bundle chứa thông tin mạng + khóa công khai gốc, giúp thiết bị vào mạng P2P mà **không cần máy chủ trung tâm**. **Quản trị nhóm**: giao diện xem thành viên, vai trò; mỗi thao tác thêm/xóa thành viên đều kích hoạt đúng luồng Proposal và Commit đã trình bày.
>
> Hai use case tiếp theo ở slide kế.

// Nói với giọng thực tế, đi nhanh qua từng ảnh.

---

## Slide 15 — 3.3. Ứng dụng desktop: Chat nhóm & Chia sẻ tệp (10:50 – 11:15)

> **Group Chat**: tin nhắn mã hóa MLS hiển thị đồng bộ trên nhiều thiết bị, thể hiện việc điều phối epoch và HLC hoạt động trong thực tế. **Chia sẻ tệp mã hóa**: dùng MLS Exporter để dẫn xuất khóa AES-256-GCM, truyền tệp qua libp2p stream được xác thực — khóa bí mật **không bao giờ đi qua mạng**.
>
> Đây là minh chứng cho thấy các cơ chế lý thuyết không chỉ dừng ở mô tả, mà đã đi vào một hệ thống chạy được thực tế, đầu cuối.

// Nói với giọng thực tế, cho hội đồng thấy đồ án có sản phẩm cụ thể.

---

## Slide 16 — 3.4. RQ1 + RQ2: Single-Writer & Chaos test (11:15 – 12:10)

> Phần lý thuyết đã trình bày xong. Bây giờ em xin chuyển sang phần đánh giá thực nghiệm để trả lời câu hỏi: các cơ chế đó có thực sự hoạt động không? Em bắt đầu với hai câu hỏi nghiên cứu đầu tiên.
>
> **RQ1** — Single-Writer có thực sự giảm xung đột? Cách đo: 5 nút, mỗi nút đề xuất 10 proposal. Nếu xử lý ngay không qua Single-Writer, tỷ lệ Commit thành công giảm từ 1.00 xuống còn 0.33. Nhưng với cơ chế gom batch của Single-Writer, tỷ lệ thành công giữ nguyên **1.00**, và số Commit luôn đúng bằng 1.
>
> **RQ2** — Hệ thống có hội tụ sau phân vùng mạng không? Em chạy chaos test với 5 nút, phân vùng lặp lại mỗi 600 mili giây. Kết quả: khi mạng nối lại, tất cả các nút đều **hội tụ về cùng epoch và cùng tree hash** — hoàn toàn không cần Delivery Service.

// Giữ nhịp đều — đây là slide số liệu, nói rõ con số.

---

## Slide 17 — 3.5. RQ3: Benchmark mật mã (12:10 – 12:35)

> **RQ3** — Chi phí mật mã MLS so với mã hóa từng cặp là bao nhiêu? Thiết lập: tạo nhóm với số thành viên từ 16 đến 4096, mã hóa một tin nhắn 1 KB gửi đến toàn bộ nhóm, lặp 100 lần và lấy thời gian trung vị.
>
> Kết quả với nhóm **4096 thành viên**: MLS chỉ tốn **78,95 ms**, trong khi mã hóa từng cặp tốn **955,18 ms** — nhanh hơn **12 lần**. Độ trễ đuôi p99 cũng tương tự: MLS **110,85 ms** so với từng cặp **1081,72 ms**. MLS giữ chi phí mã hóa ổn định khi nhóm lớn, trong khi từng cặp tăng tuyến tính theo số thành viên.

// Nhấn mạnh "12 lần" — con số dễ nhớ nhất.

---

## Slide 18 — 3.6. RQ4: Khả năng mở rộng (12:35 – 12:55)

> **RQ4** — Khả năng mở rộng của lớp điều phối khi nhóm lớn? Cách đo: tạo nhóm N nút, một nút đề xuất Add/Remove/Update, đo tổng thời gian để toàn bộ N nút cùng tiến lên epoch tiếp theo. Kết quả trong bảng cho thấy thời gian tăng khi N lớn, nhưng đây chủ yếu là chi phí mật mã MLS, không phải lớp điều phối. Ví dụ ở 1000 nút, Add mất khoảng 2237 ms tổng — chia ra chỉ khoảng **2,2 ms trên mỗi nút**.

// Nói rõ con số per-node để hội đồng thấy không quá nặng.

---

## Slide 19 — 3.7. RQ5: Overhead + Forward Secrecy (12:55 – 13:20)

> **RQ5a** — Coordinator overhead: tỷ trọng chi phí điều phối chỉ chiếm **dưới 4%** tổng chi phí cục bộ — phần lớn thời gian vẫn nằm ở bản thân thao tác mật mã MLS.
>
> **RQ5b** — Forward Secrecy: loại 1 thành viên, gửi tin ở epoch mới, rồi kiểm tra giải mã. Kết quả: thành viên bị loại **không giải mã được** — MLS giữ đầy đủ FS/PCS ngay trên P2P.

// Nếu bị hỏi về healing theo độ sâu phân kỳ → chỉ vào Phụ lục C.

---

## Slide 20 — 4.1. Kết luận & Hướng phát triển (13:35 – 14:30)

> Trở lại với khoảng trống nghiên cứu em nêu ở đầu bài: **MLS cần Delivery Service, P2P không có, DMLS đánh đổi Forward Secrecy**. Đồ án này đã lấp khoảng trống đó bằng **4 cơ chế điều phối phối hợp**, hoàn toàn không sửa lõi MLS — trong đó Fork Healing đưa K nút lệch nhánh hội tụ chỉ bằng **1 Commit duy nhất**, bất kể độ sâu phân kỳ. Lựa chọn **Eventual Consistency** phù hợp bài toán nhắn tin: chấp nhận phân vùng, ưu tiên sẵn sàng, không đóng băng khi mất quorum. Và quan trọng nhất: **Forward Secrecy và PCS được giữ nguyên** — điều mà hướng tiếp cận hiện có chưa đạt được.
>
> Về giới hạn, em nhìn nhận thẳng thắn: phần lý thuyết mới dừng ở lập luận theo bất biến, chưa có kiểm chứng hình thức đầy đủ; thực nghiệm chủ yếu chạy trên mạng giả lập; và group state vẫn truyền toàn bộ qua gRPC giữa Go và Rust.
>
> Hướng phát triển: chứng minh formal giao thức, mở rộng thực nghiệm trên nhiều máy vật lý, và tối ưu giao tiếp Go–Rust bằng delta state.
>
> Em xin cảm ơn quý thầy cô đã lắng nghe, và em rất mong nhận được câu hỏi cũng như góp ý từ hội đồng.

// Dừng, mỉm cười. Câu mở đầu nối lại với khoảng trống ở slide 4 — tạo cảm giác bài trình bày có đầu có cuối, khép kín.

---

## Ghi chú tổng thể khi luyện tập

- **Tốc độ nói mục tiêu**: khoảng 150 từ/phút cho phần lý thuyết (slide 7-12), có thể nhanh hơn ở phần giới thiệu/kết luận.
- **3 slide cần luyện kỹ nhất vì dễ vấp**: Slide 11 (Fork Healing, 5 bước + O(K²) vs O(1) + crypto-shredding), Slide 10 (công thức History Hash), Slide 12 (thuật toán HLC_Now).
- **Nếu bị hỏi giờ chạy chậm**: có thể cắt bớt phần "lưu ý phụ" (in nghiêng `//`) — không đọc — để rút ngắn, giữ nguyên phần in đậm là nội dung cốt lõi.
- **Câu hỏi khả năng cao sẽ được hỏi** (chuẩn bị sẵn câu trả lời):
  1. *"Tại sao không dùng Raft/Paxos luôn cho đơn giản?"* → Raft là CP, đóng băng khi phân vùng — không phù hợp P2P/local-first. Bảng so sánh chi tiết ở slide 6.
  2. *"Nếu 2 Token Holder ở 2 phân vùng cùng lúc thì sao?"* → Đó chính là kịch bản dẫn tới Fork Detection + Healing ở slide 10-11; Single-Writer chỉ giảm xung đột **khi cùng ActiveView**, không loại bỏ hoàn toàn fork.
  3. *"HLC có đảm bảo thứ tự tuyệt đối không?"* → Không, chỉ đảm bảo happens-before + total order cục bộ nhất quán mỗi nút, không phải đồng bộ đồng hồ tuyệt đối toàn cục.
  4. *"Sau Fork Healing, tin nhắn cũ trong lúc phân vùng có bị mất không?"* → Autonomous Replay ở slide 11: mỗi nút chỉ phát lại tin của chính mình; nếu tác giả offline vĩnh viễn trước khi mạng hồi phục, tin đó tạm thời không phục hồi được — đây là giới hạn đã ghi nhận.
  5. *"Đồ án này khác DMLS ở điểm gì?"* → DMLS sửa bên trong MLS và đánh đổi Forward Secrecy; em giữ nguyên MLS, thêm lớp điều phối bên ngoài, giữ đầy đủ FS/PCS. Đây là điểm khác biệt cốt lõi — đã nêu ở slide 6, 7 và 20.
  6. *"MLS đã có Signal, WhatsApp dùng rồi, tại sao cần P2P?"* → Signal/WhatsApp vẫn phụ thuộc máy chủ trung tâm — điểm lỗi đơn và kiểm soát dữ liệu. Xu hướng local-first (Kleppmann 2019) đang đòi hỏi ứng dụng không phụ thuộc đám mây. MLS hiện không phục hồi được lớp ứng dụng này.
