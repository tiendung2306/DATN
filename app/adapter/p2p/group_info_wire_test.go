package p2p

import (
	"bytes"
	"strings"
	"testing"
)

func TestGroupInfoWire_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	req := GroupInfoRequestV1{
		V:               1,
		GroupID:         "group-1",
		WithRatchetTree: true,
	}
	if err := WriteGroupInfoJSONFrame(&buf, &req); err != nil {
		t.Fatalf("WriteGroupInfoJSONFrame: %v", err)
	}
	var got GroupInfoRequestV1
	if err := ReadGroupInfoJSONFrame(&buf, &got); err != nil {
		t.Fatalf("ReadGroupInfoJSONFrame: %v", err)
	}
	if got.V != req.V || got.GroupID != req.GroupID || got.WithRatchetTree != req.WithRatchetTree {
		t.Fatalf("decoded request mismatch: got=%+v want=%+v", got, req)
	}
}

func TestGroupInfoWire_RejectsEmptyOrOversizedFrame(t *testing.T) {
	var bad bytes.Buffer
	bad.Write([]byte{0, 0, 0, 0}) // invalid zero-length frame marker for this protocol
	var req GroupInfoRequestV1
	if err := ReadGroupInfoJSONFrame(&bad, &req); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("expected size validation error for zero frame, got: %v", err)
	}

	tooBig := map[string]string{"x": strings.Repeat("a", groupInfoMaxFrame+32)}
	if err := WriteGroupInfoJSONFrame(&bytes.Buffer{}, tooBig); err == nil || !strings.Contains(err.Error(), "size") {
		t.Fatalf("expected size validation error for oversized payload, got: %v", err)
	}
}
