import pandas as pd
import matplotlib.pyplot as plt
import os

# Set style
plt.style.use('ggplot')
script_dir = os.path.dirname(os.path.abspath(__file__))
output_dir = os.path.join(script_dir, 'plots')
os.makedirs(output_dir, exist_ok=True)
data_dir = os.path.join(script_dir, 'data')

def plot_scalability():
    print("Plotting Chart D: MLS Scalability...")
    df = pd.read_csv(os.path.join(data_dir, 'scalability_mls.csv'))
    
    # Calculate stats
    stats = df.groupby('group_size').agg({
        'encrypt_duration_ms': ['mean', 'std'],
        'add_member_duration_ms': ['mean', 'std']
    }).reset_index()
    
    fig, ax1 = plt.subplots(figsize=(10, 6))

    # Plot Add Member (O(log N)) on primary Y axis
    color = 'tab:red'
    ax1.set_xlabel('Group Size (Number of Members)')
    ax1.set_ylabel('Add Member Time (ms)', color=color)
    ax1.errorbar(stats['group_size'], stats['add_member_duration_ms']['mean'], 
                 yerr=stats['add_member_duration_ms']['std'], fmt='-o', color=color, capsize=5, label='Add Member (O(log N))')
    ax1.tick_params(axis='y', labelcolor=color)
    ax1.grid(True, alpha=0.3)

    # Create second Y axis for Encryption (O(1))
    ax2 = ax1.twinx()
    color = 'tab:blue'
    ax2.set_ylabel('Encryption Time (ms)', color=color)
    ax2.errorbar(stats['group_size'], stats['encrypt_duration_ms']['mean'], 
                 yerr=stats['encrypt_duration_ms']['std'], fmt='-s', color=color, capsize=5, label='Message Encrypt (O(1))')
    ax2.tick_params(axis='y', labelcolor=color)
    ax2.set_ylim(0, max(5, stats['encrypt_duration_ms']['mean'].max() * 2))

    # Add theoretical O(log N) curve for comparison
    import numpy as np
    x_val = np.linspace(2, stats['group_size'].max(), 100)
    # Scale log(N) to match the starting point of Add Member
    log_scale = stats['add_member_duration_ms']['mean'].iloc[0] / np.log2(2)
    y_log = log_scale * np.log2(x_val)
    ax1.plot(x_val, y_log, '--', color='grey', label='Theoretical Crypto (O(log N))')

    plt.title('MLS Scalability: Group Operations vs. Messaging')
    
    # Combined legend
    lines, labels = ax1.get_legend_handles_labels()
    lines2, labels2 = ax2.get_legend_handles_labels()
    ax2.legend(lines + lines2, labels + labels2, loc='upper left')

    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'Evaluation_MLS_Scalability_O_logN.png'), dpi=300)
    plt.close()

def plot_latency_breakdown():
    print("Plotting Chart C: Latency Breakdown...")
    df = pd.read_csv(os.path.join(data_dir, 'latency_breakdown.csv'))
    
    # Calculate averages
    avg_encrypt = df['encrypt_ms'].mean()
    avg_decrypt = df['decrypt_ms'].mean()
    avg_storage = df['storage_ms'].mean()
    avg_total = df['total_software_ms'].mean()
    
    # "Other" represents the glue logic, HLC, and IPC overhead not caught in sub-metrics
    avg_other = max(0, avg_total - (avg_encrypt + avg_decrypt + avg_storage))
    
    labels = ['MLS Encrypt', 'MLS Decrypt', 'Storage (SQLite)', 'IPC/Logic Overhead']
    values = [avg_encrypt, avg_decrypt, avg_storage, avg_other]
    
    plt.figure(figsize=(8, 6))
    plt.bar(labels, values, color=['#3498db', '#9b59b6', '#e67e22', '#95a5a6'])
    plt.ylabel('Time (ms)')
    plt.title('Software Overhead Breakdown (End-to-End)')
    plt.xticks(rotation=15)
    
    # Add text labels on top of bars
    for i, v in enumerate(values):
        plt.text(i, v + 0.05, f"{v:.2f}ms", ha='center')
        
    plt.tight_layout()
    plt.savefig(os.path.join(output_dir, 'Evaluation_EndToEnd_Latency_Breakdown.png'), dpi=300)
    plt.close()

def plot_single_writer_latency():
    print("Plotting Chart A: Single-Writer Latency...")
    df = pd.read_csv(os.path.join(data_dir, 'single_writer_latency.csv'))
    
    plt.figure(figsize=(8, 5))
    # Using a CDF plot or Histogram
    plt.hist(df['proposal_to_commit_ms'], bins=10, density=True, cumulative=True, 
             histtype='step', linewidth=2, label='CDF')
    
    plt.xlabel('Commit Latency (ms)')
    plt.ylabel('Probability')
    plt.title('Cumulative Distribution of Commit Latency (Single-Writer)')
    plt.grid(True)
    
    # Add vertical line for p95
    p95 = df['proposal_to_commit_ms'].quantile(0.95)
    plt.axvline(p95, color='r', linestyle='--', label=f'p95 = {p95:.2f}ms')
    
    plt.legend()
    plt.savefig(os.path.join(output_dir, 'Evaluation_SingleWriter_Commit_Latency_CDF.png'), dpi=300)
    plt.close()

if __name__ == "__main__":
    if not os.path.exists(data_dir):
        print(f"Error: Data directory '{data_dir}' not found.")
    else:
        plot_scalability()
        plot_latency_breakdown()
        plot_single_writer_latency()
        print(f"Done! Plots saved in '{output_dir}' directory.")
