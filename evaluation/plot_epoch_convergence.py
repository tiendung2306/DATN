import pandas as pd
import matplotlib.pyplot as plt
import os

# Paths
CSV_PATH = os.path.join("data", "epoch_convergence_metrics.csv")
OUTPUT_DIR = "plots"
OUTPUT_IMAGE = os.path.join(OUTPUT_DIR, "epoch_convergence.png")

def plot_epoch_convergence():
    if not os.path.exists(CSV_PATH):
        print(f"Error: CSV file not found at {CSV_PATH}. Run the Go sweep test first!")
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

    # Sleek colors
    add_color = '#1F77B4'      # Sleek Blue (Add Member)
    remove_color = '#D62728'   # Crimson Red (Remove Member)
    update_color = '#2E7D32'   # Emerald Green (Update Member)

    # Plot lines
    plt.plot(df['GroupSize'], df['AddMemberMs'], 'o-', 
             label='Thêm Thành Viên (Add Member)', 
             color=add_color, linewidth=2.5, markersize=8, markeredgecolor='white', markeredgewidth=1.5)
             
    plt.plot(df['GroupSize'], df['RemoveMemberMs'], 's-', 
             label='Xóa Thành Viên (Remove Member)', 
             color=remove_color, linewidth=2.5, markersize=8, markeredgecolor='white', markeredgewidth=1.5)

    plt.plot(df['GroupSize'], df['UpdateMemberMs'], '^-', 
             label='Cập Nhật Khóa/Tin Nhắn (Update/Commit)', 
             color=update_color, linewidth=2.5, markersize=8, markeredgecolor='white', markeredgewidth=1.5)

    plt.xscale('log')
    plt.xticks(df['GroupSize'], [str(x) for x in df['GroupSize']])

    plt.title("ĐÁNH GIÁ HIỆU NĂNG MỞ RỘNG QUY MÔ CỦA COORDINATION LAYER\n"
              "Độ trễ Hội Tụ Epoch Toàn Node Theo Quy Mô Nhóm N", 
              fontsize=12, fontweight='bold', pad=15)
              
    plt.xlabel("Số lượng thành viên trong nhóm (N) - Thang Log", fontsize=11, labelpad=10)
    plt.ylabel("Thời gian hội tụ epoch trung bình (ms)", fontsize=11, labelpad=10)
    
    plt.grid(True, which="both", linestyle='--', alpha=0.5, color='#E0E0E0')
    plt.legend(loc='upper left', fontsize=10, frameon=True, framealpha=0.95, edgecolor='#CCCCCC', shadow=False)

    # Add text-based caption box emphasizing the message
    plt.figtext(0.5, -0.05, 
                "Thông điệp chính: Quy mô nhóm càng lớn thì thời gian hội tụ epoch càng tăng (do số lượng chữ ký, gRPC IPC và I/O SQLite cần đồng bộ tăng lên),\n"
                "tuy nhiên mức độ tăng trưởng tiệm cận tuyến tính và hoàn toàn nằm trong ngưỡng chấp nhận được (thời gian khoảng 1 giây đối với nhóm 1000 node giả lập).", 
                ha="center", fontsize=9.5, bbox={"facecolor":"#FFF9E6", "alpha":0.8, "pad":8, "edgecolor":"#FFE0B2", "linewidth":1}, fontstyle="italic")

    # Add data labels dynamically above the points
    # Since values might be very small, let's adjust vertical offset based on Y values
    y_max = max(df['AddMemberMs'].max(), df['RemoveMemberMs'].max(), df['UpdateMemberMs'].max())
    offset_up = y_max * 0.04
    offset_down = y_max * 0.06

    for idx, row in df.iterrows():
        # Annotate key interesting points to avoid text clutter (e.g. 5, 100, 1000)
        if row['GroupSize'] in [5, 100, 1000]:
            plt.text(row['GroupSize'], row['AddMemberMs'] + offset_up, f"{row['AddMemberMs']:.2f}ms", 
                     ha='center', va='bottom', fontsize=9, color=add_color, fontweight='bold')
            plt.text(row['GroupSize'], row['RemoveMemberMs'] - offset_down, f"{row['RemoveMemberMs']:.2f}ms", 
                     ha='center', va='top', fontsize=9, color=remove_color, fontweight='bold')
            plt.text(row['GroupSize'], row['UpdateMemberMs'] + offset_up, f"{row['UpdateMemberMs']:.2f}ms", 
                     ha='center', va='bottom', fontsize=9, color=update_color, fontweight='bold')

    plt.tight_layout()
    plt.savefig(OUTPUT_IMAGE, dpi=300, bbox_inches='tight')
    print(f"Success! Scalability chart has been saved to: {OUTPUT_IMAGE}")

if __name__ == "__main__":
    plot_epoch_convergence()
