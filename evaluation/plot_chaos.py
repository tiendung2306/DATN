import pandas as pd
import matplotlib.pyplot as plt
import os

# Đường dẫn tới file CSV (sinh ra sau khi chạy Chaos Test)
CSV_PATH = os.path.join("..", "app", "coordination", "chaos_metrics.csv")
OUTPUT_IMAGE = "convergence_chart.png"

def plot_convergence():
    if not os.path.exists(CSV_PATH):
        print(f"Lỗi: Không tìm thấy file {CSV_PATH}. Hãy chạy Chaos Test trước!")
        return

    # Đọc dữ liệu
    df = pd.read_csv(CSV_PATH)
    
    # Chuẩn hóa thời gian bắt đầu từ 0
    start_time = df['WallTimeMs'].min()
    df['RelativeTimeSec'] = (df['WallTimeMs'] - start_time) / 1000.0

    # Thiết lập biểu đồ
    plt.figure(figsize=(15, 8))
    plt.style.use('seaborn-v0_8-muted') 

    # Vẽ đường cho từng Node với một khoảng lệch nhỏ (Jitter) để tránh đè lên nhau
    nodes = sorted(df['NodeID'].unique())
    colors = ['#1f77b4', '#ff7f0e', '#2ca02c', '#d62728', '#9467bd']
    
    # Khoảng lệch Y để nhìn rõ 5 đường khi hội tụ
    # Ví dụ: Node_0 lệch 0.0, Node_1 lệch 0.05, v.v.
    Y_OFFSET_STEP = 0.05

    for i, node in enumerate(nodes):
        node_data = df[df['NodeID'] == node].copy()
        
        # Thêm offset vào Epoch để hiển thị tách biệt
        offset = i * Y_OFFSET_STEP
        display_epoch = node_data['Epoch'] + offset
        
        # Vẽ đường chính
        plt.step(node_data['RelativeTimeSec'], display_epoch, 
                 where='post', label=node, color=colors[i % len(colors)], 
                 linewidth=2.5, alpha=0.9)
        
        # Bổ sung điểm kết thúc (End-point marker)
        last_x = node_data['RelativeTimeSec'].iloc[-1]
        last_y = display_epoch.iloc[-1]
        final_val = int(node_data['Epoch'].iloc[-1])
        
        # Vẽ dấu chấm tại điểm cuối
        plt.plot(last_x, last_y, 'o', color=colors[i % len(colors)], markersize=8)
        
        # Thêm nhãn chữ (Node Name + Final Epoch)
        plt.text(last_x + 0.5, last_y, f"{node} (E:{final_val})", 
                 va='center', fontsize=10, fontweight='bold', color=colors[i % len(colors)])

    # Cấu hình trục và tiêu đề
    plt.title("Empirical Proof: MLS Epoch Convergence under Network Chaos", fontsize=18, fontweight='bold', pad=20)
    plt.xlabel("Time (seconds)", fontsize=14, labelpad=10)
    plt.ylabel("MLS Epoch (with visual offset)", fontsize=14, labelpad=10)
    
    # Hiển thị chú thích
    plt.legend(title="Network Nodes", loc='upper left', fontsize=12, frameon=True, shadow=True)
    
    # Tăng độ phân giải cho trục Y
    max_epoch = int(df['Epoch'].max())
    plt.yticks(range(max_epoch + 2))
    
    # Thêm lưới
    plt.grid(True, which='both', linestyle='--', alpha=0.5)
    
    # Lưu ảnh
    plt.savefig(OUTPUT_IMAGE, dpi=300, bbox_inches='tight')
    print(f"Thành công! Biểu đồ chuyên nghiệp đã được lưu tại: {OUTPUT_IMAGE}")
    print(f"Mẹo: Bạn sẽ thấy 5 đường kẻ song song sát nhau khi chúng có cùng Epoch.")

if __name__ == "__main__":
    plot_convergence()

if __name__ == "__main__":
    plot_convergence()
