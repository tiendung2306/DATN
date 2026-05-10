//go:build business_integration

// Sprint 2 — BI-070–BI-076 (messaging).

package service

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"app/coordination"
)

func TestBusinessP1_Sprint2_BI070_SendAndListMessage(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "msg-070"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	text := "hello sprint 2"
	if err := rt.SendGroupMessage(gid, text); err != nil {
		t.Fatalf("SendGroupMessage: %v", err)
	}
	msgs, err := rt.GetGroupMessages(gid, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != text {
		t.Fatalf("content=%q", msgs[0].Content)
	}
}

func TestBusinessP1_Sprint2_BI071_SendEmpty_Blocked(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "msg-071"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	err := rt.SendGroupMessage(gid, "   ")
	if err == nil {
		t.Fatal("expected empty message error")
	}
	if !strings.Contains(err.Error(), "ERR_MESSAGE_EMPTY") {
		t.Fatalf("err=%v", err)
	}
}

func TestBusinessP1_Sprint2_BI072_SendTooLong_Blocked(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "msg-072"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("あ", MaxDMMessageRunes+1)
	err := rt.SendGroupMessage(gid, long)
	if err == nil {
		t.Fatal("expected length error")
	}
	if !errors.Is(err, ErrTextExceedsLimit) && !strings.Contains(err.Error(), "TEXT_TOO_LONG") {
		t.Fatalf("err=%v", err)
	}
}

func TestBusinessP1_Sprint2_BI073_GetGroupMessages_Pagination(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "msg-073"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if err := rt.SendGroupMessage(gid, strings.Repeat("x", i+3)); err != nil {
			t.Fatal(err)
		}
	}
	page1, err := rt.GetGroupMessages(gid, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != 2 {
		t.Fatalf("page1 len=%d", len(page1))
	}
	page2, err := rt.GetGroupMessages(gid, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 2 {
		t.Fatalf("page2 len=%d", len(page2))
	}
}

func TestBusinessP1_Sprint2_BI074_SendAfterLeave_Blocked(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "msg-074"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	if err := rt.LeaveGroup(gid); err != nil {
		t.Fatal(err)
	}
	err := rt.SendGroupMessage(gid, "nope")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not in group") {
		t.Fatalf("err=%v", err)
	}
}

func TestBusinessP1_Sprint2_BI075_RetryMessage(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "msg-075"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	body := "retry body"
	if err := rt.SendGroupMessage(gid, body); err != nil {
		t.Fatal(err)
	}
	msgs, err := rt.GetGroupMessages(gid, 5, 0)
	if err != nil || len(msgs) == 0 {
		t.Fatal("no messages")
	}
	mid := msgs[0].MessageID
	if err := rt.RetryMessage(gid, mid); err != nil {
		t.Fatalf("RetryMessage: %v", err)
	}
}

func TestBusinessP1_Sprint2_BI076_DeleteLocalMessage(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "msg-076"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	if err := rt.SendGroupMessage(gid, "to delete"); err != nil {
		t.Fatal(err)
	}
	msgs, err := rt.GetGroupMessages(gid, 5, 0)
	if err != nil || len(msgs) != 1 {
		t.Fatal("expected one message")
	}
	mid := msgs[0].MessageID
	if err := rt.DeleteLocalMessage(gid, mid); err != nil {
		t.Fatalf("DeleteLocalMessage: %v", err)
	}
	after, err := rt.GetGroupMessages(gid, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 0 {
		t.Fatalf("want 0 messages after delete, got %d", len(after))
	}
}

func TestBusinessP1_Sprint2_BI074_AccessRevoked_BlocksSend(t *testing.T) {
	rt, mock := businessRuntimeAuthorizedWithMockMLS(t)
	gid := "msg-074-revoke"
	if err := rt.CreateGroupChat(gid, "group", ""); err != nil {
		t.Fatal(err)
	}
	localPub := bizMLSIdentityPubFromRuntimeDB(t, rt)
	mock.SetHasMemberFunc(func(_ []byte, identity []byte) (bool, error) {
		return bytes.Equal(identity, localPub), nil
	})
	kp, err := rt.GenerateKeyPackage()
	if err != nil {
		t.Fatal(err)
	}
	if _, errAdd := rt.AddMemberToGroup(gid, testPeerID(t), kp.PublicHex); errAdd != nil {
		t.Fatalf("AddMemberToGroup: %v", errAdd)
	}
	mock.SetHasMemberFunc(func([]byte, []byte) (bool, error) {
		return false, nil
	})
	sendErr := rt.SendGroupMessage(gid, "revoked path")
	if !errors.Is(sendErr, coordination.ErrAccessRevoked) {
		t.Fatalf("SendGroupMessage err=%v want ErrAccessRevoked", sendErr)
	}
}
