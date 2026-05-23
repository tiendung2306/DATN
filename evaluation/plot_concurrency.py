import pandas as pd
import matplotlib.pyplot as plt
import os

# Paths
CSV_PATH = os.path.join("data", "concurrency_metrics.csv")
OUTPUT_DIR = "plots"
OUTPUT_IMAGE = os.path.join(OUTPUT_DIR, "concurrency_chart.png")

def plot_concurrency():
    if not os.path.exists(CSV_PATH):
        print(f"Error: CSV file not found at {CSV_PATH}. Run the Go sweep test first!")
        return

    # Ensure output directory exists
    if not os.path.exists(OUTPUT_DIR):
        os.makedirs(OUTPUT_DIR)

    # Read data
    df = pd.read_csv(CSV_PATH)
    
    # Split into Baseline and Optimized
    baseline_df = df[df['Strategy'] == 'Baseline'].sort_values('Concurrency')
    optimized_df = df[df['Strategy'] == 'Optimized'].sort_values('Concurrency')

    # Convert success rate to percentage
    baseline_df['SuccessPercent'] = baseline_df['SuccessRate'] * 100.0
    optimized_df['SuccessPercent'] = optimized_df['SuccessRate'] * 100.0

    # Set up matplotlib style for professional looks
    plt.style.use('seaborn-v0_8-muted')
    fig, (ax1, ax2) = plt.subplots(1, 2, figsize=(16, 7))

    # Harmonious colors
    baseline_color = '#d62728'  # Sleek Red
    optimized_color = '#1f77b4' # Sleek Blue

    # ----------------- Subplot 1: Number of Commits -----------------
    ax1.plot(baseline_df['Concurrency'], baseline_df['Commits'], 
             'o--', label='Baseline (Immediate Commit)', color=baseline_color, 
             linewidth=2.5, markersize=8)
    ax1.plot(optimized_df['Concurrency'], optimized_df['Commits'], 
             's-', label='Optimized (1s Batching Delay)', color=optimized_color, 
             linewidth=2.5, markersize=8)
    
    ax1.set_title("1. Số lượng MLS Commit Thực Tế Phát Hành", fontsize=14, fontweight='bold', pad=15)
    ax1.set_xlabel("Mức độ đồng thời (Số node gửi proposal cùng lúc)", fontsize=12, labelpad=10)
    ax1.set_ylabel("Số lượng Commits", fontsize=12, labelpad=10)
    ax1.set_xticks(df['Concurrency'].unique())
    ax1.set_yticks(range(0, int(df['Commits'].max()) + 2))
    ax1.grid(True, linestyle='--', alpha=0.5)
    ax1.legend(loc='upper left', fontsize=11, frameon=True, shadow=True)

    # Add data annotations on points for Subplot 1
    for x, y in zip(baseline_df['Concurrency'], baseline_df['Commits']):
        ax1.text(x, y + 0.25, f"{int(y)}", ha='center', va='bottom', fontsize=10, color=baseline_color, fontweight='bold')
    for x, y in zip(optimized_df['Concurrency'], optimized_df['Commits']):
        ax1.text(x, y - 0.35, f"{int(y)}", ha='center', va='top', fontsize=10, color=optimized_color, fontweight='bold')

    # ----------------- Subplot 2: First-Attempt Success Rate -----------------
    ax2.plot(baseline_df['Concurrency'], baseline_df['SuccessPercent'], 
             'o--', label='Baseline (Immediate Commit)', color=baseline_color, 
             linewidth=2.5, markersize=8)
    ax2.plot(optimized_df['Concurrency'], optimized_df['SuccessPercent'], 
             's-', label='Optimized (1s Batching Delay)', color=optimized_color, 
             linewidth=2.5, markersize=8)
    
    ax2.set_title("2. Tỷ Lệ Đề Xuất Thành Công Ngay Lần Đầu", fontsize=14, fontweight='bold', pad=15)
    ax2.set_xlabel("Mức độ đồng thời (Số node gửi proposal cùng lúc)", fontsize=12, labelpad=10)
    ax2.set_ylabel("Tỷ lệ thành công (%)", fontsize=12, labelpad=10)
    ax2.set_xticks(df['Concurrency'].unique())
    ax2.set_ylim(0, 110)
    ax2.grid(True, linestyle='--', alpha=0.5)
    ax2.legend(loc='lower left', fontsize=11, frameon=True, shadow=True)

    # Add data annotations on points for Subplot 2
    for x, y in zip(baseline_df['Concurrency'], baseline_df['SuccessPercent']):
        ax2.text(x, y + 3, f"{y:.1f}%", ha='center', va='bottom', fontsize=10, color=baseline_color, fontweight='bold')
    for x, y in zip(optimized_df['Concurrency'], optimized_df['SuccessPercent']):
        ax2.text(x, y - 6, f"{y:.1f}%", ha='center', va='top', fontsize=10, color=optimized_color, fontweight='bold')

    # Overall Layout Polish
    plt.suptitle("ĐÁNH GIÁ HIỆU NĂNG PHÂN PHỐI ĐỀ XUẤT MLS (CONCURRENCY SWEEP EVALUATION)\n"
                 "So sánh Cơ chế Commit Ngay lập tức (Baseline) vs Gom Batch 1 Giây (Optimized)", 
                 fontsize=16, fontweight='bold', y=1.02)
    plt.tight_layout()
    
    # Save Image
    plt.savefig(OUTPUT_IMAGE, dpi=300, bbox_inches='tight')
    print(f"Success! Academic chart has been saved to: {OUTPUT_IMAGE}")

if __name__ == "__main__":
    plot_concurrency()
