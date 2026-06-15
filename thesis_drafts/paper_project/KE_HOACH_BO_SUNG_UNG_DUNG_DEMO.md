# Kế hoạch bổ sung ứng dụng demo vào đồ án

## Mục tiêu chỉnh sửa
- [x] Đưa ứng dụng demo trở thành một đầu ra chính thức của đồ án, song song với phần giao thức.
- [x] Bổ sung nội dung vào phần tóm tắt, Chương 1, Chương 3, Chương 5 và Chương 6 để làm rõ vai trò của ứng dụng desktop.
- [x] Tạo phụ lục riêng cho bộ ảnh ứng dụng demo.
- [x] Bổ sung vào phụ lục phần mô tả khối lượng hiện thực của ứng dụng desktop, không để phụ lục chỉ còn vai trò đặt ảnh.
- [x] Cài cơ chế placeholder để tài liệu vẫn biên dịch được khi chưa có ảnh thật.
- [ ] Thay toàn bộ placeholder bằng ảnh chụp thật sau khi hoàn tất quá trình chụp màn hình.

## Các file sẽ sửa
- [x] `thesis_drafts/paper_project/DoAn.tex`
- [x] `thesis_drafts/paper_project/Chuong/0_3_Tom_tat_noi_dung.tex`
- [x] `thesis_drafts/paper_project/Chuong/1_Gioi_thieu.tex`
- [x] `thesis_drafts/paper_project/Chuong/3_De_xuat.tex`
- [x] `thesis_drafts/paper_project/Chuong/5_Thuc_nghiem.tex`
- [x] `thesis_drafts/paper_project/Chuong/6_Ket_luan.tex`
- [x] `thesis_drafts/paper_project/Chuong/Phu_luc_Ung_dung_demo.tex`
- [x] `thesis_drafts/paper_project/Hinh/app_demo/README.md`
- [x] `thesis_drafts/paper_project/KE_HOACH_BO_SUNG_UNG_DUNG_DEMO.md`

## Danh sách ảnh bắt buộc
- [ ] `app_01_welcome.png`
- [ ] `app_02_awaiting_bundle_alice.png`
- [ ] `app_03_admin_issue_bundle_admin.png`
- [ ] `app_04_workspace_overview_alice.png`
- [ ] `app_05_group_chat_admin.png`
- [ ] `app_05_group_chat_alice.png`
- [ ] `app_05_group_chat_bob.png`
- [ ] `app_06_group_management_alice.png`

Ảnh nên chụp thêm nếu còn thời gian:
- [ ] `app_07_activity_alice.png`
- [ ] `app_10_channel_post_alice.png`
- [ ] `app_11_create_group_modal_alice.png`
- [ ] `app_12_add_member_modal_alice.png`
- [ ] `app_13_diagnostics_network_alice.png`
- [ ] `app_14_group_diagnostics_alice.png`
- [ ] `app_15_admin_quick_setup.png`

## Chú thích dự kiến cho từng ảnh
- `app_01_welcome.png`: Màn hình khởi tạo định danh cục bộ của ứng dụng thử nghiệm.
- `app_02_awaiting_bundle_alice.png`: Thiết bị thành viên ở trạng thái chờ bundle.
- `app_03_admin_issue_bundle_admin.png`: Bảng điều khiển quản trị phát hành bundle cho thiết bị mới.
- `app_04_workspace_overview_alice.png`: Tổng quan giao diện chính của ứng dụng desktop.
- `app_05_group_chat_admin.png`, `app_05_group_chat_alice.png`, `app_05_group_chat_bob.png`: Kịch bản trao đổi tin nhắn nhóm trên nhiều nút của ứng dụng thử nghiệm.
- `app_06_group_management_alice.png`: Màn hình quản trị thành viên và chính sách mời trong nhóm.
- `app_07_activity_alice.png`: Màn hình hoạt động và thông báo của người dùng.
- `app_10_channel_post_alice.png`: Không gian kênh theo mô hình bài viết -- phản hồi.
- `app_11_create_group_modal_alice.png`: Hộp thoại tạo nhóm, DM hoặc kênh.
- `app_12_add_member_modal_alice.png`: Hộp thoại mời thêm thành viên vào nhóm.
- `app_13_diagnostics_network_alice.png`: Màn hình chẩn đoán mạng và trạng thái cục bộ.
- `app_14_group_diagnostics_alice.png`: Màn hình chẩn đoán nhóm hoặc lịch sử fork-heal.
- `app_15_admin_quick_setup.png`: Màn hình tạo tổ chức gốc của quản trị viên.

## Checklist chụp và chèn ảnh
- [ ] Chuẩn bị bộ dữ liệu demo nhất quán với ba định danh `Admin`, `Alice`, `Bob`.
- [ ] Chụp `Welcome` và `Admin Quick Setup`.
- [ ] Chụp `Awaiting Bundle` của `Alice`.
- [ ] Chụp `Admin Panel` khi đã mở khóa và có ít nhất một mục trong `Issuance History`.
- [ ] Chụp giao diện chính tổng quan của `Alice` với thanh bên có dữ liệu thật.
- [ ] Chụp ba cửa sổ group chat của `Admin`, `Alice`, `Bob` trong cùng một thời điểm logic.
- [ ] Chụp màn hình quản trị nhóm hoặc `Group Settings`.
- [ ] Chụp thêm `Activity`, `Channel Post`, `Create Group`, `Add Member`, `Diagnostics` nếu muốn phụ lục đầy đủ hơn.
- [ ] Chép ảnh vào `thesis_drafts/paper_project/Hinh/app_demo/` đúng tên file đã định.
- [ ] Biên dịch lại `DoAn.tex` để kiểm tra placeholder đã được thay bằng ảnh thật.
- [ ] Rà soát lại caption, tham chiếu `Hình~\ref{...}` và mật độ hình trong Chương 5 trước khi chốt bản in.
