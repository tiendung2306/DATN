package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"backend/db"
	"backend/p2p"
)

// run is the top-level orchestrator. It wires together infrastructure
// (database, identity, crypto engine) and dispatches to the appropriate handler.
//
// Returning an error causes main() to log it and exit with code 1.
// Returning nil means the program finished cleanly (including one-shot commands).
func run(cfg *Config) error {
	if err := os.MkdirAll(".local", 0700); err != nil {
		return fmt.Errorf("create .local dir: %w", err)
	}

	database, err := db.InitDB(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("database init: %w", err)
	}
	defer database.Close()

	privKey, err := p2p.GetOrCreateIdentity(database)
	if err != nil {
		return fmt.Errorf("P2P identity: %w", err)
	}

	// ── Commands that don't need the Rust crypto engine ───────────────────────
	switch {
	case cfg.AdminSetup:
		return cmdAdminSetup(database, cfg.AdminPassphrase)
	case cfg.CreateBundle:
		return cmdCreateBundle(database, privKey, cfg)
	case cfg.ImportBundle != "":
		return cmdImportBundle(database, privKey, cfg.ImportBundle)
	}

	// ── Start Rust crypto engine (needed for --setup and normal operation) ────
	mlsClient, conn, stopEngine, engineErr := startCryptoEngine(context.Background())
	// stopEngine is always a valid func (no-op when startup failed); safe to always defer.
	defer stopEngine()
	if conn != nil {
		defer conn.Close()
	}
	if engineErr != nil {
		slog.Warn("Crypto Engine unavailable — some features disabled", "error", engineErr)
	}

	// ── One-shot onboarding command ───────────────────────────────────────────
	if cfg.Setup {
		return cmdSetup(context.Background(), database, privKey, mlsClient)
	}

	// ── Long-running: start the P2P node ─────────────────────────────────────
	return startNode(context.Background(), cfg, database, privKey)
}
