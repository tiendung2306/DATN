# Current State of SECURE PRIVATE P2P COMMUNICATION SYSTEM Project

This document serves as a short-term memory for the AI Agent.

## 1. Project Overview
A serverless, zero-trust P2P communication platform using Go (Wails) and Rust (OpenMLS).

## 2. Completed Tasks

### Phase 1: System Architecture & Setup (Completed)
*   Monorepo, Sidecar lifecycle, gRPC IPC, and CGO-free SQLite.

### Phase 2: P2P Networking Layer (Completed)
*   **Persistent PeerID:** Identity is stored in SQLite `system_config` table.
*   **Resilient IP Detection (Hybrid):** 
    *   Uses a UDP trick (8.8.8.8) to find the primary internet-facing IP.
    *   **Failover:** If offline, it scans and filters network interfaces, ignoring virtual ones (Docker, WSL, VMWare) to find a valid LAN IP.
*   **Libp2p Host:** Bound to the specific "best" IP found, improving stability and mDNS accuracy.
*   **Log Noise Suppression (2-Layer):**
    *   **Layer 1:** Set libp2p `mdns` log level to `error` via `github.com/ipfs/go-log/v2`.
    *   **Layer 2:** Custom `LogFilterHandler` for `slog` to intercept and drop annoying "no such interface" warnings common on Windows with virtual adapters.
*   **Hybrid Discovery:** mDNS (Local) + Kademlia DHT (Global) + Dynamic Bootstrap (Docker/Manual).
*   **GossipSub:** Global chat topic `/org/chat/global` implemented and tested with periodic Pings.

## 3. Technical Decisions & Knowledge
*   **Windows mDNS Issue:** Resolved the "No such interface" spam by binding the Host to a specific IP and filtering out virtual adapters. This ensures the app is "production-ready" for developers and users with complex network setups (Docker/WSL2).
*   **Identity Stability:** PeerID persistence is achieved by storing the Marshaled Private Key in the DB.
*   **Headless Mode:** Essential for Docker testing and server-side operations.

## 4. Current Progress & Next Steps
**Phase 2 is fully verified.**

**Next: Phase 3 - IDENTITY & ADMIN ONBOARDING**
*   Implement Root of Trust (Admin Keys).
*   Implement Invitation Token generation/verification.
*   Implement `ConnectionGater` for strict network access control.
