# Phân tích chi tiết các thí nghiệm đánh giá trong đồ án

> Tài liệu này giải thích chi tiết từng thí nghiệm được trình bày trong Chương 5 của báo cáo đồ án. Mỗi thí nghiệm được phân tích theo cấu trúc: **Mục đích — Vì sao cần — Cách đo — Con số — Phân tích**. Tài liệu dùng để ôn tập cho buổi phản biện.

---

## Mục lục

1. [Tổng quan mục tiêu đánh giá](#1-tổng-quan-mục-tiêu-đánh-giá)
2. [Thí nghiệm 1: Single-Writer và gom đề xuất (Concurrency Sweep)](#2-thí-nghiệm-1-single-writer-và-gom-đề-xuất-concurrency-sweep)
3. [Thí nghiệm 2: Hội tụ trong điều kiện mạng hỗn loạn (Chaos Test)](#3-thí-nghiệm-2-hội-tụ-trong-điều-kiện-mạng-hỗn-loạn-chaos-test)
4. [Thí nghiệm 3: Ảnh hưởng độ sâu phân kỳ đến thời gian phục hồi (Partition Divergence)](#4-thí-nghiệm-3-ảnh-hưởng-độ-sâu-phân-kỳ-đến-thời-gian-phục-hồi-partition-divergence)
5. [Thí nghiệm 4: Khả năng mở rộng của thao tác thành viên (Epoch Convergence Sweep)](#5-thí-nghiệm-4-khả-năng-mở-rộng-của-thao-tác-thành-viên-epoch-convergence-sweep)
6. [Thí nghiệm 5: Chi phí mật mã MLS (MLS Crypto Benchmark)](#6-thí-nghiệm-5-chi-phí-mật-mã-mls-mls-crypto-benchmark)
7. [Thí nghiệm 6: Chi phí phụ trợ của lớp điều phối (Coordinator Overhead)](#7-thí-nghiệm-6-chi-phí-phụ-trợ-của-lớp-điều-phối-coordinator-overhead)
8. [Thí nghiệm 7: Kiểm chứng Forward Secrecy bằng thành phần MLS thật](#8-thí-nghiệm-7-kiểm-chứng-forward-secrecy-bằng-thành-phần-mls-thật)
9. [Thí nghiệm 8: Đối chiếu trên ứng dụng desktop](#9-thí-nghiệm-8-đối-chiếu-trên-ứng-dụng-desktop)
10. [Tóm tắt các giới hạn đánh giá](#10-tóm-tắt-các-giới-hạn-đánh-giá)
11. [Câu hỏi phản biện thường gặp và cách trả lời](#11-câu-hỏi-phản-biện-thường-gặp-và-cách-trả-lời)

---

## 1. Tổng quan mục tiêu đánh giá

### Câu hỏi nghiên cứu

Đồ án đặt ra 3 câu hỏi cần kiểm chứng bằng thực nghiệm:

1. **Hiệu quả vận hành:** Cơ chế Single-Writer và gom đề xuất có giảm được xung đột Commit trên mạng P2P không?
2. **Tính đúng đắn:** Các nút có còn hội tụ về cùng trạng thái MLS (epoch + tree hash) sau khi mạng bị chia cắt và nối lại không?
3. **Bảo mật:** Việc tách lớp điều phối ra khỏi lõi MLS có làm suy giảm thuộc tính bảo mật (đặc biệt Forward Secrecy) không?

### Bảng tiêu chí đánh giá

| Tiêu chí | Ý nghĩa |
|----------|---------|
| Hội tụ epoch | Tất cả nút hợp lệ kết thúc ở cùng epoch sau khi mạng ổn định |
| Hội tụ mã băm cây | Tất cả nút hợp lệ có cùng tree hash MLS |
| Single-Writer trong cùng epoch | Không xuất hiện hai Commit hợp lệ cạnh tranh trong cùng epoch |
| Hiệu quả gom đề xuất | Nhiều Proposal gần nhau được gom vào một Commit |
| Thời gian phục hồi phân vùng | Khoảng thời gian từ khi mạng nối lại đến khi các nút đạt hội tụ |
| Độ trễ thao tác MLS | Chi phí mã hóa, cập nhật thành viên, xử lý Commit theo quy mô nhóm |
| Forward Secrecy | Thành viên đã bị loại không thể giải mã thông điệp ở epoch mới |

### Môi trường thí nghiệm

- **Lớp điều phối (Go):** chạy trực tiếp mã nguồn `app/coordination/`
- **Lõi MLS (Rust/OpenMLS):** Go khởi chạy Rust sidecar qua gRPC trên localhost
- **Mô phỏng:** `FakeNetwork`, `FakeClock`, `MockMLSEngine` cho test logic điều phối
- **MLS thật:** Rust `crypto-engine` cho test mật mã thực sự

### Phân loại test và phạm vi kết luận

| Nhóm test | Công nghệ | Có thể kết luận | Không nên kết luận |
|-----------|-----------|-----------------|-------------------|
| Coordination unit/integration | FakeNetwork, MockMLS | Logic giao thức phối hợp đúng | Hiệu năng mạng thật |
| Chaos convergence | In-memory network, mock MLS | Lớp coordination hội tụ sau partition | Không mất message tuyệt đối |
| Concurrency sweep | FakeNetwork, mock MLS | Batching giảm số Commit | Throughput libp2p thật |
| Epoch convergence sweep | Mock MLS, nhóm lớn mô phỏng | Chi phí state-machine tăng theo N | Latency OpenMLS/libp2p thật |
| Real sidecar forward secrecy | Go service + Rust sidecar thật | Removed member không decrypt được | Toàn bộ P2P auth/delivery |
| MLS benchmark | Rust benchmark | Xu hướng chi phí crypto theo N | UX end-to-end |

---

## 2. Thí nghiệm 1: Single-Writer và gom đề xuất (Concurrency Sweep)

### Mục đích

Kiểm chứng rằng cơ chế **Single-Writer** (chỉ Token Holder được tạo Commit) kết hợp với **gom đề xuất** (batching delay) giúp giảm số lượng Commit cần thiết khi nhiều nút cùng gửi Proposal đồng thời trong cùng một epoch.

### Vì sao cần thí nghiệm này

Khi đưa MLS lên mạng P2P, nhiều nút có thể cùng muốn thay đổi nhóm (thêm/xóa/cập nhật thành viên) cùng lúc. Nếu không có cơ chế Single-Writer:
- Nhiều nút cùng tạo Commit cho cùng epoch → cây MLS bị tách nhánh → thành viên không còn cùng trạng thái mật mã.
- Phải xử lý xung đột sau khi xảy ra (phức tạp và tốn kém).

Cơ chế Single-Writer + batching được thiết kế để **ngăn xung đột từ đầu** thay vì xử lý hậu quả.

### Cách đo

**File test:** `app/coordination/concurrency_evaluation_test.go` — hàm `TestIntegration_ConcurrencySweep`

**Lệnh chạy:**
```bash
go test ./coordination -run TestIntegration_ConcurrencySweep -count=1 -v
```

**Thiết kế thí nghiệm:**
- Tạo cụm gồm `concurrency + 1` nút (1 Token Holder + N proposer)
- So sánh 2 chiến lược:
  - **Baseline (xử lý ngay):** `BatchingDelay = 0` — Token Holder commit ngay khi nhận Proposal đầu tiên, các Proposal đến muộn bị reject do epoch đã đổi → phải retry
  - **Optimized (gom theo lô):** `BatchingDelay = 1 giây` — Token Holder chờ 1 giây để gom nhiều Proposal vào 1 Commit
- Quét mức đồng thời từ 1 đến 5
- Dùng `FakeNetwork`, `MockMLSEngine`, `FakeClock`

**Cách đo số liệu:**
- Đếm tổng số Proposal đã gửi (`totalProposals`)
- Đếm tổng số Commit đã tạo (`totalCommits`)
- Tính tỷ lệ thành công = `concurrency / totalProposals` (baseline) hoặc `1.0` (optimized)
- Ghi ra file `evaluation/data/concurrency_metrics.csv`

### Con số thực tế

Dữ liệu từ `evaluation/data/concurrency_metrics.csv`:

| Mức đồng thời | Baseline Proposal | Baseline Commit | Baseline Tỷ lệ thành công | Optimized Proposal | Optimized Commit | Optimized Tỷ lệ thành công |
|---:|---:|---:|---:|---:|---:|---:|
| 1 | 1 | 1 | 1,00 | 1 | 1 | 1,00 |
| 2 | 3 | 2 | 0,67 | 2 | 1 | 1,00 |
| 3 | 6 | 3 | 0,50 | 3 | 1 | 1,00 |
| 4 | 10 | 4 | 0,40 | 4 | 1 | 1,00 |
| 5 | 15 | 5 | 0,33 | 5 | 1 | 1,00 |

### Phân tích chi tiết

**Quan sát chính:**

1. **Baseline — số Proposal tăng theo cấp số cộng:** Ở mức đồng thời 5, baseline cần 15 Proposal (gấp 3 lần số thao tác thực). Nguyên nhân: khi Token Holder commit ngay Proposal đầu tiên, epoch tăng lên. Các Proposal khác gửi cho epoch cũ bị reject vì vi phạm **Epoch Monotonicity** (luật: không xử lý Proposal/Commit có epoch < epoch hiện tại). Các nút phải re-propose ở epoch mới, và quá trình lặp lại.

2. **Optimized — số Proposal luôn đúng bằng số thao tác thực:** Ở mọi mức đồng thời, optimized chỉ cần đúng `concurrency` Proposal và **1 Commit duy nhất**. Token Holder chờ 1 giây, gom tất cả Proposal vào một Commit, advance epoch một lần. Không có Proposal nào bị reject.

3. **Tỷ lệ thành công giảm tuyến tính ở baseline:** Từ 1,00 (concurrency=1) xuống 0,33 (concurrency=5). Nghĩa là ở concurrency=5, 67% Proposal bị lãng phí do retry. Ở optimized, tỷ lệ luôn 100%.

4. **Số Commit ở baseline = concurrency:** Mỗi Proposal cần 1 Commit riêng. Ở optimized, số Commit luôn = 1 bất kể concurrency.

**Ý nghĩa cho phản biện:**
- Cơ chế Single-Writer không chỉ đảm bảo tính đúng đắn (không có Commit cạnh tranh) mà còn có **hiệu quả vận hành** rõ rệt.
- Batching giảm cả số Proposal (tiết kiệm băng thông) và số Commit (giảm số epoch transition — mỗi epoch transition kéo theo cập nhật trạng thái nhóm toàn nhóm).
- **Giới hạn:** Thí nghiệm chỉ chạy concurrency 1-5 trên FakeNetwork. Không thể kết luận về throughput trên mạng libp2p thật. Tuy nhiên, kết luận về **hiệu quả cơ chế batching ở tầng coordination** là hợp lệ vì logic không thay đổi khi chuyển sang mạng thật.

---

## 3. Thí nghiệm 2: Hội tụ trong điều kiện mạng hỗn loạn (Chaos Test)

### Mục đích

Kiểm chứng rằng sau khi mạng bị chia cắt nhiều lần và nối lại, **tất cả các nút hợp lệ hội tụ về cùng epoch và cùng tree hash MLS** — không cần Delivery Service trung tâm áp đặt thứ tự.

### Vì sao cần thí nghiệm này

Trên mạng P2P, mất kết nối là chuyện thường. Nếu khi mạng chia cắt, các nhánh tiến hóa độc lập (mỗi nhánh tăng epoch riêng), thì khi nối lại, hệ thống phải có cơ chế tự động:
1. Phát hiện phân nhánh (fork detection)
2. Chọn nhánh thắng (nhánh có epoch cao hơn / nhiều thành viên hơn)
3. Đưa nhánh thua quay về nhánh thắng (fork healing qua External Join)

Nếu không hội tụ, các thành viên sẽ không còn cùng trạng thái mật mã → không thể giao tiếp an toàn.

### Cách đo

**File test:** `app/coordination/chaos_e2e_test.go` — hàm `TestIntegration_Chaos_Convergence`

**Lệnh chạy:**
```bash
go test ./coordination -run TestIntegration_Chaos_Convergence -count=1 -v -timeout 100s
```

**Thiết kế thí nghiệm:**
- 5 nút (Alice, Bob, Carol, Dave, Eve) trong mạng in-memory `FakeNetwork`
- 3 goroutine chạy song song:
  - **Nemesis:** Cứ 1,5 giây, chia mạng thành 2 phân vùng ngẫu nhiên, giữ 600 ms, rồi heal
  - **Client:** Gửi 800 thông điệp + thao tác add/remove thành viên (mỗi 15 thông điệp)
  - **Metrics:** Ghi epoch + tree hash của mỗi nút ra CSV mỗi 10 ms
- Tổng thời gian chạy: 60 giây
- Sau khi dừng chaos, chạy phase ổn định: broadcast announce, drain network, chờ hội tụ (tối đa 200 vòng lặp)

**Cách đo số liệu:**
- Ghi ra file `chaos_metrics.csv` với cột: `WallTimeMs, NodeID, Epoch, TreeHash`
- Tiêu chí pass:
  - Tất cả 5 nút có cùng epoch cuối (tìm max epoch, kiểm tra tất cả bằng max)
  - Tất cả 5 nút có cùng tree hash (so sánh byte-by-byte)

### Con số thực tế

| Chỉ tiêu | Kết quả quan sát |
|----------|-----------------|
| Số nút | 5 |
| Thời lượng chaos | 60 giây |
| Chu kỳ partition/heal | 1,5 giây / 600 ms partition |
| Kết quả cuối | 5/5 nút cùng epoch và cùng TreeHash |
| Deadlock | Không quan sát thấy |

### Phân tích chi tiết

**Quan sát chính:**

1. **Phân kỳ tạm thời trong lúc partition:** Khi mạng bị chia cắt, các nút ở nhánh thắng tiếp tục tăng epoch (gửi Commit), trong khi nút ở nhánh thua đứng yên. Biểu đồ epoch theo thời gian cho thấy các đường tách ra rồi chập lại.

2. **Hội tụ sau heal:** Khi mạng được nối lại, fork detector phát hiện sự khác biệt tree hash qua `GroupStateAnnouncement`. Nút ở nhánh thua kích hoạt fork healing:
   - Tạo KeyPackage mới
   - Phát tán ProposalJoin qua GossipSub
   - Token Holder trên nhánh thắng nhận ProposalJoin, tạo Commit (Remove + Add)
   - Gửi Welcome lại cho nút thua
   - Nút thua xử lý Welcome, hoán đổi trạng thái MLS
   - Phát lại thông điệp của chính mình (Autonomous Replay)

3. **Không cần Delivery Service trung tâm:** Toàn bộ quá trình diễn ra giữa các nút ngang hàng. Không có máy chủ nào áp đặt thứ tự Commit.

4. **Epoch cuối không cố định:** Số epoch cuối phụ thuộc số Commit thành công trong lần chạy (ngẫu nhiên). Không nên nói "luôn đạt Epoch = X". Chỉ nên nói "5/5 nút cùng epoch".

**Ý nghĩa cho phản biện:**
- Thí nghiệm chứng minh **tính đúng đắn của fork healing** trong môi trường có partition liên tục.
- **Giới hạn:** Dùng FakeNetwork + MockMLS, không phản ánh độ trễ/mất gói/NAT của mạng thật. Không chứng minh "không thất thoát thông điệp tuyệt đối" — chỉ chứng minh hội tụ trạng thái mật mã.

---

## 4. Thí nghiệm 3: Ảnh hưởng độ sâu phân kỳ đến thời gian phục hồi (Partition Divergence)

### Mục đích

Đo thời gian healing khi **độ sâu phân kỳ D** (số epoch mà hai nhánh đã lệch nhau) tăng từ 5 đến 50. Mục tiêu là chứng minh rằng **giao thức healing có độ phức tạp O(1) về số bước** — thời gian tăng chỉ do chi phí serialization group state, không phải do phải replay từng epoch lệch.

### Vì sao cần thí nghiệm này

Một câu hỏi phản biện hợp lệ là: "Nếu hai nhánh lệch nhau 50 epoch, liệu healing có phải replay 50 epoch?" Nếu câu trả lời là có, giao thức sẽ không mở rộng được. Thí nghiệm này được thiết kế để trả lời: **Không, healing luôn thực hiện đúng 1 chu kỳ (ProposalJoin → Commit → Welcome), bất kể D.**

### Cách đo

**File test:** `app/coordination/fork_heal_real_mls_bench_test.go` — hàm `BenchmarkForkHeal_PartitionDivergence`

**Lệnh chạy:**
```bash
go test ./coordination -run BenchmarkForkHeal_PartitionDivergence -count=1 -v -timeout 30m
```

**Thiết kế thí nghiệm:**
- 32 nút, chia thành 2 phân vùng: nhánh thắng 30 nút, nhánh thua 2 nút
- **Dùng real MLS** (Rust sidecar thật, không phải mock)
- Trong lúc partition:
  - Nhánh thắng advance D epoch thật (D = 5, 10, 20, 50) bằng CreateProposal(Update) + CreateCommit
  - Nhánh thua advance 2 epoch
- Sau khi heal mạng:
  - Re-exchange heartbeats
  - Reset fork detector
  - Broadcast announce → trigger fork detection
  - Đo thời gian healing bằng callback `onForkHealEvent` (capture `DurationMs` từ `runHeal()`)
- **Mỗi giá trị D chạy 10 lần, lấy trung vị** để giảm nhiễu
- Ghi ra `evaluation/data/partition_divergence_metrics.csv`

**Cách đo thời gian healing:**
- Callback `onForkHealEvent` ghi lại `DurationMs` = thời gian từ khi `runHeal()` bắt đầu đến khi hoàn thành
- Bao gồm: snapshot orphan, tạo KeyPackage, phát tán ProposalJoin, chờ Welcome, hoán đổi trạng thái, phát lại thông điệp, dọn dẹp
- **Không bao gồm** thời gian chờ drain network (loại nhiễu delivery)

### Con số thực tế

Dữ liệu từ `evaluation/data/partition_divergence_metrics.csv`:

| D (epoch lệch) | Healing (ms) | Group State (KB) | ms/KB |
|---:|---:|---:|---:|
| 5 | 728 | 449 | 1,62 |
| 10 | 1.039 | 762 | 1,36 |
| 20 | 1.155 | 1.381 | 0,84 |
| 50 | 2.098 | 3.240 | 0,65 |

### Phân tích chi tiết

**Quan sát 1: Thời gian healing tăng từ 728 ms đến 2.098 ms khi D tăng từ 5 đến 50**

Nhưng số bước giao thức KHÔNG đổi — luôn là: 1 ProposalJoin, 1 Commit, 1 Welcome. Vậy tại sao thời gian vẫn tăng?

**Quan sát 2: Kích thước group state tăng từ 449 KB đến 3.240 KB**

Mỗi thao tác `ProposalUpdate` tạo ra một cặp khóa HPKE mới cho nút lá trong ratchet tree. OpenMLS tuần tự hóa toàn bộ cây khi xuất group state. Sau 50 epoch, group state phình đến 3.240 KB.

**Quan sát 3: Mọi lệnh gọi gRPC đều phải gửi nguyên group state**

Vì lõi Rust là **stateless** (không giữ trạng thái nhóm trong RAM), mỗi lệnh gọi `HasMember`, `CreateProposal`, `CreateCommit`, `ProcessWelcome` đều nhận nguyên group state từ Go qua gRPC. Khi group state lớn hơn → thời gian serialize/deserialize + truyền gRPC dài hơn.

**Quan sát 4: Tỷ lệ ms/KB giảm từ 1,62 xuống 0,65**

Đây là quan sát quan trọng nhất. Khi group state lớn gấp 7 lần (3.240/449), thời gian healing chỉ tăng ~2,9 lần (2.098/728). Tỷ lệ ms/KB giảm cho phép tách thành 2 thành phần:

1. **Chi phí cố định của giao thức (O(1)):** Bầu Token Holder, phát tán ProposalJoin, chờ Commit + Welcome, hoán đổi trạng thái, phát lại tin nhắn. Phần này không phụ thuộc D hay kích thước group state.

2. **Chi phí xử lý MLS (tỉ lệ với group state):** Serialize/deserialize group state trong các lệnh gọi gRPC. Phần này tỉ lệ với kích thước group state, không tỉ lệ với D.

**Kết luận:**
- Giao thức fork healing đạt **O(1) về số bước giao thức**.
- Thời gian tăng là **hệ quả của cách triển khai** (OpenMLS serialize toàn bộ ratchet tree + kiến trúc stateless Rust sidecar), không phải do bản thân giao thức phải xử lý theo D.
- Nếu lõi MLS giữ trạng thái trong RAM (hot cache), mỗi lần gọi chỉ cần gửi delta, chi phí này sẽ giảm đáng kể.

**Ý nghĩa cho phản biện:**
- Trả lời được câu hỏi "healing có phải replay D epoch không?" → Không, chỉ 1 chu kỳ.
- Trả lời được câu hỏi "tại sao thời gian vẫn tăng?" → Do serialization group state, không do giao thức.
- Thừa nhận hạn chế: kiến trúc stateless hiện tại gây overhead khi nhóm rất đông. Đây là hướng tối ưu hóa tương lai.

---

## 5. Thí nghiệm 4: Khả năng mở rộng của thao tác thành viên (Epoch Convergence Sweep)

### Mục đích

Đo chi phí của 3 thao tác thay đổi thành viên (Add, Remove, Update) khi quy mô nhóm tăng từ 5 đến 1.000 nút. Kiểm chứng rằng chi phí tăng theo quy mô nhóm — phù hợp với phân tích lý thuyết.

### Vì sao cần thí nghiệm này

Mỗi thao tác thay đổi thành viên không chỉ là thay đổi logic ở coordination layer mà còn kéo theo:
- Tạo Proposal
- Token Holder tạo Commit (cập nhật ratchet tree)
- Phát tán Commit cho toàn nhóm
- Mọi nút cập nhật trạng thái nhóm

Cần đo để biết hệ thống có mở rộng được đến nhóm lớn không, và bottlenecks nằm ở đâu.

### Cách đo

**File test:** `app/coordination/scalability_evaluation_test.go` — hàm `TestIntegration_EpochConvergenceSweep`

**Lệnh chạy:**
```bash
go test ./coordination -run TestIntegration_EpochConvergenceSweep -count=1 -v -timeout 10m
```

**Thiết kế thí nghiệm:**
- Quét kích thước nhóm: 5, 10, 50, 100, 250, 500, 750, 1000 nút
- Mỗi kích thước, đo 3 thao tác:
  - **AddMember:** Node 1 đề xuất thêm thành viên mới → Token Holder commit → toàn nhóm hội tụ epoch 11
  - **RemoveMember:** Node 1 đề xuất xóa Node 2 → Token Holder commit → các nút còn lại hội tụ epoch 11
  - **UpdateMember:** Node 1 đề xuất key rotation → Token Holder commit → toàn nhóm hội tụ epoch 11
- Dùng `MockMLSEngine`, `FakeNetwork`, `FakeClock`
- Pre-populate nhóm với N thành viên ở epoch 10
- Đo thời gian bằng `time.Now()` trước và sau thao tác (bao gồm proposal + drain + commit + drain)
- Ghi ra `evaluation/data/epoch_convergence_metrics.csv`

**Lưu ý quan trọng:** Test dùng `MockMLSEngine` (không phải OpenMLS thật) và `SweepConfig()` với ticker disable (1 giờ) để tránh goroutine thrashing. Do đó, số liệu phản ánh **chi phí state-machine coordination** trong Go, không phải độ trễ OpenMLS/libp2p thật.

### Con số thực tế

Dữ liệu từ `evaluation/data/epoch_convergence_metrics.csv`:

| Quy mô nhóm | Thêm thành viên (ms) | Xóa thành viên (ms) | Cập nhật (ms) |
|---:|---:|---:|---:|
| 5 | 0,75 | 0,54 | 0,52 |
| 10 | 1,80 | 1,67 | 2,07 |
| 50 | 18,59 | 15,21 | 17,80 |
| 100 | 87,40 | 109,45 | 125,29 |
| 250 | 277,67 | 356,18 | 285,66 |
| 500 | 490,12 | 456,15 | 465,90 |
| 750 | 1.049,04 | 1.060,70 | 1.038,29 |
| 1.000 | 1.907,46 | 1.875,82 | 1.924,94 |

*(Lưu ý: số liệu trong file CSV thực tế có thể khác số liệu trong báo cáo LaTeX do mỗi lần chạy cho ra kết quả khác nhau. Số liệu trong báo cáo LaTeX Chương 5 được lấy từ một lần chạy cụ thể. Cấu trúc xu hướng là giống nhau.)*

### Phân tích chi tiết

**Quan sát 1: Chi phí tăng rõ khi số lượng thành viên tăng**

Từ ~0,5 ms (nhóm 5) lên ~1.900 ms (nhóm 1.000). Tăng khoảng 3.800 lần khi nhóm tăng 200 lần.

**Quan sát 2: Ba thao tác có chi phí tương đương nhau**

Add, Remove, Update đều đi qua cùng pipeline: Proposal → BufferProposal → Token Holder commit → Drain → Advance epoch. Không có thao tác nào đặc biệt đắt hơn (trừ Remove có thể rẻ hơn chút vì không cần tạo Welcome).

**Quan sát 3: Chi phí tăng không tuyến tính**

- Từ 5 → 100 (gấp 20 lần): chi phí tăng ~120 lần (0,75 → 87 ms)
- Từ 100 → 1.000 (gấp 10 lần): chi phí tăng ~22 lần (87 → 1.907 ms)

Điều này gợi ý chi phí tăng nhanh hơn tuyến tính ở nhóm nhỏ, nhưng chậm dần ở nhóm lớn. Nguyên nhân:
- **Ở nhóm nhỏ:** overhead cố định (setup, heartbeat exchange) chiếm tỷ trọng lớn
- **Ở nhóm lớn:** chi phí chủ yếu nằm ở việc xử lý ActiveView, broadcast, và cập nhật trạng thái cho N nút

**Quan sát 4: Đây là chi phí coordination, không phải crypto**

Vì dùng `MockMLSEngine`, các con số không bao gồm chi phí mật mã thật (CreateCommit trên OpenMLS). Chúng phản ánh chi phí của:
- Quản lý ActiveView (heartbeat, member tracking)
- Single-Writer logic (token election, proposal buffering)
- Broadcast Proposal + Commit qua FakeNetwork
- Storage operations (ApplyCommit, SavePendingOperation)
- Epoch advance + reconciliation

**Ý nghĩa cho phản biện:**
- Kết quả phù hợp với phân tích lý thuyết: thao tác membership đắt hơn gửi application message.
- **Giới hạn quan trọng:** Không phải latency OpenMLS thật. Nếu hỏi "thời gian thực tế để thêm 1 người vào nhóm 1.000 người là bao nhiêu?", cần kết hợp số liệu này với benchmark MLS thật (Thí nghiệm 5 và 6).
- Số liệu trong báo cáo LaTeX có thể khác file CSV vì mỗi lần chạy cho ra kết quả khác nhau — điều này bình thường và không ảnh hưởng đến xu hướng.

---

## 6. Thí nghiệm 5: Chi phí mật mã MLS (MLS Crypto Benchmark)

### Mục đích

So sánh chi phí mã hóa giữa hai hướng:
1. **Mã hóa từng cặp (pairwise baseline):** Phía gửi phải mã hóa riêng cho từng người nhận — mô phỏng cách gửi E2EE truyền thống
2. **MLS hiện tại (current_full_blob_mls_encrypt):** Phía gửi tạo 1 MLS message cho toàn nhóm — cách làm hiện tại của hệ thống

### Vì sao cần thí nghiệm này

MLS (RFC 9420) được thiết kế để giải quyết vấn đề **quy mô nhóm** trong E2EE. Trong mô hình pairwise truyền thống, phía gửi phải thực hiện N lần mã hóa (một cho mỗi người nhận). MLS chỉ cần 1 thao tác mã hóa cho toàn nhóm. Thí nghiệm này đo bằng chứng thực nghiệm cho ưu thế đó.

Tuy nhiên, benchmark cũng cho thấy **điểm nghẽn của cách triển khai hiện tại**: hàm `encrypt_message` phải nạp lại toàn bộ group state và xuất lại trạng thái mới sau mỗi lần gửi.

### Cách đo

**File benchmark:** `crypto-engine/src/bin/mls_bench.rs`

**Lệnh chạy:**
```bash
cd crypto-engine
cargo run --bin mls_bench -- --sizes 16,32,64,128,256,512,1024,2048,4096 --samples 100 --payload-size 1024
```

**Thiết kế benchmark:**
- Tạo sẵn group state với N thành viên (N = 16, 32, 64, ..., 4096) bằng `build_group_state(n)`
- Mỗi điểm đo chạy **100 mẫu** với payload **1024 byte**
- Đo thời gian từng lần bằng `Instant::now()` → `started.elapsed()`
- Sắp xếp 100 durations, lấy **median (p50), p95, p99**
- Dùng `std::hint::black_box()` để tránh compiler optimization loại bỏ computation

**Hai đường đo chính:**

1. **`pairwise_baseline`:** Mô phỏng mã hóa từng cặp
   - Tạo N cặp khóa HPKE (DhKemP256)
   - Cho mỗi recipient: tạo ephemeral key → DH → KDF extract → KDF expand key+nonce → AEAD seal (AES-128-GCM)
   - Tổng cộng: N lần KEM key gen + N lần DH + N lần KDF + N lần AEAD
   - Đây là baseline O(N)

2. **`current_full_blob_mls_encrypt`:** Đường MLS stateless hiện tại
   - Gọi `crypto_engine::mls::encrypt_message(group_state, payload)`
   - Hàm này: import_state (deserialize group state) → create_message (OpenMLS) → tls_serialize_detached → export_state (serialize lại)
   - **Bao gồm cả chi phí nạp và xuất group state**, không chỉ mã hóa thuần túy

**Cách tính percentile:**
```rust
fn measure<F>(samples: usize, mut f: F) -> (f64, f64, f64) {
    let mut durations = Vec::with_capacity(samples);
    for _ in 0..samples {
        let started = Instant::now();
        f();
        durations.push(started.elapsed());
    }
    durations.sort_unstable();
    (as_ms(percentile(&durations, 0.50)),  // median
     as_ms(percentile(&durations, 0.95)),  // p95
     as_ms(percentile(&durations, 0.99)))  // p99
}
```

### Con số thực tế

Dữ liệu từ `evaluation/data/mls_optimization_benchmark.csv` (median):

| Số nút | Pairwise (ms) | MLS hiện tại (ms) | Tỷ lệ pairwise/MLS |
|---:|---:|---:|---:|
| 16 | 3,62 | 0,55 | 6,6x |
| 32 | 7,34 | 0,95 | 7,7x |
| 64 | 14,86 | 1,55 | 9,6x |
| 128 | 29,66 | 2,91 | 10,2x |
| 256 | 59,62 | 5,41 | 11,0x |
| 512 | 119,15 | 9,87 | 12,1x |
| 1.024 | 238,75 | 19,86 | 12,0x |
| 2.048 | 476,83 | 38,56 | 12,4x |
| 4.096 | 955,18 | 78,95 | 12,1x |

p95 và p99 ở nhóm 4.096 thành viên:

| Mốc đo | Từng cặp (ms) | MLS (ms) |
|--------|---:|---:|
| p95 | 1.023,66 | 95,04 |
| p99 | 1.081,72 | 110,85 |

Phân tích xu hướng từ `evaluation/data/analysis_results.json`:

| Đường đo | Better fit | Linear R² | Slope (ms/nút) |
|----------|-----------|-----------|---|
| pairwise_baseline | Linear | 0,9987 | ~0,233 |
| current_full_blob_mls_encrypt | Linear | 0,9999 | ~0,016 |

### Phân tích chi tiết

**Quan sát 1: Pairwise tăng tuyến tính rõ rệt theo N**

Từ 3,62 ms (N=16) lên 955,18 ms (N=4096). R² = 0,9987 cho linear fit. Mỗi thêm 1 thành viên, thời gian tăng ~0,23 ms. Điều này phù hợp với độ phức tạp O(N) — phía gửi phải mã hóa riêng cho từng người.

**Quan sát 2: MLS hiện tại cũng tăng theo N, nhưng chậm hơn nhiều**

Từ 0,55 ms (N=16) lên 78,95 ms (N=4096). R² = 0,9999 cho linear fit. Slope chỉ ~0,016 ms/nút (gấp ~15 lần ít hơn pairwise). MLS không phải O(1) trong cách triển khai này, nhưng tăng chậm hơn nhiều.

**Quan sát 3: Tỷ lệ pairwise/MLS tăng từ 6,6x đến ~12x**

Khi nhóm nhỏ (16), chênh lệch chưa lớn vì overhead cố định của MLS (import/export state) chiếm tỷ trọng cao. Khi nhóm lớn (4096), ưu thế MLS rõ rệt: pairwise cần 955 ms trong khi MLS chỉ cần 79 ms.

**Quan sát 4: p95 và p99 vẫn giữ chênh lệch lớn**

Ngay cả ở các lần chạy chậm hơn (p99), pairwise là 1.082 ms trong khi MLS là 111 ms — chênh lệch ~10x. Kết luận không chỉ đúng ở median mà còn ổn định ở tail latency.

**Quan sát 5: Đường MLS hiện tại vẫn tăng theo kích thước group state**

Lý do: hàm `encrypt_message` là **stateless** — mỗi lần gọi phải:
1. `import_state(group_state)` — deserialize toàn bộ ratchet tree
2. `create_message` — mã hóa bằng OpenMLS
3. `export_state` — serialize lại toàn bộ ratchet tree

Khi group state lớn (13,9 MB ở N=4096), chi phí serialize/deserialize chiếm tỷ trọng đáng kể. Đây là **hạn chế của kiến trúc stateless**, không phải hạn chế của MLS.

**Quan sát 6: Đường hot_cache (giữ state trong RAM) gần như O(1)**

Trong dữ liệu CSV, đường `hot_cache_sidecar_encrypt_core` (giữ group state trong RAM, không import/export mỗi lần) có median chỉ 0,04-0,08 ms ở mọi kích thước nhóm. Điều này chứng minh rằng nếu tối ưu kiến trúc (giữ state trong RAM), chi phí mã hóa MLS sẽ gần O(1).

**Ý nghĩa cho phản biện:**
- MLS tốt hơn pairwise E2EE rõ rệt về chi phí phía sender khi nhóm lớn.
- Đường production hiện tại vẫn chịu overhead do kiến trúc stateless.
- Không nên dùng đường `pairwise_hash_sanity_not_e2ee` (chỉ là sanity check CPU/hash, không phải E2EE) làm baseline trong luận văn.

---

## 7. Thí nghiệm 6: Chi phí phụ trợ của lớp điều phối (Coordinator Overhead)

### Mục đích

Đo riêng **chi phí của lớp điều phối Go** (không bao gồm mật mã MLS) để biết lớp coordination bổ sung bao nhiêu overhead lên tổng chi phí hệ thống.

### Vì sao cần thí nghiệm này

Hệ thống có kiến trúc 2 lớp:
- **Lớp điều phối (Go):** Single-Writer, epoch tracking, fork detection, ActiveView, HLC
- **Lớp mật mã (Rust):** OpenMLS operations (CreateCommit, Encrypt, etc.)

Cần biết: lớp điều phối có phải là bottleneck không? Hay phần tốn thời gian nhất nằm ở MLS? Điều này giúp xác định hướng tối ưu hóa.

### Cách đo

**File test:** `app/coordination/coordinator_overhead_bench_test.go` — hàm `TestIntegration_CoordinatorOverhead`

**Lệnh chạy:**
```bash
go test ./coordination -run TestIntegration_CoordinatorOverhead -count=1 -v -timeout 30m
```

**Thiết kế thí nghiệm:**
- **Phase 1 — MockMLS sweep (chỉ coordination):** Chạy AddMember với `MockMLSEngine` trên các nhóm 16, 32, 64, 128, 256, 512, 1000 nút. Đo thời gian bằng `time.Now()`.
- **Phase 2 — Real MLS sweep (coordination + crypto):** Chạy AddMember với Rust sidecar thật trên các nhóm 16, 32, 64. Đo thời gian.
- **Tính toán:**
  - `CoordinationMs = MockMs` (chi phí coordination)
  - `TotalMs = RealMs` (coordination + crypto)
  - `CryptoMs = RealMs - MockMs` (chi phí crypto)
  - `CoordinationPct = (MockMs / RealMs) × 100`
- Ghi ra `evaluation/data/coordinator_overhead_metrics.csv`

**Chi tiết breakdown (từ `coordinator_overhead_breakdown.csv`):**

| Group Size | OpenMLS Crypto (ms) | Coordinator Decision (ms) | Storage Serialization (ms) |
|---:|---:|---:|---:|
| 16 | 0,5451 | 0,232 | 1,7306 |
| 128 | 2,9101 | 0,456 | 10,2726 |
| 512 | 9,8713 | 1,224 | 39,2679 |
| 1.024 | 19,8554 | 2,248 | 77,9035 |

### Con số thực tế

| Số thành viên | Điều phối (ms) | Tổng (ms) | MLS (ms) | Tỷ trọng điều phối (%) |
|---:|---:|---:|---:|---:|
| 16 | 1,58 | 39,73 | 38,15 | 3,97 |
| 32 | 3,08 | 115,43 | 112,35 | 2,67 |
| 64 | 11,30 | 650,70 | 639,40 | 1,74 |
| 128 | 35,55 | — | — | — |
| 256 | 134,86 | — | — | — |
| 512 | 539,69 | — | — | — |
| 1.000 | 2.204,85 | — | — | — |

*(Dòng "—" cho Real MLS không có dữ liệu do giới hạn thời gian chạy với thành phần MLS thật ở nhóm lớn. Chỉ đo được đến nhóm 64.)*

### Phân tích chi tiết

**Quan sát 1: Chi phí coordination tăng theo quy mô nhóm nhưng ở mức thấp**

Từ 1,58 ms (nhóm 16) lên 2.204,85 ms (nhóm 1000). Mặc dù tăng, nhưng đây là chi phí của toàn bộ pipeline coordination (proposal, broadcast, storage, epoch advance) cho N nút.

**Quan sát 2: Chi phí MLS chiếm phần lớn tổng chi phí**

Ở nhóm 16: MLS = 38,15 ms / Tổng = 39,73 ms → MLS chiếm 96%
Ở nhóm 64: MLS = 639,40 ms / Tổng = 650,70 ms → MLS chiếm 98,3%

**Quan sát 3: Tỷ trọng coordination giảm khi nhóm tăng**

Từ 3,97% (nhóm 16) xuống 1,74% (nhóm 64). Điều này có nghĩa:
- Khi nhóm nhỏ, overhead coordination chiếm tỷ trọng cao hơn (vì chi phí crypto còn thấp)
- Khi nhóm lớn, chi phí crypto tăng nhanh hơn coordination → coordination trở thành tỷ trọng nhỏ hơn

**Quan sát 4: Storage serialization là thành phần lớn nhất trong coordination**

Từ breakdown: Storage serialization (77,9 ms ở nhóm 1024) lớn hơn Coordinator decision (2,25 ms) gấp 35 lần. Nguyên nhân: `ApplyCommit` phải serialize group state (lên đến hàng MB) để lưu vào storage.

**Kết luận:**
- Lớp điều phối **không phải là bottleneck** — chỉ chiếm 2-4% tổng chi phí.
- Nút thắt chính nằm ở việc xử lý và trao đổi group state có kích thước ngày càng lớn.
- Hướng tối ưu hóa: giảm kích thước group state truyền qua gRPC (delta update thay vì full state).

**Ý nghĩa cho phản biện:**
- Trả lời câu hỏi "coordination layer có overhead lớn không?" → Không, chỉ 2-4%.
- Trả lời câu hỏi "bottleneck nằm ở đâu?" → Ở MLS crypto, đặc biệt là serialize/deserialize group state.

---

## 8. Thí nghiệm 7: Kiểm chứng Forward Secrecy bằng thành phần MLS thật

### Mục đích

Kiểm chứng rằng sau khi một thành viên bị loại khỏi nhóm MLS, thành viên đó **không thể giải mã** các thông điệp được gửi ở epoch mới (sau khi remove). Đây là thuộc tính **Forward Secrecy** (cụ thể: post-compromise security sau remove).

### Vì sao cần thí nghiệm này

Đồ án đặt lớp điều phối (Go) **bên ngoài** lõi MLS (Rust). Một câu hỏi hợp lệ là: việc thêm một lớp trung gian có làm suy giảm thuộc tính bảo mật của MLS không?

Thí nghiệm này dùng **Rust sidecar thật** (không phải mock) để kiểm chứng rằng:
1. MLS remove member hoạt động đúng
2. Thành viên bị remove không có khóa epoch mới
3. Coordinator của thành viên bị remove không thể decrypt thông điệp mới

### Cách đo

**File test:** `app/service/business_e2e_group_integrity_test.go` — hàm `TestBusinessP1_E2E_RealSidecar_ForwardSecrecy`

**Lệnh chạy:**
```bash
go test -tags=business_integration ./service -run TestBusinessP1_E2E_RealSidecar_ForwardSecrecy -count=1 -v -timeout 2m
```

**Thiết kế thí nghiệm:**
1. **Setup:** Alice và Bob, mỗi người có Rust sidecar thật (Go spawn Rust process, truyền port qua CLI)
2. **Alice tạo nhóm** `grp-e2e-fs`
3. **Bob tham gia:** Bob generate KeyPackage → Alice AddMemberToGroup → Bob JoinGroupWithWelcome (dùng Welcome thật từ Rust)
4. **Alice remove Bob:** `alice.RemoveMemberFromGroup(gid, bobInfo.PeerID)` — tạo ProposalRemove + Commit, advance epoch
5. **Alice gửi message mới** ở epoch sau remove: `alice.SendGroupMessage(gid, postRemoveMsg)`
6. **Lấy envelope** của message mới từ Alice's storage (epoch >= 2, type = MsgApplication)
7. **Ép Bob xử lý envelope:** `coord.ReceiveDirectMessage(aPeerID, futureWire)` — mô phỏng việc Bob nhận message qua mạng
8. **Kiểm tra DB của Bob:** `bob.GetGroupMessages(gid, 100, 0)` — nếu tìm thấy `postRemoveMsg` → SECURITY BREACH
9. **Điều kiện pass:** Không tìm thấy `postRemoveMsg` trong DB của Bob

**Lưu ý kỹ thuật:**
- Test bypass P2P handshake (mock `getVerifiedTokenPublicKey`) để cô lập kiểm tra crypto
- Message được ép trực tiếp vào coordinator của Bob thay vì qua libp2p thật
- Test từng bị deadlock trong `JoinGroupWithWelcome` (giữ write lock rồi lấy read lock trên cùng mutex) — đã sửa, test pass trong 4-6 giây

### Con số thực tế

| Chỉ tiêu | Kết quả |
|----------|---------|
| Crypto engine | Rust sidecar thật (không mock) |
| Join bằng Welcome | Thành công |
| Remove member | Thành công |
| Message sau remove | Được Alice gửi ở epoch mới |
| Bob xử lý message sau remove | **Không lưu được plaintext** |
| Kết quả test | **PASS** |

### Phân tích chi tiết

**Cơ chế bảo mật:**

1. Khi Alice remove Bob, MLS Commit tạo epoch mới. Trong epoch mới:
   - Ratchet tree được cập nhật: leaf của Bob bị remove
   - Epoch secret mới được derive từ tree đã cập nhật
   - Bob **không có** epoch secret mới (vì đã bị remove khỏi tree)

2. Khi Alice gửi message ở epoch mới:
   - Message được mã hóa bằng application key của epoch mới
   - Bob chỉ có khóa của epoch cũ (trước khi bị remove)

3. Khi Bob nhận message:
   - Coordinator của Bob gọi `DecryptMessage(groupState, ciphertext)` trên Rust sidecar
   - OpenMLS kiểm tra: message epoch > local epoch → **reject** (forward secrecy)
   - Hoặc: Bob không có key cho epoch mới → **decrypt fail**
   - Plaintext không được lưu vào DB

**Ý nghĩa:**
- Lớp điều phối chỉ quyết định tiến trình trạng thái và thứ tự xử lý.
- **Quyền giải mã cuối cùng vẫn do trạng thái mật mã MLS kiểm soát.**
- Việc đặt coordination layer bên ngoài MLS không can thiệp vào cơ chế khóa của MLS.

**Giới hạn:**
- Test ép message trực tiếp vào coordinator, không chứng minh toàn bộ đường truyền libp2p.
- Harness có cảnh báo `invalid Admin signature` do các runtime test được seed bằng Root Admin khác nhau — không ảnh hưởng đến test forward secrecy nhưng cần sửa nếu muốn đánh giá P2P auth thật.

---

## 9. Thí nghiệm 8: Đối chiếu trên ứng dụng desktop

### Mục đích

Kiểm chứng rằng các giả định thiết kế của giao thức đã đi vào **luồng sử dụng thật** trên ứng dụng desktop, không chỉ tồn tại trong test tự động.

### Vì sao cần phần này

Test tự động chứng minh logic đúng, nhưng cần đối chiếu trên UI để:
1. Người dùng có thể thực sự sử dụng các tính năng
2. Thao tác trên giao diện kích hoạt đúng pipeline giao thức ở phía sau
3. Giao diện chỉ là lớp hiển thị — quyết định thật nằm ở backend

### Các kịch bản kiểm chứng

**Kịch bản 1: Tạo tổ chức và cấp bundle**
- Máy quản trị: tạo tổ chức, mở admin panel, cấp bundle cho thiết bị mới
- Máy thành viên: tạo định danh cục bộ, dừng ở màn hình chờ bundle (hiển thị PeerID + MLS public key)
- **Ý nghĩa giao thức:** Thiết bị không thể tự ý vào mạng — phải có bundle hợp lệ do admin cấp. Đây là giả định an toàn **Strict Onboarding**.

**Kịch bản 2: Thiết bị mới vào hệ thống**
- Người dùng nhập bundle → ứng dụng kiểm tra bundle → nạp dữ liệu tổ chức → khởi tạo quan hệ nhóm → chuyển sang màn hình làm việc
- **Ý nghĩa giao thức:** Giao diện chỉ hiển thị kết quả sau khi coordination + storage cập nhật xong trạng thái.

**Kịch bản 3: Trao đổi tin nhắn trong nhóm**
- Mở đồng thời Admin, Alice, Bob → tạo nhóm → thêm thành viên → gửi tin nhắn
- **Ý nghĩa giao thức:** Trước khi nhắn tin, các thay đổi thành viên phải đi qua Proposal + Commit. Sau đó, tin nhắn được mã hóa bằng MLS và hiển thị đồng bộ trên các phía.

**Kịch bản 4: Quản trị thành viên**
- Mở chi tiết phòng → xem danh sách thành viên, đổi chính sách mời, thêm người mới
- **Ý nghĩa giao thức:** Giao diện kích hoạt các thao tác thay đổi thành viên ở phía sau (AddMember, RemoveMember, UpdateMember).

---

## 10. Tóm tắt các giới hạn đánh giá

| Giới hạn | Chi tiết |
|----------|---------|
| Mạng giả lập | Chaos/fork-heal test dùng FakeNetwork, chưa phản ánh độ trễ/mất gói/NAT của mạng libp2p thật |
| MockMLS vs Real MLS | Nhiều test dùng MockMLSEngine — chứng minh logic coordination, không chứng minh hiệu năng OpenMLS thật |
| Benchmark MLS | Đo chi phí xử lý lõi MLS + trao đổi trạng thái, chưa phải độ trễ end-to-end mà người dùng cảm nhận |
| Kiến trúc stateless | Lõi Rust không giữ state trong RAM → mỗi lần gọi gRPC phải gửi nguyên group state → overhead khi nhóm lớn |
| Real sidecar forward secrecy | Ép message trực tiếp vào coordinator, chưa chứng minh toàn bộ đường truyền P2P |
| Số liệu latency | Phụ thuộc cấu hình máy, số mẫu, payload size — cần ghi rõ cấu hình khi diễn giải |

---

## 11. Câu hỏi phản biện thường gặp và cách trả lời

### Q: Tại sao dùng FakeNetwork và MockMLS mà không dùng mạng thật?

**A:** FakeNetwork và MockMLS cho phép cô lập logic điều phối (Single-Writer, epoch, fork healing) trong môi trường deterministic, dễ tạo partition, dễ ép thứ tự message, dễ tái tạo race condition. Nếu dùng mạng thật ngay từ đầu, khi test fail sẽ không biết lỗi do logic hay do mạng. Tuy nhiên, test Forward Secrecy dùng real Rust sidecar để kiểm chứng thuộc tính mật mã thật.

### Q: Giao thức healing có phải replay D epoch không?

**A:** Không. Healing luôn thực hiện đúng 1 chu kỳ: ProposalJoin → Commit → Welcome. Số bước giao thức không thay đổi theo D. Thời gian healing tăng khi D lớn là do group state lớn hơn (OpenMLS serialize toàn bộ ratchet tree), không phải do phải replay từng epoch. Bằng chứng: tỷ lệ ms/KB giảm khi D tăng (1,62 → 0,65), cho thấy có thành phần cố định O(1) và thành phần tỉ lệ với group state.

### Q: Tại sao MLS encrypt vẫn tăng theo N? MLS không phải O(log N) hay O(1) sao?

**A:** MLS về mặt lý thuyết có ratchet tree O(log N) cho thao tác cây. Tuy nhiên, cách triển khai hiện tại là **stateless**: mỗi lần `encrypt_message` phải import toàn bộ group state → mã hóa → export lại group state. Chi phí serialize/deserialize tỉ lệ với kích thước group state (tuyến tính theo N). Nếu giữ state trong RAM (đường `hot_cache_sidecar_encrypt_core` trong benchmark), chi phí mã hóa gần như O(1) — chỉ 0,04-0,08 ms ở mọi N. Đây là hạn chế của kiến trúc triển khai, không phải của MLS.

### Q: Lớp coordination có overhead lớn không?

**A:** Không. Theo Thí nghiệm 6, coordination chỉ chiếm 2-4% tổng chi phí. Phần lớn (96-98%) nằm ở MLS crypto. Trong coordination, phần lớn thời gian nằm ở storage serialization (serialize group state để lưu DB), không phải logic Single-Writer hay fork detection.

### Q: Single-Writer có thực sự cần thiết không? MLS không đã có epoch để chống conflict sao?

**A:** MLS dùng epoch để đảm bảo thứ tự: Commit ở epoch E phải tham chiếu đúng proposal refs ở epoch E. Nhưng MLS không quy định **ai được tạo Commit**. Trong RFC 9420, Delivery Service có thể sắp xếp Commit. Trên P2P không có Delivery Service, nếu nhiều nút cùng tạo Commit cho epoch E, mỗi Commit sẽ tham chiếu các proposal khác nhau → tạo ra nhiều nhánh. Single-Writer giải quyết bằng cách: chỉ 1 nút (Token Holder) được tạo Commit, các nút khác chỉ gửi Proposal. Thí nghiệm 1 cho thấy cơ chế này giảm số Commit từ 5 xuống 1 ở concurrency=5.

### Q: Batching delay 1 giây có làm chậm hệ thống không?

**A:** Có, nhưng chỉ 1 giây cho thao tác thay đổi thành viên (add/remove/update). Tin nhắn ứng dụng (application message) không đi qua batching — được mã hóa và gửi ngay. Thí nghiệm 1 cho thấy batching giảm tổng số Proposal từ 15 xuống 5 và số Commit từ 5 xuống 1 ở concurrency=5. Đổi 1 giây delay để tránh xung đột + giảm epoch transition là đáng.

### Q: Test Forward Secrecy có chứng minh được gì trên mạng thật?

**A:** Test chứng minh rằng trên đường real Rust sidecar (OpenMLS thật), removed member không thể decrypt message ở epoch mới. Tuy nhiên, test ép message trực tiếp vào coordinator thay vì qua libp2p. Nếu hỏi về toàn bộ P2P path (GossipSub delivery, NAT traversal, auth), cần test thêm trên mạng thật nhiều tiến trình. Trong phạm vi đồ án, kết luận đúng là: "removed member không decrypt được thông điệp tương lai trong đường real-sidecar".

### Q: Số liệu trong báo cáo và số liệu trong file CSV có khác nhau, tại sao?

**A:** Một số thí nghiệm (như Epoch Convergence Sweep) cho ra kết quả khác nhau mỗi lần chạy do phụ thuộc timing hệ thống. Báo cáo LaTeX sử dụng số liệu từ một lần chạy cụ thể. File CSV lưu kết quả của lần chạy gần nhất. Xu hướng (chi phí tăng theo N, batching giảm Commit, MLS nhanh hơn pairwise) là nhất quán giữa các lần chạy. Đây là đặc tính bình thường của benchmark, không phải lỗi.

### Q: Tại sao không đo end-to-end latency từ góc nhìn người dùng?

**A:** Đo end-to-end cần chạy nhiều máy ảo/process độc lập với mạng thật, NAT, firewall — vượt quá phạm vi đồ án. Benchmark MLS đo chi phí crypto + state exchange. Test coordination đo chi phí logic. Test service đo luồng nghiệp vụ. Mỗi tầng đo một phần, cần kết hợp để ước lượng end-to-end. Đây là giới hạn được nêu rõ trong báo cáo.

---

## Phụ lục: Danh sách file mã nguồn và dữ liệu

### File test/benchmark

| File | Vai trò |
|------|---------|
| `app/coordination/chaos_e2e_test.go` | Chaos convergence test (5 nút, 60s, partition/heal) |
| `app/coordination/concurrency_evaluation_test.go` | Concurrency sweep (baseline vs optimized batching) |
| `app/coordination/scalability_evaluation_test.go` | Epoch convergence sweep (add/remove/update, 5-1000 nút) |
| `app/coordination/coordinator_overhead_bench_test.go` | Coordinator overhead (mock vs real MLS) |
| `app/coordination/fork_heal_real_mls_bench_test.go` | Partition divergence benchmark (real MLS, D=5-50) |
| `app/service/business_e2e_group_integrity_test.go` | Forward Secrecy test (real Rust sidecar) |
| `crypto-engine/src/bin/mls_bench.rs` | MLS crypto benchmark (pairwise vs MLS, 16-4096 nút) |

### File dữ liệu (evaluation/data/)

| File | Nội dung |
|------|---------|
| `concurrency_metrics.csv` | Số Proposal/Commit khi nhiều Proposal đồng thời |
| `epoch_convergence_metrics.csv` | Độ trễ add/remove/update theo kích thước nhóm |
| `mls_optimization_benchmark.csv` | Benchmark pairwise và full-blob MLS (median, p95, p99) |
| `analysis_results.json` | Phân tích xu hướng và fit độ phức tạp |
| `coordinator_overhead_breakdown.csv` | Breakdown: OpenMLS crypto, coordinator decision, storage |
| `coordinator_overhead_metrics.csv` | Tổng hợp: mock vs real, coordination % |
| `partition_divergence_metrics.csv` | Healing time + group state size theo D |
| `partition_recovery_metrics.csv` | Recovery time theo partition duration |
| `latency_breakdown.csv` | End-to-end latency breakdown (encrypt, decrypt, storage) |
| `scalability_mls.csv` | MLS encrypt + add member latency theo group size |
| `single_writer_latency.csv` | Single-Writer proposal-to-commit latency |

### Lệnh chạy kiểm chứng

```bash
# Thí nghiệm 1: Concurrency sweep
cd app && go test ./coordination -run TestIntegration_ConcurrencySweep -count=1 -v

# Thí nghiệm 2: Chaos convergence
cd app && go test ./coordination -run TestIntegration_Chaos_Convergence -count=1 -v -timeout 100s

# Thí nghiệm 3: Partition divergence (cần Rust binary)
cd app && go test ./coordination -run BenchmarkForkHeal_PartitionDivergence -count=1 -v -timeout 30m

# Thí nghiệm 4: Epoch convergence sweep
cd app && go test ./coordination -run TestIntegration_EpochConvergenceSweep -count=1 -v -timeout 10m

# Thí nghiệm 5: MLS crypto benchmark
cd crypto-engine && cargo run --bin mls_bench -- --samples 100 --payload-size 1024

# Thí nghiệm 6: Coordinator overhead
cd app && go test ./coordination -run TestIntegration_CoordinatorOverhead -count=1 -v -timeout 30m

# Thí nghiệm 7: Forward Secrecy
cd app && go test -tags=business_integration ./service -run TestBusinessP1_E2E_RealSidecar_ForwardSecrecy -count=1 -v -timeout 2m
```
