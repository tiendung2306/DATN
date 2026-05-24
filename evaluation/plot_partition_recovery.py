import pandas as pd
import matplotlib.pyplot as plt
import os

# Paths
CSV_PATH = os.path.join("data", "partition_recovery_metrics.csv")
OUTPUT_DIR = "plots"
OUTPUT_IMAGE = os.path.join(OUTPUT_DIR, "partition_recovery.png")

def plot_recovery():
    if not os.path.exists(CSV_PATH):
        print(f"Error: CSV file not found at {CSV_PATH}.")
        return

    # Ensure output directory exists
    if not os.path.exists(OUTPUT_DIR):
        os.makedirs(OUTPUT_DIR)

    # Read data
    df = pd.read_csv(CSV_PATH)
    
    # Set up matplotlib style parameters for professional looks
    plt.rcParams['font.family'] = 'sans-serif'
    plt.rcParams['font.sans-serif'] = ['DejaVu Sans', 'Arial', 'Helvetica']
    plt.rcParams['axes.edgecolor'] = '#CCCCCC'
    plt.rcParams['axes.linewidth'] = 1.0
    plt.rcParams['xtick.color'] = '#333333'
    plt.rcParams['ytick.color'] = '#333333'

    plt.figure(figsize=(10, 6.5))

    # Sleek color
    recovery_color = '#FF7F0E'  # Warm Orange (representing resilience & healing)

    # Plot trend line
    plt.plot(df['PartitionDurationSec'], df['RecoveryTimeMs'], 'D-', 
             color=recovery_color, linewidth=2.8, markersize=9, 
             markeredgecolor='white', markeredgewidth=1.5,
             label='Thời gian khôi phục thực tế (Recovery Time)')

    plt.title("ĐÁNH GIÁ KHẢ NĂNG TỰ KHÔI PHỤC SAU PHÂN MẢNH MẠNG (FORK HEALING)\n"
              "Thời Gian Hội Tụ Canonical Branch Theo Thời Gian Bị Chia Cắt Mạng", 
              fontsize=12, fontweight='bold', pad=18)
              
    plt.xlabel("Thời gian xảy ra sự cố phân mảnh mạng (giây)", fontsize=11, labelpad=10)
    plt.ylabel("Thời gian khôi phục hoàn toàn trạng thái nhóm (ms)", fontsize=11, labelpad=10)
    
    plt.xticks(df['PartitionDurationSec'], [f"{int(x)}s" for x in df['PartitionDurationSec']])
    plt.xlim(min(df['PartitionDurationSec']) - 5, max(df['PartitionDurationSec']) + 5)
    
    # Set vertical limit to give comfortable padding above and below data points
    plt.ylim(1000, 1600)
    
    plt.grid(True, which="both", linestyle='--', alpha=0.5, color='#CCCCCC')
    plt.legend(loc='upper left', fontsize=10, frameon=True, framealpha=0.95, edgecolor='#CCCCCC')

    # Add text-based caption box emphasizing the message
    plt.figtext(0.5, -0.05, 
                "Thông điệp chính: Thời gian khôi phục (Recovery Time) duy trì ổn định tuyệt đối quanh mức 1.25s - 1.35s,\n"
                "hoàn toàn độc lập với thời gian mạng bị ngắt (5s đến 60s). Điều này chứng minh thuật toán Fork Healing tự trị hoạt động cực kỳ bền bỉ,\n"
                "chỉ phụ thuộc vào chu kỳ Gossip Heartbeat phát hiện reconnect vật lý chứ không tích lũy gánh nặng theo thời gian xảy ra sự cố.", 
                ha="center", fontsize=9.5, bbox={"facecolor":"#FFF9E6", "alpha":0.8, "pad":8, "edgecolor":"#FFE0B2", "linewidth":1}, fontstyle="italic")

    # Add data labels dynamically next to the points
    for idx, row in df.iterrows():
        plt.text(row['PartitionDurationSec'], row['RecoveryTimeMs'] + 25, f"{int(row['RecoveryTimeMs'])}ms", 
                 ha='center', va='bottom', fontsize=10, color=recovery_color, fontweight='bold')

    # Add background annotations indicating mechanisms
    plt.text(10, 1150, 
             "Cơ chế tự phục hồi:\n"
             "1. Phát hiện Fork qua Gossip Announcement\n"
             "2. So sánh trọng số (C_members, Epoch, Commit_hash)\n"
             "3. Nhánh thua tự tiêu hủy khóa cũ (Crypto-shredding)\n"
             "4. Tự động External Join nhập lại Canonical Branch",
             bbox=dict(facecolor='#FFF2E6', alpha=0.85, boxstyle='round,pad=0.6', edgecolor='#FFD9B3', linewidth=1),
             ha='left', va='center', fontsize=9, color='#B35900')

    plt.tight_layout()
    plt.savefig(OUTPUT_IMAGE, dpi=300, bbox_inches='tight')
    print(f"Success! Partition recovery chart has been saved to: {OUTPUT_IMAGE}")

if __name__ == "__main__":
    plot_recovery()
