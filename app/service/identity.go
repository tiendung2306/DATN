package service

import (
	"fmt"
	"log/slog"
	"os"

	"app/adapter/p2p"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// GetAppState returns the current onboarding state as a string.
// Possible values: UNINITIALIZED, AWAITING_BUNDLE, AUTHORIZED, ADMIN_READY, ERROR.
func (r *Runtime) GetAppState() string {
	if r.db == nil {
		return "ERROR"
	}
	state, err := DetermineAppState(r.db)
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
func (r *Runtime) GetOnboardingInfo() (*OnboardingInfo, error) {
	if r.db == nil || r.privKey == nil {
		return nil, fmt.Errorf("app not initialized")
	}
	info, err := p2p.GetOnboardingInfo(r.db, r.privKey)
	if err != nil {
		return nil, err
	}
	return &OnboardingInfo{PeerID: info.PeerID, PublicKeyHex: info.PublicKeyHex}, nil
}

// GenerateKeys generates the MLS key pair for this node via the Rust crypto engine.
func (r *Runtime) GenerateKeys() (*OnboardingInfo, error) {
	if r.db == nil || r.privKey == nil {
		return nil, fmt.Errorf("app not initialized")
	}
	if r.mlsClient == nil {
		return nil, fmt.Errorf("crypto engine not available — build the Rust project first:\n  cd crypto-engine && cargo build")
	}
	has, err := r.db.HasMLSIdentity()
	if err != nil {
		return nil, fmt.Errorf("check identity: %w", err)
	}
	if has {
		return nil, fmt.Errorf("key pair already exists; use GetOnboardingInfo to retrieve it")
	}
	if err := p2p.OnboardNewUser(r.appCtx(), r.db, r.mlsClient); err != nil {
		return nil, fmt.Errorf("generate key pair: %w", err)
	}
	info, err := p2p.GetOnboardingInfo(r.db, r.privKey)
	if err != nil {
		return nil, err
	}
	return &OnboardingInfo{PeerID: info.PeerID, PublicKeyHex: info.PublicKeyHex}, nil
}

// OpenAndImportBundle opens a system file dialog for the user to select a .bundle file.
func (r *Runtime) OpenAndImportBundle() error {
	if r.db == nil || r.privKey == nil {
		return fmt.Errorf("app not initialized")
	}
	path, err := wailsRuntime.OpenFileDialog(r.appCtx(), wailsRuntime.OpenDialogOptions{
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
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read bundle file: %w", err)
	}
	if err := p2p.ImportInvitationBundle(r.db, r.privKey, data); err != nil {
		return err
	}
	return r.launchP2PNode()
}

// ExportIdentity exports local identity data to an encrypted .backup file.
func (r *Runtime) ExportIdentity(passphrase string) error {
	if r.db == nil || r.privKey == nil {
		return fmt.Errorf("app not initialized")
	}
	if passphrase == "" {
		return fmt.Errorf("passphrase is required")
	}

	backupBytes, err := ExportIdentityBackup(r.db, r.privKey, passphrase)
	if err != nil {
		return err
	}

	outPath, err := wailsRuntime.SaveFileDialog(r.appCtx(), wailsRuntime.SaveDialogOptions{
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
func (r *Runtime) ImportIdentityFromFile(passphrase string, force bool) error {
	if r.db == nil {
		return fmt.Errorf("app not initialized")
	}
	if passphrase == "" {
		return fmt.Errorf("passphrase is required")
	}

	hasIdentity, err := r.db.HasMLSIdentity()
	if err != nil {
		return fmt.Errorf("check existing identity: %w", err)
	}
	hasBundle, err := r.db.HasAuthBundle()
	if err != nil {
		return fmt.Errorf("check existing auth bundle: %w", err)
	}
	if (hasIdentity || hasBundle) && !force {
		return fmt.Errorf("existing identity data found; set force=true to replace")
	}

	path, err := wailsRuntime.OpenFileDialog(r.appCtx(), wailsRuntime.OpenDialogOptions{
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
	if _, err := ImportIdentityBackup(r.db, data, passphrase); err != nil {
		return err
	}
	if err := ApplyIdentityImportSideEffects(r.db); err != nil {
		return fmt.Errorf("apply identity import side effects: %w", err)
	}

	slog.Info("Identity imported via GUI. Restart app to apply and trigger session takeover.")
	return nil
}
