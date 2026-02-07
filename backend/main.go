package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"backend/db"
	"backend/mls_service"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 1. Setup Structured Logging
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	slog.Info("Starting Secure P2P System Backend")

	// 2. Initialize Database
	database, err := db.InitDB("app.db")
	if err != nil {
		slog.Error("Database initialization failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// 3. Start Rust Crypto Engine (Sidecar)
	pm := NewProcessManager()
	port, err := pm.StartCryptoEngine()
	if err != nil {
		slog.Warn("Crypto Engine sidecar start issue", "error", err)
		slog.Info("Tip: Make sure to build the rust project: cd crypto-engine && cargo build")
	}
	defer pm.StopCryptoEngine()

	// 4. Setup gRPC Client and Ping if engine started
	if port > 0 {
		go func() {
			time.Sleep(1 * time.Second) // Give it a moment to start
			
			addr := fmt.Sprintf("127.0.0.1:%d", port)
			// gRPC Dial is deprecated, but using for simplicity or NewClient
			conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				slog.Error("Failed to connect to gRPC server", "error", err)
				return
			}
			defer conn.Close()

			client := mls_service.NewMLSCryptoServiceClient(conn)
			
			ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
			defer cancel()

			resp, err := client.Ping(ctx, &mls_service.PingRequest{})
			if err != nil {
				slog.Error("Ping to Crypto Engine failed", "error", err)
			} else {
				slog.Info("Ping Success!", "response", resp.Message, "rust_time", resp.Timestamp)
			}
		}()
	}

	// Setup Signal Handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	slog.Info("Application is running. Press Ctrl+C to stop.")
	<-stop

	slog.Info("Shutting down...")
}