# Research Evaluation Tools

Thư mục này chứa các công cụ phục vụ việc thu thập số liệu và chứng minh nghiên cứu cho luận văn.

## 1. Vẽ biểu đồ hội tụ (Convergence Chart)

Script `plot_chaos.py` sẽ đọc dữ liệu từ file `chaos_metrics.csv` sinh ra bởi bộ Chaos Test và vẽ biểu đồ thể hiện sự thay đổi Epoch theo thời gian của các node.

### Yêu cầu cài đặt
Bạn cần cài đặt Python và các thư viện hỗ trợ:
```bash
pip install pandas matplotlib
```

### Cách sử dụng
1. Chạy Chaos Test để sinh dữ liệu mới:
   ```bash
   cd app
   go test -v ./coordination -run TestIntegration_Chaos_Convergence
   ```
2. Chạy script vẽ biểu đồ:
   ```bash
   cd evaluation
   python plot_chaos.py
   ```

### Ý nghĩa biểu đồ
- **Divergence (Phân kỳ):** Khi mạng bị chia cắt, các đường kẻ sẽ tách nhau ra (node ở nhánh thắng tăng Epoch, node ở nhánh thua đứng yên).
- **Convergence (Hội tụ):** Khi mạng được hàn gắn (Heal), các đường kẻ sẽ chập lại làm một đường duy nhất tại cùng một giá trị Epoch. Đây là bằng chứng cho thuật toán **Fork Healing** hoạt động đúng.
