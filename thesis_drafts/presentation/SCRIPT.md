# Kịch bản thuyết trình chi tiết — Đồ án tốt nghiệp
**Tổng thời lượng mục tiêu: ~14 phút nói + 1 phút dự phòng (giới hạn 15 phút)**

Ghi chú cách dùng: mỗi slide có (1) thời lượng gợi ý, (2) lời nói đầy đủ để học thuộc, (3) lưu ý về giọng điệu/nhịp độ. Chỗ có `//` là ghi chú riêng cho bạn, không đọc ra.

---

## Slide 1 — Trang bìa (0:00 – 0:30)

> Kính thưa quý thầy cô trong hội đồng, em tên là Lê Tiến Dũng. Em xin trình bày đồ án tốt nghiệp với đề tài: **"Nghiên cứu và ứng dụng giao thức điều phối phi tập trung cho MLS trên mạng ngang hàng"**, dưới sự hướng dẫn của thầy Đỗ Bá Lâm.

// Nói chậm, rõ, nhìn hội đồng. Đây là câu duy nhất cần thuộc nguyên văn vì mở đầu tạo ấn tượng.

---

## Slide 2 — Nền tảng MLS (0:30 – 1:15)

> Trước khi vào bài toán, em xin giới thiệu nhanh nền tảng công nghệ mà đồ án sử dụng: giao thức **MLS — Messaging Layer Security**, được chuẩn hóa trong RFC 9420, hiện đang được dùng trong các hệ thống lớn như Signal, WhatsApp và Cisco Webex.
>
> MLS quản lý một nhóm chat bằng khái niệm **Epoch** — mỗi phiên bản trạng thái khóa nhóm là một epoch. Ba loại thông điệp chính: **Proposal** — đề nghị thay đổi thành viên; **Commit** — gom các Proposal lại và thực sự tạo ra epoch mới; và **Welcome** — thông tin cho thành viên mới gia nhập.
>
> Điểm quan trọng: Commit **không giao hoán** — tức là thứ tự xử lý Commit sẽ quyết định kết quả cuối cùng. Đây chính là nguồn gốc của bài toán mà đồ án giải quyết.
>
> Về mặt bảo mật, MLS dùng cấu trúc cây nhị phân **TreeKEM** giúp cập nhật khóa với độ phức tạp O(log N) thay vì tuyến tính, đồng thời đảm bảo hai thuộc tính quan trọng: **Forward Secrecy** — thành viên bị loại không đọc được tin nhắn tương lai, và **Post-Compromise Security** — hệ thống tự phục hồi sau khi khóa bị lộ.

// Nhấn mạnh cụm "không giao hoán" — đây là câu bản lề dẫn sang slide 3.

---

## Slide 3 — Vấn đề (1:15 – 2:15)

> Vấn đề nằm ở đây: **MLS mặc định giả định luôn có một Delivery Service trung tâm** đứng ra tuần tự hóa mọi Commit.
>
> Ở mô hình truyền thống, khi Alice tạo Commit, Delivery Service nhận, sắp thứ tự, rồi phát cho Bob, Carol, Dave theo đúng một trình tự duy nhất — E1, E2, E3. Tất cả các nút luôn đồng bộ.
>
> Nhưng trên mạng ngang hàng, **không tồn tại thực thể trung tâm đó**. Giả sử Alice và Carol cùng lúc tạo Commit ở cùng epoch E, phát qua GossipSub. Bob nhận được Commit của Alice trước, còn Dave nhận được Commit của Carol trước. Hai nút này cùng chuyển sang epoch E+1 — nhưng cây khóa của họ, tree hash của họ, hoàn toàn khác nhau. Kết quả là họ **không thể giải mã thông điệp của nhau nữa** — đây gọi là hiện tượng **phân nhánh trạng thái mật mã**.
>
> Đây chính là bài toán trung tâm mà đồ án của em giải quyết: **làm sao vận hành MLS trên P2P mà không cần Delivery Service, nhưng vẫn tránh và phục hồi được khỏi phân nhánh.**

// Đây là slide quan trọng nhất để hội đồng hiểu vấn đề — nói chậm, có thể dừng 1 giây sau từ "FORK!" để nhấn.

---

## Slide 4 — Khoảng trống nghiên cứu & Mục tiêu (2:15 – 3:00)

> Các hướng tiếp cận hiện có đều có hạn chế. **Raft hay BFT** — các thuật toán đồng thuận truyền thống — đòi hỏi truyền thông bậc O(N bình phương) và **đóng băng hoàn toàn** khi mất quorum, không phù hợp mạng P2P hay mất kết nối. **Blockchain** thì độ trễ tạo khối quá cao cho một ứng dụng chat thời gian thực. Còn **DMLS** — một hướng Internet-Draft gần đây mở rộng trực tiếp MLS — thì làm tăng độ phức tạp triển khai và phải đánh đổi Forward Secrecy.
>
> Từ đó, em xác định **4 mục tiêu** cho đồ án: giảm concurrent commits khi mạng còn liên thông; buộc mỗi nút chỉ xử lý trên epoch tương thích; phát hiện và hàn gắn phân nhánh sau khi phân vùng mạng xảy ra; và giữ được thứ tự hiển thị ổn định cho tin nhắn.

---

## Slide 5 — Đóng góp chính (3:00 – 3:45)

> Đồ án có 3 đóng góp chính, tương ứng với 3 phần em sẽ trình bày tiếp theo.
>
> **Một**, thiết kế một giao thức điều phối phi tập trung gồm 4 cơ chế — Single-Writer, Kiểm tra epoch, Fork Detection và Healing, và HLC — mà **hoàn toàn không sửa đổi lõi mật mã của MLS**.
>
> **Hai**, phân tích lý thuyết với 5 bất biến được chứng minh bảo toàn.
>
> **Ba**, triển khai một ứng dụng desktop hoàn chỉnh bằng Go và Rust thông qua Wails, theo kiến trúc stateless sidecar và hexagonal, với 4 luồng sử dụng thực tế cùng đánh giá thực nghiệm đầy đủ.

// Nói nhanh, dứt khoát — đây là slide "outline", không cần chi tiết, giữ nhịp.

---

## Slide 6 — Cơ chế 1: Single-Writer (3:45 – 5:00)

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

## Slide 7 — Cơ chế 2: Kiểm tra epoch (5:00 – 5:50)

> Cơ chế thứ hai bảo vệ tính đúng đắn ở tầng cục bộ: **kiểm tra epoch trước khi đưa thông điệp xuống lớp mật mã**.
>
> Khi một bản tin đến, nút so sánh epoch của bản tin với epoch cục bộ. Nếu **nhỏ hơn** — bản tin thuộc quá khứ — **từ chối ngay**, không cho phép kéo trạng thái lùi lại. Nếu **bằng** — xử lý bình thường. Nếu **lớn hơn** — bản tin đến từ tương lai — hệ thống **đệm lại** và kiểm tra xem đó chỉ là do mình đang chậm đồng bộ, hay đã thực sự phân nhánh.
>
> Bất biến giữ được ở đây là: epoch cục bộ của một nút là đại lượng **đơn điệu không giảm** — E tại thời điểm t+1 luôn lớn hơn hoặc bằng E tại thời điểm t.

---

## Slide 8 — Cơ chế 3: Fork Detection (5:50 – 6:50)

> Vấn đề đặt ra: khi thấy một bản tin từ tương lai, làm sao phân biệt được đó là **do mình chậm** hay **đã thực sự phân nhánh**? Đồ án giải quyết bằng **History Hash**.
>
> Mỗi nút duy trì một giá trị R(E), tính bằng cách băm lũy tiến: R(e) bằng hàm băm của R(e trừ 1) ghép với hash của Commit tại epoch e. Giá trị này tóm tắt **toàn bộ chuỗi Commit** từ epoch 1 cho tới hiện tại — không chỉ Commit gần nhất.
>
> Khi đối chiếu hai nút A và B: nếu R của B tại epoch của A **trùng khớp** với R của A, thì A chỉ đang đi chậm trên cùng một nhánh — chỉ cần bắt kịp. Nếu **khác nhau**, thì đã có phân nhánh thật sự xảy ra, và hệ thống chuyển sang quy trình Fork Healing.
>
> Ở đây em cũng phát biểu một **định nghĩa hình thức về hội tụ**: một nhóm được xem là hội tụ tại epoch E khi và chỉ khi mọi nút cùng thỏa **3 điều kiện đồng thời** — cùng epoch, cùng tree hash, **và** cùng history hash. Chỉ cùng epoch thôi là chưa đủ, vì hai nút vẫn có thể đang ở hai nhánh khác nhau dù cùng nhãn epoch.

// Nhấn mạnh phần "định nghĩa hình thức" — đây là chi tiết thể hiện tính học thuật/rigor.

---

## Slide 9 — Cơ chế 3 (tiếp): Fork Healing (6:50 – 8:15)

> Đây là phần đóng góp em cho là quan trọng nhất về mặt kỹ thuật.
>
> Quy trình gồm 5 bước. **Bước 1**: chọn nhánh thắng bằng hàm trọng số W(B) — ưu tiên theo thứ tự: số thành viên đang hoạt động nhiều hơn, rồi tới epoch cao hơn, cuối cùng là hash Commit gần nhất để phá vỡ hòa. **Bước 2**: nút ở nhánh thua hủy toàn bộ trạng thái MLS cũ và tạo KeyPackage mới. **Bước 3**: gửi một **External Proposal** dạng Add qua GossipSub. **Bước 4**: Token Holder của nhánh thắng gom **tất cả** các External Proposal đó — dù có bao nhiêu nút cần gia nhập lại — vào **đúng một Commit duy nhất**, kèm Remove cho lá cũ nếu cần, rồi sinh Welcome. **Bước 5**: nút thua xử lý Welcome, khôi phục trạng thái, và tự phát lại các tin nhắn của chính mình — gọi là Autonomous Replay.
>
> Tại sao dùng External Proposal quan trọng đến vậy? Vì nếu để **mỗi nút tự tạo Commit riêng**, K nút cần heal sẽ tạo ra K Commit cạnh tranh nhau, phần lớn bị từ chối và phải thử lại — chi phí tăng lên **O(K bình phương)**. Trong khi cách tiếp cận của đồ án, nhờ Token Holder gom lại, chi phí chỉ là **O(1)** — đúng một Commit bất kể K lớn cỡ nào.
>
> Một giới hạn quan trọng em cũng đã lường trước: sau khi heal, nút gia nhập lại nhánh thắng **không có khóa bí mật của các epoch cũ** trong thời gian phân vùng — đây là vấn đề Backward Secrecy. Đồ án xử lý bằng nguyên tắc: mỗi nút **chỉ được tự mã hóa lại tin nhắn của chính mình**, không ai được đại diện mã hóa lại cho người khác — giữ đúng ranh giới thẩm quyền mà MLS quy định.

// Slide dài nhất, quan trọng nhất — luyện nói mạch lạc, không vấp. Có thể chia hơi ở "Tại sao dùng External Proposal..." để lấy hơi.

---

## Slide 10 — Cơ chế 4: HLC (8:15 – 9:15)

> Cơ chế cuối cùng giải quyết một vấn đề khác: **thứ tự hiển thị**.
>
> MLS có hai loại thông điệp: **Control message** — Proposal, Commit, làm thay đổi khóa nhóm; và **Application message** — tin nhắn chat thực sự. Khi có Delivery Service, DS gắn timestamp quyết định thứ tự hiển thị. Nhưng trên P2P, không có DS, và đồng hồ vật lý giữa các máy **không đồng bộ** — không thể tin cậy physical clock thuần túy.
>
> Giải pháp là **Hybrid Logical Clock** — HLC, biểu diễn bằng bộ ba (L, C, NodeID): L là thời gian logic bám theo thời gian vật lý, C là bộ đếm phân biệt các sự kiện cùng L, NodeID phá vỡ hòa. Thuật toán HLC_Now lấy giá trị lớn hơn giữa L cục bộ và thời gian vật lý; nếu bằng L cũ thì tăng counter, nếu không thì reset counter về 0.
>
> Điểm mấu chốt: HLC **chỉ áp dụng cho application message**, hoàn toàn **không can thiệp vào Control message** — Commit vẫn dùng epoch ordering như bình thường. Hai loại thứ tự này — thứ tự mật mã và thứ tự hiển thị — được **tách biệt hoàn toàn**, để tránh nhầm lẫn giữa đồng hồ ứng dụng với cơ chế điều phối trạng thái mật mã.

---

## Slide 11 — 5 bất biến lý thuyết (9:15 – 10:00)

> Tổng kết phần lý thuyết, đồ án phân tích và chứng minh **5 bất biến** được bảo toàn: epoch cục bộ chỉ tăng hoặc giữ nguyên; chỉ có một Token Holder khi các nút cùng ActiveView; phân nhánh chỉ được kết luận sau khi đối chiếu lịch sử qua History Hash; không bao giờ gộp trực tiếp hai trạng thái đã tách nhánh mà luôn đi qua con đường hợp lệ của MLS; và HLC không bao giờ được phép quyết định Commit.
>
> Em xin nói rõ phạm vi: các phân tích này được xây dựng trong giả định các nút tuân thủ giao thức và OpenMLS xử lý mật mã đúng — đây là lập luận theo bất biến, **chưa phải kiểm chứng hình thức đầy đủ**, và đó cũng là một hướng phát triển em sẽ đề cập ở cuối bài.

// Nói với giọng thẳng thắn, tự tin — chủ động nêu giới hạn cho thấy trung thực khoa học, ghi điểm với hội đồng.

---

## Slide 12 — Kiến trúc phần mềm (10:00 – 11:00)

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

## Slide 13 — Ứng dụng desktop (11:00 – 11:50)

> Ứng dụng thử nghiệm triển khai trọn vẹn 4 luồng: **Onboarding** — thiết bị tự sinh khóa, quản trị viên duyệt và cấp Bundle để vào mạng, hoàn toàn không cần máy chủ trung tâm. **Group Chat** — tin nhắn mã hóa MLS hiển thị đồng bộ trên nhiều thiết bị. **Quản trị nhóm** — mỗi thao tác thêm/xóa thành viên đều kích hoạt đúng luồng Proposal và Commit đã trình bày. Và **Chia sẻ tệp mã hóa** — dùng MLS Exporter để dẫn xuất khóa AES-256-GCM, truyền qua libp2p stream, khóa **không bao giờ đi qua mạng**.
>
> Đây là minh chứng cho thấy các cơ chế lý thuyết không chỉ dừng ở mô tả, mà đã đi vào một hệ thống chạy được thực tế, đầu cuối.

---

## Slide 14 — Thực nghiệm 1 (11:50 – 12:45)

> Chuyển sang phần đánh giá thực nghiệm, trả lời hai câu hỏi nghiên cứu.
>
> **RQ1** — Single-Writer có thực sự giảm xung đột? Kết quả: nếu xử lý ngay không qua Single-Writer, tỷ lệ Commit thành công giảm từ 1.00 xuống còn 0.33 khi có 5 nút cùng thao tác. Nhưng với cơ chế gom theo lô của Single-Writer, tỷ lệ thành công giữ nguyên **1.00 ở mọi mức**, và số Commit luôn đúng bằng 1.
>
> **RQ2** — Hệ thống có hội tụ được sau phân vùng mạng không? Em chạy chaos test với 5 nút, phân vùng lặp lại mỗi 600 mili giây. Kết quả: khi mạng nối lại, tất cả các nút đều **hội tụ về cùng epoch và cùng tree hash** — mà hoàn toàn không cần Delivery Service.

---

## Slide 15 — Thực nghiệm 2 (12:45 – 13:40)

> **RQ3** — Chi phí mật mã và điều phối là bao nhiêu? Benchmark với nhóm 4096 thành viên: MLS chỉ tốn **78.95 mili giây** để cập nhật khóa, so với **955.18 mili giây** nếu mã hóa từng cặp — chênh lệch hơn **12 lần**. Về phần điều phối do đồ án thêm vào, tỷ trọng chi phí chỉ chiếm **dưới 4%** tổng chi phí cục bộ — phần lớn thời gian vẫn nằm ở bản thân thao tác mật mã MLS.
>
> **RQ4** — Lớp điều phối có phá vỡ Forward Secrecy của MLS không? Em kiểm thử với **MLS thật**: Alice tạo nhóm, Bob tham gia, Alice loại Bob, rồi gửi tin ở epoch mới — kết quả Bob **hoàn toàn không giải mã được**. Điều này xác nhận lớp điều phối không hề phá vỡ các bảo đảm bảo mật gốc của MLS.

---

## Slide 16 — Kết luận & Hướng phát triển (13:40 – 14:30)

> Tóm lại, đồ án đã đề xuất một giao thức điều phối phi tập trung gồm 4 cơ chế phối hợp, hoàn toàn không sửa lõi MLS; trong đó Fork Healing đưa K nút hội tụ chỉ bằng đúng 1 Commit; kèm phân tích lý thuyết 5 bất biến; và một ứng dụng desktop hoàn chỉnh với kiến trúc stateless sidecar, hexagonal, cùng 4 use case thực tế.
>
> Về giới hạn, em nhìn nhận thẳng thắn: phần lý thuyết mới dừng ở lập luận theo bất biến, chưa có kiểm chứng hình thức đầy đủ; thực nghiệm chủ yếu chạy trên mạng giả lập; và hiện tại group state vẫn được truyền toàn bộ qua gRPC giữa Go và Rust, tạo thêm chi phí khi nhóm lớn.
>
> Hướng phát triển tiếp theo: kiểm chứng hình thức các bất biến, mở rộng thực nghiệm trên nhiều máy thật, và tối ưu hóa truyền tải bằng snapshot hoặc delta giữa hai tầng.
>
> Em xin cảm ơn quý thầy cô đã lắng nghe, và em rất mong nhận được câu hỏi cũng như góp ý từ hội đồng.

// Dừng, mỉm cười, chờ câu hỏi. Đây là lúc quan trọng để thể hiện sự tự tin và làm chủ đồ án.

---

## Ghi chú tổng thể khi luyện tập

- **Tốc độ nói mục tiêu**: khoảng 150 từ/phút cho phần lý thuyết (slide 6-11), có thể nhanh hơn ở phần giới thiệu/kết luận.
- **3 slide cần luyện kỹ nhất vì dễ vấp**: Slide 9 (Fork Healing, 5 bước + O(K²) vs O(1) + Backward Secrecy), Slide 8 (công thức History Hash), Slide 10 (thuật toán HLC_Now).
- **Nếu bị hỏi giờ chạy chậm**: có thể cắt bớt phần "lưu ý phụ" (in nghiêng `//`) — không đọc — để rút ngắn, giữ nguyên phần in đậm là nội dung cốt lõi.
- **Câu hỏi khả năng cao sẽ được hỏi** (chuẩn bị sẵn câu trả lời):
  1. *"Tại sao không dùng Raft/Paxos luôn cho đơn giản?"* → Trả lời bằng lập luận CAP ở slide 6: Raft là CP, sẽ đóng băng khi phân vùng — không phù hợp P2P/local-first.
  2. *"Nếu 2 Token Holder ở 2 phân vùng cùng lúc thì sao?"* → Đó chính là kịch bản dẫn tới Fork Detection + Healing ở slide 8-9; Single-Writer chỉ giảm xung đột **khi cùng ActiveView**, không loại bỏ hoàn toàn fork.
  3. *"HLC có đảm bảo thứ tự tuyệt đối không?"* → Không, chỉ đảm bảo happens-before + total order cục bộ nhất quán mỗi nút, không phải đồng bộ đồng hồ tuyệt đối toàn cục.
  4. *"Sau Fork Healing, tin nhắn cũ trong lúc phân vùng có bị mất không?"* → Nhắc lại phần Backward Secrecy ở slide 9: mỗi nút tự phát lại tin của chính mình; nếu tác giả offline vĩnh viễn trước khi mạng hồi phục, tin đó tạm thời không phục hồi được — đây là giới hạn đã ghi nhận.
