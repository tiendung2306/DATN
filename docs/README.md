# Project Documentation Index

> **Dự án:** Decentralized Coordination Protocol wrapping MLS (RFC 9420) for P2P Networks  
> **Loại:** Khóa luận tốt nghiệp — Nghiên cứu + Ứng dụng

Tài liệu phân tích chi tiết toàn bộ dự án, được tách thành các file chuyên biệt:

## Architecture

| File | Nội dung |
|------|----------|
| [architecture-overview.md](architecture-overview.md) | Tổng quan kiến trúc hexagonal, sơ đồ layers, cấu trúc thư mục |
| [coordination-layer.md](coordination-layer.md) | Lớp Coordination Protocol: Coordinator, SingleWriter, EpochTracker, HLC, ActiveView, ForkDetector, Commit/Application/Heal handling |
| [adapter-layer.md](adapter-layer.md) | Lớp Adapter: P2P (libp2p), Sidecar (gRPC), Store (SQLite) |
| [service-layer.md](service-layer.md) | Lớp Service: Runtime orchestration, group ops, messaging, identity, session, invite |
| [admin-config.md](admin-config.md) | Admin key management, InvitationToken, CLI config, domain types |

## Crypto & Protocol

| File | Nội dung |
|------|----------|
| [crypto-engine.md](crypto-engine.md) | Rust crypto engine: OpenMLS, tonic gRPC server, stateless vs cached RPCs |
| [security-protocol.md](security-protocol.md) | Security rules, protocol invariants, crypto choices, offline handling, key retention |

## Frontend & Storage

| File | Nội dung |
|------|----------|
| [frontend.md](frontend.md) | React + Wails frontend: feature-first structure, Zustand stores, runtimeClient |
| [storage.md](storage.md) | SQLite storage: configuration, schema, stateless persistence pattern |

## Flows & Testing

| File | Nội dung |
|------|----------|
| [flows.md](flows.md) | Luồng hoạt động chính: onboarding, group creation, message send, commit, fork healing |
| [evaluation-testing.md](evaluation-testing.md) | Đánh giá: Python scripts, benchmark data, Go tests, integration tests |
| [deployment.md](deployment.md) | Demo control app, dev scripts, Docker |

## Existing Docs

| File | Nội dung |
|------|----------|
| [single_writer_protocol.md](single_writer_protocol.md) | Chi tiết giao thức Single-Writer (có sẵn) |
| [testing/](testing/) | Kế hoạch test integration (có sẵn) |
