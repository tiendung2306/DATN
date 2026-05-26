package config

import "flag"

// Config holds all CLI-parsed configuration for this process.
type Config struct {
	DBPath         string
	RuntimeDir     string
	P2PPort        int
	BootstrapAddr  string
	WriteBootstrap string
	Headless       bool
	ControlPort    int
	ControlToken   string
	InstanceLabel  string

	Setup              bool
	ImportBundle       string
	ExportIdentity     bool
	ImportIdentityPath string
	ExportOutputPath   string
	IdentityPassphrase string
	Force              bool

	AdminSetup      bool
	AdminPassphrase string
	CreateBundle    bool
	BundleName      string
	BundlePeerID    string
	BundlePubKey    string
	BundleOutput    string

	StoreNode              bool
	BlindStoreParticipant  bool
	OfflineReplicaK        int
	RuntimeEventReplay     bool
	FileTransferChunkBytes int
}

// Parse parses command-line flags into Config. Call once from main.
func Parse() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.DBPath, "db", ".local/app.db", "Path to SQLite database file")
	flag.StringVar(&cfg.RuntimeDir, "runtime-dir", "", "Directory for demo/runtime artifacts")
	flag.IntVar(&cfg.P2PPort, "p2p-port", 4001, "Port for P2P connections")
	flag.StringVar(&cfg.BootstrapAddr, "bootstrap", "", "Multiaddr of bootstrap peer (overrides stored bundle)")
	flag.StringVar(&cfg.WriteBootstrap, "write-bootstrap", "", "Write this node's multiaddress to a file after startup")
	flag.BoolVar(&cfg.Headless, "headless", false, "Run in headless mode (no GUI)")
	flag.IntVar(&cfg.ControlPort, "control-port", 0, "Localhost demo-control API port (0 disables)")
	flag.StringVar(&cfg.ControlToken, "control-token", "", "Bearer token for the demo-control API")
	flag.StringVar(&cfg.InstanceLabel, "instance-label", "", "Human-readable label for demo-managed instances")

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
	flag.BoolVar(&cfg.StoreNode, "store-node", false, "Enable store-node mode: always persist blind-store envelopes for offline recovery")
	flag.BoolVar(&cfg.BlindStoreParticipant, "blind-store-participant", true, "Enable selective blind-store participation for non-store nodes")
	flag.IntVar(&cfg.OfflineReplicaK, "offline-replica-k", 2, "Number of non-store blind-store peers targeted for replica persistence")
	flag.BoolVar(&cfg.RuntimeEventReplay, "runtime-event-replay-enabled", true, "Enable durable runtime event log + replay APIs")
	flag.IntVar(&cfg.FileTransferChunkBytes, "file-chunk-bytes", 1<<20, "Plaintext chunk size for MLS file transfer (Phase 8)")

	flag.Parse()
	if cfg.StoreNode {
		cfg.BlindStoreParticipant = true
	}
	if cfg.OfflineReplicaK < 0 {
		cfg.OfflineReplicaK = 0
	}
	if cfg.FileTransferChunkBytes <= 0 {
		cfg.FileTransferChunkBytes = 1 << 20
	}
	if cfg.RuntimeDir != "" {
		if cfg.DBPath == ".local/app.db" {
			cfg.DBPath = cfg.RuntimeDir + "/app.db"
		}
		if cfg.ExportOutputPath == ".local/identity.backup" {
			cfg.ExportOutputPath = cfg.RuntimeDir + "/identity.backup"
		}
		if cfg.BundleOutput == ".local/invite.bundle" {
			cfg.BundleOutput = cfg.RuntimeDir + "/invite.bundle"
		}
		if cfg.WriteBootstrap == "" {
			cfg.WriteBootstrap = cfg.RuntimeDir + "/bootstrap.txt"
		}
	}
	return cfg
}

// IsCommand returns true when any one-shot command flag is active.
func (c *Config) IsCommand() bool {
	return c.Setup ||
		c.AdminSetup ||
		c.CreateBundle ||
		c.ImportBundle != "" ||
		c.ExportIdentity ||
		c.ImportIdentityPath != ""
}
