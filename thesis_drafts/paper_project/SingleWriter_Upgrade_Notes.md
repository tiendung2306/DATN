# Bổ sung cơ chế Single-Writer — Ghi chú sau nộp đồ án

> **Ghi chú:** Tài liệu này được viết **sau khi đã nộp đồ án**, dùng cho buổi bảo vệ. Nội dung ghi nhận lỗ hổng phát hiện muộn và cơ chế cải thiện đề xuất. Không thay đổi nội dung đồ án đã nộp.

---

## 1. Lỗ hổng phát hiện

### 1.1. Mô tả lỗ hổng

Cơ chế Single-Writer trong đồ án dùng **timeout cố định** (`TokenHolderTimeout = 4s`) để quyết định khi nào Token Holder bị coi là không phản hồi và cần chuyển quyền (failover). Cơ chế suspend hiện tại có một rủi ro nghiêm trọng:

**Rủi ro suspend nhầm Token Holder khi mạng vẫn liên thông.**

### 1.2. Nguyên nhân

GossipSub không đảm bảo delivery đồng đều. Một gói tin có thể bị trễ 2–3s do mesh healing, congestion, hoặc jitter mạng. Trong khoảng timeout cố định 4s:

1. Token Holder vẫn sống, vẫn gửi heartbeat đều, chỉ đang gom Proposal hoặc Commit bị trễ trên đường truyền.
2. Non-holder đếm timeout 4s, không thấy Commit → suspend holder → bầu holder mới.
3. Các nút khác có thể nhận heartbeat bình thường → không suspend → **ActiveView phân kỳ**.
4. Hai nhóm nút bầu ra hai Token Holder khác nhau → **fork ngay khi mạng vẫn liên thông**.

### 1.3. Hệ quả

- Fork xảy ra không chỉ do network partition (đã có Fork Healing xử lý) mà còn do **jitter trong điều kiện mạng bình thường**.
- Fork Healing phải can thiệp thường xuyên hơn, tăng chi phí phục hồi.
- Timeout cố định không thích ứng: quá dài cho LAN (RTT < 1ms), có thể quá ngắn cho WAN (RTT 100–200ms).

---

## 2. Cơ chế cải thiện đề xuất

Đề xuất hai cơ chế bổ sung cho nhau:

| Cơ chế | Trả lời câu hỏi | Mục đích |
|---|---|---|
| Adaptive Epoch Lease | "Timeout nên bao nhiêu?" | Timeout thích ứng theo đặc tính mạng |
| φ-Accrual Failure Detector | "Khi nào mới suspend?" | Phân biệt holder chậm vs holder chết |

---

## 3. Adaptive Epoch Lease

### 3.1. Ý tưởng

Thay vì timeout cố định, Token Holder nhận một **lease** — khoảng thời gian có giới hạn mà trong đó holder có quyền phát hành Commit. Lease được tính từ các tham số mạng quan sát được cục bộ.

### 3.2. Công thức

$$L(E) = W_{batch} + \widehat{C}_{commit} + 2 \times \widehat{RTT}_{p95}$$

Trong đó:

- **$W_{batch}$**: cửa sổ gom lô Proposal (hiện tại cố định 1s, có thể adaptive)
- **$\widehat{C}_{commit}$**: EWMA thời gian thực hiện `CreateCommit` (đo cục bộ khi holder tạo Commit)
- **$\widehat{RTT}_{p95}$**: percentile 95 của độ trễ lan truyền GossipSub, ước lượng từ heartbeat echo

### 3.3. Ý nghĩa từng thành phần

Holder cần đủ thời gian để:

1. **Gom Proposal** ($W_{batch}$): đợi các Proposal đồng thời đến
2. **Tạo Commit bằng MLS** ($\widehat{C}_{commit}$): thao tác mật mã, đo được trực tiếp
3. **Commit lan truyền đến tất cả nút** ($2 \times \widehat{RTT}_{p95}$): GossipSub broadcast

### 3.4. Biên an toàn

Mỗi nút cộng thêm margin $\epsilon$ vào lease cục bộ:

$$L_{node}(E) = L(E) \times (1 + \epsilon), \quad \epsilon = 0.3$$

Đảm bảo nút có ước lượng nhanh nhất không hết hạn trước nút chậm nhất. Vì tất cả nút quan sát cùng mạng, sự chênh lệch ước lượng giữa các nút nhỏ, và margin 30% là đủ để hấp thụ.

### 3.5. Cơ sở lý thuyết

Mô hình lease-based mutual exclusion của Gray & Cheriton (1989): holder nhận lease có thời hạn, hết hạn → quyền chuyển giao. Khác với timeout cố định, lease được tính từ tham số mạng quan sát được, nên thích ứng tự nhiên.

### 3.6. Ước lượng RTT trong mạng P2P

Trong GossipSub P2P, không có request-response trực tiếp. RTT được ước lượng qua **heartbeat echo** — tận dụng heartbeat đã tồn tại, không thêm message:

1. Nút A broadcast heartbeat mang timestamp cục bộ `t1` (đồng hồ của A)
2. Nút B nhận heartbeat của A, ghi nhớ `t1`
3. Nút B broadcast heartbeat kế tiếp, nhúng `t1` vào (echo lại timestamp gần nhất nhận được từ A)
4. Nút A nhận heartbeat của B, thấy `t1` trong đó
5. Nút A tính: `RTT_to_B = t_now - t1`

```
A --heartbeat(t1)--> B
                      |
A <--heartbeat(t1)-- B

RTT_to_B = t_now - t1   (cùng đồng hồ của A, không cần sync)
```

**Không cần đồng bộ đồng hồ:** Cả `t1` lẫn `t_now` đều từ đồng hồ của A. Hiệu số chính xác bất kể A và B có chênh lệch thời gian bao nhiêu.

**Nhiều nút:** Mỗi nút tính RTT riêng cho từng peer, sau đó lấy p95 trên toàn ActiveView:

$$\widehat{RTT}_{p95} = \text{Percentile}_{95}\{RTT_{A \to B},\; RTT_{A \to C},\; RTT_{A \to D},\; \ldots\}$$

- **p95 thay vì max:** Max bị outlier kéo (một nút lag spike → max = 500ms → timeout phình không cần thiết). p95 bỏ qua 5% nút chậm nhất.
- **p95 thay vì trung bình:** Trung bình bị các nút nhanh kéo xuống. Commit phải đến tất cả nút, p95 đảm bảo 95% nút nhận Commit trong thời gian lease.

---

## 4. φ-Accrual Failure Detector

### 4.1. Ý tưởng

Thay vì suspend nhị phân ("timeout hết → suspend"), dùng **φ-accrual failure detector** (Hayashibara et al., 2004) — bộ phát hiện dựa trên xác suất, tính **mức độ nghi ngờ tích lũy** theo thời gian.

### 4.2. φ-score

$$\varphi(t) = -\log_{10}\left(1 - F_{HB}(t - t_{last})\right)$$

Trong đó:

- $t_{last}$: thời điểm nhận heartbeat cuối cùng từ holder
- $t - t_{last}$: khoảng thời gian không nhận được heartbeat
- $F_{HB}$: hàm phân phối tích lũy (CDF) của lịch sử khoảng cách heartbeat, duy trì cục bộ

### 4.3. Ý nghĩa

$\varphi = k$ nghĩa là xác suất holder còn sống mà không gửi heartbeat là $10^{-k}$:

| $\varphi$ | Xác suất holder còn sống | Diễn giải |
|---|---|---|
| 1 | 10% | Có thể chậm, chờ thêm |
| 4 | 0.01% | Gần như chắc chắn chết |
| 8 | $10^{-8}$ | Cực kỳ chắc chắn chết |

### 4.4. Logic failover hai pha

Suspend chỉ xảy ra khi **cả hai điều kiện** thỏa:

$$\text{Suspend}(H) \iff \underbrace{L_{node}(E) \text{ đã hết}}_{\text{lease expired}} \;\wedge\; \underbrace{\varphi > \varphi_{threshold}}_{\text{accrual confirmation}}$$

Với $\varphi_{threshold} = 8$.

**Diễn giải:**

- **Lease hết + $\varphi > 8$:** Holder gần như chắc chắn chết → **suspend ngay**
- **Lease hết + $\varphi \leq 1$:** Holder vẫn gửi heartbeat đều → còn sống nhưng chậm → **gia hạn lease một lần** (thêm 50% $L_{node}$)
- **Lease gia hạn hết + $\varphi > 1$:** Đã cho cơ hội thứ hai mà vẫn không commit → **suspend**
- **Lease hết + $1 < \varphi \leq 8$:** Không chắc chắn → **chờ thêm** cho đến khi $\varphi > 8$ hoặc lease gia hạn hết

### 4.5. Tại sao giải quyết được suspend phân kỳ

**Kịch bản jitter — holder còn sống, heartbeat bị trễ:**

| Nút | Lease hết? | $\varphi$ | Hành động |
|-----|-----------|-----------|-----------|
| N1 (nhận HB đều) | Có | 0.3 | Gia hạn — không suspend |
| N2 (HB trễ nhẹ) | Có | 1.5 | Chờ — không suspend |
| N3 (HB trễ nặng) | Có | 5.0 | Chờ — không suspend |

→ **Không nút nào suspend.** So với thiết kế hiện tại: N3 sẽ suspend sau 4s, tạo fork.

**Kịch bản holder chết thật:**

| Nút | Lease hết? | $\varphi$ | Hành động |
|-----|-----------|-----------|-----------|
| Tất cả | Có | →∞ | **Suspend đồng loạt** |

→ Tất cả nút suspend gần như đồng thời (vì $\varphi$ tăng cùng tốc độ khi không nhận heartbeat), bầu lại holder mới → **không fork**.

### 4.6. Cơ sở lý thuyết

φ-accrual detector được Hayashibara et al. (2004) đề xuất, đã được chứng minh trong thực tế (Apache Cassandra sử dụng variant này). Ưu điểm cốt lõi: thích ứng tự động với jitter mạng — trong mạng ổn định, $\varphi$ tăng nhanh → phát hiện chết nhanh; trong mạng không ổn định, $\varphi$ tăng chậm → ít false positive. Không cần tune thủ công.

### 4.7. Xác suất false positive

$$P(\text{false\_suspend}) = P(\text{lease\_expired} \mid \text{alive}) \times P(\varphi > 8 \mid \text{alive})$$

Với $\varphi_{threshold} = 8$: $P(\varphi > 8 \mid \text{alive}) < 10^{-8}$. Do đó $P(\text{false\_suspend}) < 10^{-8}$ — gần như loại bỏ hoàn toàn false positive.

---

## 5. Hai cơ chế bổ sung cho nhau

| | Adaptive Lease | φ-Accrual |
|---|---|---|
| **Giải quyết gì** | Timeout nên bao nhiêu? | Khi nào mới suspend? |
| **Cách làm** | Tính từ RTT + commit time thực tế | Tính xác suất holder còn sống |
| **Hiệu quả** | Timeout thích ứng mạng, không dùng magic number | Không suspend nhầm khi holder chỉ chậm |
| **Message thêm** | Không — tận dụng heartbeat echo | Không — tận dụng heartbeat history |

**Lease** trả lời "bao lâu", **φ-Accrual** trả lời "có chắc chưa". Hai cái cùng nhau: timeout adaptive + xác nhận bằng xác suất trước khi suspend.

---

## 6. Tham chiếu

- Hayashibara, N., Defago, X., Yared, R., & Katayama, T. (2004). The φ-accrual failure detector. *Proc. IEEE SRDS*.
- Gray, C., & Cheriton, D. (1989). Leases: An efficient fault-tolerant mechanism for distributed file cache consistency. *Proc. ACM SOSP*.
- Apache Cassandra — sử dụng φ-accrual variant trong thực tế production.
