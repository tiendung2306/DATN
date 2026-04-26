package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"app/adapter/store"
	"app/admin"
	"app/config"
	"app/coordination"

	p2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func setupServiceRuntime(t *testing.T) *Runtime {
	t.Helper()
	d, err := store.InitDB(":memory:")
	if err != nil {
		t.Fatalf("InitDB: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return &Runtime{
		ctx:          context.Background(),
		db:           d,
		cfg:          &config.Config{P2PPort: 0},
		coordinators: make(map[string]*coordination.Coordinator),
	}
}

func TestSessionStatusStates(t *testing.T) {
	rt := setupServiceRuntime(t)
	status, err := rt.GetSessionStatus()
	if err != nil {
		t.Fatalf("GetSessionStatus: %v", err)
	}
	if status.State != SessionStateUnknown {
		t.Fatalf("state = %q, want unknown", status.State)
	}

	if err := rt.db.SetConfig(sessionStartedAtConfigKey, encodeInt64Config(100)); err != nil {
		t.Fatalf("Set started: %v", err)
	}
	status, err = rt.GetSessionStatus()
	if err != nil {
		t.Fatalf("GetSessionStatus active: %v", err)
	}
	if status.State != SessionStateActive || status.SessionStartedAt != 100 {
		t.Fatalf("status = %+v, want active started at 100", status)
	}

	if err := rt.db.SetConfig(sessionReplacedAtConfigKey, encodeInt64Config(200)); err != nil {
		t.Fatalf("Set replaced: %v", err)
	}
	status, err = rt.GetSessionStatus()
	if err != nil {
		t.Fatalf("GetSessionStatus replaced: %v", err)
	}
	if status.State != SessionStateReplaced || status.ReplacedDetectedAt != 200 {
		t.Fatalf("status = %+v, want replaced at 200", status)
	}
	if err := rt.ensureSessionActive(); !errors.Is(err, ErrSessionReplaced) {
		t.Fatalf("ensureSessionActive err = %v, want ErrSessionReplaced", err)
	}
}

func TestRuntimeHealthDefault(t *testing.T) {
	rt := &Runtime{}
	health := rt.GetRuntimeHealth()
	if health.StartupStage != startupStageNotStarted {
		t.Fatalf("StartupStage = %q, want %q", health.StartupStage, startupStageNotStarted)
	}
	if health.P2PRunning || health.CryptoReady {
		t.Fatalf("health = %+v, want inactive defaults", health)
	}
}

func TestParseDeviceRequestJSON(t *testing.T) {
	rt := &Runtime{}
	peerID := testPeerID(t)
	pubHex := testMLSPublicKeyHex(t)

	req, err := rt.ParseDeviceRequestJSON(`{"version":1,"peer_id":"` + peerID + `","mls_public_key":"` + pubHex + `"}`)
	if err != nil {
		t.Fatalf("ParseDeviceRequestJSON: %v", err)
	}
	if req.PeerID != peerID || req.PublicKeyHex != pubHex {
		t.Fatalf("request = %+v", req)
	}

	if _, err := rt.ParseDeviceRequestJSON(`{"version":2,"peer_id":"` + peerID + `","mls_public_key":"` + pubHex + `"}`); err == nil {
		t.Fatalf("expected unsupported version error")
	}
	if _, err := rt.ParseDeviceRequestJSON(`{"version":1,"peer_id":"bad","mls_public_key":"` + pubHex + `"}`); err == nil {
		t.Fatalf("expected invalid peer error")
	}
	if _, err := rt.ParseDeviceRequestJSON(`{"version":1,"peer_id":"` + peerID + `","mls_public_key":"abcd"}`); err == nil {
		t.Fatalf("expected invalid public key length error")
	}
}

func TestCreateBundleFromRequest(t *testing.T) {
	rt := setupServiceRuntime(t)
	priv, _, err := p2pCrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	rt.privKey = priv
	if err := rt.InitAdminKey("passphrase"); err != nil {
		t.Fatalf("InitAdminKey: %v", err)
	}

	peerID := testPeerID(t)
	pubHex := testMLSPublicKeyHex(t)
	bundleJSON, err := rt.CreateBundleFromRequest(IssueBundleRequest{
		DisplayName:     "Alice",
		PeerID:          peerID,
		PublicKeyHex:    pubHex,
		AdminPassphrase: "passphrase",
	})
	if err != nil {
		t.Fatalf("CreateBundleFromRequest: %v", err)
	}
	bundle, err := admin.DeserializeBundle([]byte(bundleJSON))
	if err != nil {
		t.Fatalf("ParseInvitationBundle: %v", err)
	}
	if bundle.Token.PeerID != peerID || !strings.EqualFold(hex.EncodeToString(bundle.Token.PublicKey), pubHex) {
		t.Fatalf("token = %+v, want peer/public key binding", bundle.Token)
	}

	_, err = rt.CreateBundleFromRequest(IssueBundleRequest{
		DisplayName:     "Alice",
		PeerID:          peerID,
		PublicKeyHex:    pubHex,
		AdminPassphrase: "wrong",
	})
	if err == nil {
		t.Fatalf("expected wrong passphrase error")
	}
}

func testPeerID(t *testing.T) string {
	t.Helper()
	priv, _, err := p2pCrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey: %v", err)
	}
	return id.String()
}

func testMLSPublicKeyHex(t *testing.T) string {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	return hex.EncodeToString(pub)
}
