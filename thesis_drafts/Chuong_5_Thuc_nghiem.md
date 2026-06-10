# CHƯƠNG 5. ĐÁNH GIÁ THỰC NGHIỆM

Chương này đánh giá hai nhóm thuộc tính của hệ thống: tính đúng đắn của giao thức phối hợp phi tập trung và chi phí thực thi của các đường mật mã MLS. Các kết quả được rút ra từ test tự động trong mã nguồn và các artifact đo đạc trong thư mục `evaluation/`.

## 5.1. Các tham số đánh giá

Các tham số chính gồm:

| Tham số | Ý nghĩa |
|---|---|
| Epoch convergence | Các nút hội tụ về cùng epoch sau phân mảnh hoặc thao tác đồng thời |
| TreeHash convergence | Các nút có cùng hash cây MLS sau khi heal |
| Commit safety | Không có nhiều Commit hợp lệ cạnh tranh trong cùng epoch trên cùng một nhánh |
| Replay correctness | Autonomous Replay chỉ phát lại thông điệp do chính nút tạo ra, không phát lại thông điệp của nút khác |
| Recovery time | Thời gian từ lúc heal mạng đến khi các nút hội tụ |
| Proposal success rate | Tỷ lệ Proposal được commit thành công, đặc biệt khi có nhiều Proposal đồng thời |
| Cryptographic latency | Độ trễ mã hóa/cập nhật nhóm theo kích thước nhóm |
| State size | Kích thước `group_state` khi số thành viên tăng |

Ngoài ra, các test bảo mật kiểm tra onboarding token binding, single-active-device, replay metadata và một test dùng real Rust sidecar kiểm tra thành viên đã bị xóa không đọc được thông điệp tương lai.

## 5.2. Phương pháp thí nghiệm

### 5.2.1. Môi trường và nguyên tắc đánh giá

Các thí nghiệm được chạy trên môi trường phát triển cục bộ của dự án, cùng mã nguồn dùng để xây dựng ứng dụng. Nhóm test Go được chạy bằng `go test`; nhóm benchmark MLS được lấy từ artifact trong thư mục `evaluation/data`. Các test real-sidecar sử dụng Rust `crypto-engine` thật thông qua gRPC sidecar do runtime Go khởi động, đúng với nguyên tắc Go spawn Rust sidecar và truyền port qua CLI.

Các kết quả trong chương này được diễn giải theo nguyên tắc sau:

- Test dùng `FakeNetwork`/`MockMLSEngine` chỉ chứng minh logic phối hợp và state machine, không chứng minh hiệu năng mạng libp2p thật.
- Test dùng Rust sidecar thật có giá trị mạnh hơn về mặt mật mã, nhưng nếu vẫn ép message trực tiếp vào coordinator thì chưa phải đánh giá toàn bộ đường truyền P2P.
- Benchmark MLS đo chi phí mã hóa/cập nhật nhóm ở sidecar, không đại diện trực tiếp cho độ trễ giao diện người dùng.
- Các kết quả latency cần được đọc cùng cấu hình chạy, số mẫu và artifact sinh ra.

### 5.2.2. Lệnh chạy và artifact dữ liệu

Các lệnh kiểm chứng chính:

```text
go test ./coordination -run TestIntegration_Chaos_Convergence -count=1 -v -timeout 100s
go test ./coordination -run TestIntegration_ForkHeal_ConvergesReplayAndPersistsHistory -count=1 -v
go test ./coordination -run TestIntegration_ConcurrencySweep -count=1 -v
go test ./coordination -run TestIntegration_EpochConvergenceSweep -count=1 -v -timeout 10m
go test -tags=business_integration ./service -run TestBusinessP1_E2E_RealSidecar_ForwardSecrecy -count=1 -v -timeout 2m
```

Các artifact dữ liệu dùng trong chương:

| Artifact | Nội dung |
|---|---|
| `evaluation/data/concurrency_metrics.csv` | Số Proposal/Commit khi nhiều Proposal đồng thời |
| `evaluation/data/epoch_convergence_metrics.csv` | Độ trễ add/remove/update member theo kích thước nhóm |
| `evaluation/data/mls_optimization_benchmark.csv` | Kết quả benchmark pairwise, full-blob MLS và hot-cache |
| `evaluation/data/analysis_results.json` | Phân tích xu hướng và fit độ phức tạp từ benchmark |
| `app/coordination/chaos_metrics.csv` | Diễn biến epoch/tree hash trong chaos test |

### 5.2.3. Phân loại test và phạm vi kết luận

Để tránh overclaim, các test được phân loại theo mức độ gần với hệ thống thật:

| Nhóm test | Công nghệ dùng | Mục tiêu kiểm tra | Có thể kết luận | Không nên kết luận |
|---|---|---|---|---|
| Coordination unit/integration | `FakeNetwork`, `FakeClock`, `MockMLSEngine` | Single-Writer, epoch, HLC, fork-heal, replay | Logic giao thức phối hợp đúng trong môi trường kiểm soát | Hiệu năng mạng thật hoặc OpenMLS thật |
| Chaos convergence | In-memory network, mock MLS | Hội tụ epoch/TreeHash sau phân mảnh | Lớp coordination có khả năng hội tụ sau partition/heal | Không chứng minh không mất message tuyệt đối |
| Concurrency sweep | FakeNetwork, mock MLS | So sánh commit ngay và batching | Batching giảm số Commit khi Proposal đồng thời | Không đại diện cho throughput libp2p thật |
| Epoch convergence sweep | Mock MLS, nhóm lớn mô phỏng | Chi phí state-machine khi membership thay đổi | Chi phí tăng theo quy mô nhóm trong mô phỏng Go | Không phải latency OpenMLS/libp2p thật |
| Real sidecar forward secrecy | Go service + Rust sidecar thật | Member bị remove không decrypt được message epoch sau | Thuộc tính remove-member forward secrecy trên đường real-sidecar | Chưa chứng minh toàn bộ P2P auth/delivery |
| MLS benchmark | Rust benchmark | Chi phí pairwise/full-blob/hot-cache | So sánh xu hướng chi phí crypto theo $N$ | Không đại diện trực tiếp cho UX end-to-end |

### 5.2.4. Số lần chạy và cách đọc số liệu

Các test đúng đắn như chaos, fork-heal, concurrency và forward secrecy được chạy với `-count=1` trong lần kiểm chứng gần nhất để xác nhận trạng thái hiện tại của mã nguồn. Riêng benchmark MLS trong `mls_bench.rs` dùng nhiều mẫu cho mỗi kích thước nhóm; kết quả trong bảng chương này ưu tiên median để giảm ảnh hưởng của nhiễu hệ thống.

Đối với các kết quả latency, luận văn không nên diễn giải từng con số đơn lẻ như hằng số tuyệt đối. Cách đọc phù hợp là so sánh xu hướng: đường pairwise tăng tuyến tính rõ, đường full-blob MLS tăng theo kích thước state, đường hot-cache encrypt gần phẳng, còn hot-cache update commit vẫn tăng đáng kể khi nhóm lớn.

### 5.2.5. Nhóm test giao thức phối hợp

Các test trong `app/coordination` dùng `FakeNetwork`, `FakeClock` và `MockMLSEngine`. Đây không phải mạng libp2p thật và không chạy OpenMLS thật; mục tiêu của nhóm test này là cô lập logic phối hợp: Single-Writer, epoch check, HLC, fork detection, fork healing, replay và crash safety.

Nhóm test này phù hợp để chứng minh bất biến thuật toán vì môi trường deterministic, dễ tạo phân mảnh, dễ ép thứ tự thông điệp và dễ tái hiện race condition. Tuy nhiên, khi trình bày trong đồ án cần nói rõ đây là mô phỏng ở tầng coordination, không phải benchmark mạng vật lý.

### 5.2.6. Chaos test hội tụ

Chaos test chính là `TestIntegration_Chaos_Convergence` trong `app/coordination/chaos_e2e_test.go`. Kịch bản gồm 5 nút trong mạng in-memory. Nemesis định kỳ chia mạng thành hai phân vùng, giữ phân vùng trong 600 ms rồi heal. Đồng thời, các nút gửi thông điệp và thực hiện add/remove thành viên. Test chạy 60 giây và ghi epoch/tree hash ra CSV mỗi 10 ms.

Tiêu chí pass hiện tại đã được siết lại gồm:

- tất cả node hội tụ về cùng epoch cuối;
- tất cả node hội tụ về cùng `TreeHash` cuối.

Epoch cuối không phải hằng số cố định. Nó phụ thuộc số Commit thành công trong lần chạy, nên không nên viết “luôn đạt Epoch = 23”. Trong lần kiểm chứng gần nhất, test pass với 5/5 node cùng epoch và cùng TreeHash sau giai đoạn ổn định cuối.

### 5.2.7. Test fork-heal và replay

Các test fork-heal integration kiểm tra riêng việc nhánh thua External Join vào nhánh thắng, TreeHash hội tụ, lịch sử heal được ghi lại, và replay chỉ áp dụng cho thông điệp của chính tác giả. Nhóm crash-safety kiểm tra resume ở nhiều pha khác nhau, job id, branch mismatch, outbox recovery và lỗi storage.

Các test này là bằng chứng mạnh cho thiết kế state machine của fork healing, mặc dù vẫn dùng mock MLS. Chúng nên được đưa vào đồ án như kiểm chứng tầng giao thức.

### 5.2.8. Test tích hợp nghiệp vụ và real sidecar

Các test trong `app/service` với build tag `business_integration` kiểm tra luồng sản phẩm như tạo nhóm, invite, auto-join, remove member, backup, session takeover và diagnostics. Phần lớn test nghiệp vụ dùng MLS mock để chạy nhanh và cô lập logic Go/service.

Test đáng chú ý nhất về mật mã là `TestBusinessP1_E2E_RealSidecar_ForwardSecrecy`, dùng Rust sidecar thật để kiểm tra rằng Bob sau khi bị Alice remove khỏi nhóm không thể decrypt thông điệp Alice gửi ở epoch sau đó. Lần chạy đầu ngày 10/06/2026 bị timeout do deadlock trong `JoinGroupWithWelcome`: hàm join giữ write lock của `Runtime` rồi gọi helper lấy read lock trên cùng mutex. Sau khi sửa deadlock này, test chạy lại pass trong khoảng 4-6 giây. Vì vậy test này có thể dùng làm bằng chứng thực nghiệm cho đường real-sidecar ở phạm vi remove-member forward secrecy.

### 5.2.9. Benchmark tối ưu hóa MLS

Benchmark `crypto-engine/src/bin/mls_bench.rs` đo các kích thước nhóm $N = 16, 32, \ldots, 4096$, mỗi điểm 100 mẫu, payload 1024 byte. Các đường đo chính:

- `pairwise_baseline`: bọc khóa kiểu pairwise cho từng người nhận bằng thao tác HPKE-like, dùng làm baseline $O(N)$.
- `current_full_blob_mls_encrypt`: đường MLS stateless hiện tại, mỗi lần mã hóa import/export toàn bộ `group_state`.
- `hot_cache_sidecar_encrypt_core`: đường benchmark hot-cache, nhóm được preload trong RAM và hot RPC chỉ truyền `group_id`.
- `hot_cache_sidecar_update_commit_core`: cập nhật nhóm trên đường hot-cache.

Đường `pairwise_hash_sanity_not_e2ee` chỉ là sanity check CPU/hash, không nên dùng làm baseline E2EE trong luận văn.

## 5.3. Kết quả thí nghiệm 1: Hội tụ sau phân mảnh mạng

Chaos test pass sau 60 giây với 5 nút và workload đồng thời. Kết quả này cho thấy lớp phối hợp có khả năng đưa các node trở lại một trạng thái chung sau khi mạng bị chia cắt và nối lại. Trong quá trình chạy, log ghi nhận các sự kiện buffer future epoch, reject stale epoch, schedule fork heal, complete fork heal và rebase pending operation. Đây là hành vi phù hợp với thiết kế: trong lúc phân mảnh, các nhánh có thể tạm thời tiến hóa khác nhau; sau khi heal, nhánh thua External Join vào nhánh thắng.

Kết quả được tổng hợp bằng biểu đồ epoch theo thời gian và bảng tóm tắt:

| Chỉ tiêu | Kết quả quan sát |
|---|---|
| Số node | 5 |
| Thời lượng chaos | 60 giây |
| Chu kỳ partition/heal | 1,5 giây / 600 ms |
| Kết quả cuối | 5/5 node cùng epoch và cùng TreeHash |
| Deadlock | Không quan sát thấy trong test pass |

Không nên khẳng định “không thất thoát thông điệp tuyệt đối” chỉ từ chaos test này. Kết luận đúng hơn là: cơ chế Autonomous Replay và envelope replay đã có test riêng cho non-repudiation, deduplication và replay order; còn số lượng thông điệp mất/khôi phục nên được báo cáo bằng metric riêng nếu muốn đưa thành kết quả định lượng.

## 5.4. Kết quả thí nghiệm 2: Proposal đồng thời và batching

Test `TestIntegration_ConcurrencySweep` so sánh hai cấu hình:

- baseline commit ngay khi nhận Proposal;
- optimized batching delay 1 giây.

Kết quả trong `evaluation/data/concurrency_metrics.csv` cho thấy khi concurrency tăng từ 1 đến 5, baseline cần nhiều Proposal retry hơn, còn batching commit tất cả Proposal trong một Commit:

| Concurrency | Baseline proposals | Baseline commits | Optimized proposals | Optimized commits |
|---:|---:|---:|---:|---:|
| 1 | 1 | 1 | 1 | 1 |
| 2 | 3 | 2 | 2 | 1 |
| 3 | 6 | 3 | 3 | 1 |
| 4 | 10 | 4 | 4 | 1 |
| 5 | 15 | 5 | 5 | 1 |

Kết quả này chứng minh batching làm giảm số Commit khi nhiều Proposal xuất hiện gần nhau trong cùng epoch. Tuy nhiên, test chỉ chạy concurrency 1-5 trên FakeNetwork, nên kết luận nên giới hạn ở tầng coordination, không mở rộng thành claim hiệu năng mạng thật.

## 5.5. Kết quả thí nghiệm 3: Chi phí thao tác thành viên theo kích thước nhóm

Test `TestIntegration_EpochConvergenceSweep` đo thời gian xử lý ba thao tác membership trên các kích thước nhóm từ 5 đến 1000 node. Lần chạy kiểm chứng ngày 10/06/2026 hoàn tất sau khoảng 490 giây và ghi dữ liệu vào `evaluation/data/epoch_convergence_metrics.csv`.

Kết quả trong CSV:

| Group size | Add member (ms) | Remove member (ms) | Update member (ms) |
|---:|---:|---:|---:|
| 5 | 2,27 | 0,54 | 0,53 |
| 10 | 1,10 | 1,08 | 3,49 |
| 50 | 12,37 | 8,86 | 10,52 |
| 100 | 24,99 | 22,62 | 17,89 |
| 250 | 129,90 | 180,07 | 237,00 |
| 500 | 645,70 | 633,72 | 620,82 |
| 750 | 1555,11 | 1380,78 | 1519,21 |
| 1000 | 2491,75 | 2452,22 | 2336,13 |

Kết quả cho thấy chi phí thao tác thành viên tăng mạnh khi kích thước nhóm lớn. Đây là bằng chứng phù hợp với phân tích lý thuyết: mỗi thao tác membership không chỉ là một thay đổi logic ở coordination layer mà còn kéo theo việc cập nhật trạng thái nhóm và phát tán Commit cho toàn nhóm. Tuy nhiên, cần nhấn mạnh rằng test này vẫn dùng `MockMLSEngine` và `FakeNetwork`, nên các con số phản ánh chi phí mô phỏng/state-machine trong Go hơn là độ trễ OpenMLS/libp2p thật trên mạng vật lý.

## 5.6. Kết quả thí nghiệm 4: Chi phí mật mã và kích thước nhóm

Benchmark MLS optimization cho thấy baseline pairwise tăng tuyến tính rõ rệt theo số thành viên. Ở $N=4096$, median của `pairwise_baseline` là khoảng 955 ms, trong khi `current_full_blob_mls_encrypt` khoảng 79 ms. Đường hot-cache cho thao tác gửi tin nhắn chỉ khoảng 0,077 ms ở $N=4096$, cho thấy chi phí import/export full state là nút thắt lớn của production stateless path.

Một số điểm dữ liệu chính:

| N | Pairwise median (ms) | Full-blob MLS encrypt (ms) | Hot-cache encrypt (ms) | Hot-cache update commit (ms) |
|---:|---:|---:|---:|---:|
| 16 | 3,62 | 0,55 | 0,043 | 0,91 |
| 256 | 59,62 | 5,41 | 0,046 | 5,44 |
| 1024 | 238,75 | 19,86 | 0,052 | 19,11 |
| 4096 | 955,18 | 78,95 | 0,077 | 73,73 |

Kết luận nên viết thận trọng:

- MLS tốt hơn pairwise E2EE về chi phí gửi từ phía sender khi nhóm lớn.
- Production full-blob stateless path vẫn chịu overhead tăng theo kích thước state.
- Hot-cache benchmark chứng minh tiềm năng tối ưu hóa rất lớn cho gửi tin nhắn ứng dụng.
- Dữ liệu hiện tại không ủng hộ câu “update commit gần $O(\log N)$” trong triển khai đo được; phân tích fit hiện có cho thấy đường update commit vẫn gần tuyến tính trong dải benchmark. Vì vậy phần này nên trình bày như một kết quả thực nghiệm: gửi tin hot-cache gần phẳng, còn update commit vẫn cần tối ưu thêm.

## 5.7. Kết quả thí nghiệm 5: Forward Secrecy sau khi xóa thành viên

Test `TestBusinessP1_E2E_RealSidecar_ForwardSecrecy` kiểm tra đường real Rust sidecar thay vì MLS mock. Kịch bản gồm Alice tạo nhóm, Bob tham gia bằng Welcome thật, Alice remove Bob, sau đó Alice gửi một thông điệp mới ở epoch sau khi remove. Test lấy envelope thông điệp sau khi remove và ép coordinator của Bob xử lý envelope đó. Điều kiện pass là Bob không decrypt và không lưu được nội dung thông điệp mới.

Kết quả kiểm chứng sau khi sửa deadlock:

| Chỉ tiêu | Kết quả quan sát |
|---|---|
| Crypto engine | Rust sidecar thật |
| Join bằng Welcome | Thành công |
| Remove member | Thành công |
| Message sau remove | Được Alice gửi ở epoch mới |
| Bob xử lý message sau remove | Không lưu được plaintext |
| Kết quả test | PASS |

Kết quả này là bằng chứng thực nghiệm cho thuộc tính forward secrecy sau khi remove member trong đường tích hợp Go + Rust sidecar. Tuy nhiên, test vẫn ép message trực tiếp vào coordinator thay vì chứng minh toàn bộ đường truyền libp2p ngoài mạng thật; do đó nên kết luận ở mức “removed member không decrypt được thông điệp tương lai trong đường real-sidecar”, không mở rộng thành đánh giá toàn diện của toàn bộ mạng P2P.

## 5.8. Hạn chế của đánh giá

Các kết quả hiện tại đủ tốt để đưa vào đồ án nếu trình bày đúng phạm vi, nhưng cần nêu rõ hạn chế:

1. Chaos/fork-heal test dùng FakeNetwork và MockMLSEngine, phù hợp kiểm chứng thuật toán phối hợp nhưng chưa thay thế được test libp2p/OpenMLS end-to-end nhiều tiến trình.
2. Benchmark hot-cache là đường nghiên cứu/tối ưu hóa, không phải production path mặc định.
3. Test real sidecar forward secrecy đã pass sau khi sửa deadlock ở `JoinGroupWithWelcome`, nhưng harness vẫn có cảnh báo `invalid Admin signature` do các runtime test được seed bằng các Root Admin khác nhau. Cảnh báo này không làm hỏng test forward secrecy vì test truyền envelope trực tiếp vào coordinator, nhưng cần sửa riêng nếu muốn dùng harness này để đánh giá kết nối P2P/auth thật.
4. Các kết quả latency cần ghi rõ cấu hình máy, số mẫu, payload size và cách lấy median/p95/p99.

Với cách trình bày này, chương 5 vừa thể hiện được đóng góp thực nghiệm, vừa tránh overclaim so với bằng chứng hiện có trong mã nguồn.
