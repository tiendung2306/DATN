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

    # Removed text-based caption box as requested

    # Add data labels dynamically next to the points
    for idx, row in df.iterrows():
        plt.text(row['PartitionDurationSec'], row['RecoveryTimeMs'] + 25, f"{int(row['RecoveryTimeMs'])}ms", 
                 ha='center', va='bottom', fontsize=10, color=recovery_color, fontweight='bold')

    # Removed background annotations as requested

    plt.tight_layout()
    plt.savefig(OUTPUT_IMAGE, dpi=300, bbox_inches='tight')
    print(f"Success! Partition recovery chart has been saved to: {OUTPUT_IMAGE}")

if __name__ == "__main__":
    plot_recovery()
