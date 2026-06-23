# Kế hoạch Nâng cấp Kiến trúc: Bidirectional Autonomous Replay & Message Batching

> **Reviewer:** Senior System Architect
> **Đánh giá chung:** Ý tưởng dùng "Nhánh thắng tự phát lại tin nhắn" kết hợp "Gom cục tin nhắn (Batching)" để xử lý khoảng trống Backward Secrecy mà không nghẽn mạng là một thiết kế **xuất sắc**. Tuy nhiên, để đưa vào đồ án và mã nguồn một cách hàn lâm và "sạch" nhất, chúng ta cần tái cấu trúc (refactor) lại quy trình một cách khắt khe. 

Dưới đây là bản thiết kế hệ thống chi tiết để bạn tự tay thực hiện.

---

## PHẦN A: KẾ HOẠCH CẬP NHẬT QUYỂN ĐỒ ÁN (LATEX)

Để hội đồng thấy rõ chiều sâu thiết kế, bạn cần rải rác ý tưởng này xuyên suốt từ Chương 3 đến Chương 6, tạo thành một mạch logic không thể phản bác.

### 1. Chương 3: Đề xuất (Cập nhật Thiết kế Giao thức)
- **Mở rộng định nghĩa "Autonomous Replay":** Từ chỗ chỉ là "Nhánh thua tự khôi phục tin của mình", nâng cấp thành cơ chế **Phát lại Tự trị Hai chiều (Bidirectional Autonomous Replay)**.
  - Cụ thể: Khi nhận diện một mốc `HealEpoch` (thời điểm hợp nhất nhánh thành công), *cả nút nhánh thua lẫn nút nhánh thắng* đều quét lại Local Outbox. 
- **Đưa khái niệm "Message Batching" vào kiến trúc:** 
  - Thay vì thiết kế cũ là 1 Envelope = 1 Application Message, vẽ lại sơ đồ cấu trúc tin nhắn hoặc bổ sung mục: *"Tối ưu hóa Băng thông bằng Message Batching"*. Nêu rõ: Các tin nhắn lẻ tẻ sẽ được bọc vào một `BatchedEnvelope` trước khi đẩy lên GossipSub, giúp giới hạn số lượng Packet tối đa bằng với số lượng thiết bị đang online ($O(N)$ thay vì $O(M)$).

### 2. Chương 4: Phân tích Lý thuyết (Chứng minh tính đúng đắn)
- **Bảo mật (Security):** Viết 1 đoạn khẳng định cơ chế này **KHÔNG vi phạm E2EE và Backward Secrecy**. Lý do: Việc re-encrypt được thực hiện bởi chính *tác giả* (người giữ Private Key gốc) trên bộ khóa của Epoch mới. Đây là một thao tác hợp lệ của MLS.
- **Tính nhân quả (Causality):** Nhắc lại vai trò của **HLC (Hybrid Logical Clock)**. Mặc dù hàng nghìn tin nhắn bị "nhồi" vào 1 cục và gửi đến lộn xộn sau khi hết đứt mạng, lớp UI vẫn sắp xếp lại chuẩn xác nhờ mốc thời gian logic HLC đính kèm trong từng tin.

### 3. Chương 6: Kết luận và Giới hạn (Chặn trước câu hỏi của Hội đồng)
- Thêm ngay một phần **"Giới hạn của Cơ chế Phát lại Hai chiều"**:
  > *"Nhược điểm của phương pháp này là bài toán 'Người vắng mặt' (Offline Author). Nếu một thành viên gửi tin nhắn trong lúc mạng phân mảnh, nhưng lại ngắt kết nối (offline) trước khoảnh khắc Fork Heal, hệ thống sẽ vĩnh viễn mất đi tin nhắn đó cho đến khi tác giả online trở lại, do không ai khác có thẩm quyền (Private Key) để mã hóa lại bản rõ thay họ."*

---

## PHẦN B: KẾ HOẠCH TÁI CẤU TRÚC MÃ NGUỒN (CODE REFACTORING)

Dưới góc nhìn của một Kỹ sư, code hiện tại ở `coordinator.go` (quanh dòng 3145) đang vi phạm nguyên tắc Tối ưu hóa mạng (N+1 query/network problem). Dưới đây là kế hoạch sửa code:

### Bước 1: Định nghĩa lại cấu trúc dữ liệu (Data Structures)
- Trong file định nghĩa type (`types.go` hoặc `proto`), bạn cần thêm một định dạng thông điệp mới dành riêng cho Batching:
```go
type BatchedApplicationEvent struct {
    Events []ApplicationEvent `json:"events"`
}
```
- Khai báo thêm hằng số loại thông điệp `MsgApplicationBatched` để phân biệt với tin nhắn đơn lẻ thông thường.

### Bước 2: Sửa luồng Gom tin nhắn (Batching Logic) trong `coordinator.go`
- Tìm đoạn vòng lặp `for _, outbound := range outboundList` hiện tại.
- **Xóa bỏ** việc gọi `broadcastOutboundReplay` lẻ tẻ.
- **Thay bằng:**
  1. Quét toàn bộ `outboundList`.
  2. Map chúng sang mảng `[]ApplicationEvent`.
  3. Lọc bỏ các tin rác hoặc trùng lặp (nếu cần).
  4. Ném cả mảng đó vào `BatchedApplicationEvent`.
  5. Gọi một hàm mới: `c.broadcastBatchedOutboundReplay(batchedEvents)`. Hàm này sẽ chuyển mảng thành JSON/Protobuf, gọi Rust sidecar để **mã hóa 1 lần** (hoặc mã hóa từng tin nhưng nén chung vào 1 Envelope) và bắn lên GossipSub.

### Bước 3: Sửa luồng Kích hoạt ở Nhánh Thắng (Trigger Logic)
- Code hiện tại chỉ cho nút ở nhánh thua chạy Replay khi nó hoàn tất `EXTERNAL_JOINED`.
- **Cần thêm:** Ở hàm `processCommit` (nơi Token Holder và các nút nhánh thắng nhận bản tin External Join từ nhánh thua), bạn phải chèn logic:
  > *Nếu nhận diện đây là một Commit sinh ra từ ExternalJoin (có cờ báo hiệu Fork Heal), tự động kích hoạt tiến trình `BatchReplay` cục bộ để xả tin nhắn lịch sử của mình cho thành viên mới.*

### Bước 4: Sửa luồng Nhận và Xử lý (Receiver & Deduplication)
- Cập nhật hàm `handleApplicationMessage` (hoặc tạo hàm mới `handleBatchedMessage`) để bóc tách mảng `[]ApplicationEvent`.
- Duyệt qua mảng đó, và điều quan trọng nhất: **Insert vào SQLite với cơ chế chống trùng lặp (Idempotent / UPSERT)** dựa vào `EventID`. Nếu đã tồn tại thì bỏ qua, nếu mới thì lưu và đẩy lên UI (UI sẽ tự động sort theo HLC).

---

**Kết luận từ Senior:** Đây là một bản thiết kế "sạch", bao quát toàn bộ từ học thuyết đến thực tiễn. Bạn hãy lưu lại bản kế hoạch này. Bao giờ sẵn sàng "vọc" code hoặc sửa LaTeX, cứ lấy file này ra làm kim chỉ nam! Mọi thứ đều đã được tính toán kỹ lưỡng để không phá vỡ kiến trúc cũ của bạn.
