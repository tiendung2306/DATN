package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"backend/db"
	"backend/mls_service"
	"backend/p2p"

	log "github.com/ipfs/go-log/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// CLI Flags
	headless := flag.Bool("headless", false, "Run in headless mode (no GUI)")
	dbPath := flag.String("db", "app.db", "Path to SQLite database file")
	bootstrapAddr := flag.String("bootstrap", "", "Multiaddr of a bootstrap peer")
	p2pPort := flag.Int("p2p-port", 4001, "Port for P2P connections")
	writeBootstrap := flag.String("write-bootstrap", "", "Path to write the node's multiaddress")
	flag.Parse()

	// 1. Setup Structured Logging
	opts := &slog.HandlerOptions{Level: slog.LevelDebug}
	baseHandler := slog.NewTextHandler(os.Stdout, opts)
	filterHandler := &LogFilterHandler{baseHandler}
	logger := slog.New(filterHandler)
	slog.SetDefault(logger)

	// Silence noisy libp2p mDNS logs that fail on virtual interfaces
	log.SetLogLevel("mdns", "error")

	slog.Info("Starting Secure P2P System Backend", "headless", *headless)

	// 2. Initialize Database
	database, err := db.InitDB(*dbPath)
	if err != nil {
		slog.Error("Database initialization failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	// 3. Initialize P2P Node
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	privKey, err := p2p.GetOrCreateIdentity(database)
	if err != nil {
		slog.Error("Failed to get P2P identity", "error", err)
		os.Exit(1)
	}

	p2pNode, err := p2p.NewP2PNode(ctx, privKey, *p2pPort)
	if err != nil {
		slog.Error("Failed to initialize P2P node", "error", err)
		os.Exit(1)
	}
	defer p2pNode.Close()

	// Write bootstrap address to file if requested
	if *writeBootstrap != "" {
		// Wait a small bit for addresses to be finalized
		go func() {
			time.Sleep(2 * time.Second)
			addrs := p2pNode.Host.Addrs()
			if len(addrs) > 0 {
				// Use the first non-loopback address
				var targetAddr string
				for _, a := range addrs {
					s := a.String()
					if !strings.Contains(s, "127.0.0.1") && !strings.Contains(s, "::1") {
						targetAddr = fmt.Sprintf("%s/p2p/%s", s, p2pNode.Host.ID().String())
						break
					}
				}
				if targetAddr != "" {
					err := os.WriteFile(*writeBootstrap, []byte(targetAddr), 0644)
					if err != nil {
						slog.Error("Failed to write bootstrap file", "error", err)
					} else {
						slog.Info("Wrote bootstrap address to file", "path", *writeBootstrap, "addr", targetAddr)
					}
				}
			}
		}()
	}

	// Handle Bootstrap
	if *bootstrapAddr != "" {
		go func() {
			slog.Info("Connecting to bootstrap peer", "addr", *bootstrapAddr)
			if err := p2pNode.ConnectToPeer(ctx, *bootstrapAddr); err != nil {
				slog.Warn("Bootstrap connection failed", "error", err)
			}
		}()
	}

	// Join Global Chat Room
	chatRoom, err := p2p.JoinChatRoom(ctx, p2pNode.PubSub, p2p.GlobalTopicName)
	if err != nil {
		slog.Error("Failed to join global chat room", "error", err)
	}

	// Periodic ping to Global Topic (for testing)
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				msg := fmt.Sprintf("Ping from %s at %s", p2pNode.Host.ID().String(), time.Now().Format(time.RFC3339))
				if err := chatRoom.Publish(ctx, []byte(msg)); err != nil {
					slog.Warn("Failed to publish ping", "error", err)
				}
			}
		}
	}()

	// 4. Start Rust Crypto Engine (Sidecar)
	pm := NewProcessManager()
	port, err := pm.StartCryptoEngine()
	if err != nil {
		slog.Warn("Crypto Engine sidecar start issue", "error", err)
		slog.Info("Tip: Make sure to build the rust project: cd crypto-engine && cargo build")
	}
	defer pm.StopCryptoEngine()

	// 5. Setup gRPC Client and Ping if engine started
	if port > 0 {
		go func() {
			time.Sleep(1 * time.Second) // Give it a moment to start

			addr := fmt.Sprintf("127.0.0.1:%d", port)
			conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
			if err != nil {
				slog.Error("Failed to connect to gRPC server", "error", err)
				return
			}
			defer conn.Close()

			client := mls_service.NewMLSCryptoServiceClient(conn)

			pingCtx, pingCancel := context.WithTimeout(context.Background(), time.Second*2)
			defer pingCancel()

			resp, err := client.Ping(pingCtx, &mls_service.PingRequest{})
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

// LogFilterHandler wraps a slog.Handler to filter out specific noisy messages.
type LogFilterHandler struct {
	slog.Handler
}

func (h *LogFilterHandler) Handle(ctx context.Context, r slog.Record) error {
	// Filter out the annoying mDNS warning on Windows
	if strings.Contains(r.Message, "mdns: Failed to set multicast interface") {
		return nil
	}
	return h.Handler.Handle(ctx, r)
}
