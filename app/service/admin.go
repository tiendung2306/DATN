package service

import (
	"fmt"
	"os"

	"app/admin"
	"app/adapter/p2p"

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

	outPath, err := wailsRuntime.SaveFileDialog(r.ctx, wailsRuntime.SaveDialogOptions{
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

	return r.launchP2PNode()
}
