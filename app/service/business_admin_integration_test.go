//go:build business_integration

// BI-011–BI-016 admin APIs on the same temp DB pattern as other business Integration tests.

package service

import (
	"strings"
	"testing"
)

func TestBusinessP1_Sprint5_BI011_InitAdminKey(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	if err := rt.InitAdminKey("admin-pass-s5"); err != nil {
		t.Fatalf("InitAdminKey: %v", err)
	}
	has, err := rt.HasAdminKey()
	if err != nil || !has {
		t.Fatalf("HasAdminKey=%v err=%v", has, err)
	}
	st, err := rt.GetAdminStatus()
	if err != nil || !st.HasAdminKey {
		t.Fatalf("GetAdminStatus %+v err=%v", st, err)
	}
}

func TestBusinessP1_Sprint5_BI012_VerifyAdminPassphrase(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	if err := rt.InitAdminKey("correct-admin-pass"); err != nil {
		t.Fatal(err)
	}
	if err := rt.VerifyAdminPassphrase("wrong"); err == nil {
		t.Fatal("expected error for wrong passphrase")
	}
	if err := rt.VerifyAdminPassphrase("correct-admin-pass"); err != nil {
		t.Fatalf("VerifyAdminPassphrase: %v", err)
	}
}

func TestBusinessP1_Sprint5_BI013_ParseDeviceRequestJSON_Valid(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	info, err := rt.GetOnboardingInfo()
	if err != nil {
		t.Fatal(err)
	}
	raw := `{"version":1,"peer_id":"` + info.PeerID + `","mls_public_key":"` + info.PublicKeyHex + `"}`
	req, err := rt.ParseDeviceRequestJSON(raw)
	if err != nil {
		t.Fatalf("ParseDeviceRequestJSON: %v", err)
	}
	if req.PeerID != info.PeerID || req.PublicKeyHex != info.PublicKeyHex {
		t.Fatalf("unexpected req %+v", req)
	}
}

func TestBusinessP1_Sprint5_BI014_ParseDeviceRequestJSON_Invalid(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	if _, err := rt.ParseDeviceRequestJSON(`{"version":2}`); err == nil {
		t.Fatal("expected version error")
	}
	if _, err := rt.ParseDeviceRequestJSON(`{`); err == nil {
		t.Fatal("expected json error")
	}
}

func TestBusinessP1_Sprint5_BI015_CreateBundleFromRequest(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	pass := "adm-bundle-s5"
	if err := rt.InitAdminKey(pass); err != nil {
		t.Fatal(err)
	}
	info, err := rt.GetOnboardingInfo()
	if err != nil {
		t.Fatal(err)
	}
	jsonStr, err := rt.CreateBundleFromRequest(IssueBundleRequest{
		DisplayName:     "Device",
		PeerID:          info.PeerID,
		PublicKeyHex:    info.PublicKeyHex,
		AdminPassphrase: pass,
		Note:            "integration",
	})
	if err != nil {
		t.Fatalf("CreateBundleFromRequest: %v", err)
	}
	if jsonStr == "" || !strings.Contains(jsonStr, "peer_id") {
		t.Fatalf("unexpected bundle string")
	}
}

func TestBusinessP1_Sprint5_BI016_ListIssuanceHistory_AfterBundle(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	pass := "adm-issue-s5"
	if err := rt.InitAdminKey(pass); err != nil {
		t.Fatal(err)
	}
	info, err := rt.GetOnboardingInfo()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rt.CreateBundleFromRequest(IssueBundleRequest{
		DisplayName:     "HistUser",
		PeerID:          info.PeerID,
		PublicKeyHex:    info.PublicKeyHex,
		AdminPassphrase: pass,
	}); err != nil {
		t.Fatal(err)
	}
	hist, err := rt.ListIssuanceHistory()
	if err != nil {
		t.Fatalf("ListIssuanceHistory: %v", err)
	}
	if len(hist) == 0 {
		t.Fatal("expected issuance record")
	}
	found := false
	for _, r := range hist {
		if strings.Contains(r.DisplayName, "Hist") || r.PeerID == info.PeerID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("unexpected history %+v", hist)
	}
}
