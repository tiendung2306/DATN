package service

import (
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"app/adapter/p2p"
	"app/admin"

	"github.com/libp2p/go-libp2p/core/peer"
	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// InitAdminKey generates the Root Admin key pair and encrypts it with the passphrase.
func (r *Runtime) InitAdminKey(passphrase string) error {
	if r.db == nil {
		return fmt.Errorf("app not initialized")
	}
	if passphrase == "" {
		return fmt.Errorf("passphrase is required")
	}
	_, err := admin.SetupAdminKey(r.db, passphrase)
	return err
}

// CreateBundleRequest holds the parameters for CreateBundle.
type CreateBundleRequest struct {
	DisplayName     string `json:"display_name"`
	PeerID          string `json:"peer_id"`
	PublicKeyHex    string `json:"public_key_hex"`
	AdminPassphrase string `json:"admin_passphrase"`
}

type AdminStatus struct {
	HasAdminKey bool `json:"has_admin_key"`
	Unlocked    bool `json:"unlocked"`
}

type DeviceAccessRequest struct {
	Version      int    `json:"version"`
	PeerID       string `json:"peer_id"`
	PublicKeyHex string `json:"mls_public_key"`
}

type IssueBundleRequest struct {
	DisplayName     string `json:"display_name"`
	PeerID          string `json:"peer_id"`
	PublicKeyHex    string `json:"public_key_hex"`
	AdminPassphrase string `json:"admin_passphrase"`
	ExpiresAt       int64  `json:"expires_at,omitempty"`
	Note            string `json:"note,omitempty"`
}

// CreateBundle creates a signed InvitationBundle for a new user and saves it via a save dialog.
func (r *Runtime) CreateBundle(req CreateBundleRequest) (string, error) {
	if r.db == nil || r.privKey == nil {
		return "", fmt.Errorf("app not initialized")
	}
	if req.DisplayName == "" || req.PeerID == "" || req.PublicKeyHex == "" || req.AdminPassphrase == "" {
		return "", fmt.Errorf("all fields (display_name, peer_id, public_key_hex, admin_passphrase) are required")
	}

	adminPrivKey, err := admin.UnlockAdminKey(r.db, req.AdminPassphrase)
	if err != nil {
		return "", fmt.Errorf("unlock admin key: %w", err)
	}

	bootstrapAddr, err := BuildAdminBootstrapAddr(r.privKey, r.cfg.P2PPort)
	if err != nil {
		return "", err
	}

	bundleData, err := admin.CreateInvitationBundle(
		adminPrivKey, req.DisplayName, req.PeerID, req.PublicKeyHex, bootstrapAddr,
	)
	if err != nil {
		return "", fmt.Errorf("create bundle: %w", err)
	}

	outPath, err := wailsRuntime.SaveFileDialog(r.appCtx(), wailsRuntime.SaveDialogOptions{
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
		return "", nil
	}

	if err := os.WriteFile(outPath, bundleData, 0600); err != nil {
		return "", fmt.Errorf("write bundle file: %w", err)
	}
	return outPath, nil
}

func (r *Runtime) GetAdminStatus() (AdminStatus, error) {
	hasKey, err := r.HasAdminKey()
	if err != nil {
		return AdminStatus{}, err
	}
	return AdminStatus{HasAdminKey: hasKey, Unlocked: false}, nil
}

func (r *Runtime) ParseDeviceRequestJSON(data string) (DeviceAccessRequest, error) {
	var req DeviceAccessRequest
	if err := json.Unmarshal([]byte(strings.TrimSpace(data)), &req); err != nil {
		return DeviceAccessRequest{}, fmt.Errorf("invalid request JSON: %w", err)
	}
	if err := validateDeviceAccessRequest(req); err != nil {
		return DeviceAccessRequest{}, err
	}
	return req, nil
}

func (r *Runtime) CreateBundleFromRequest(req IssueBundleRequest) (string, error) {
	if r.db == nil || r.privKey == nil {
		return "", fmt.Errorf("app not initialized")
	}
	if req.ExpiresAt != 0 {
		return "", fmt.Errorf("custom bundle expiry is not supported yet")
	}
	if err := validateIssueBundleRequest(req); err != nil {
		return "", err
	}
	adminPrivKey, err := admin.UnlockAdminKey(r.db, req.AdminPassphrase)
	if err != nil {
		return "", fmt.Errorf("unlock admin key: %w", err)
	}
	bootstrapAddr, err := BuildAdminBootstrapAddr(r.privKey, r.cfg.P2PPort)
	if err != nil {
		return "", err
	}
	bundleData, err := admin.CreateInvitationBundle(
		adminPrivKey,
		strings.TrimSpace(req.DisplayName),
		strings.TrimSpace(req.PeerID),
		strings.TrimSpace(req.PublicKeyHex),
		bootstrapAddr,
	)
	if err != nil {
		return "", fmt.Errorf("create bundle: %w", err)
	}
	return string(bundleData), nil
}

func validateDeviceAccessRequest(req DeviceAccessRequest) error {
	if req.Version != 1 {
		return fmt.Errorf("unsupported request version %d", req.Version)
	}
	if _, err := peer.Decode(strings.TrimSpace(req.PeerID)); err != nil {
		return fmt.Errorf("invalid peer_id: %w", err)
	}
	pub, err := hex.DecodeString(strings.TrimSpace(req.PublicKeyHex))
	if err != nil {
		return fmt.Errorf("invalid mls_public_key: %w", err)
	}
	if len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid mls_public_key length: got %d, want %d", len(pub), ed25519.PublicKeySize)
	}
	return nil
}

func validateIssueBundleRequest(req IssueBundleRequest) error {
	if strings.TrimSpace(req.DisplayName) == "" {
		return fmt.Errorf("display_name is required")
	}
	if req.AdminPassphrase == "" {
		return fmt.Errorf("admin_passphrase is required")
	}
	return validateDeviceAccessRequest(DeviceAccessRequest{
		Version:      1,
		PeerID:       req.PeerID,
		PublicKeyHex: req.PublicKeyHex,
	})
}

// HasAdminKey returns true if a Root Admin key has been initialized on this machine.
func (r *Runtime) HasAdminKey() (bool, error) {
	if r.db == nil {
		return false, nil
	}
	return r.db.HasConfig(admin.AdminKeyConfigKey)
}

// CreateAndImportSelfBundle creates and imports a bundle for this node in one step (admin shortcut).
func (r *Runtime) CreateAndImportSelfBundle(displayName, passphrase string) error {
	if r.db == nil || r.privKey == nil {
		return fmt.Errorf("app not initialized")
	}
	if passphrase == "" {
		return fmt.Errorf("admin passphrase is required")
	}
	if displayName == "" {
		displayName = "Admin"
	}

	adminPrivKey, err := admin.UnlockAdminKey(r.db, passphrase)
	if err != nil {
		return fmt.Errorf("unlock admin key: %w", err)
	}

	info, err := p2p.GetOnboardingInfo(r.db, r.privKey)
	if err != nil {
		return fmt.Errorf("get onboarding info: %w", err)
	}

	bootstrapAddr, err := BuildAdminBootstrapAddr(r.privKey, r.cfg.P2PPort)
	if err != nil {
		return err
	}

	bundleData, err := admin.CreateInvitationBundle(
		adminPrivKey, displayName, info.PeerID, info.PublicKeyHex, bootstrapAddr,
	)
	if err != nil {
		return fmt.Errorf("create bundle: %w", err)
	}

	if err := p2p.ImportInvitationBundle(r.db, r.privKey, bundleData); err != nil {
		return fmt.Errorf("import self bundle: %w", err)
	}

	if err := r.launchP2PNode(); err != nil {
		r.setP2PStatus(false, "P2P startup failed")
		return err
	}
	r.setP2PStatus(true, "P2P node running")
	return nil
}
