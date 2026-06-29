package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type remoteControlError struct {
	Error string `json:"error"`
}

type remoteDemoCreateGroupRequest struct {
	GroupID    string `json:"group_id"`
	GroupType  string `json:"group_type"`
	CategoryID string `json:"category_id,omitempty"`
}

type remoteDemoInvitePeerRequest struct {
	GroupID string `json:"group_id"`
	PeerID  string `json:"peer_id"`
}

type remoteDemoSendMessageRequest struct {
	GroupID string `json:"group_id"`
	Message string `json:"message"`
}

type remoteControlStatus struct {
	InstanceLabel string                 `json:"instance_label,omitempty"`
	RuntimeDir    string                 `json:"runtime_dir"`
	ProcessID     int                    `json:"process_id"`
	AppState      string                 `json:"app_state"`
	Health        map[string]interface{} `json:"health"`
	Network       map[string]interface{} `json:"network"`
	Diagnostics   map[string]interface{} `json:"diagnostics"`
	TimestampMs   int64                  `json:"timestamp_ms"`
}

func (a *App) controlAction(id string, action string) error {
	req, err := a.newControlRequest(id, http.MethodPost, fmt.Sprintf("/v1/actions/%s", action), nil)
	if err != nil {
		return err
	}
	return a.doControlNoContent(req)
}

func (a *App) controlDemoCreateGroup(id string, groupID string, groupType string) error {
	reqBody := remoteDemoCreateGroupRequest{
		GroupID:   strings.TrimSpace(groupID),
		GroupType: strings.TrimSpace(groupType),
	}
	req, err := a.newControlJSONRequest(id, http.MethodPost, "/v1/demo/create-group", reqBody)
	if err != nil {
		return err
	}
	return a.doControlNoContent(req)
}

func (a *App) controlDemoInvitePeer(id string, groupID string, peerID string) error {
	reqBody := remoteDemoInvitePeerRequest{
		GroupID: strings.TrimSpace(groupID),
		PeerID:  strings.TrimSpace(peerID),
	}
	req, err := a.newControlJSONRequest(id, http.MethodPost, "/v1/demo/invite-peer", reqBody)
	if err != nil {
		return err
	}
	return a.doControlNoContent(req)
}

func (a *App) controlDemoSendMessage(id string, groupID string, message string) error {
	reqBody := remoteDemoSendMessageRequest{
		GroupID: strings.TrimSpace(groupID),
		Message: message,
	}
	req, err := a.newControlJSONRequest(id, http.MethodPost, "/v1/demo/send-message", reqBody)
	if err != nil {
		return err
	}
	return a.doControlNoContent(req)
}

func (a *App) controlDemoGroups(id string) ([]map[string]interface{}, error) {
	req, err := a.newControlRequest(id, http.MethodGet, "/v1/demo/groups", nil)
	if err != nil {
		return nil, err
	}
	var out []map[string]interface{}
	if err := a.doControlJSON(req, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (a *App) controlDemoGroupMembers(id string, groupID string) ([]DemoClusterMember, error) {
	req, err := a.newControlRequest(id, http.MethodGet, fmt.Sprintf("/v1/demo/group-members?group_id=%s", groupID), nil)
	if err != nil {
		return nil, err
	}
	var raw []map[string]interface{}
	if err := a.doControlJSON(req, &raw); err != nil {
		return nil, err
	}
	out := make([]DemoClusterMember, 0, len(raw))
	for _, item := range raw {
		out = append(out, DemoClusterMember{
			PeerID:      stringFromAny(item["peer_id"]),
			DisplayName: stringFromAny(item["display_name"]),
			IsOnline:    boolFromAny(item["is_online"]),
			Role:        stringFromAny(item["role"]),
		})
	}
	return out, nil
}

func (a *App) controlDemoGroupMessages(id string, groupID string, limit int) ([]DemoClusterMessage, error) {
	req, err := a.newControlRequest(id, http.MethodGet, fmt.Sprintf("/v1/demo/group-messages?group_id=%s&limit=%d", groupID, limit), nil)
	if err != nil {
		return nil, err
	}
	var raw []map[string]interface{}
	if err := a.doControlJSON(req, &raw); err != nil {
		return nil, err
	}
	out := make([]DemoClusterMessage, 0, len(raw))
	for _, item := range raw {
		out = append(out, DemoClusterMessage{
			MessageID:         stringFromAny(item["message_id"]),
			Sender:            stringFromAny(item["sender"]),
			SenderDisplayName: stringFromAny(item["sender_display_name"]),
			Content:           stringFromAny(item["content"]),
			Timestamp:         int64(intFromAny(item["timestamp"])),
			IsMine:            boolFromAny(item["is_mine"]),
		})
	}
	return out, nil
}

func (a *App) controlDemoGroupStatus(id string, groupID string) (DemoGroupStatus, error) {
	req, err := a.newControlRequest(id, http.MethodGet, fmt.Sprintf("/v1/demo/group-status?group_id=%s", groupID), nil)
	if err != nil {
		return DemoGroupStatus{}, err
	}
	var raw map[string]interface{}
	if err := a.doControlJSON(req, &raw); err != nil {
		return DemoGroupStatus{}, err
	}
	return DemoGroupStatus{
		GroupID:           stringFromAny(raw["group_id"]),
		Epoch:             uint64(intFromAny(raw["epoch"])),
		TokenHolder:       stringFromAny(raw["token_holder"]),
		TokenHolderPeerID: stringFromAny(raw["token_holder_peer_id"]),
		ActiveMembers:     intFromAny(raw["active_members"]),
		ActiveView:        stringSliceFromAny(raw["active_view"]),
		TreeHashShort:     stringFromAny(raw["tree_hash_short"]),
		IsHealing:         boolFromAny(raw["is_healing"]),
	}, nil
}

func (a *App) fetchControlStatus(id string) (*remoteControlStatus, error) {
	req, err := a.newControlRequest(id, http.MethodGet, "/v1/status", nil)
	if err != nil {
		return nil, err
	}
	var out remoteControlStatus
	if err := a.doControlJSON(req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (a *App) newControlRequest(id string, method string, path string, body io.Reader) (*http.Request, error) {
	profile, token, err := a.lookupProfileAndToken(id)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", profile.ControlPort, path), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Demo-Token", token)
	return req, nil
}

func (a *App) newControlJSONRequest(id string, method string, path string, payload interface{}) (*http.Request, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := a.newControlRequest(id, method, path, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

func (a *App) doControlNoContent(req *http.Request) error {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return decodeRemoteError(resp)
	}
	return nil
}

func (a *App) doControlJSON(req *http.Request, out interface{}) error {
	client := http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return decodeRemoteError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func decodeRemoteError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)
	var remoteErr remoteControlError
	if json.Unmarshal(body, &remoteErr) == nil && strings.TrimSpace(remoteErr.Error) != "" {
		return errors.New(remoteErr.Error)
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return fmt.Errorf("remote control returned %s", resp.Status)
	}
	return errors.New(text)
}
