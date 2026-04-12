package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"app/adapter/p2p"
	"app/adapter/sidecar"
	"app/adapter/store"
	"app/config"
	"app/service"
)

// Run executes CLI / headless mode: database, identity, optional crypto engine, then commands or P2P node.
func Run(cfg *config.Config) error {
	if err := os.MkdirAll(".local", 0700); err != nil {
		return fmt.Errorf("create .local dir: %w", err)
	}

	database, err := store.InitDB(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("database init: %w", err)
	}
	defer database.Close()

	privKey, err := p2p.GetOrCreateIdentity(database)
	if err != nil {
		return fmt.Errorf("P2P identity: %w", err)
	}

	switch {
	case cfg.AdminSetup:
		return cmdAdminSetup(database, cfg.AdminPassphrase)
	case cfg.CreateBundle:
		return cmdCreateBundle(database, privKey, cfg)
	case cfg.ImportBundle != "":
		return cmdImportBundle(database, privKey, cfg.ImportBundle)
	case cfg.ExportIdentity:
		return cmdExportIdentity(database, privKey, cfg)
	case cfg.ImportIdentityPath != "":
		return cmdImportIdentity(database, cfg)
	}

	mlsClient, conn, stopEngine, engineErr := sidecar.StartCryptoEngine(context.Background())
	defer stopEngine()
	if conn != nil {
		defer conn.Close()
	}
	if engineErr != nil {
		slog.Warn("Crypto Engine unavailable — some features disabled", "error", engineErr)
	}

	if cfg.Setup {
		return cmdSetup(context.Background(), database, privKey, mlsClient)
	}

	return service.StartCLIHeadlessNode(context.Background(), cfg, database, privKey)
}
