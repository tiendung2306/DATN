package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"backend/db"
	"backend/p2p"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
)

// startNode determines the app state and either prints onboarding guidance
// or starts the fully-authenticated P2P node and blocks until shutdown.
func startNode(ctx context.Context, cfg *Config, database *db.Database, privKey p2pCrypto.PrivKey) error {
	state, err := DetermineAppState(database)
	if err != nil {
		return fmt.Errorf("determine app state: %w", err)
	}
	slog.Info("Application state", "state", state.String())

	switch state {
	case StateUninitialized:
		printUninitialized()
		return nil

	case StateAwaitingBundle:
		info, err := p2p.GetOnboardingInfo(database, privKey)
		if err != nil {
			return fmt.Errorf("read onboarding info: %w", err)
		}
		printAwaitingBundle(info)
		return nil
	}

	// StateAuthorized or StateAdminReady — start the P2P node.
	return runP2PNode(ctx, cfg, database, privKey, state)
}

// runP2PNode initialises the P2P host, connects to bootstrap, joins the chat room,
// and blocks until SIGINT/SIGTERM.
func runP2PNode(
	ctx context.Context,
	cfg *Config,
	database *db.Database,
	privKey p2pCrypto.PrivKey,
	state AppState,
) error {
	bundle, err := database.GetAuthBundle()
	if err != nil {
		return fmt.Errorf("load auth bundle: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	localToken := p2p.BuildLocalToken(bundle)
	node, err := p2p.NewP2PNode(ctx, privKey, cfg.P2PPort, localToken, bundle.RootPublicKey)
	if err != nil {
		return fmt.Errorf("initialize P2P node: %w", err)
	}
	defer node.Close()

	if cfg.WriteBootstrap != "" {
		go writeBootstrapFile(node, cfg.WriteBootstrap)
	}

	connectBootstrap(ctx, node, cfg.BootstrapAddr, bundle.BootstrapAddr)

	if err := joinChatRoom(ctx, node); err != nil {
		slog.Warn("Could not join global chat room", "error", err)
	}

	slog.Info("Node is running. Press Ctrl+C to stop.",
		"state", state.String(),
		"peerID", node.Host.ID().String(),
	)
	waitForShutdown()
	slog.Info("Shutting down...")
	return nil
}

// connectBootstrap dials the bootstrap peer in a goroutine.
// It prefers the CLI --bootstrap flag; falls back to the address in the bundle.
func connectBootstrap(ctx context.Context, node *p2p.P2PNode, flagAddr, bundleAddr string) {
	addr := flagAddr
	if addr == "" {
		addr = bundleAddr
	}
	if addr == "" {
		return
	}
	go func() {
		slog.Info("Connecting to bootstrap peer", "addr", addr)
		if err := node.ConnectToPeer(ctx, addr); err != nil {
			slog.Warn("Bootstrap connection failed", "error", err)
		}
	}()
}

// joinChatRoom subscribes to the global chat topic and starts the ping loop.
func joinChatRoom(ctx context.Context, node *p2p.P2PNode) error {
	chatRoom, err := p2p.JoinChatRoom(ctx, node.PubSub, p2p.GlobalTopicName)
	if err != nil {
		return err
	}
	go pingLoop(ctx, node, chatRoom)
	return nil
}

// pingLoop publishes a heartbeat message every 30 seconds (for integration testing).
func pingLoop(ctx context.Context, node *p2p.P2PNode, chatRoom *p2p.ChatRoom) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			msg := fmt.Sprintf("Ping from %s at %s",
				node.Host.ID().String(), time.Now().Format(time.RFC3339))
			if err := chatRoom.Publish(ctx, []byte(msg)); err != nil {
				slog.Warn("Failed to publish ping", "error", err)
			}
		}
	}
}

// waitForShutdown blocks until SIGINT or SIGTERM is received.
func waitForShutdown() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}

// writeBootstrapFile waits for the host addresses to settle, then writes the
// first public (non-loopback) multiaddr to the given file path.
func writeBootstrapFile(node *p2p.P2PNode, path string) {
	time.Sleep(2 * time.Second)
	for _, a := range node.Host.Addrs() {
		s := a.String()
		if strings.Contains(s, "127.0.0.1") || strings.Contains(s, "::1") {
			continue
		}
		full := fmt.Sprintf("%s/p2p/%s", s, node.Host.ID().String())
		if err := os.WriteFile(path, []byte(full), 0644); err != nil {
			slog.Error("Failed to write bootstrap file", "error", err)
		} else {
			slog.Info("Wrote bootstrap address to file", "path", path, "addr", full)
		}
		return
	}
}
