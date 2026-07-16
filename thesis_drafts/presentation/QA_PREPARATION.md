# Bộ Q&A bảo vệ đồ án — 15 phút hỏi đáp

> Hội đồng ATTT, nghe 20 phút trình bày + 15 phút hỏi.
> Câu hỏi thực tế: hiểu thêm đồ án, vấn đề phổ biến, pitfalls thường gặp.
> Mỗi câu trả lời ~45–60 giây. Ngắn, dứt khoát, đỡ phải nói nhiều.

## Nhóm 1 — Câu hỏi hiểu thêm đồ án (rất dễ bị hỏi)

### Q1. Em có thể tóm tắt lại đóng góp chính của đồ án trong 1–2 câu?

Đồ án đề xuất một lớp điều phối phi tập trung bao quanh MLS — không sửa RFC 9420 — gồm 4 cơ chế: Single-Writer để giảm xung đột Commit, kiểm tra epoch để ngăn trạng thái lùi, Fork Detection & Healing để hội tụ sau phân vùng mạng, và HLC để ổn định thứ tự hiển thị tin nhắn. Kết quả cốt lõi: giữ nguyên Forward Secrecy và PCS của MLS trong môi trường P2P không có Delivery Service.

---

### Q2. Tại sao lại chọn MLS mà không dùng Signal Protocol (Double Ratchet)?

Double Ratchet mã hóa từng cặp — độ phức tạp O(N) cho mỗi lần cập nhật khóa. Với nhóm 4096 người, cần 4096 lần mã hóa. MLS dùng TreeKEM — cấu trúc cây nhị phân — chỉ cần log₂(N) = 12 lần. Thực nghiệm RQ3 cho thấy MLS nhanh hơn 12 lần ở nhóm 4096. Ngoài ra, MLS là chuẩn IETF (RFC 9420), đang được Signal, WhatsApp, Cisco Webex triển khai — nên chọn MLS là chọn hướng đi của ngành.

---

### Q3. Đồ án sửa đổi MLS hay chỉ bổ sung bên ngoài?

**Chỉ bổ sung bên ngoài, không sửa MLS.** Đây là điểm khác biệt cốt lõi so với DMLS (Internet-Draft gần nhất) — DMLS sửa bên trong MLS và phải đánh đổi Forward Secrecy. Đồ án giữ nguyên lõi mật mã, thêm lớp điều phối ở phía ngoài. Rust sidecar chỉ thực hiện các phép biến đổi mật mã chuẩn theo RFC 9420, hoàn toàn không biết về Single-Writer hay epoch check.

---

### Q4. Token Holder được bầu như thế nào? Có cần bỏ phiếu không?

Không cần bỏ phiếu. Công thức: `TH(E) = argmin H(group_id || E || peer_id)` — lấy SHA-256 của group ID ghép epoch ghép peer ID, ai có hash nhỏ nhất thì được chọn. Vì tất cả nút cùng đầu vào → cùng kết quả → không cần truyền thông thêm. Chi phí chỉ là O(N) tính toán cục bộ.

---

### Q5. Fork Healing hoạt động như thế nào, đơn giản nhất có thể nói?

5 bước: (1) Chọn nhánh thắng theo trọng số — ưu tiên nhánh có nhiều thành viên hoạt động hơn. (2) Nút nhánh thua hủy trạng thái MLS cũ, tạo khóa mới. (3) Gửi yêu cầu gia nhập qua External Proposal. (4) Token Holder nhánh thắng gom tất cả yêu cầu vào **một Commit duy nhất**. (5) Các nút phát lại tin nhắn của chính mình. Điểm quan trọng: dù K nút lệch nhánh, chỉ cần 1 Commit để hàn gắn toàn bộ.

---

## Nhóm 2 — Câu hỏi về vấn đề phổ biến / pitfalls

### Q6. Nếu mạng phân vùng, hai bên mỗi bên tự bầu Token Holder và tạo Commit — vậy Single-Writer có ý nghĩa gì?

Đúng, khi phân vùng, mỗi phân vùng có ActiveView khác nhau → bầu TH khác nhau → fork xảy ra. Nhưng Single-Writer **không nhằm loại bỏ hoàn toàn fork** — nó giảm fork khi mạng còn liên thông (RQ1: tỷ lệ Commit thành công 1.00 vs 0.33). Khi phân vùng đã xảy ra, Fork Detection + Healing đảm nhiệm việc hội tụ. Hai cơ chế bổ sung cho nhau: Single-Writer giảm xác suất, Healing xử lý hậu quả.

---

### Q7. Vậy sau khi hợp nhất, tin nhắn trong thời gian phân vùng có bị mất không?

Nút nhánh thua không có khóa giải mã tin của nhánh thắng trong thời gian phân vùng — đây là Forward Secrecy, có chủ đích. Để phục hồi, mỗi nút **tự phát lại tin do chính mình tạo** (Autonomous Replay) bằng khóa epoch mới. Nếu tác giả gốc vẫn offline khi mạng hồi phục, tin đó tạm thời không phục hồi được — đây là giới hạn đã ghi nhận. Trade-off: ưu tiên bảo mật (không chia sẻ khóa cũ) hơn lịch sử 100%.

---

### Q8. Em nói "không sửa MLS" — nhưng thêm External Proposal, thêm History Hash, thêm HLC... đó có phải sửa MLS không?

Không. Cần phân biệt rõ:
- **External Proposal** là cơ chế **có sẵn** trong RFC 9420 (Section 12) — đồ án chỉ sử dụng nó, không tạo mới.
- **History Hash** là giá trị nút tự tính bên ngoài MLS — không nằm trong thông điệp MLS, không ảnh hưởng key schedule.
- **HLC** gắn vào application message metadata — không thay đổi Commit, Proposal, hay Welcome.
- **Epoch check** là bước kiểm tra trước khi đưa tin xuống MLS — MLS vẫn xử lý bình thường sau khi check.

Tất cả đều ở lớp điều phối bên ngoài. Rust sidecar nhận state → tính MLS → trả kết quả, không biết gì về các cơ chế này.

---

### Q9. Đồ án chưa có chứng minh hình thức (formal proof) — làm sao đảm bảo giao thức đúng?

Đồ án công nhận thẳng thắn giới hạn này. Những gì đã làm: (1) phát biểu bất biến hình thức — epoch đơn điệu, Token Holder duy nhất, định nghĩa hội tụ 3 điều kiện. (2) lập luận theo bất biến dưới giả định vận hành. (3) thực nghiệm đối chiếu — RQ1–RQ5 kiểm chứng trên hệ thống chạy thực tế. Những gì chưa làm: TLA+/Coq. Đây là hướng phát triển ưu tiên đã ghi trong luận văn. Các kết luận cần hiểu trong phạm vi giả định đã nêu.

---

### Q10. Thực nghiệm chạy trên máy local, mạng giả lập — kết quả có đáng tin không?

Phần lớn số liệu đo **chi phí CPU**, không phụ thuộc mạng: RQ1 (tỷ lệ Commit), RQ3 (benchmark mã hóa), RQ5a (overhead điều phối) — đều đo thời gian tính toán. RQ2 (chaos test) và RQ4 (scalability) có thể thay đổi absolute latency trên WAN, nhưng trend — hội tụ sau phân vùng, O(log N) — vẫn đúng. Hướng phát triển: thực nghiệm multi-machine WAN thực.

---

## Nhóm 3 — Câu hỏi về bảo mật (hội đồng ATTT quan tâm)

### Q11. Forward Secrecy được giữ nguyên như thế nào khi Fork Healing cho nút nhánh thua gia nhập lại?

Fork Healing dùng External Proposal + Welcome — cơ chế chuẩn MLS để thêm thành viên mới. Welcome chứa **Group Secret hiện tại**, không chứa epoch secret của các epoch trước. Do đó nút mới gia nhập **không giải mã được tin quá khứ** — đây chính là Forward Secrecy. RQ5b đã kiểm chứng: thành viên bị loại không giải mã được tin ở epoch mới.

---

### Q12. Nếu kẻ tấn công ghi lại toàn bộ traffic mạng, sau đó thỏa hiệp một thành viên, kẻ tấn công đọc được gì?

- **Tin quá khứ:** Không đọc được — mỗi epoch có epoch secret mới, không suy ngược được (Forward Secrecy).
- **Tin hiện tại:** Đọc được nếu có đầy đủ trạng thái thiết bị.
- **Tin tương lai:** Không đọc được — khi thành viên bị thỏa hiệp update key hoặc bị remove, hệ thống tự phục hồi (Post-Compromise Security).

Ngoài ra, Fork Healing thêm crypto-shredding: nút nhánh thua hủy khóa cũ trước khi gia nhập nhánh thắng.

---

### Q13. Private key được lưu ở đâu? Có bao giờ gửi qua mạng không?

Private key sinh cục bộ trên thiết bị, lưu trong SQLite mã hóa. **Không bao giờ gửi qua mạng** — ngay cả khi mã hóa. Di chuyển sang thiết bị mới phải export ra file `.backup` mã hóa bằng passphrase, transfer thủ công (USB, QR). Chính sách Single Active Device: một tài khoản chỉ hợp lệ trên một thiết bị.

---

### Q14. Nếu Token Holder bị thỏa hiệp hoặc cố tình phá hoại thì sao?

Token Holder độc hại chỉ có thể: (1) từ chối tạo Commit → gây DoS tạm thời → failover sẽ loại nó và bầu TH mới. (2) chọn Proposal nào để gom → nhưng **không thể伪造 Proposal của người khác** vì mỗi Proposal có chữ ký MLS. (3) Token Holder **không có đặc quyền đọc tin nhắn** — nó vẫn là thành viên bình thường về mặt mật mã. Tóm lại: TH độc hại gây phiền toái, không gây vi phạm bảo mật.

---

### Q15. Việc chọn nhánh thắng theo số thành viên hoạt động — kẻ tấn công có thể thao túng không?

Cần nhiều nút hợp lệ để tăng số thành viên → cần onboarding qua InvitationToken ký bởi Root Admin Key. Không thể thao túng nếu chưa thỏa hiệp Root Admin Key. Ngoài ra, quy tắc so sánh theo thứ tự từ điển (số thành viên → epoch → hash Commit) là deterministic — tất cả nút quan sát cùng tập nhánh sẽ đi đến cùng quyết định.

---

## Nhóm 4 — Câu hỏi về ứng dụng thực tế

### Q16. Đồ án này áp dụng được vào thực tế ở đâu?

- **Mạng nội bộ doanh nghiệp** cần E2EE nhưng không muốn phụ thuộc server bên thứ ba.
- **Môi trường mất kết nối** — field operations, disaster recovery — cần liên lạc khi không có hạ tầng server.
- **Ứng dụng local-first** — người dùng sở hữu dữ liệu, không phụ thuộc đám mây, theo xu hướng Kleppmann 2019.

---

### Q17. MLS đã có Signal, WhatsApp dùng rồi — tại sao cần P2P?

Signal/WhatsApp vẫn **phụ thuộc máy chủ trung tâm** — điểm lỗi đơn, rò rỉ metadata (ai nói chuyện với ai, khi nào), phụ thuộc đám mây. Đồ án chứng minh: có thể vận hành MLS trên P2P mà vẫn giữ FS/PCS — mở ra lớp ứng dụng E2EE không có điểm lỗi đơn, người dùng tự chủ dữ liệu.

---

### Q18. Đồ án có giới hạn gì cần khắc phục?

Ba giới hạn chính đã ghi trong luận văn:
1. **Chưa có formal verification** — mới dừng ở lập luận theo bất biến, chưa có TLA+/Coq.
2. **Thực nghiệm trên mạng giả lập** — chưa chạy trên nhiều máy vật lý WAN.
3. **Group state truyền toàn bộ** qua gRPC giữa Go và Rust — khi nhóm lớn, state vài MB. Cần tối ưu delta state.

---

### Q19. Nếu được làm lại, em sẽ thay đổi gì?

1. Delta state thay vì full state — giảm overhead khi nhóm lớn.
2. Formal verification (TLA+) song song với implementation.
3. Multi-device support từ đầu — hiện tại Single Active Device là hạn hạn chế về UX.

---

### Q20. Đóng góp lớn nhất của đồ án là gì, nếu chỉ chọn một?

**Fork Healing bằng External Proposal + 1 Commit duy nhất.** Đây là cơ chế không có trong công trình trước — DMLS merge trực tiếp và phải đánh đổi Forward Secrecy. Đồ án giải quyết K nút lệch nhánh, bất kể độ sâu phân kỳ, chỉ cần 1 Commit, và giữ nguyên FS/PCS. Kết hợp crypto-shredding và Autonomous Replay đảm bảo cả bảo mật lẫn phục hồi dữ liệu.
