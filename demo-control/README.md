# DATN Demo Control

`demo-control` là desktop control plane cho thesis demo, giờ được chia làm **2 lane độc lập**:

- **GUI Demo**: chạy `SecureP2P.exe` thật trên Windows để demo giao diện và luồng người dùng.
- **Headless Demo**: chạy node trong Docker headless để demo group chat, gửi tin, partition/heal bằng Docker network thật.

## Mục tiêu

- Không trộn semantics giữa node GUI và node Docker.
- Partition/heal chỉ xảy ra ở **Headless Demo** và chỉ bằng Docker network.
- `demo-control` đủ để điều khiển cụm headless mà không cần mở GUI của từng node.

## Hai lane chính

### GUI Demo

- Build flow riêng: Rust sidecar release + `wails build` trong `app/`.
- Chạy node bằng `SecureP2P.exe` đã build sẵn.
- Phù hợp để demo onboarding, chat UI, và thao tác người dùng cuối.
- Không có chức năng Docker partition trong lane này.

### Headless Demo

- Build flow riêng: `docker build -t secure-p2p:latest .`
- Chạy cluster Docker headless trên shared network `datn_p2p_net`.
- Có control flow `Prepare Demo Cluster` để:
  - ưu tiên chuẩn bị theo các node đang được chọn trong UI; nếu chưa chọn node nào thì fallback sang toàn bộ node headless đang chạy
  - kiểm tra chỉ các node mục tiêu đang ở `AUTHORIZED` hoặc `ADMIN_READY`
  - tạo group `demo`
  - mời các node mục tiêu còn lại vào group
  - poll roster cho đến khi cụm sẵn sàng
- Có thể:
  - gửi message vào group `demo`
  - split cluster theo selection
  - isolate một node
  - heal toàn bộ về shared network

## Control API của app

Ngoài các endpoint lifecycle/diagnostics cũ, headless demo dùng thêm lớp wrapper nghiệp vụ:

- `POST /v1/demo/create-group`
- `POST /v1/demo/invite-peer`
- `POST /v1/demo/send-message`
- `GET /v1/demo/groups`
- `GET /v1/demo/group-members`
- `GET /v1/demo/group-messages`
- `GET /v1/demo/group-status`

Các endpoint này chỉ bọc mỏng các method backend đã có như `CreateGroupChat`, `InvitePeerToGroup`, `SendGroupMessage`, `GetGroupMembers`, `GetGroupMessages`, `GetGroupStatus`.

## Workspace và templates

- Workspace mới có 2 lane: `gui_lane` và `headless_lane`.
- Runtime/template tách riêng:
  - `.demo-control/gui/runtimes/*`
  - `.demo-control/gui/templates/*`
  - `.demo-control/headless/runtimes/*`
  - `.demo-control/headless/templates/*`
- `Start` sẽ tự seed runtime từ `template_dir` đúng một lần khi runtime đang trống hoặc thiếu `app.db`.
- `Capture Runtime As Template` cho phép chốt một runtime tốt thành baseline demo.

## Notes vận hành

- `Prepare Demo Cluster` **không** làm full PKI onboarding từ đầu.
- Các node demo nên được seed từ runtime/template đã có identity + bundle hợp lệ.
- Nếu workspace cũ chỉ có `instances[]` kiểu phẳng, `demo-control` sẽ migrate sang mô hình hai lane khi startup.
- Nếu workspace legacy còn giữ path headless cũ dưới `.demo-control/runtimes` hoặc `.demo-control/templates`, `demo-control` sẽ cố gắng normalize chúng sang lane root mới `.demo-control/headless/...` khi startup; nếu phát hiện conflict nguồn/đích thì sẽ giữ nguyên metadata cũ và cảnh báo operator.

## Development

```bash
cd demo-control
wails dev
```

## Production build

```bash
cd demo-control
wails build
```
