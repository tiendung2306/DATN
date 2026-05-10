//go:build business_integration

// Sprint 2 — BI-077–BI-080 (channel posts / comments / pagination).

package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBusinessP1_Sprint2_BI077_ChannelPost_Listed(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	cat := businessEnsureCategory(t, rt, "BI-077")
	gid := "chan-077"
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	payload := `{"type":"post","title":"T","body":"Hello channel integration"}`
	if err := rt.SendGroupMessage(gid, payload); err != nil {
		t.Fatalf("SendGroupMessage: %v", err)
	}
	posts, err := rt.GetGroupPosts(gid, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 1 {
		t.Fatalf("want 1 post, got %d", len(posts))
	}
}

func TestBusinessP1_Sprint2_BI078_Channel_InvalidPayloadRejected(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	cat := businessEnsureCategory(t, rt, "BI-078")
	gid := "chan-078"
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	bad := `{"type":"comment","post_id":"","body":"x"}`
	err := rt.SendGroupMessage(gid, bad)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "ERR_CHANNEL_PAYLOAD_INVALID") {
		t.Fatalf("err=%v", err)
	}
}

func TestBusinessP1_Sprint2_BI079_ChannelComment_OnPost(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	cat := businessEnsureCategory(t, rt, "BI-079")
	gid := "chan-079"
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	postJSON := `{"type":"post","title":"Root","body":"Post body"}`
	if err := rt.SendGroupMessage(gid, postJSON); err != nil {
		t.Fatal(err)
	}
	posts, err := rt.GetGroupPosts(gid, 5, 0)
	if err != nil || len(posts) != 1 {
		t.Fatalf("posts=%v err=%v", posts, err)
	}
	postID := posts[0].MessageID

	commentPayload := map[string]string{
		"type":    "comment",
		"post_id": postID,
		"body":    "First comment",
	}
	cb, _ := json.Marshal(commentPayload)
	if err := rt.SendGroupMessage(gid, string(cb)); err != nil {
		t.Fatalf("comment send: %v", err)
	}
	comments, err := rt.GetPostComments(gid, postID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 1 {
		t.Fatalf("want 1 comment, got %d", len(comments))
	}
}

func TestBusinessP1_Sprint2_BI080_GetGroupPosts_PaginationExcludesComments(t *testing.T) {
	rt, _ := businessRuntimeAuthorizedWithMockMLS(t)
	cat := businessEnsureCategory(t, rt, "BI-080")
	gid := "chan-080"
	if err := rt.CreateGroupChat(gid, "channel", cat); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		p := `{"type":"post","title":"P","body":"` + strings.Repeat("b", i+1) + `"}`
		if err := rt.SendGroupMessage(gid, p); err != nil {
			t.Fatal(err)
		}
	}
	posts, err := rt.GetGroupPosts(gid, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 {
		t.Fatalf("want 2 posts on page, got %d", len(posts))
	}
	all, err := rt.GetGroupMessages(gid, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("total messages=%d want 3", len(all))
	}
}
