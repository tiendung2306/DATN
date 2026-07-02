# Đánh giá & Thử nghiệm

> Xem thêm: [Index](README.md) · [Coordination Layer](coordination-layer.md) · [Service Layer](service-layer.md) · [Architecture Overview](architecture-overview.md)

## Tổng quan

Hệ thống đánh giá gồm 3 phần:
1. **Go tests** — coordination layer (chaos, concurrency, scalability, fork healing) + service layer (business integration)
2. **Python scripts** — phân tích dữ liệu benchmark, vẽ biểu đồ
3. **Evaluation data** — CSV/JSON metrics từ test runs, PNG plots cho thesis

## Scripts Python (`evaluation/`)

| Script | Mục đích |
|--------|----------|
| `plot_chaos.py` | Vẽ biểu đồ hội tụ epoch từ `chaos_metrics.csv` — chaos engineering convergence |
| `analyze_mls_optimization.py` | Phân tích tối ưu hóa MLS — stateless vs cached performance |
| `plot_concurrency.py` | Vẽ biểu đồ đồng thời — concurrent proposals same epoch |
| `plot_coordinator_overhead.py` | Vẽ biểu đồ overhead coordinator — breakdown by component |
| `plot_epoch_convergence.py` | Vẽ biểu đồ hội tụ epoch — partition recovery convergence |
| `plot_evaluation.py` | Vẽ biểu đồ đánh giá tổng hợp |
| `plot_optimization_results.py` | Vẽ biểu đồ kết quả tối ưu hóa MLS |
| `plot_partition_recovery.py` | Vẽ biểu đồ phục hồi sau partition |
| `plot_rq1.py` | Vẽ biểu đồ RQ1 — concurrency correctness |
| `plot_thesis_chapter5.py` | Vẽ biểu đồ cho thesis Chapter 5 |

## Dữ liệu đánh giá (`evaluation/data/` — 11 files)

| File | Nội dung |
|------|----------|
| `analysis_results.json` | Kết quả phân tích tổng hợp — all metrics in one JSON |
| `concurrency_metrics.csv` | Metrics đồng thời — concurrent proposals, commit latency |
| `coordinator_overhead_breakdown.csv` | Phân tích overhead coordinator — per-component timing |
| `coordinator_overhead_metrics.csv` | Coordinator overhead summary metrics |
| `epoch_convergence_metrics.csv` | Epoch convergence metrics — partition recovery timing |
| `latency_breakdown.csv` | End-to-end latency breakdown — encrypt, broadcast, decrypt |
| `mls_optimization_benchmark.csv` | Stateless vs cached MLS performance benchmark |
| `partition_divergence_metrics.csv` | Partition divergence metrics |
| `partition_recovery_metrics.csv` | Partition recovery metrics |
| `scalability_mls.csv` | Scalability metrics — group sizes 16-4096, operation latency |
| `single_writer_latency.csv` | Single-Writer commit latency metrics |

## Plots (`evaluation/plots/` — 14 PNG files)

| Plot | Mô tả |
|------|-------|
| `Evaluation_EndToEnd_Latency_Breakdown.png` | Latency breakdown: encrypt → broadcast → decrypt → ACK |
| `Evaluation_MLS_Epoch_Convergence_Network_Chaos.png` | Epoch convergence under network chaos — partition + recovery |
| `Evaluation_MLS_Scalability_O_logN.png` | MLS scalability — O(log N) tree operations vs group size |
| `Evaluation_SingleWriter_Commit_Latency_CDF.png` | Single-Writer commit latency CDF |
| `MLS_Optimization_Comparison.png` | Stateless vs cached — speedup factor per operation |
| `MLS_Optimization_Messaging_Zoom.png` | MLS optimization messaging zoom |
| `chart_a_single_writer.png` | Chart A — single-writer commit latency |
| `chart_c_breakdown.png` | Chart C — latency breakdown |
| `chart_d_scalability.png` | Chart D — scalability |
| `concurrency_chart.png` | Concurrent proposals — throughput vs batch size |
| `coordinator_overhead_breakdown.png` | Coordinator overhead — SingleWriter, EpochTracker, ForkDetector, HLC |
| `epoch_convergence.png` | Epoch convergence after partition heal |
| `partition_recovery.png` | Partition recovery time |
| `rq1_concurrency_correctness.png` | RQ1 — concurrency correctness |

## Go Tests — Coordination Layer (`app/coordination/`)

### Chaos & Convergence

| File | Nội dung |
|------|----------|
| `chaos_e2e_test.go` | Chaos engineering convergence — simulate network partitions, verify epoch convergence |
| `concurrency_evaluation_test.go` | Concurrent proposals same epoch — measure throughput, verify Single-Writer invariant |
| `scalability_evaluation_test.go` | Group sizes 16-4096 — measure MLS operation latency, verify O(log N) scaling |
| `coordinator_overhead_bench_test.go` | Benchmark coordinator overhead — per-component timing (SingleWriter, EpochTracker, etc.) |

### Fork Healing (7 files)

| File | Nội dung |
|------|----------|
| `fork_heal_phoenix_protocol_test.go` | Phoenix protocol — full fork heal lifecycle with real MLS |
| `fork_heal_crash_safety_test.go` | Crash safety — verify state recovery after crash mid-heal |
| `fork_heal_partition_sweep_test.go` | Partition sweep — various partition patterns + heal verification |
| `fork_heal_replay_robustness_test.go` | Replay robustness — autonomous replay correctness under various conditions |
| `fork_heal_bidirectional_batching_test.go` | Bidirectional batching — proposals from both branches during heal |
| `fork_heal_real_mls_bench_test.go` | Real MLS benchmark — fork heal with real Rust sidecar (not mock) |
| `fork_heal_orchestrator_test.go` | Orchestrator — multi-node fork heal coordination |

### Recovery & Offline

| File | Nội dung |
|------|----------|
| `recovery_replay_robustness_test.go` | Recovery replay — autonomous replay correctness, non-repudiation |
| `messaging_offline_blindstore_e2e_test.go` | E2E offline blind-store — message delivery via blind-store replication |
| `sidecar_helper_test.go` | Test helper — real sidecar integration setup |

## Go Tests — Service Layer (`app/service/`)

20+ business integration test files covering toàn bộ flows:

| File | Nội dung |
|------|----------|
| `business_admin_integration_test.go` | Admin key setup, bundle creation, token verification |
| `business_app_state_integration_test.go` | App state machine transitions (UNINITIALIZED → AWAITING_BUNDLE → AUTHORIZED) |
| `business_channel_categories_integration_test.go` | Channel category CRUD + P2P sync |
| `business_channel_messaging_integration_test.go` | Channel post/comment flows — structured content |
| `business_crosscutting_integration_test.go` | Cross-cutting concerns — auth, session, profile sync |
| `business_diagnostics_integration_test.go` | Diagnostic snapshot, export to file |
| `business_diagnostics_replay_integration_test.go` | Diagnostic replay — runtime event replay |
| `business_dm_realsidecar_integration_test.go` | DM with real Rust sidecar (not mock) — full MLS path |
| `business_e2e_group_integrity_test.go` | End-to-end group integrity — MLS state consistency |
| `business_fork_heal_integration_test.go` | Fork healing integration — service layer orchestration |
| `business_identity_backup_integration_test.go` | Identity export/import — encrypted backup file |
| `business_integration_harness_test.go` | Test harness — shared setup, mock sidecar, helpers |
| `business_integration_mls_mock_test.go` | MLS mock integration — mock engine for fast tests |
| `business_invite_integration_test.go` | Multi-node invite approval workflow |
| `business_invite_request_integration_test.go` | Invite request flow — request, approve, reject |
| `business_join_roster_sync_integration_test.go` | Join group + roster sync — member directory consistency |
| `business_known_peers_integration_test.go` | Known peers management — peer discovery, connection |
| `business_members_integration_test.go` | Member management — add, remove, role promotion |
| `business_messaging_integration_test.go` | Message send/receive/retry — optimistic UI, status tracking |
| `business_rejoin_integration_test.go` | Rejoin after leave — Welcome processing, state recovery |
| `business_runtime_events_integration_test.go` | Runtime event log + replay — durable event store |
| `business_runtime_lifecycle_integration_test.go` | Runtime startup/shutdown lifecycle — resource cleanup |
| `business_session_integration_test.go` | Single active device enforcement — session replacement |
| `business_sprint6_integration_test.go` | Sprint 6 integration — file transfer, channel categories |

## Go Tests — Demo Control (`demo-control/`)

| File | Nội dung |
|------|----------|
| `app_test.go` | Demo control app integration — REST API, multi-instance coordination |

## Rust Tests (`crypto-engine/`)

30+ unit tests trong `src/mls.rs`:

| Category | Tests |
|----------|-------|
| **Group creation** | `test_create_group`, `test_create_group_state_serialization` |
| **Proposal** | `test_create_add_proposal`, `test_create_remove_proposal`, `test_process_proposal` |
| **Commit** | `test_create_commit`, `test_stage_commit`, `test_process_commit`, `test_commit_with_multiple_proposals` |
| **Welcome** | `test_process_welcome`, `test_welcome_with_multiple_members` |
| **Encryption** | `test_encrypt_decrypt_message`, `test_encrypt_large_message` |
| **ExternalJoin** | `test_external_join`, `test_external_join_with_ratchet_tree` |
| **KeyPackage** | `test_generate_key_package`, `test_key_package_validation` |
| **Membership** | `test_has_member`, `test_list_member_identities`, `test_add_remove_members` |
| **ExportSecret** | `test_export_secret`, `test_export_secret_different_labels` |
| **Cached path** | `test_load_unload_group`, `test_cached_encrypt_decrypt`, `test_occ_validation` |

## Cách chạy tests

### Coordination tests

```bash
# Chaos convergence test
cd app
go test -v ./coordination -run TestIntegration_Chaos_Convergence

# Fork healing tests
go test -v ./coordination -run TestForkHeal

# Scalability evaluation
go test -v ./coordination -run TestScalability

# Concurrency evaluation
go test -v ./coordination -run TestConcurrency

# All coordination tests
go test -v ./coordination/...

# With real sidecar (requires Rust binary built)
go test -v ./coordination -run TestRealMLS -tags sidecar
```

### Service integration tests

```bash
# All business integration tests
cd app
go test -v ./service -run TestBusiness

# Specific flow
go test -v ./service -run TestBusinessMessaging

# With real sidecar
go test -v ./service -run TestBusinessDM -tags sidecar
```

### Rust tests

```bash
cd crypto-engine
cargo test

# Run specific test
cargo test test_create_group

# Run benchmark binary
cargo run --bin mls_bench
```

### Python evaluation

```bash
cd evaluation

# Plot chaos convergence
python plot_chaos.py

# Analyze MLS optimization
python analyze_mls_optimization.py

# Plot all
python plot_concurrency.py
python plot_coordinator_overhead.py
python plot_epoch_convergence.py
python plot_evaluation.py
python plot_optimization_results.py
python plot_partition_recovery.py
python plot_rq1.py
python plot_thesis_chapter5.py
```

## Test Documentation

| File | Nội dung |
|------|----------|
| `docs/testing/BUSINESS_INTEGRATION_TEST_SCENARIOS.md` | Chi tiết business integration test scenarios — step-by-step test cases |
| `docs/testing/DEMO_APP_INTEGRATION_TEST_PLAN.md` | Demo app integration test plan — multi-instance test procedures |
