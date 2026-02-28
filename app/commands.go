package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"app/admin"
	"app/db"
	"app/mls_service"
	"app/p2p"

	"crypto/ed25519"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	p2pPeer "github.com/libp2p/go-libp2p/core/peer"
)

// cmdAdminSetup generates the Root Admin Ed25519 key pair and stores it encrypted.
func cmdAdminSetup(database *db.Database, passphrase string) error {
	if passphrase == "" {
		return fmt.Errorf("--admin-passphrase is required for --admin-setup")
	}
	pubKey, err := admin.SetupAdminKey(database, passphrase)
	if err != nil {
		return fmt.Errorf("admin setup: %w", err)
	}
	printAdminSetupResult(pubKey)
	return nil
}

// cmdCreateBundle signs an InvitationToken for a new user and writes the bundle file.
func cmdCreateBundle(database *db.Database, libp2pPrivKey p2pCrypto.PrivKey, cfg *Config) error {
	if cfg.AdminPassphrase == "" {
		return fmt.Errorf("--admin-passphrase is required for --create-bundle")
	}
	if cfg.BundleName == "" {
		return fmt.Errorf("--bundle-name is required (use the DisplayName the user sent you)")
	}
	if cfg.BundlePeerID == "" || cfg.BundlePubKey == "" {
		return fmt.Errorf("--bundle-peer-id and --bundle-pub-key are both required")
	}

	adminPrivKey, err := admin.UnlockAdminKey(database, cfg.AdminPassphrase)
	if err != nil {
		return fmt.Errorf("unlock admin key: %w", err)
	}

	bootstrapAddr, err := buildAdminBootstrapAddr(libp2pPrivKey, cfg.P2PPort)
	if err != nil {
		return err
	}

	bundleData, err := admin.CreateInvitationBundle(
		adminPrivKey, cfg.BundleName, cfg.BundlePeerID, cfg.BundlePubKey, bootstrapAddr,
	)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}

	if err := os.WriteFile(cfg.BundleOutput, bundleData, 0600); err != nil {
		return fmt.Errorf("write bundle file %q: %w", cfg.BundleOutput, err)
	}

	printBundleCreated(cfg.BundleName, cfg.BundlePeerID, bootstrapAddr, cfg.BundleOutput)
	return nil
}

// cmdSetup generates the MLS key pair for this node via the Rust crypto engine.
func cmdSetup(
	ctx context.Context,
	database *db.Database,
	privKey p2pCrypto.PrivKey,
	mlsClient mls_service.MLSCryptoServiceClient,
) error {
	if mlsClient == nil {
		return fmt.Errorf("crypto engine required for --setup; run: cd crypto-engine && cargo build")
	}
	if err := p2p.OnboardNewUser(ctx, database, mlsClient); err != nil {
		return fmt.Errorf("generate key pair: %w", err)
	}
	info, err := p2p.GetOnboardingInfo(database, privKey)
	if err != nil {
		return fmt.Errorf("read onboarding info: %w", err)
	}
	printSetupResult(info)
	return nil
}

// cmdImportBundle validates and stores an InvitationBundle received from Admin.
func cmdImportBundle(database *db.Database, privKey p2pCrypto.PrivKey, path string) error {
	bundleData, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read bundle file %q: %w", path, err)
	}
	if err := p2p.ImportInvitationBundle(database, privKey, bundleData); err != nil {
		return fmt.Errorf("import bundle: %w", err)
	}
	slog.Info("Bundle imported successfully! You can now start the node normally.")
	return nil
}

// buildAdminBootstrapAddr constructs the full multiaddr for Admin's P2P endpoint.
// The /p2p/PEERID suffix is mandatory — libp2p's Noise Protocol uses it to
// authenticate the remote peer's identity during connection establishment.
func buildAdminBootstrapAddr(privKey p2pCrypto.PrivKey, port int) (string, error) {
	peerID, err := p2pPeer.IDFromPrivateKey(privKey)
	if err != nil {
		return "", fmt.Errorf("derive admin PeerID: %w", err)
	}
	return fmt.Sprintf("/ip4/%s/tcp/%d/p2p/%s", p2p.GetBestLocalIP(), port, peerID), nil
}

// ─── Print helpers ────────────────────────────────────────────────────────────

func printAdminSetupResult(pubKey ed25519.PublicKey) {
	fmt.Println()
	fmt.Println("Root Admin key generated and stored (encrypted).")
	fmt.Printf("Root Public Key (hex): %x\n", pubKey)
	fmt.Println()
	fmt.Println("IMPORTANT: Keep your passphrase safe. If lost, the admin key cannot be recovered.")
}

func printBundleCreated(name, peerID, bootstrapAddr, outputPath string) {
	fmt.Println()
	fmt.Printf("InvitationBundle created: %s\n", outputPath)
	fmt.Printf("  User      : %s\n", name)
	fmt.Printf("  PeerID    : %s\n", peerID)
	fmt.Printf("  Bootstrap : %s\n", bootstrapAddr)
	fmt.Println()
	fmt.Printf("Send %s to the user via out-of-band channel (Zalo, email, USB, etc.).\n", outputPath)
}

func printSetupResult(info *p2p.OnboardingInfo) {
	fmt.Println()
	fmt.Println("Key pair generated! Send these two values to Admin:")
	fmt.Println("─────────────────────────────────────────────────────────────────")
	fmt.Printf("  PeerID    : %s\n", info.PeerID)
	fmt.Printf("  PublicKey : %s\n", info.PublicKeyHex)
	fmt.Println("─────────────────────────────────────────────────────────────────")
	fmt.Println("Admin will assign your display name and send back a .bundle file.")
	fmt.Println()
	fmt.Println("After receiving the .bundle file from Admin, run:")
	fmt.Println("  backend --import-bundle invite.bundle")
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
