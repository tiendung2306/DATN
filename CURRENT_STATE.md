# Current State of SECURE PRIVATE P2P COMMUNICATION SYSTEM Project

This document serves as a short-term memory for the AI Agent, providing a snapshot of the project's current status, completed tasks, and immediate next steps. It is updated at the end of each session and read at the beginning of a new one to maintain context.

## 1. Project Overview

The project is a **Secure Private P2P Communication System**, a graduation thesis focusing on a serverless, zero-trust internal communication platform using a **Pure P2P architecture** and the **MLS (Messaging Layer Security)** protocol. It employs a **Local-First, Sidecar Architecture** with a **Go host application (Wails)** managing a **headless Rust cryptographic engine** via gRPC over localhost.

## 2. Completed Tasks (as per PROJECT_PLAN.md - Section 1.1 to 1.4)

The following tasks have been successfully completed:

*   **Project Infrastructure:**

    *   Initialized Git repository and configured remote origin.

    *   Created a comprehensive root `.gitignore` covering Go, Rust, Node.js, and Wails.

*   **Monorepo Structure & Protobuf Definition (1.1):**

 ... (Done)
*   **Rust gRPC Server Implementation (1.2):** ... (Done)
*   **Go Process Manager (Sidecar Logic) (1.3):**
    *   Implemented `ProcessManager` in `backend/process.go`.
    *   Added logic to find a free port and spawn the Rust binary.
    *   Captured Rust stdout/stderr and integrated them into Go's structured logging.
*   **Database & Logging Setup (1.4):**
    *   Initialized `log/slog` for structured logging in the backend.
    *   Set up SQLite with `modernc.org/sqlite` (CGO-free).
    *   Created `users` and `messages` tables automatically on startup.
    *   Implemented gRPC client connection and tested via a `Ping` call to the sidecar.

## 3. Technical Decisions & Knowledge

*   **Cross-platform Compatibility:**
    *   Used `modernc.org/sqlite` (CGO-free) to ensure the backend compiles and runs on Windows, Linux, and macOS without requiring a C compiler.
    *   Implemented OS-aware binary path discovery in `ProcessManager` using `runtime.GOOS`.
    *   Used `filepath.Join` for all path manipulations to handle different path separators (`\` vs `/`).
*   **Sidecar Lifecycle:** The Go application acts as the supervisor, using `context.Context` to ensure the Rust engine is terminated when the Go process exits.
*   **Networking Strategy:** Decided on `go-libp2p` for decentralized communication, which inherently supports cross-platform network abstraction.

## 4. Current Progress & Next Steps

Phần "1. SYSTEM ARCHITECTURE & SETUP" đã hoàn tất về mặt hạ tầng cốt lõi.

Bước tiếp theo theo `PROJECT_PLAN.md` là **"2. P2P NETWORKING LAYER"**:

*   **2.1. Libp2p Host Configuration:** Thiết lập host libp2p với TCP/QUIC, Noise, Yamux.
*   **2.2. Discovery Mechanism:** Triển khai mDNS và Kademlia DHT.
*   **2.3. GossipSub Implementation:** Triển khai cơ chế PubSub cho các topic chat.

---
**AI Agent:** Please proceed with the tasks outlined in "2. P2P NETWORKING LAYER" using `PROJECT_PLAN.md` and `AGENT.md` as primary guidance.
