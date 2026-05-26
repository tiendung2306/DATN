package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

type ControlInstanceStatus struct {
	InstanceLabel string              `json:"instance_label,omitempty"`
	RuntimeDir    string              `json:"runtime_dir"`
	ProcessID     int                 `json:"process_id"`
	AppState      string              `json:"app_state"`
	Health        RuntimeHealth       `json:"health"`
	Network       NetworkSettings     `json:"network"`
	Diagnostics   DiagnosticsSnapshot `json:"diagnostics"`
	TimestampMs   int64               `json:"timestamp_ms"`
}

func (r *Runtime) startControlServer() error {
	r.mu.Lock()
	if r.cfg == nil || r.cfg.ControlPort == 0 {
		r.mu.Unlock()
		return nil
	}
	if r.controlServer != nil {
		r.mu.Unlock()
		return nil
	}
	port := r.cfg.ControlPort
	r.mu.Unlock()

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/status", r.controlHTTP("GET", r.handleControlStatus))
	mux.HandleFunc("/v1/events", r.controlHTTP("GET", r.handleControlEvents))
	mux.HandleFunc("/v1/actions/reconnect-p2p", r.controlHTTP("POST", r.handleControlReconnectP2P))
	mux.HandleFunc("/v1/actions/disconnect-p2p", r.controlHTTP("POST", r.handleControlDisconnectP2P))
	mux.HandleFunc("/v1/actions/resume-p2p", r.controlHTTP("POST", r.handleControlResumeP2P))
	mux.HandleFunc("/v1/actions/export-diagnostics", r.controlHTTP("POST", r.handleControlExportDiagnostics))
	mux.HandleFunc("/v1/actions/trigger-offline-sync", r.controlHTTP("POST", r.handleControlTriggerOfflineSync))
	mux.HandleFunc("/v1/actions/shutdown", r.controlHTTP("POST", r.handleControlShutdown))

	srv := &http.Server{
		Addr:              fmt.Sprintf("127.0.0.1:%d", port),
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}

	r.mu.Lock()
	r.controlServer = srv
	r.mu.Unlock()

	go func() {
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			slog.Warn("Demo control API stopped unexpectedly", "error", err)
		}
	}()
	return nil
}

func (r *Runtime) controlHTTP(method string, fn func(http.ResponseWriter, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if req.Method != method {
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
			return
		}
		if !r.controlAuthorized(req) {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		fn(w, req)
	}
}

func (r *Runtime) controlAuthorized(req *http.Request) bool {
	r.mu.RLock()
	token := ""
	if r.cfg != nil {
		token = r.cfg.ControlToken
	}
	r.mu.RUnlock()
	if token == "" {
		return true
	}
	return req.Header.Get("X-Demo-Token") == token
}

func (r *Runtime) handleControlStatus(w http.ResponseWriter, _ *http.Request) {
	netSettings, _ := r.GetNetworkSettings()
	diag, _ := r.GetDiagnosticsSnapshot()
	r.mu.RLock()
	label := ""
	if r.cfg != nil {
		label = r.cfg.InstanceLabel
	}
	out := ControlInstanceStatus{
		InstanceLabel: label,
		RuntimeDir:    r.localDir(),
		ProcessID:     os.Getpid(),
		AppState:      r.getAppStateUnlocked(),
		Health:        r.health,
		Network:       netSettings,
		Diagnostics:   diag,
		TimestampMs:   time.Now().UnixMilli(),
	}
	r.mu.RUnlock()
	writeControlJSON(w, out)
}

func (r *Runtime) handleControlEvents(w http.ResponseWriter, req *http.Request) {
	lastSeq := int64(0)
	limit := 200
	if raw := req.URL.Query().Get("since"); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &lastSeq)
	}
	if raw := req.URL.Query().Get("limit"); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &limit)
	}
	events, err := r.GetRuntimeEventsSince(lastSeq, limit)
	writeControlResult(w, events, err)
}

func (r *Runtime) handleControlReconnectP2P(w http.ResponseWriter, _ *http.Request) {
	writeControlResult(w, map[string]string{"status": "ok"}, r.ReconnectP2P())
}

func (r *Runtime) handleControlDisconnectP2P(w http.ResponseWriter, _ *http.Request) {
	writeControlResult(w, map[string]string{"status": "ok"}, r.DisconnectP2P())
}

func (r *Runtime) handleControlResumeP2P(w http.ResponseWriter, _ *http.Request) {
	writeControlResult(w, map[string]string{"status": "ok"}, r.ResumeP2P())
}

func (r *Runtime) handleControlExportDiagnostics(w http.ResponseWriter, _ *http.Request) {
	path, err := r.ExportDiagnostics()
	writeControlResult(w, map[string]string{"path": path}, err)
}

func (r *Runtime) handleControlTriggerOfflineSync(w http.ResponseWriter, _ *http.Request) {
	writeControlResult(w, map[string]string{"status": "ok"}, r.TriggerOfflineSync())
}

func (r *Runtime) handleControlShutdown(w http.ResponseWriter, _ *http.Request) {
	writeControlJSON(w, map[string]string{"status": "shutting_down"})
	go func() {
		time.Sleep(150 * time.Millisecond)
		r.mu.Lock()
		r.teardown()
		r.mu.Unlock()
		os.Exit(0)
	}()
}

func writeControlResult(w http.ResponseWriter, data interface{}, err error) {
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	writeControlJSON(w, data)
}

func writeControlJSON(w http.ResponseWriter, data interface{}) {
	_ = json.NewEncoder(w).Encode(data)
}

func (r *Runtime) stopControlServer(ctx context.Context) error {
	r.mu.Lock()
	srv := r.controlServer
	r.controlServer = nil
	r.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
