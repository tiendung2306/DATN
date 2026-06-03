# Stage 1: Build Rust Crypto Engine
FROM rust:1.80-slim-bookworm AS rust-builder
WORKDIR /usr/src
COPY proto/ ./proto/
COPY crypto-engine/ ./crypto-engine/
WORKDIR /usr/src/crypto-engine
RUN apt-get update && apt-get install -y protobuf-compiler libssl-dev pkg-config && rm -rf /var/lib/apt/lists/*
RUN --mount=type=cache,target=/usr/local/cargo/registry \
    --mount=type=cache,target=/usr/src/crypto-engine/target \
    cargo build --release

# Stage 2: Build Go Backend
FROM golang:1.24-alpine AS go-builder
WORKDIR /app
COPY app/go.mod app/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY app/ .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o backend-app .

# Stage 3: Final Runtime Image
FROM debian:bookworm-slim
WORKDIR /app

# Install dependencies for SQLite, networking and iptables
RUN apt-get update && apt-get install -y ca-certificates net-tools iproute2 procps curl iptables && rm -rf /var/lib/apt/lists/*

# Copy binaries
COPY --from=rust-builder /usr/src/crypto-engine/target/release/crypto-engine ./crypto-engine
COPY --from=go-builder /app/backend-app ./backend-app

# Create shared directory for bootstrap
RUN mkdir -p /shared && chmod 777 /shared

# Entrypoint script to handle bootstrap writing and reading
COPY entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

ENTRYPOINT ["/app/entrypoint.sh"]
