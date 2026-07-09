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

## Slide 3 — 1.1. Nền tảng MLS (0:40 – 1:15)

> Trước khi vào bài toán, em xin giới thiệu nhanh nền tảng công nghệ mà đồ án sử dụng: giao thức **MLS — Messaging Layer Security**, được chuẩn hóa trong RFC 9420, hiện đang được dùng trong các hệ thống lớn như Signal, WhatsApp và Cisco Webex.
>
> MLS quản lý một nhóm chat bằng khái niệm **Epoch** — mỗi phiên bản trạng thái khóa nhóm là một epoch. Ba loại thông điệp chính: **Proposal** — đề nghị thay đổi thành viên; **Commit** — gom các Proposal lại và thực sự tạo ra epoch mới; và **Welcome** — thông tin cho thành viên mới gia nhập.
>
> Điểm quan trọng: Commit **không giao hoán** — tức là thứ tự xử lý Commit sẽ quyết định kết quả cuối cùng. Đây chính là nguồn gốc của bài toán mà đồ án giải quyết.
>
> Về mặt bảo mật, MLS dùng cấu trúc cây nhị phân **TreeKEM** giúp cập nhật khóa với độ phức tạp O(log N) thay vì tuyến tính, đồng thời đảm bảo hai thuộc tính quan trọng: **Forward Secrecy** — thành viên bị loại không đọc được tin nhắn tương lai, và **Post-Compromise Security** — hệ thống tự phục hồi sau khi khóa bị lộ.

// Nhấn mạnh cụm "không giao hoán" — đây là câu bản lề dẫn sang slide 4.

---

## Slide 4 — 1.2. Vấn đề (1:15 – 2:15)

> Vấn đề nằm ở đây: **MLS mặc định giả định luôn có một Delivery Service trung tâm** đứng ra tuần tự hóa mọi Commit.
>
> Ở mô hình truyền thống, khi Alice tạo Commit, Delivery Service nhận, sắp thứ tự, rồi phát cho Bob, Carol, Dave theo đúng một trình tự duy nhất — E1, E2, E3. Tất cả các nút luôn đồng bộ.
>
> Nhưng trên mạng ngang hàng, **không tồn tại thực thể trung tâm đó**. Giả sử Alice và Carol cùng lúc tạo Commit ở cùng epoch E, phát qua GossipSub. Bob nhận được Commit của Alice trước, còn Dave nhận được Commit của Carol trước. Hai nút này cùng chuyển sang epoch E+1 — nhưng cây khóa của họ, tree hash của họ, hoàn toàn khác nhau. Kết quả là họ **không thể giải mã thông điệp của nhau nữa** — đây gọi là hiện tượng **phân nhánh trạng thái mật mã**.
>
> Đây chính là bài toán trung tâm mà đồ án của em giải quyết: **làm sao vận hành MLS trên P2P mà không cần Delivery Service, nhưng vẫn tránh và phục hồi được khỏi phân nhánh.**

// Đây là slide quan trọng nhất để hội đồng hiểu vấn đề — nói chậm, có thể dừng 1 giây sau từ "FORK!" để nhấn.

---

## Slide 5 — 1.3. Mục tiêu nghiên cứu (2:15 – 2:50)

> Từ bài toán đó, em xác định **4 mục tiêu** cho đồ án: giảm concurrent commits khi mạng còn liên thông; buộc mỗi nút chỉ xử lý trên epoch tương thích; phát hiện và hàn gắn phân nhánh sau khi phân vùng mạng xảy ra; và giữ được thứ tự hiển thị ổn định cho tin nhắn.
>
// Nói nhanh — phần so sánh chi tiết với Raft/BFT/Blockchain/DMLS sẽ ở slide tiếp theo.

---

## Slide 6 — 1.4. So sánh với các hướng tiếp cận hiện có (2:50 – 3:35)

> Em xin so sánh giải pháp đề xuất với các hướng tiếp cận hiện có.
>
> **Raft hay BFT** — các thuật toán đồng thuận quorum — đảm bảo nhất quán mạnh, nhưng **đóng băng hoàn toàn** khi mất quá nửa nút, và chi phí truyền thông lên tới O(N bình phương). **Blockchain** chịu phân vùng tốt hơn nhưng độ trễ tạo khối quá cao cho chat thời gian thực. Còn **DMLS** — Internet-Draft gần đây mở rộng MLS cho môi trường phi tập trung — thì tăng độ phức tạp và phải đánh đổi Forward Secrecy.
>
> Giải pháp đề xuất chọn **Eventual Consistency** thay vì nhất quán mạnh, chấp nhận phân vùng nhỏ vẫn hoạt động, chi phí tạo Commit chỉ **O(1)** — một Token Holder phát sóng một lần, và đặc biệt là **giữ nguyên toàn bộ FS/PCS của MLS**.

// Nói với giọng tự tin — đây là slide chứng minh bạn đã nghiên cứu đầy đủ các hướng thay thế.

---

## Slide 7 — 1.5. Đóng góp chính (3:35 – 3:55)

> Đồ án có **2 nhóm đóng góp chính**. Về **nghiên cứu**, em đề xuất 4 cơ chế điều phối — Single-Writer, kiểm tra epoch, Fork Detection/Healing và HLC — hoàn toàn **không sửa lõi MLS**, chỉ đóng gói bên ngoài RFC 9420. Phần nghiên cứu này cũng có cơ sở lý thuyết từ CAP/SMR/FLP để chọn AP + Eventual Consistency, cùng phân tích chi phí O(1)/O(log N). Về **ứng dụng**, em xây dựng desktop thử nghiệm bằng Wails + Go + Rust, triển khai 4 use case thực tế và chạy thực nghiệm RQ1–RQ5 trên mạng giả lập.
>
> Kết quả cốt lõi: giữ nguyên FS/PCS của MLS trong môi trường P2P không có Delivery Service.

// Nói nhanh, dứt khoát — slide outline, giữ nhịp.

---

## Slide 8 — 2.1. Cơ chế 1: Single-Writer (3:55 – 5:10)

> Cơ chế đầu tiên: **Single-Writer theo epoch**.
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
> **5 bước hàn gắn**. **Bước 1**: chọn nhánh thắng theo quy tắc xác định — ưu tiên số thành viên đang hoạt động, rồi epoch cao hơn, rồi hash Commit gần nhất để phá vỡ hòa. **Bước 2**: nút ở nhánh thua **hủy MLS cũ** và tạo KeyPackage mới. **Bước 3**: gửi một **External Proposal** dạng Add qua GossipSub. **Bước 4**: Token Holder của nhánh thắng gom tất cả các External Proposal vào **đúng một Commit duy nhất**, kèm Welcome. **Bước 5**: các nút replay tin nhắn hợp lệ trên nhánh mới — Autonomous Replay.
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

> Chuyển sang phần đánh giá thực nghiệm, trả lời hai câu hỏi nghiên cứu.
>
> **RQ1** — Single-Writer có thực sự giảm xung đột? Cách đo: 5 nút, mỗi nút đề xuất 10 proposal. Nếu xử lý ngay không qua Single-Writer, tỷ lệ Commit thành công giảm từ 1.00 xuống còn 0.33. Nhưng với cơ chế gom batch của Single-Writer, tỷ lệ thành công giữ nguyên **1.00**, và số Commit luôn đúng bằng 1.
>
> **RQ2** — Hệ thống có hội tụ sau phân vùng mạng không? Em chạy chaos test với 5 nút, phân vùng lặp lại mỗi 600 mili giây. Kết quả: khi mạng nối lại, tất cả các nút đều **hội tụ về cùng epoch và cùng tree hash** — hoàn toàn không cần Delivery Service.

// Giữ nhịp đều — đây là slide số liệu, nói rõ con số.

---

## Slide 17 — 3.5. RQ3: Benchmark mật mã (12:10 – 12:35)

> **RQ3** — Chi phí mật mã MLS so với mã hóa từng cặp là bao nhiêu? Thiết lập: tạo nhóm với số thành viên tăng dần, mã hóa một tin nhắn 1 KB gửi đến toàn bộ nhóm, lặp 100 lần và lấy thời gian trung vị.
>
> Kết quả với nhóm **4096 thành viên**: MLS chỉ tốn **78,95 ms**, trong khi mã hóa từng cặp tốn **955,18 ms** — nhanh hơn **12 lần**. Độ trễ đuôi p99 cũng tương tự: MLS **110,85 ms** so với từng cặp **1081,72 ms**. MLS giữ chi phí mã hóa ổn định khi nhóm lớn, trong khi từng cặp tăng tuyến tính theo số thành viên.

// Nhấn mạnh "12 lần" — con số dễ nhớ nhất.

---

## Slide 18 — 3.6. RQ4: Khả năng mở rộng (12:35 – 12:55)

> **RQ4** — Khả năng mở rộng của lớp điều phối khi nhóm lớn? Cách đo: tạo nhóm N nút, một nút đề xuất Add/Remove/Update, đo tổng thời gian để toàn bộ N nút cùng tiến lên epoch tiếp theo. Kết quả trong bảng cho thấy thời gian tăng khi N lớn, nhưng đây chủ yếu là chi phí mật mã MLS, không phải lớp điều phối. Ví dụ ở 1000 nút, Add mất khoảng 2237 ms tổng — chia ra chỉ khoảng **2,2 ms trên mỗi nút**.

// Nói rõ con số per-node để hội đồng thấy không quá nặng.

---

## Slide 19 — 3.7. RQ5: Phục hồi phân nhánh + Overhead + FS (12:55 – 13:35)

> **RQ5a** — Fork healing có thực sự O(1)? Biểu đồ chuẩn hóa theo KB group state, kết hợp với bảng thời gian thực tế: với D=5, heal mất **728 ms** cho state **449 KB**; với D=50, heal mất **2098 ms** cho state **3240 KB**. Tại sao tổng ms tăng? Vì nhánh sâu hơn thì group state lớn hơn, phải serialize nhiều dữ liệu hơn. Nhưng nếu tính **ms trên mỗi KB**, con số giảm từ **1,62 xuống 0,65** — chứng tỏ chi phí giao thức không phụ thuộc vào độ sâu D. Nói cách khác, dù nhánh sâu bao nhiêu thì quy trình heal vẫn chỉ cần **1 External Commit**.
>
> **RQ5b** — Coordinator overhead: tỷ trọng chi phí điều phối chỉ chiếm **dưới 4%** tổng chi phí cục bộ — phần lớn thời gian vẫn nằm ở bản thân thao tác mật mã MLS.
>
> **RQ5c** — Forward Secrecy: loại 1 thành viên, gửi tin ở epoch mới, rồi kiểm tra giải mã. Kết quả: thành viên bị loại **không giải mã được**.

// Nhấn mạnh bảng RQ5a — đó là dữ liệu mới giúp bạn nói rõ O(1).

---

## Slide 20 — 4.1. Kết luận & Hướng phát triển (13:35 – 14:30)

> Tóm lại, đồ án đã đề xuất một giao thức điều phối phi tập trung gồm 4 cơ chế phối hợp, hoàn toàn không sửa lõi MLS; trong đó Fork Healing đưa K nút hội tụ chỉ bằng đúng 1 Commit; lựa chọn Eventual Consistency phù hợp bài toán nhắn tin — chấp nhận phân vùng, ưu tiên sẵn sàng, không đóng băng khi mất quorum; và một ứng dụng desktop hoàn chỉnh bằng Wails + libp2p + OpenMLS thật, chạy thực tế với 5 nút.
>
> Về giới hạn, em nhìn nhận thẳng thắn: phần lý thuyết mới dừng ở lập luận theo bất biến, chưa có kiểm chứng hình thức đầy đủ; thực nghiệm chủ yếu chạy trên mạng giả lập; và hiện tại group state vẫn được truyền toàn bộ qua gRPC giữa Go và Rust, tạo thêm chi phí khi nhóm lớn.
>
> Hướng phát triển tiếp theo: chứng minh formal giao thức điều phối, mở rộng thực nghiệm trên nhiều máy vật lý thực tế, và tối ưu giao tiếp Go–Rust bằng delta state thay vì truyền toàn bộ.
>
> Em xin cảm ơn quý thầy cô đã lắng nghe, và em rất mong nhận được câu hỏi cũng như góp ý từ hội đồng.

// Dừng, mỉm cười, chờ câu hỏi. Đây là lúc quan trọng để thể hiện sự tự tin và làm chủ đồ án.

---

## Ghi chú tổng thể khi luyện tập

- **Tốc độ nói mục tiêu**: khoảng 150 từ/phút cho phần lý thuyết (slide 7-12), có thể nhanh hơn ở phần giới thiệu/kết luận.
- **3 slide cần luyện kỹ nhất vì dễ vấp**: Slide 11 (Fork Healing, 5 bước + O(K²) vs O(1) + crypto-shredding), Slide 10 (công thức History Hash), Slide 12 (thuật toán HLC_Now).
- **Nếu bị hỏi giờ chạy chậm**: có thể cắt bớt phần "lưu ý phụ" (in nghiêng `//`) — không đọc — để rút ngắn, giữ nguyên phần in đậm là nội dung cốt lõi.
- **Câu hỏi khả năng cao sẽ được hỏi** (chuẩn bị sẵn câu trả lời):
  1. *"Tại sao không dùng Raft/Paxos luôn cho đơn giản?"* → Trả lời bằng lập luận CAP ở slide 8: Raft là CP, sẽ đóng băng khi phân vùng — không phù hợp P2P/local-first. Bảng so sánh chi tiết ở slide 6.
  2. *"Nếu 2 Token Holder ở 2 phân vùng cùng lúc thì sao?"* → Đó chính là kịch bản dẫn tới Fork Detection + Healing ở slide 10-11; Single-Writer chỉ giảm xung đột **khi cùng ActiveView**, không loại bỏ hoàn toàn fork.
  3. *"HLC có đảm bảo thứ tự tuyệt đối không?"* → Không, chỉ đảm bảo happens-before + total order cục bộ nhất quán mỗi nút, không phải đồng bộ đồng hồ tuyệt đối toàn cục.
  4. *"Sau Fork Healing, tin nhắn cũ trong lúc phân vùng có bị mất không?"* → Nhắc lại phần Autonomous Replay ở slide 11: mỗi nút chỉ phát lại tin của chính mình; nếu tác giả offline vĩnh viễn trước khi mạng hồi phục, tin đó tạm thời không phục hồi được — đây là giới hạn đã ghi nhận.
