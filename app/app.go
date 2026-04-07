package main

import (
	"context"
	"embed"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"app/admin"
	"app/coordination"
	"app/db"
	"app/mls_service"
	"app/p2p"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"google.golang.org/grpc"
)

//go:embed all:frontend/dist
var assets embed.FS

// App is the Wails application struct. All exported methods are automatically
// available as TypeScript functions in the frontend via the auto-generated bindings.
type App struct {
	ctx        context.Context
	cfg        *Config
	db         *db.Database
	privKey    p2pCrypto.PrivKey
	mlsClient  mls_service.MLSCryptoServiceClient
	conn       *grpc.ClientConn
	stopEngine func()
	node       *p2p.P2PNode
	nodeCancel context.CancelFunc
	mu         sync.Mutex

	// Coordination stack (initialized after P2P node starts)
	transport    *p2p.LibP2PTransport
	coordStorage *db.SQLiteCoordinationStorage
	mlsEngine    coordination.MLSEngine
	coordinators map[string]*coordination.Coordinator
}

const killSessionPendingConfigKey = "kill_session_pending"

// NewApp creates the App instance that Wails will bind.
func NewApp(cfg *Config) *App {
	return &App{
		cfg:        cfg,
		stopEngine: func() {},
	}
}

// ─── Wails lifecycle ──────────────────────────────────────────────────────────

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	if err := os.MkdirAll(".local", 0700); err != nil {
		slog.Error("Failed to create .local dir", "error", err)
		return
	}

	database, err := db.InitDB(a.cfg.DBPath)
	if err != nil {
		slog.Error("Database init failed", "error", err)
		return
	}
	a.db = database

	privKey, err := p2p.GetOrCreateIdentity(database)
	if err != nil {
		slog.Error("P2P identity init failed", "error", err)
		return
	}
	a.privKey = privKey

	client, conn, stopFn, err := startCryptoEngine(ctx)
	a.stopEngine = stopFn
	if conn != nil {
		a.conn = conn
	}
	if err != nil {
		slog.Warn("Crypto engine unavailable — GenerateKeys will be disabled", "error", err)
	} else {
		a.mlsClient = client
	}

	state, err := DetermineAppState(database)
	if err != nil {
		slog.Error("Failed to determine app state", "error", err)
		return
	}
	slog.Info("App state on startup", "state", state.String())

	if state == StateAuthorized || state == StateAdminReady {
		if err := a.launchP2PNode(); err != nil {
			slog.Error("Failed to start P2P node on startup", "error", err)
		} else if err := consumeKillSessionPendingFlag(a.db); err != nil {
			slog.Warn("Failed to clear kill session pending flag", "error", err)
		}
	}
}

func (a *App) domReady(_ context.Context) {}

func (a *App) beforeClose(_ context.Context) bool { return false }

func (a *App) shutdown(_ context.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.teardown()
}

// teardown releases all resources. Must be called with a.mu held.
func (a *App) teardown() {
	a.stopCoordinatorsLocked()

	if a.nodeCancel != nil {
		a.nodeCancel()
		a.nodeCancel = nil
	}
	if a.node != nil {
		a.removeKPOfferHandler()
		a.removeWelcomeDeliveryHandler()
		a.node.Close()
		a.node = nil
	}
	if a.conn != nil {
		a.conn.Close()
		a.conn = nil
	}
	a.stopEngine()
	a.stopEngine = func() {}
	if a.db != nil {
		a.db.Close()
		a.db = nil
	}
}

// ─── Exported bindings ────────────────────────────────────────────────────────

// GetAppState returns the current onboarding state as a string.
// Possible values: UNINITIALIZED, AWAITING_BUNDLE, AUTHORIZED, ADMIN_READY, ERROR.
func (a *App) GetAppState() string {
	if a.db == nil {
		return "ERROR"
	}
	state, err := DetermineAppState(a.db)
	if err != nil {
		return "ERROR"
	}
	return state.String()
}

// OnboardingInfo holds the two values a user sends to Admin out-of-band.
type OnboardingInfo struct {
	PeerID       string `json:"peer_id"`
	PublicKeyHex string `json:"public_key_hex"`
}

// GetOnboardingInfo returns the PeerID and MLS public key for this node.
// The user sends these two values to Admin to receive an InvitationBundle.
func (a *App) GetOnboardingInfo() (*OnboardingInfo, error) {
	if a.db == nil || a.privKey == nil {
		return nil, fmt.Errorf("app not initialized")
	}
	info, err := p2p.GetOnboardingInfo(a.db, a.privKey)
	if err != nil {
		return nil, err
	}
	return &OnboardingInfo{PeerID: info.PeerID, PublicKeyHex: info.PublicKeyHex}, nil
}

// GenerateKeys generates the MLS key pair for this node via the Rust crypto engine.
// Transitions the app from UNINITIALIZED → AWAITING_BUNDLE.
// Returns the onboarding info to display and copy to Admin.
func (a *App) GenerateKeys() (*OnboardingInfo, error) {
	if a.db == nil || a.privKey == nil {
		return nil, fmt.Errorf("app not initialized")
	}
	if a.mlsClient == nil {
		return nil, fmt.Errorf("crypto engine not available — build the Rust project first:\n  cd crypto-engine && cargo build")
	}
	has, err := a.db.HasMLSIdentity()
	if err != nil {
		return nil, fmt.Errorf("check identity: %w", err)
	}
	if has {
		return nil, fmt.Errorf("key pair already exists; use GetOnboardingInfo to retrieve it")
	}
	if err := p2p.OnboardNewUser(a.ctx, a.db, a.mlsClient); err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}
	info, err := p2p.GetOnboardingInfo(a.db, a.privKey)
	if err != nil {
		return nil, err
	}
	return &OnboardingInfo{PeerID: info.PeerID, PublicKeyHex: info.PublicKeyHex}, nil
}

// OpenAndImportBundle opens a system file dialog for the user to select a .bundle
// file, then validates and imports it. On success, the P2P node starts automatically.
func (a *App) OpenAndImportBundle() error {
	if a.db == nil || a.privKey == nil {
		return fmt.Errorf("app not initialized")
	}
	path, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Invitation Bundle",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Bundle Files (*.bundle)", Pattern: "*.bundle"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return fmt.Errorf("open dialog: %w", err)
	}
	if path == "" {
		return nil // user cancelled
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read bundle file: %w", err)
	}
	if err := p2p.ImportInvitationBundle(a.db, a.privKey, data); err != nil {
		return err
	}
	return a.launchP2PNode()
}

// ExportIdentity exports local identity data to an encrypted .backup file.
// The user provides a passphrase that protects the backup with Argon2id+AES-GCM.
func (a *App) ExportIdentity(passphrase string) error {
	if a.db == nil || a.privKey == nil {
		return fmt.Errorf("app not initialized")
	}
	if passphrase == "" {
		return fmt.Errorf("passphrase is required")
	}

	backupBytes, err := ExportIdentityBackup(a.db, a.privKey, passphrase)
	if err != nil {
		return err
	}

	outPath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Save Identity Backup",
		DefaultFilename: "identity.backup",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Identity Backup (*.backup)", Pattern: "*.backup"},
		},
	})
	if err != nil {
		return fmt.Errorf("save dialog: %w", err)
	}
	if outPath == "" {
		return nil
	}

	if err := os.WriteFile(outPath, backupBytes, 0600); err != nil {
		return fmt.Errorf("write backup file: %w", err)
	}
	return nil
}

// ImportIdentityFromFile imports an encrypted .backup and replaces current local identity data.
// The caller should restart app after success so all in-memory components reload cleanly.
func (a *App) ImportIdentityFromFile(passphrase string, force bool) error {
	if a.db == nil {
		return fmt.Errorf("app not initialized")
	}
	if passphrase == "" {
		return fmt.Errorf("passphrase is required")
	}

	hasIdentity, err := a.db.HasMLSIdentity()
	if err != nil {
		return fmt.Errorf("check existing identity: %w", err)
	}
	hasBundle, err := a.db.HasAuthBundle()
	if err != nil {
		return fmt.Errorf("check existing auth bundle: %w", err)
	}
	if (hasIdentity || hasBundle) && !force {
		return fmt.Errorf("existing identity data found; set force=true to replace")
	}

	path, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: "Select Identity Backup",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Identity Backup (*.backup)", Pattern: "*.backup"},
			{DisplayName: "All Files (*.*)", Pattern: "*.*"},
		},
	})
	if err != nil {
		return fmt.Errorf("open dialog: %w", err)
	}
	if path == "" {
		return nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read backup file: %w", err)
	}
	if _, err := ImportIdentityBackup(a.db, data, passphrase); err != nil {
		return err
	}
	if _, err := resetSessionStartedAt(a.db); err != nil {
		return fmt.Errorf("reset session start: %w", err)
	}
	if err := a.db.SetConfig(killSessionPendingConfigKey, []byte("1")); err != nil {
		return fmt.Errorf("set kill session pending flag: %w", err)
	}

	slog.Info("Identity imported via GUI. Restart app to apply and trigger session takeover.")
	return nil
}

// InitAdminKey generates the Root Admin key pair and encrypts it with the passphrase.
// Run once on the Admin machine. Transitions the node to ADMIN_READY.
func (a *App) InitAdminKey(passphrase string) error {
	if a.db == nil {
		return fmt.Errorf("app not initialized")
	}
	if passphrase == "" {
		return fmt.Errorf("passphrase is required")
	}
	_, err := admin.SetupAdminKey(a.db, passphrase)
	return err
}

// CreateBundleRequest holds the parameters for CreateBundle.
type CreateBundleRequest struct {
	DisplayName     string `json:"display_name"`
	PeerID          string `json:"peer_id"`
	PublicKeyHex    string `json:"public_key_hex"`
	AdminPassphrase string `json:"admin_passphrase"`
}

// CreateBundle creates a signed InvitationBundle for a new user and saves it via
// a system save-file dialog. Returns the output path chosen by the user.
func (a *App) CreateBundle(req CreateBundleRequest) (string, error) {
	if a.db == nil || a.privKey == nil {
		return "", fmt.Errorf("app not initialized")
	}
	if req.DisplayName == "" || req.PeerID == "" || req.PublicKeyHex == "" || req.AdminPassphrase == "" {
		return "", fmt.Errorf("all fields (display_name, peer_id, public_key_hex, admin_passphrase) are required")
	}

	adminPrivKey, err := admin.UnlockAdminKey(a.db, req.AdminPassphrase)
	if err != nil {
		return "", fmt.Errorf("unlock admin key: %w", err)
	}

	bootstrapAddr, err := buildAdminBootstrapAddr(a.privKey, a.cfg.P2PPort)
	if err != nil {
		return "", err
	}

	bundleData, err := admin.CreateInvitationBundle(
		adminPrivKey, req.DisplayName, req.PeerID, req.PublicKeyHex, bootstrapAddr,
	)
	if err != nil {
		return "", fmt.Errorf("create bundle: %w", err)
	}

	outPath, err := wailsRuntime.SaveFileDialog(a.ctx, wailsRuntime.SaveDialogOptions{
		Title:           "Save Invitation Bundle",
		DefaultFilename: req.DisplayName + ".bundle",
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Bundle Files (*.bundle)", Pattern: "*.bundle"},
		},
	})
	if err != nil {
		return "", fmt.Errorf("save dialog: %w", err)
	}
	if outPath == "" {
		return "", nil // user cancelled
	}

	if err := os.WriteFile(outPath, bundleData, 0600); err != nil {
		return "", fmt.Errorf("write bundle file: %w", err)
	}
	return outPath, nil
}

// PeerInfo holds display information about a single connected peer.
type PeerInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Verified    bool   `json:"verified"`
}

// NodeStatus holds the full runtime status of the local P2P node.
type NodeStatus struct {
	State          string     `json:"state"`
	PeerID         string     `json:"peer_id"`
	DisplayName    string     `json:"display_name"`
	IsRunning      bool       `json:"is_running"`
	ConnectedPeers []PeerInfo `json:"connected_peers"`
}

// GetNodeStatus returns the current runtime status of the P2P node.
// Safe to call at any time; returns gracefully if the node is not yet started.
func (a *App) GetNodeStatus() *NodeStatus {
	a.mu.Lock()
	defer a.mu.Unlock()

	status := &NodeStatus{
		State:          a.getAppStateUnlocked(),
		IsRunning:      a.node != nil,
		ConnectedPeers: []PeerInfo{},
	}

	if a.db != nil && a.privKey != nil {
		if info, err := p2p.GetOnboardingInfo(a.db, a.privKey); err == nil {
			status.PeerID = info.PeerID
		}
		if identity, err := a.db.GetMLSIdentity(); err == nil {
			status.DisplayName = identity.DisplayName
		}
	}

	if a.node != nil {
		for _, pid := range a.node.Host.Network().Peers() {
			peer := PeerInfo{ID: pid.String()}
			if a.node.AuthProtocol != nil {
				peer.Verified = a.node.AuthProtocol.IsVerified(pid)
				if tok := a.node.AuthProtocol.GetVerifiedToken(pid); tok != nil {
					peer.DisplayName = tok.DisplayName
				}
			}
			status.ConnectedPeers = append(status.ConnectedPeers, peer)
		}
	}

	return status
}

// getAppStateUnlocked reads app state without acquiring the mutex.
// Used internally when the caller already holds the lock.
func (a *App) getAppStateUnlocked() string {
	if a.db == nil {
		return "ERROR"
	}
	state, err := DetermineAppState(a.db)
	if err != nil {
		return "ERROR"
	}
	return state.String()
}

// HasAdminKey returns true if a Root Admin key has been initialized on this machine.
// Used by the UI to show the admin self-setup shortcut in the AwaitingBundle screen.
func (a *App) HasAdminKey() (bool, error) {
	if a.db == nil {
		return false, nil
	}
	return a.db.HasConfig(admin.AdminKeyConfigKey)
}

// CreateAndImportSelfBundle is an admin shortcut: creates an InvitationBundle for
// this node itself (using its own PeerID and MLS public key), signs it with the
// admin key, and imports it directly — no file export/import needed.
// This transitions the node from AWAITING_BUNDLE → ADMIN_READY in one step.
func (a *App) CreateAndImportSelfBundle(displayName, passphrase string) error {
	if a.db == nil || a.privKey == nil {
		return fmt.Errorf("app not initialized")
	}
	if passphrase == "" {
		return fmt.Errorf("admin passphrase is required")
	}
	if displayName == "" {
		displayName = "Admin"
	}

	adminPrivKey, err := admin.UnlockAdminKey(a.db, passphrase)
	if err != nil {
		return fmt.Errorf("unlock admin key: %w", err)
	}

	info, err := p2p.GetOnboardingInfo(a.db, a.privKey)
	if err != nil {
		return fmt.Errorf("get onboarding info: %w", err)
	}

	bootstrapAddr, err := buildAdminBootstrapAddr(a.privKey, a.cfg.P2PPort)
	if err != nil {
		return err
	}

	bundleData, err := admin.CreateInvitationBundle(
		adminPrivKey, displayName, info.PeerID, info.PublicKeyHex, bootstrapAddr,
	)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}

	if err := p2p.ImportInvitationBundle(a.db, a.privKey, bundleData); err != nil {
		return fmt.Errorf("import self bundle: %w", err)
	}

	return a.launchP2PNode()
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

// launchP2PNode starts the libp2p node in the background.
// Acquires the mutex; safe to call from any goroutine.
func (a *App) launchP2PNode() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.node != nil {
		return nil // already running
	}
	if a.db == nil || a.privKey == nil {
		return fmt.Errorf("app not initialized")
	}

	bundle, err := a.db.GetAuthBundle()
	if err != nil {
		return fmt.Errorf("load auth bundle: %w", err)
	}

	nodeCtx, cancel := context.WithCancel(a.ctx)
	localToken := p2p.BuildLocalToken(bundle)
	hs, err := buildLocalAuthHandshake(a.db, localToken.PeerID)
	if err != nil {
		cancel()
		return fmt.Errorf("build auth handshake: %w", err)
	}
	hs.Token = localToken
	node, err := p2p.NewP2PNode(nodeCtx, a.privKey, a.cfg.P2PPort, localToken, bundle.RootPublicKey, hs)
	if err != nil {
		cancel()
		return fmt.Errorf("init P2P node: %w", err)
	}
	a.node = node
	a.nodeCancel = cancel

	go connectBootstrap(nodeCtx, node, a.cfg.BootstrapAddr, bundle.BootstrapAddr)

	if err := joinChatRoom(nodeCtx, node); err != nil {
		slog.Warn("Could not join global chat room", "error", err)
	}

	slog.Info("P2P node started via GUI", "peerID", node.Host.ID().String())

	a.initCoordinationStackLocked()
	a.registerKPOfferHandler()
	a.registerWelcomeDeliveryHandler()
	a.node.Host.Network().Notify(&peerConnectedHook{app: a})

	// Advertise local KeyPackage to DHT so others can invite us (works offline).
	go a.advertiseKeyPackage()

	return nil
}

// ─── Wails runner ─────────────────────────────────────────────────────────────

// runWailsApp starts the Wails GUI application. Called from main() when no
// one-shot command flags are set and --headless is not specified.
func runWailsApp(cfg *Config) {
	app := NewApp(cfg)
	err := wails.Run(&options.App{
		Title:  "Secure P2P",
		Width:  1100,
		Height: 720,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 17, G: 24, B: 39, A: 255},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
	})
	if err != nil {
		slog.Error("Wails application error", "error", err)
		os.Exit(1)
	}
}
