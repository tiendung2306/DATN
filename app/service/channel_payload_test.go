package service

import "testing"

func TestValidateChannelOutboundMessage_PostAndComment(t *testing.T) {
	validPost := `{"type":"post","title":"Roadmap","body":"Hello channel"}`
	if err := validateChannelOutboundMessage(validPost); err != nil {
		t.Fatalf("valid post should pass: %v", err)
	}

	validComment := `{"type":"comment","post_id":"post-1","body":"@Admin hi","mentions":[{"user_id":"peer-1","display_name":"Admin","start":0,"end":6}]}`
	if err := validateChannelOutboundMessage(validComment); err != nil {
		t.Fatalf("valid comment should pass: %v", err)
	}
}

func TestValidateChannelOutboundMessage_InvalidPayloads(t *testing.T) {
	cases := []string{
		`{"type":"comment","post_id":"","body":"x"}`,
		`{"type":"comment","post_id":"post-1","body":"x","mentions":[{"user_id":"","display_name":"A","start":0,"end":2}]}`,
		`{"type":"comment","post_id":"post-1","body":"hello","mentions":[{"user_id":"peer-1","display_name":"A","start":0,"end":8}]}`,
		`{"type":"unknown","body":"x"}`,
	}
	for _, tc := range cases {
		if err := validateChannelOutboundMessage(tc); err == nil {
			t.Fatalf("payload should fail validation: %s", tc)
		}
	}
}

func TestValidateChannelOutboundMessage_LegacyReplyAndPlainText(t *testing.T) {
	legacyReply := `{"type":"reply","parent_id":"post-1","content":"legacy works"}`
	if err := validateChannelOutboundMessage(legacyReply); err != nil {
		t.Fatalf("legacy reply should pass migration: %v", err)
	}
	if err := validateChannelOutboundMessage("plain legacy message"); err != nil {
		t.Fatalf("plain text should pass migration: %v", err)
	}
}
