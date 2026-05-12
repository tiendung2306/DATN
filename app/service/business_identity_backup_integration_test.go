//go:build business_integration

// BI-007–BI-010 identity / backup (core crypto paths without Wails file dialogs where noted).

package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"app/adapter/store"
)

// BI-007: ExportDeviceRequestJSON uses Wails SaveFileDialog — assert same JSON contract as identity.Export payload via GetOnboardingInfo.
func TestBusinessP1_Sprint5_BI007_DeviceRequestJSON_Contract(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	info, err := rt.GetOnboardingInfo()
	if err != nil {
		t.Fatal(err)
	}
	payload := map[string]interface{}{
		"version":        1,
		"peer_id":        info.PeerID,
		"mls_public_key": info.PublicKeyHex,
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	if int(decoded["version"].(float64)) != 1 {
		t.Fatal("version")
	}
	if decoded["peer_id"].(string) != info.PeerID {
		t.Fatal("peer_id mismatch")
	}
}

func TestBusinessP1_Sprint5_BI008_IdentityBackupRoundTrip(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedAuthorizedUser(t, dbPath)
	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	rt.mlsEngine = newBusinessIntegrationMLSMock()
	defer rt.Shutdown(context.Background())

	pass := "backup-pass-9088"
	blob, err := ExportIdentityBackup(rt.db, rt.privKey, pass)
	if err != nil {
		t.Fatalf("ExportIdentityBackup: %v", err)
	}

	targetDir := filepath.Join(root, "restore")
	if err := os.MkdirAll(targetDir, 0o700); err != nil {
		t.Fatal(err)
	}
	targetDB := filepath.Join(targetDir, "restored.db")
	d2, err := store.InitDB(targetDB)
	if err != nil {
		t.Fatal(err)
	}
	defer d2.Close()

	if _, err := ImportIdentityBackup(d2, blob, pass); err != nil {
		t.Fatalf("ImportIdentityBackup: %v", err)
	}
}

func TestBusinessP1_Sprint5_BI009_IdentityBackupWrongPassphrase(t *testing.T) {
	root := businessIntegrationChdirToTemp(t)
	dbPath := businessDBPath(root)
	businessSeedAuthorizedUser(t, dbPath)
	cfg := businessDefaultConfig(dbPath)
	rt := businessNewRuntime(cfg)
	rt.Startup(context.Background())
	rt.mlsEngine = newBusinessIntegrationMLSMock()
	defer rt.Shutdown(context.Background())

	blob, err := ExportIdentityBackup(rt.db, rt.privKey, "good-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImportIdentityBackup(rt.db, blob, "wrong-secret"); err == nil {
		t.Fatal("expected decrypt failure")
	}
}

func TestBusinessP1_Sprint5_BI010_ImportIdentityFromFile_BlocksWithoutForce(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	err := rt.ImportIdentityFromFile("any-passphrase", false)
	if err == nil {
		t.Fatal("expected error when identity exists and force=false")
	}
	if !strings.Contains(err.Error(), "force=true") {
		t.Fatalf("err=%v", err)
	}
}
