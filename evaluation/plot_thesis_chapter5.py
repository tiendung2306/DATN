import pandas as pd
import matplotlib.pyplot as plt
import matplotlib
import os

matplotlib.rcParams.update({
    'font.size': 12,
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
ax.set_xlabel("Thoi luong phan vung (giay)")
ax.set_ylabel("Thoi gian phuc hoi (ms)")
ax.set_title("Thoi gian phuc hoi sau phan vung mang")
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

ax1.set_xlabel("So nut dong thuc hien heal")
ax1.set_ylabel("So Commits can tao")
l1 = ax1.plot(baseline["Concurrency"], baseline["Commits"], "s--", color="#d62728", linewidth=2, markersize=8, label="Baseline (External Commit) - Commits")
l2 = ax1.plot(optimized["Concurrency"], optimized["Commits"], "o-", color="#2ca02c", linewidth=2, markersize=8, label="External Proposal - Commits")
ax1.tick_params(axis="y")
ax1.set_ylim(bottom=0)

ax2 = ax1.twinx()
ax2.set_ylabel("Ti le thanh cong")
l3 = ax2.plot(baseline["Concurrency"], baseline["SuccessRate"], "s--", color="#d62728", linewidth=1.5, markersize=6, alpha=0.5, label="Baseline - Success Rate")
l4 = ax2.plot(optimized["Concurrency"], optimized["SuccessRate"], "o-", color="#2ca02c", linewidth=1.5, markersize=6, alpha=0.5, label="External Proposal - Success Rate")
ax2.set_ylim(0, 1.1)
ax2.tick_params(axis="y")

lines = l1 + l2 + l3 + l4
labels = [l.get_label() for l in lines]
ax1.legend(lines, labels, loc="center left", fontsize=9)
ax1.set_title("So sanh thundering herd: External Proposal vs External Commit")
ax1.grid(True, alpha=0.3)
fig.tight_layout()
fig.savefig(os.path.join(OUT_DIR, "thundering_herd.png"))
plt.close(fig)

# ── Plot 3: Partition divergence depth vs healing time ──
df_div = pd.read_csv(os.path.join(DATA_DIR, "partition_divergence_metrics.csv"))
fig, ax = plt.subplots(figsize=(7, 4.5))
ax.plot(df_div["DivergenceDepth"], df_div["HealingTimeMs"], "o-", color="#ff7f0e", linewidth=2, markersize=8)
ax.set_xlabel("Do sau phan ky (so epoch lech)")
ax.set_ylabel("Thoi gian healing (ms)")
ax.set_title("Anh huong do sau phan ky den thoi gian healing")
ax.grid(True, alpha=0.3)
ax.set_ylim(bottom=0)
fig.tight_layout()
fig.savefig(os.path.join(OUT_DIR, "partition_divergence.png"))
plt.close(fig)

# ── Plot 4: Coordinator overhead — Mock vs Real (crypto) ──
df_oh = pd.read_csv(os.path.join(DATA_DIR, "coordinator_overhead_metrics.csv"))
df_real = df_oh.dropna(subset=["RealMs"])
fig, ax = plt.subplots(figsize=(7, 4.5))
ax.plot(df_oh["GroupSize"], df_oh["MockMs"], "s--", color="#1f77b4", linewidth=2, markersize=7, label="Mock (khong crypto that)")
ax.plot(df_real["GroupSize"], df_real["RealMs"], "o-", color="#d62728", linewidth=2, markersize=7, label="Real (co crypto MLS)")
ax.set_xlabel("Kich thuoc nhom")
ax.set_ylabel("Thoi gian (ms)")
ax.set_title("Chi phu tro cua lop dieu phoi: Mock vs Real")
ax.set_xscale("log", base=2)
ax.set_yscale("log")
ax.legend()
ax.grid(True, alpha=0.3, which="both")
fig.tight_layout()
fig.savefig(os.path.join(OUT_DIR, "coordinator_overhead_real.png"))
plt.close(fig)

print("Done: 4 plots saved to", OUT_DIR)
