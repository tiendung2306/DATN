import pandas as pd
import matplotlib.pyplot as plt
import numpy as np
import os

# Set style
plt.style.use('ggplot')
script_dir = os.path.dirname(os.path.abspath(__file__))
output_dir = os.path.join(script_dir, 'plots')
os.makedirs(output_dir, exist_ok=True)
data_file = os.path.join(script_dir, 'data', 'mls_optimization_benchmark.csv')

def plot_optimization():
    if not os.path.exists(data_file):
        print(f"Error: Data file '{data_file}' not found.")
        return

    print(f"Reading data from {data_file}...")
    df = pd.read_csv(data_file)
    
    plt.figure(figsize=(12, 8))
    
    label_map = {
        'pairwise_baseline': 'Mô hình cũ: Mã hóa 1-1 (VD: Signal)',
        'current_full_blob_mls_encrypt': 'Kiến trúc MLS cũ (Thắt cổ chai I/O)',
        'hot_cache_sidecar_encrypt_core': 'Nhắn Tin (Data Plane)',
        'hot_cache_sidecar_update_commit_core': 'Tổng Thời gian Thêm Người',
        'hot_cache_update_self_update_part': 'Thêm Người: Tạo Commit/Welcome',
        'hot_cache_update_merge_pending_part': 'Thêm Người: Hợp nhất (Merge Tree)',
    }
    
    # We want to ignore serialization in the main plot to reduce noise
    exclude_ops = ['hot_cache_update_serialize_commit_part', 'hot_cache_sidecar_update_commit_core']
    
    operations = [op for op in df['operation'].unique() if op not in exclude_ops]
    # Re-add the total commit if we want to show it, or we can just plot the breakdown. Let's show total for context.
    operations.insert(0, 'hot_cache_sidecar_update_commit_core')
    
    colors = ['tab:blue', 'tab:orange', 'tab:green', 'tab:red', 'tab:purple', 'tab:cyan', 'tab:brown']
    markers = ['o', 's', '^', 'D', 'x', '*', '+']
    
    for i, op in enumerate(operations):
        op_data = df[df['operation'] == op].sort_values('n')
        display_label = label_map.get(op, op)
        plt.plot(op_data['n'], op_data['median_ms'], marker=markers[i % len(markers)], 
                 color=colors[i % len(colors)], label=display_label, linewidth=2)
        
    plt.xscale('log', base=2)
    plt.xlabel('Số lượng thành viên trong nhóm (N)', fontsize=12)
    plt.ylabel('Thời gian xử lý / Độ trễ (mili-giây)', fontsize=12)
    plt.title('Biểu đồ 1: So sánh Hiệu năng Kiến trúc Bộ nhớ đệm (Stateful)', fontsize=14, fontweight='bold')
    plt.grid(True, which="both", ls="-", alpha=0.5)
    plt.legend(fontsize=10)
    
    plt.tight_layout()
    output_path = os.path.join(output_dir, 'MLS_Optimization_Comparison.png')
    plt.savefig(output_path, dpi=300)
    print(f"Plot saved to {output_path}")

    # Create a zoomed-in plot for the fast operations
    plt.figure(figsize=(12, 8))
    fast_ops = ['hot_cache_sidecar_encrypt_core', 'pairwise_baseline']
    for i, op in enumerate(fast_ops):
        if op in df['operation'].values:
            op_data = df[df['operation'] == op].sort_values('n')
            display_label = label_map.get(op, op)
            plt.plot(op_data['n'], op_data['median_ms'], marker=markers[i], 
                     color=colors[i], label=display_label, linewidth=2)

    plt.xscale('log', base=2)
    plt.xlabel('Số lượng thành viên trong nhóm (N)', fontsize=12)
    plt.ylabel('Thời gian xử lý / Độ trễ (mili-giây)', fontsize=12)
    plt.title('Biểu đồ 2: So sánh Tác vụ Nhắn tin cốt lõi (Data Plane)', fontsize=14, fontweight='bold')
    plt.grid(True, which="both", ls="-", alpha=0.5)
    plt.legend(fontsize=10)
    
    plt.tight_layout()
    output_path_zoom = os.path.join(output_dir, 'MLS_Optimization_Messaging_Zoom.png')
    plt.savefig(output_path_zoom, dpi=300)
    print(f"Zoomed plot saved to {output_path_zoom}")

if __name__ == "__main__":
    plot_optimization()
