package main

import "flag"

// Config holds all CLI-parsed configuration for this process.
// It is the single source of truth for runtime parameters.
type Config struct {
	// Runtime
	DBPath         string
	P2PPort        int
	BootstrapAddr  string
	WriteBootstrap string
	Headless       bool

	// Onboarding
	Setup        bool
	ImportBundle string
	ExportIdentity     bool
	ImportIdentityPath string
	ExportOutputPath   string
	IdentityPassphrase string
	Force              bool

	// Admin-only
	AdminSetup      bool
	AdminPassphrase string
	CreateBundle    bool
	BundleName      string
	BundlePeerID    string
	BundlePubKey    string
	BundleOutput    string
}

func parseCLI() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.DBPath, "db", ".local/app.db", "Path to SQLite database file")
	flag.IntVar(&cfg.P2PPort, "p2p-port", 4001, "Port for P2P connections")
	flag.StringVar(&cfg.BootstrapAddr, "bootstrap", "", "Multiaddr of bootstrap peer (overrides stored bundle)")
	flag.StringVar(&cfg.WriteBootstrap, "write-bootstrap", "", "Write this node's multiaddress to a file after startup")
	flag.BoolVar(&cfg.Headless, "headless", false, "Run in headless mode (no GUI)")

	flag.BoolVar(&cfg.Setup, "setup", false, "Generate MLS key pair (first-time setup, run once)")
	flag.StringVar(&cfg.ImportBundle, "import-bundle", "", "Path to .bundle file received from Admin")
	flag.BoolVar(&cfg.ExportIdentity, "export-identity", false, "Export local identity to encrypted .backup")
	flag.StringVar(&cfg.ImportIdentityPath, "import-identity", "", "Path to encrypted .backup file")
	flag.StringVar(&cfg.ExportOutputPath, "export-output", ".local/identity.backup", "Output path for --export-identity")
	flag.StringVar(&cfg.IdentityPassphrase, "identity-passphrase", "", "Passphrase used for identity backup import/export")
	flag.BoolVar(&cfg.Force, "force", false, "Force destructive operations like replacing existing identity on import")

	flag.BoolVar(&cfg.AdminSetup, "admin-setup", false, "Generate Root Admin key pair (run once on admin machine)")
	flag.StringVar(&cfg.AdminPassphrase, "admin-passphrase", "", "Passphrase to encrypt/unlock the Root Admin key")
	flag.BoolVar(&cfg.CreateBundle, "create-bundle", false, "Create an InvitationBundle for a new user (Admin only)")
	flag.StringVar(&cfg.BundleName, "bundle-name", "", "Display name for the new user (used with --create-bundle)")
	flag.StringVar(&cfg.BundlePeerID, "bundle-peer-id", "", "Libp2p PeerID of the new user (used with --create-bundle)")
	flag.StringVar(&cfg.BundlePubKey, "bundle-pub-key", "", "Hex MLS public key of the new user (used with --create-bundle)")
	flag.StringVar(&cfg.BundleOutput, "bundle-output", ".local/invite.bundle", "Output path for the generated .bundle file")

	flag.Parse()
	return cfg
}

// IsCommand returns true when any one-shot command flag is active.
// Used by main() to decide whether to run in CLI mode or launch the Wails GUI.
func (c *Config) IsCommand() bool {
	return c.Setup ||
		c.AdminSetup ||
		c.CreateBundle ||
		c.ImportBundle != "" ||
		c.ExportIdentity ||
		c.ImportIdentityPath != ""
}
