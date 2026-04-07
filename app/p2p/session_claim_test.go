package p2p

import (
	"crypto/ed25519"
	"testing"
)

func TestBuildAndVerifySessionClaim(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	claim, err := BuildSessionClaim(priv.Seed(), "peer-1", 12345)
	if err != nil {
		t.Fatalf("BuildSessionClaim: %v", err)
	}
	if err := VerifySessionClaim(claim, "peer-1", pub); err != nil {
		t.Fatalf("VerifySessionClaim: %v", err)
	}
}

func TestVerifySessionClaimWrongKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	wrongPub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey wrong: %v", err)
	}

	claim, err := BuildSessionClaim(priv.Seed(), "peer-1", 12345)
	if err != nil {
		t.Fatalf("BuildSessionClaim: %v", err)
	}
	if err := VerifySessionClaim(claim, "peer-1", wrongPub); err == nil {
		t.Fatalf("expected verify failure with wrong pubkey")
	}
}
