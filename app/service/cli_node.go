package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"app/config"
	"app/adapter/p2p"
	"app/adapter/store"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
)

// StartCLIHeadlessNode determines app state and either prints guidance or runs the P2P node until SIGINT.
func StartCLIHeadlessNode(ctx context.Context, cfg *config.Config, database *store.Database, privKey p2pCrypto.PrivKey) error {
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

	return runP2PNodeCLI(ctx, cfg, database, privKey, state)
}

func runP2PNodeCLI(
	ctx context.Context,
	cfg *config.Config,
	database *store.Database,
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
	hs, err := buildLocalAuthHandshake(database, localToken.PeerID)
	if err != nil {
		return fmt.Errorf("build auth handshake: %w", err)
	}
	hs.Token = localToken
	node, err := p2p.NewP2PNode(ctx, privKey, cfg.P2PPort, localToken, bundle.RootPublicKey, hs)
	if err != nil {
		return fmt.Errorf("initialize P2P node: %w", err)
	}
	defer node.Close()
	if err := consumeKillSessionPendingFlag(database); err != nil {
		slog.Warn("failed to clear kill session pending flag", "error", err)
	}

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

func joinChatRoom(ctx context.Context, node *p2p.P2PNode) error {
	chatRoom, err := p2p.JoinChatRoom(ctx, node.PubSub, p2p.GlobalTopicName)
	if err != nil {
		return err
	}
	go pingLoop(ctx, node, chatRoom)
	return nil
}

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

func waitForShutdown() {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}

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

func printUninitialized() {
	fmt.Println()
	fmt.Println("This node has no key pair yet. Run setup first:")
	fmt.Println("  backend --setup")
	fmt.Println()
}

func printAwaitingBundle(info *p2p.OnboardingInfo) {
	fmt.Println()
	fmt.Println("Waiting for InvitationBundle from Admin.")
	fmt.Println("Send these two values to Admin via Zalo/email:")
	fmt.Println("─────────────────────────────────────────────────────────────────")
	fmt.Printf("  PeerID    : %s\n", info.PeerID)
	fmt.Printf("  PublicKey : %s\n", info.PublicKeyHex)
	fmt.Println("─────────────────────────────────────────────────────────────────")
	fmt.Println()
	fmt.Println("After receiving the .bundle file from Admin, run:")
	fmt.Println("  backend --import-bundle invite.bundle")
	fmt.Println()
}
