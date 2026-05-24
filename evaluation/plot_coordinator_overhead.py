import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
import os

# Paths
CSV_PATH = os.path.join("data", "coordinator_overhead_breakdown.csv")
OUTPUT_DIR = "plots"
OUTPUT_IMAGE = os.path.join(OUTPUT_DIR, "coordinator_overhead_breakdown.png")

def plot_overhead():
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

    plt.figure(figsize=(11, 7))

    # Categories and counts
    group_sizes = df['GroupSize'].astype(str).tolist()
    x = np.arange(len(group_sizes))
    width = 0.5  # Width of the bars

    # Stacks data
    crypto = df['OpenMLSCryptoMs'].values
    coord = df['CoordinatorDecisionMs'].values
    storage = df['StorageSerializationMs'].values
    p2p = df['P2PPropagationMs'].values
    
    # Sleek palette
    color_p2p = '#2CA02C'     # Soft Green (P2P Network)
    color_crypto = '#1F77B4'  # Sleek Royal Blue (OpenMLS)
    color_storage = '#FF7F0E' # Warm Orange (SQLite)
    color_coord = '#D62728'   # Crimson Red (Coordinator - highlighted tiny sliver)

    # Plotting stacked bars
    bars_p2p = plt.bar(x, p2p, width, label='Độ trễ truyền mạng P2P (P2P propagation)', color=color_p2p, edgecolor='white', linewidth=0.5)
    bars_crypto = plt.bar(x, crypto, width, bottom=p2p, label='Mã hóa/Giải mã OpenMLS (OpenMLS crypto)', color=color_crypto, edgecolor='white', linewidth=0.5)
    bars_storage = plt.bar(x, storage, width, bottom=p2p+crypto, label='Lưu trữ & Tuần tự hóa SQLite (Storage/serialization)', color=color_storage, edgecolor='white', linewidth=0.5)
    bars_coord = plt.bar(x, coord, width, bottom=p2p+crypto+storage, label='Xử lý điều phối của Coordinator (Decision overhead)', color=color_coord, edgecolor='white', linewidth=0.5)

    plt.title("PHÂN RÃ CHI PHÍ VÀ ĐỘ TRỄ HỘI TỤ THEO QUY MÔ NHÓM N\n"
              "Độ tương quan giữa Coordinator Overhead với Chi phí Mật mã & Truyền thông P2P", 
              fontsize=12, fontweight='bold', pad=18)
              
    plt.xlabel("Quy mô nhóm (Số lượng node thành viên)", fontsize=11, labelpad=10)
    plt.ylabel("Tổng thời gian hội tụ epoch tích lũy (ms)", fontsize=11, labelpad=10)
    
    plt.xticks(x, [f"N = {size}" for size in group_sizes])
    plt.grid(True, which="both", axis='y', linestyle='--', alpha=0.4, color='#CCCCCC')
    
    # Place legend cleanly
    plt.legend(loc='upper left', fontsize=9.5, frameon=True, framealpha=0.95, edgecolor='#CCCCCC')

    # Add text-based caption box emphasizing the message
    plt.figtext(0.5, -0.05, 
                "Thông điệp chính: Coordinator Decision chỉ chiếm dưới 1% tổng thời gian hội tụ Epoch (ví dụ: chỉ 2.5ms trên tổng 1017.5ms ở nhóm 1000 node).\n"
                "Overhead của cơ chế Single-Writer hoàn toàn chấp nhận được, chi phí chủ yếu nằm ở truyền thông mạng P2P và tính toán mật mã OpenMLS ở quy mô lớn.", 
                ha="center", fontsize=9.5, bbox={"facecolor":"#FFF9E6", "alpha":0.8, "pad":8, "edgecolor":"#FFE0B2", "linewidth":1}, fontstyle="italic")

    # Add text annotations dynamically on bars
    for i in range(len(group_sizes)):
        total = p2p[i] + crypto[i] + storage[i] + coord[i]
        
        # We write total height at the top of each bar
        plt.text(i, total + 12, f"{total:.1f} ms", ha='center', va='bottom', fontsize=9.5, fontweight='bold', color='#333333')
        
        # Annotate percentages inside segments if visible enough (e.g. for N >= 100)
        if total > 30:
            p2p_pct = (p2p[i] / total) * 100
            crypto_pct = (crypto[i] / total) * 100
            storage_pct = (storage[i] / total) * 100
            coord_pct = (coord[i] / total) * 100
            
            # Print segment values/percentages with neat vertical centering
            # P2P
            plt.text(i, p2p[i]/2, f"{p2p_pct:.1f}%", ha='center', va='center', fontsize=8, color='white', fontweight='bold')
            # Crypto
            plt.text(i, p2p[i] + crypto[i]/2, f"{crypto_pct:.1f}%", ha='center', va='center', fontsize=8, color='white', fontweight='bold')
            # Storage/SQLite - place only if segment is wide enough
            if storage[i] > 3:
                plt.text(i, p2p[i] + crypto[i] + storage[i]/2, f"{storage[i]:.1f}ms\n({storage_pct:.1f}%)", ha='center', va='center', fontsize=7.5, color='#333333')
            # Coordinator Decision - print text next to it because it is too small to fit inside
            plt.text(i + 0.32, total - 2, f"Coord:\n{coord[i]}ms ({coord_pct:.2f}%)", ha='left', va='center', fontsize=8, color=color_coord, fontweight='bold')

    plt.tight_layout()
    plt.savefig(OUTPUT_IMAGE, dpi=300, bbox_inches='tight')
    print(f"Success! Stacked cost chart has been saved to: {OUTPUT_IMAGE}")

if __name__ == "__main__":
    plot_overhead()
