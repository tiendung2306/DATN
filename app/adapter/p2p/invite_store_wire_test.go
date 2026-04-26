package p2p

import (
	"bytes"
	"testing"
)

func TestWelcomeListWireRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := WelcomeListResponseV1{
		V: 1,
		Invites: []WelcomeListItemV1{
			{
				GroupID:      "group-1",
				Welcome:      []byte("welcome"),
				SourcePeerID: "peer-a",
				CreatedAt:    123,
			},
		},
	}

	if err := WriteInviteStoreJSONFrame(&buf, &want); err != nil {
		t.Fatalf("WriteInviteStoreJSONFrame: %v", err)
	}
	var got WelcomeListResponseV1
	if err := ReadInviteStoreJSONFrame(&buf, &got); err != nil {
		t.Fatalf("ReadInviteStoreJSONFrame: %v", err)
	}
	if got.V != want.V || len(got.Invites) != 1 {
		t.Fatalf("response mismatch: %+v", got)
	}
	if got.Invites[0].GroupID != "group-1" || string(got.Invites[0].Welcome) != "welcome" {
		t.Fatalf("invite mismatch: %+v", got.Invites[0])
	}
}
