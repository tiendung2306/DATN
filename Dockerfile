# Stage 1: Build Rust Crypto Engine
FROM rust:1.93-slim AS rust-builder
WORKDIR /usr/src
COPY proto/ ./proto/
COPY crypto-engine/ ./crypto-engine/
WORKDIR /usr/src/crypto-engine
RUN apt-get update && apt-get install -y protobuf-compiler libssl-dev pkg-config && rm -rf /var/lib/apt/lists/*
RUN cargo build --release

# Stage 2: Build Go Backend
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN go build -o backend-app .

# Stage 3: Final Runtime Image
FROM debian:bookworm-slim
WORKDIR /app

# Install dependencies for SQLite and networking
RUN apt-get update && apt-get install -y ca-certificates net-tools iproute2 procps curl && rm -rf /var/lib/apt/lists/*

# Copy binaries
COPY --from=rust-builder /usr/src/crypto-engine/target/release/crypto-engine ./crypto-engine
COPY --from=go-builder /app/backend-app ./backend-app

# Create shared directory for bootstrap
RUN mkdir -p /shared && chmod 777 /shared

# Entrypoint script to handle bootstrap writing and reading
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]
