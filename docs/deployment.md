# Demo Control, Scripts & Docker

> Xem thêm: [Index](README.md) · [Architecture Overview](architecture-overview.md)

## Demo Control (`demo-control/`)

- Wails app riêng cho demo control
- `app.go` — demo control logic
- `app_test.go` — tests
- Frontend riêng (React + Vite)
- Build scripts cho Windows

## Scripts (`scripts/`)

| Script | Mục đích |
|--------|----------|
| `dev-second-instance.cmd/.ps1` | Chạy instance thứ 2 |
| `dev-fourth-instance.cmd/.ps1` | Chạy instance thứ 4 |
| `dev-multi-node.sh` | Chạy multi-node trên Linux |

## Docker

- `Dockerfile` — container build
- `docker-compose.yml` — multi-container orchestration
- `entrypoint.sh` — container entrypoint
