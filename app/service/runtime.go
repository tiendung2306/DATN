package service

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"

	"app/adapter/p2p"
	"app/adapter/sidecar"
	"app/adapter/store"
	"app/config"
	"app/coordination"
	"app/mls_service"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"google.golang.org/grpc"
)

// Runtime is the Wails-bound application core (group chat, P2P, onboarding).
// All exported methods are available to the frontend via bindings.
type Runtime struct {
	ctx        context.Context
	cfg        *config.Config
	db         *store.Database
	privKey    p2pCrypto.PrivKey
	mlsClient  mls_service.MLSCryptoServiceClient
	conn       *grpc.ClientConn
	stopEngine func()
	node       *p2p.P2PNode
	nodeCancel context.CancelFunc
	mu         sync.Mutex

	uiEvents EventSink

	transport    *p2p.LibP2PTransport
	coordStorage *store.SQLiteCoordinationStorage
	mlsEngine    coordination.MLSEngine
	coordinators map[string]*coordination.Coordinator
}

// NewRuntime creates a Runtime for the given CLI config.
func NewRuntime(cfg *config.Config) *Runtime {
	return &Runtime{
		cfg:        cfg,
		stopEngine: func() {},
	}
}

// SetContext sets the runtime context (e.g. Wails ctx). Used before Startup.
func (r *Runtime) SetContext(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ctx = ctx
}

// SetEventSink sets the UI event sink (optional).
func (r *Runtime) SetEventSink(s EventSink) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.uiEvents = s
}

func (r *Runtime) emit(event string, data map[string]interface{}) {
	if r.uiEvents != nil && r.ctx != nil {
		r.uiEvents.Emit(r.ctx, event, data)
	}
}

// Startup is the Wails OnStartup hook.
func (r *Runtime) Startup(ctx context.Context) {
	r.ctx = ctx

	if err := os.MkdirAll(".local", 0700); err != nil {
		slog.Error("Failed to create .local dir", "error", err)
		return
	}

	database, err := store.InitDB(r.cfg.DBPath)
	if err != nil {
		slog.Error("Database init failed", "error", err)
		return
	}
	r.db = database

	privKey, err := p2p.GetOrCreateIdentity(database)
	if err != nil {
		slog.Error("P2P identity init failed", "error", err)
		return
	}
	r.privKey = privKey

	client, conn, stopFn, err := sidecar.StartCryptoEngine(ctx)
	r.stopEngine = stopFn
	if conn != nil {
		r.conn = conn
	}
	if err != nil {
		slog.Warn("Crypto engine unavailable — GenerateKeys will be disabled", "error", err)
	} else {
		r.mlsClient = client
	}

	state, err := DetermineAppState(database)
	if err != nil {
		slog.Error("Failed to determine app state", "error", err)
		return
	}
	slog.Info("App state on startup", "state", state.String())

	if state == StateAuthorized || state == StateAdminReady {
		if err := r.launchP2PNode(); err != nil {
			slog.Error("Failed to start P2P node on startup", "error", err)
		} else if err := consumeKillSessionPendingFlag(r.db); err != nil {
			slog.Warn("Failed to clear kill session pending flag", "error", err)
		}
	}
}

// DomReady is the Wails OnDomReady hook.
func (r *Runtime) DomReady(_ context.Context) {}

// BeforeClose is the Wails OnBeforeClose hook.
func (r *Runtime) BeforeClose(_ context.Context) bool { return false }

// Shutdown is the Wails OnShutdown hook.
func (r *Runtime) Shutdown(_ context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.teardown()
}

// teardown releases all resources. Must be called with r.mu held.
func (r *Runtime) teardown() {
	r.stopCoordinatorsLocked()

	if r.nodeCancel != nil {
		r.nodeCancel()
		r.nodeCancel = nil
	}
	if r.node != nil {
		r.removeKPOfferHandler()
		r.removeWelcomeDeliveryHandler()
		r.removeOfflineSyncHandlers()
		r.node.Close()
		r.node = nil
	}
	if r.conn != nil {
		r.conn.Close()
		r.conn = nil
	}
	r.stopEngine()
	r.stopEngine = func() {}
	if r.db != nil {
		r.db.Close()
		r.db = nil
	}
}

// launchP2PNode starts the libp2p node in the background.
// Acquires the mutex; safe to call from any goroutine.
func (r *Runtime) launchP2PNode() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.node != nil {
		return nil // already running
	}
	if r.db == nil || r.privKey == nil {
		return fmt.Errorf("app not initialized")
	}

	bundle, err := r.db.GetAuthBundle()
	if err != nil {
		return fmt.Errorf("load auth bundle: %w", err)
	}

	nodeCtx, cancel := context.WithCancel(r.ctx)
	localToken := p2p.BuildLocalToken(bundle)
	hs, err := buildLocalAuthHandshake(r.db, localToken.PeerID)
	if err != nil {
		cancel()
		return fmt.Errorf("build auth handshake: %w", err)
	}
	hs.Token = localToken
	node, err := p2p.NewP2PNode(nodeCtx, r.privKey, r.cfg.P2PPort, localToken, bundle.RootPublicKey, hs)
	if err != nil {
		cancel()
		return fmt.Errorf("init P2P node: %w", err)
	}
	r.node = node
	r.nodeCancel = cancel

	go connectBootstrap(nodeCtx, node, r.cfg.BootstrapAddr, bundle.BootstrapAddr)

	if err := joinChatRoom(nodeCtx, node); err != nil {
		slog.Warn("Could not join global chat room", "error", err)
	}

	slog.Info("P2P node started via GUI", "peerID", node.Host.ID().String())

	r.initCoordinationStackLocked()
	r.registerKPOfferHandler()
	r.registerWelcomeDeliveryHandler()
	r.registerOfflineSyncHandlers()
	r.node.Host.Network().Notify(&peerConnectedHook{rt: r})

	go r.advertiseKeyPackage()
	go r.checkOfflineDHTInboxOnce()
	go r.offlineEnvelopeGCLoop(nodeCtx)

	return nil
}
