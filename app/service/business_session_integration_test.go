//go:build business_integration

// BI-023–BI-025 session lifecycle (seed DB keys like session_health_admin_test).

package service

import (
	"errors"
	"testing"
)

func TestBusinessP1_Sprint5_BI023_GetSessionStatus_ActiveWhenStartedAtSet(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	if err := rt.db.SetConfig(sessionStartedAtConfigKey, encodeInt64Config(999)); err != nil {
		t.Fatal(err)
	}
	st, err := rt.GetSessionStatus()
	if err != nil {
		t.Fatal(err)
	}
	if st.State != SessionStateActive || st.SessionStartedAt != 999 {
		t.Fatalf("unexpected %+v", st)
	}
}

func TestBusinessP1_Sprint5_BI024_AcknowledgeSessionReplaced(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	if err := rt.db.SetConfig(sessionStartedAtConfigKey, encodeInt64Config(100)); err != nil {
		t.Fatal(err)
	}
	if err := rt.db.SetConfig(sessionReplacedAtConfigKey, encodeInt64Config(200)); err != nil {
		t.Fatal(err)
	}
	if err := rt.AcknowledgeSessionReplaced(); err != nil {
		t.Fatalf("AcknowledgeSessionReplaced: %v", err)
	}
}

func TestBusinessP1_Sprint5_BI025_SendBlockedWhenSessionReplaced(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "dm-s5-session"
	if err := rt.CreateGroupChat(gid, "dm", ""); err != nil {
		t.Fatalf("CreateGroupChat: %v", err)
	}
	if err := rt.db.SetConfig(sessionStartedAtConfigKey, encodeInt64Config(100)); err != nil {
		t.Fatal(err)
	}
	if err := rt.db.SetConfig(sessionReplacedAtConfigKey, encodeInt64Config(200)); err != nil {
		t.Fatal(err)
	}
	err := rt.SendGroupMessage(gid, "hello")
	if err == nil {
		t.Fatal("expected ErrSessionReplaced")
	}
	if !errors.Is(err, ErrSessionReplaced) {
		t.Fatalf("err=%v want ErrSessionReplaced", err)
	}
}
