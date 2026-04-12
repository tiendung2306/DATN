package sidecar

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"app/mls_service"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// StartCryptoEngine launches the Rust sidecar, waits for it to be ready,
// and returns the gRPC client, connection, and a stop function.
//
// The caller must always call stopFn() (safe even if startup failed — it is a no-op).
// The caller must call conn.Close() when conn is non-nil.
func StartCryptoEngine(ctx context.Context) (
	client mls_service.MLSCryptoServiceClient,
	conn *grpc.ClientConn,
	stopFn func(),
	err error,
) {
	pm := NewProcessManager()
	noop := func() {}

	port, err := pm.StartEngine()
	if err != nil {
		slog.Warn("Crypto Engine sidecar could not start", "error", err)
		slog.Info("Tip: build the Rust project first: cd crypto-engine && cargo build")
		return nil, nil, noop, err
	}

	client, conn, err = waitForCryptoEngine(ctx, port)
	if err != nil {
		pm.StopCryptoEngine()
		return nil, nil, noop, fmt.Errorf("crypto engine not ready: %w", err)
	}

	slog.Info("Crypto Engine connected", "port", port)
	return client, conn, pm.StopCryptoEngine, nil
}

func waitForCryptoEngine(ctx context.Context, port int) (mls_service.MLSCryptoServiceClient, *grpc.ClientConn, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials())) //nolint:staticcheck
	if err != nil {
		return nil, nil, fmt.Errorf("grpc.Dial: %w", err)
	}

	client := mls_service.NewMLSCryptoServiceClient(conn)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		pingCtx, cancel := context.WithTimeout(ctx, time.Second)
		_, pingErr := client.Ping(pingCtx, &mls_service.PingRequest{})
		cancel()
		if pingErr == nil {
			return client, conn, nil
		}
		time.Sleep(200 * time.Millisecond)
	}

	conn.Close()
	return nil, nil, fmt.Errorf("crypto engine did not respond within 10 seconds")
}
