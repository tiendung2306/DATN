package p2p

import (
	"crypto/ed25519"
	"testing"

	"app/admin"
)

func TestVerifySupersedingLocalSession(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	oldClaim, err := BuildSessionClaim(priv.Seed(), "peer-1", 100)
	if err != nil {
		t.Fatalf("Build old claim: %v", err)
	}
	newClaim, err := BuildSessionClaim(priv.Seed(), "peer-1", 200)
	if err != nil {
		t.Fatalf("Build new claim: %v", err)
	}

	ap := &AuthProtocol{
		localToken: &admin.InvitationToken{PeerID: "peer-1", PublicKey: pub},
		localHandshake: &AuthHandshakeMsg{
			Session: oldClaim,
		},
	}
	if !ap.verifySupersedingLocalSession(newClaim) {
		t.Fatalf("expected newer claim for same identity to be accepted")
	}
}

func TestVerifySupersedingLocalSessionRejectsOrdinaryPeer(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	oldClaim, err := BuildSessionClaim(priv.Seed(), "peer-1", 100)
	if err != nil {
		t.Fatalf("Build old claim: %v", err)
	}
	otherPub, otherPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("Generate other key: %v", err)
	}
	_ = otherPub
	ordinaryPeerClaim, err := BuildSessionClaim(otherPriv.Seed(), "peer-2", 200)
	if err != nil {
		t.Fatalf("Build other claim: %v", err)
	}

	ap := &AuthProtocol{
		localToken: &admin.InvitationToken{PeerID: "peer-1", PublicKey: pub},
		localHandshake: &AuthHandshakeMsg{
			Session: oldClaim,
		},
	}
	if ap.verifySupersedingLocalSession(ordinaryPeerClaim) {
		t.Fatalf("ordinary peer claim must not replace local session")
	}
}
