package service

const (
	startupStageNotStarted = "not_started"
	startupStageLocalDir   = "local_dir"
	startupStageDatabase   = "database"
	startupStageIdentity   = "identity"
	startupStageSidecar    = "sidecar"
	startupStageAppState   = "app_state"
	startupStageP2P        = "p2p"
	startupStageReady      = "ready"
)

type RuntimeHealth struct {
	StartupStage  string `json:"startup_stage"`
	AppState      string `json:"app_state"`
	P2PRunning    bool   `json:"p2p_running"`
	CryptoReady   bool   `json:"crypto_ready"`
	LastError     string `json:"last_error,omitempty"`
	LastErrorCode string `json:"last_error_code,omitempty"`
}

func (r *Runtime) GetRuntimeHealth() RuntimeHealth {
	r.mu.RLock()
	defer r.mu.RUnlock()
	h := r.health
	if h.StartupStage == "" {
		h.StartupStage = startupStageNotStarted
	}
	if h.AppState == "" && r.db != nil {
		h.AppState = r.getAppStateUnlocked()
	}
	h.P2PRunning = r.node != nil
	h.CryptoReady = r.mlsClient != nil
	return h
}

func (r *Runtime) setStartupProgress(stage, message string) {
	r.mu.Lock()
	r.health.StartupStage = stage
	if r.health.AppState == "" {
		r.health.AppState = "STARTING"
	}
	h := r.health
	r.mu.Unlock()

	r.emit("startup:progress", map[string]interface{}{
		"stage":   stage,
		"message": message,
	})
	r.emitRuntimeHealth(h)
}

func (r *Runtime) setStartupError(code, message string, fatal bool) {
	r.mu.Lock()
	r.health.LastErrorCode = code
	r.health.LastError = message
	if fatal {
		r.health.AppState = "ERROR"
	}
	h := r.health
	r.mu.Unlock()

	r.emit("startup:error", map[string]interface{}{
		"code":    code,
		"message": message,
		"fatal":   fatal,
	})
	r.emitRuntimeHealth(h)
}

func (r *Runtime) setP2PStatus(running bool, message string) {
	r.mu.Lock()
	r.health.P2PRunning = running
	h := r.health
	r.mu.Unlock()

	r.emit("p2p:status", map[string]interface{}{
		"running": running,
		"message": message,
	})
	r.emitRuntimeHealth(h)
}

func (r *Runtime) setCryptoReady(ready bool) {
	r.mu.Lock()
	r.health.CryptoReady = ready
	h := r.health
	r.mu.Unlock()
	r.emitRuntimeHealth(h)
}

func (r *Runtime) setHealthAppState(state string) {
	r.mu.Lock()
	r.health.AppState = state
	h := r.health
	r.mu.Unlock()
	r.emitRuntimeHealth(h)
}

func (r *Runtime) emitRuntimeHealth(h RuntimeHealth) {
	if h.StartupStage == "" {
		h.StartupStage = startupStageNotStarted
	}
	r.emit("runtime:health", map[string]interface{}{
		"startup_stage":   h.StartupStage,
		"app_state":       h.AppState,
		"p2p_running":     h.P2PRunning,
		"crypto_ready":    h.CryptoReady,
		"last_error":      h.LastError,
		"last_error_code": h.LastErrorCode,
	})
}
