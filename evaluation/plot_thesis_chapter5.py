import pandas as pd
import matplotlib.pyplot as plt
import matplotlib
import os

matplotlib.rcParams.update({
    'font.size': 12,
    'font.family': 'sans-serif',
    'font.sans-serif': ['DejaVu Sans', 'Arial', 'Helvetica'],
    'axes.titlesize': 14,
    'axes.labelsize': 13,
    'xtick.labelsize': 11,
    'ytick.labelsize': 11,
    'legend.fontsize': 11,
    'figure.dpi': 150,
})

DATA_DIR = os.path.join(os.path.dirname(__file__), "data")
OUT_DIR = os.path.join(os.path.dirname(__file__), "..", "thesis_drafts", "paper_project", "Hinh", "chuong5")
os.makedirs(OUT_DIR, exist_ok=True)

# ── Plot 1: Partition recovery time vs partition duration ──
df_rec = pd.read_csv(os.path.join(DATA_DIR, "partition_recovery_metrics.csv"))
fig, ax = plt.subplots(figsize=(7, 4.5))
ax.plot(df_rec["PartitionDurationSec"], df_rec["RecoveryTimeMs"], "o-", color="#1f77b4", linewidth=2, markersize=8)
ax.set_xlabel("Thời lượng phân vùng (giây)")
ax.set_ylabel("Thời gian phục hồi (ms)")
ax.set_title("Thời gian phục hồi sau phân vùng mạng")
ax.grid(True, alpha=0.3)
ax.set_ylim(bottom=0)
fig.tight_layout()
fig.savefig(os.path.join(OUT_DIR, "partition_recovery.png"))
plt.close(fig)

# ── Plot 2: Thundering herd — Commits & Success Rate vs Concurrency ──
df_con = pd.read_csv(os.path.join(DATA_DIR, "concurrency_metrics.csv"))
fig, ax1 = plt.subplots(figsize=(7, 4.5))
baseline = df_con[df_con["Strategy"] == "Baseline"]
optimized = df_con[df_con["Strategy"] == "Optimized"]

ax1.set_xlabel("Số nút đồng thực hiện heal")
ax1.set_ylabel("Số Commits cần tạo")
l1 = ax1.plot(baseline["Concurrency"], baseline["Commits"], "s--", color="#d62728", linewidth=2, markersize=8, label="Baseline (External Commit) - Commits")
l2 = ax1.plot(optimized["Concurrency"], optimized["Commits"], "o-", color="#2ca02c", linewidth=2, markersize=8, label="External Proposal - Commits")
ax1.tick_params(axis="y")
ax1.set_ylim(bottom=0)

ax2 = ax1.twinx()
ax2.set_ylabel("Tỷ lệ thành công")
l3 = ax2.plot(baseline["Concurrency"], baseline["SuccessRate"], "s--", color="#d62728", linewidth=1.5, markersize=6, alpha=0.5, label="Baseline - Success Rate")
l4 = ax2.plot(optimized["Concurrency"], optimized["SuccessRate"], "o-", color="#2ca02c", linewidth=1.5, markersize=6, alpha=0.5, label="External Proposal - Success Rate")
ax2.set_ylim(0, 1.1)
ax2.tick_params(axis="y")

lines = l1 + l2 + l3 + l4
labels = [l.get_label() for l in lines]
ax1.legend(lines, labels, loc="center left", fontsize=9)
ax1.set_title("So sánh thundering herd: External Proposal vs External Commit")
ax1.grid(True, alpha=0.3)
fig.tight_layout()
fig.savefig(os.path.join(OUT_DIR, "thundering_herd.png"))
plt.close(fig)

# ── Plot 3a: Partition divergence — Healing time + Group state size (dual axis) ──
df_div = pd.read_csv(os.path.join(DATA_DIR, "partition_divergence_metrics.csv"))
fig, ax1 = plt.subplots(figsize=(7, 4.5))
ax1.set_xlabel("Độ sâu phân kỳ D (số epoch lệch)")
ax1.set_ylabel("Thời gian healing (ms)", color="#ff7f0e")
l1 = ax1.plot(df_div["DivergenceDepth"], df_div["HealingTimeMs"], "o-", color="#ff7f0e", linewidth=2, markersize=8, label="Healing time (ms)")
ax1.tick_params(axis="y", labelcolor="#ff7f0e")
ax1.set_ylim(bottom=0)
ax1.grid(True, alpha=0.3)

ax2 = ax1.twinx()
ax2.set_ylabel("Kích thước Group State (KB)", color="#1f77b4")
l2 = ax2.plot(df_div["DivergenceDepth"], df_div["GroupStateBytes"] / 1024, "s--", color="#1f77b4", linewidth=2, markersize=7, label="Group State (KB)")
ax2.tick_params(axis="y", labelcolor="#1f77b4")
ax2.set_ylim(bottom=0)

lines = l1 + l2
labels = [l.get_label() for l in lines]
ax1.legend(lines, labels, loc="upper left", fontsize=10)
ax1.set_title("Healing time và Group State size theo độ sâu phân kỳ")
fig.tight_layout()
fig.savefig(os.path.join(OUT_DIR, "partition_divergence.png"))
plt.close(fig)

# ── Plot 3b: Normalized healing time (ms per KB of group state) — demonstrates O(1) protocol ──
df_div["NormalizedMsPerKB"] = df_div["HealingTimeMs"] / (df_div["GroupStateBytes"] / 1024)
fig, ax = plt.subplots(figsize=(7, 4.5))
ax.plot(df_div["DivergenceDepth"], df_div["NormalizedMsPerKB"], "D-", color="#2ca02c", linewidth=2, markersize=8)
ax.set_xlabel("Độ sâu phân kỳ D (số epoch lệch)")
ax.set_ylabel("Healing time / Group State size (ms/KB)")
ax.set_title("Healing time chuẩn hóa — complexity O(1) của protocol")
ax.grid(True, alpha=0.3)
ax.set_ylim(bottom=0)
fig.tight_layout()
fig.savefig(os.path.join(OUT_DIR, "partition_divergence_normalized.png"))
plt.close(fig)

# ── Plot 4: Coordinator overhead — Mock vs Real (crypto) ──
df_oh = pd.read_csv(os.path.join(DATA_DIR, "coordinator_overhead_metrics.csv"))
df_real = df_oh.dropna(subset=["RealMs"])
fig, ax = plt.subplots(figsize=(7, 4.5))
ax.plot(df_oh["GroupSize"], df_oh["MockMs"], "s--", color="#1f77b4", linewidth=2, markersize=7, label="Mock (không crypto thật)")
ax.plot(df_real["GroupSize"], df_real["RealMs"], "o-", color="#d62728", linewidth=2, markersize=7, label="Real (có crypto MLS)")
ax.set_xlabel("Kích thước nhóm")
ax.set_ylabel("Thời gian (ms)")
ax.set_title("Chi phí phụ trợ của lớp điều phối: Mock vs Real")
ax.set_xscale("log", base=2)
ax.set_yscale("log")
ax.legend()
ax.grid(True, alpha=0.3, which="both")
fig.tight_layout()
fig.savefig(os.path.join(OUT_DIR, "coordinator_overhead_real.png"))
plt.close(fig)

print("Done: 5 plots saved to", OUT_DIR)
