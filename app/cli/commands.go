package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"app/admin"
	"app/adapter/p2p"
	"app/adapter/store"
	"app/config"
	"app/mls_service"
	"app/service"

	"crypto/ed25519"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
)

func cmdAdminSetup(database *store.Database, passphrase string) error {
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

func cmdCreateBundle(database *store.Database, libp2pPrivKey p2pCrypto.PrivKey, cfg *config.Config) error {
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

	bootstrapAddr, err := service.BuildAdminBootstrapAddr(libp2pPrivKey, cfg.P2PPort)
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

func cmdSetup(
	ctx context.Context,
	database *store.Database,
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

func cmdImportBundle(database *store.Database, privKey p2pCrypto.PrivKey, path string) error {
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

func cmdExportIdentity(database *store.Database, privKey p2pCrypto.PrivKey, cfg *config.Config) error {
	if cfg.IdentityPassphrase == "" {
		return fmt.Errorf("--identity-passphrase is required for --export-identity")
	}
	backupBytes, err := service.ExportIdentityBackup(database, privKey, cfg.IdentityPassphrase)
	if err != nil {
		return fmt.Errorf("export identity backup: %w", err)
	}
	if err := os.WriteFile(cfg.ExportOutputPath, backupBytes, 0600); err != nil {
		return fmt.Errorf("write backup file %q: %w", cfg.ExportOutputPath, err)
	}
	slog.Info("Identity backup exported", "output", cfg.ExportOutputPath)
	return nil
}

func cmdImportIdentity(database *store.Database, cfg *config.Config) error {
	if cfg.IdentityPassphrase == "" {
		return fmt.Errorf("--identity-passphrase is required for --import-identity")
	}

	hasIdentity, err := database.HasMLSIdentity()
	if err != nil {
		return fmt.Errorf("check existing identity: %w", err)
	}
	hasBundle, err := database.HasAuthBundle()
	if err != nil {
		return fmt.Errorf("check existing auth bundle: %w", err)
	}
	if (hasIdentity || hasBundle) && !cfg.Force {
		return fmt.Errorf("local identity data already exists; re-run with --force to replace it")
	}

	data, err := os.ReadFile(cfg.ImportIdentityPath)
	if err != nil {
		return fmt.Errorf("read backup file %q: %w", cfg.ImportIdentityPath, err)
	}
	payload, err := service.ImportIdentityBackup(database, data, cfg.IdentityPassphrase)
	if err != nil {
		return fmt.Errorf("import identity backup: %w", err)
	}
	if err := service.ApplyIdentityImportSideEffects(database); err != nil {
		return fmt.Errorf("apply identity import: %w", err)
	}
	slog.Info("Identity imported successfully",
		"peerID", payload.BundlePeerID,
		"displayName", payload.BundleDisplayName,
	)
	slog.Info("Please restart the app to apply the imported identity and trigger session takeover.")
	return nil
}

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
