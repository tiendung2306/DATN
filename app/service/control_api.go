package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
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

type controlDemoCreateGroupRequest struct {
	GroupID    string `json:"group_id"`
	GroupType  string `json:"group_type"`
	CategoryID string `json:"category_id,omitempty"`
}

type controlDemoInvitePeerRequest struct {
	GroupID string `json:"group_id"`
	PeerID  string `json:"peer_id"`
}

type controlDemoSendMessageRequest struct {
	GroupID string `json:"group_id"`
	Message string `json:"message"`
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
	mux.HandleFunc("/v1/actions/export-diagnostics", r.controlHTTP("POST", r.handleControlExportDiagnostics))
	mux.HandleFunc("/v1/actions/trigger-offline-sync", r.controlHTTP("POST", r.handleControlTriggerOfflineSync))
	mux.HandleFunc("/v1/actions/shutdown", r.controlHTTP("POST", r.handleControlShutdown))
	mux.HandleFunc("/v1/demo/create-group", r.controlHTTP("POST", r.handleControlDemoCreateGroup))
	mux.HandleFunc("/v1/demo/invite-peer", r.controlHTTP("POST", r.handleControlDemoInvitePeer))
	mux.HandleFunc("/v1/demo/send-message", r.controlHTTP("POST", r.handleControlDemoSendMessage))
	mux.HandleFunc("/v1/demo/groups", r.controlHTTP("GET", r.handleControlDemoGroups))
	mux.HandleFunc("/v1/demo/group-members", r.controlHTTP("GET", r.handleControlDemoGroupMembers))
	mux.HandleFunc("/v1/demo/group-messages", r.controlHTTP("GET", r.handleControlDemoGroupMessages))
	mux.HandleFunc("/v1/demo/group-status", r.controlHTTP("GET", r.handleControlDemoGroupStatus))

	srv := &http.Server{
		Addr:              fmt.Sprintf("0.0.0.0:%d", port),
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
	db := r.db
	health := r.health
	if r.cfg != nil {
		label = r.cfg.InstanceLabel
	}
	r.mu.RUnlock()

	appState := "ERROR"
	if db != nil {
		if state, err := DetermineAppState(db); err == nil {
			appState = state.String()
		}
	}

	out := ControlInstanceStatus{
		InstanceLabel: label,
		RuntimeDir:    r.localDir(),
		ProcessID:     os.Getpid(),
		AppState:      appState,
		Health:        health,
		Network:       netSettings,
		Diagnostics:   diag,
		TimestampMs:   time.Now().UnixMilli(),
	}
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

func (r *Runtime) handleControlDemoCreateGroup(w http.ResponseWriter, req *http.Request) {
	var payload controlDemoCreateGroupRequest
	if err := readControlJSON(req, &payload); err != nil {
		writeControlResult(w, nil, err)
		return
	}
	groupID := strings.TrimSpace(payload.GroupID)
	if groupID == "" {
		groupID = "demo"
	}
	groupType := strings.TrimSpace(payload.GroupType)
	if groupType == "" {
		groupType = "group"
	}
	err := r.CreateGroupChat(groupID, groupType, strings.TrimSpace(payload.CategoryID))
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "already in group") {
		err = nil
	}
	writeControlResult(w, map[string]string{"group_id": groupID, "group_type": groupType}, err)
}

func (r *Runtime) handleControlDemoInvitePeer(w http.ResponseWriter, req *http.Request) {
	var payload controlDemoInvitePeerRequest
	if err := readControlJSON(req, &payload); err != nil {
		writeControlResult(w, nil, err)
		return
	}
	groupID := strings.TrimSpace(payload.GroupID)
	if groupID == "" {
		groupID = "demo"
	}
	writeControlResult(w, map[string]string{"group_id": groupID, "peer_id": strings.TrimSpace(payload.PeerID)}, r.InvitePeerToGroup(strings.TrimSpace(payload.PeerID), groupID))
}

func (r *Runtime) handleControlDemoSendMessage(w http.ResponseWriter, req *http.Request) {
	var payload controlDemoSendMessageRequest
	if err := readControlJSON(req, &payload); err != nil {
		writeControlResult(w, nil, err)
		return
	}
	groupID := strings.TrimSpace(payload.GroupID)
	if groupID == "" {
		groupID = "demo"
	}
	writeControlResult(w, map[string]string{"group_id": groupID, "status": "sent"}, r.SendGroupMessage(groupID, payload.Message))
}

func (r *Runtime) handleControlDemoGroups(w http.ResponseWriter, _ *http.Request) {
	groups, err := r.GetGroups()
	writeControlResult(w, groups, err)
}

func (r *Runtime) handleControlDemoGroupMembers(w http.ResponseWriter, req *http.Request) {
	groupID := strings.TrimSpace(req.URL.Query().Get("group_id"))
	if groupID == "" {
		groupID = "demo"
	}
	members, err := r.GetGroupMembers(groupID)
	writeControlResult(w, members, err)
}

func (r *Runtime) handleControlDemoGroupMessages(w http.ResponseWriter, req *http.Request) {
	groupID := strings.TrimSpace(req.URL.Query().Get("group_id"))
	if groupID == "" {
		groupID = "demo"
	}
	limit := 20
	if raw := strings.TrimSpace(req.URL.Query().Get("limit")); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &limit)
	}
	offset := 0
	if raw := strings.TrimSpace(req.URL.Query().Get("offset")); raw != "" {
		_, _ = fmt.Sscanf(raw, "%d", &offset)
	}
	messages, err := r.GetGroupMessages(groupID, limit, offset)
	writeControlResult(w, messages, err)
}

func (r *Runtime) handleControlDemoGroupStatus(w http.ResponseWriter, req *http.Request) {
	groupID := strings.TrimSpace(req.URL.Query().Get("group_id"))
	if groupID == "" {
		groupID = "demo"
	}
	writeControlJSON(w, r.GetGroupStatus(groupID))
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

func readControlJSON(req *http.Request, out interface{}) error {
	if req.Body == nil {
		return fmt.Errorf("request body is required")
	}
	defer req.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(req.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return fmt.Errorf("request body is required")
	}
	return json.Unmarshal(raw, out)
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
