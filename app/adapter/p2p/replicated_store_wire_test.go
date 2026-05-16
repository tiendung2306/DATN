package p2p

import (
	"bytes"
	"testing"
)

func TestReplicaStoreFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	want := []byte(`{"v":1,"namespace":"user.profile.v1","record_key":"peer-a"}`)
	if err := replicaWriteFrame(&buf, want, maxReplicaMetaBytes); err != nil {
		t.Fatalf("replicaWriteFrame: %v", err)
	}
	got, err := replicaReadFrame(&buf, maxReplicaMetaBytes)
	if err != nil {
		t.Fatalf("replicaReadFrame: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("frame mismatch got %q want %q", got, want)
	}
}

func TestReplicaStoreFrameRejectsOversize(t *testing.T) {
	var buf bytes.Buffer
	if err := replicaWriteFrame(&buf, bytes.Repeat([]byte("x"), maxReplicaMetaBytes+1), maxReplicaMetaBytes); err == nil {
		t.Fatal("expected write oversize error")
	}
	tooLarge := make([]byte, maxReplicaMetaBytes+1)
	var lenBuf [4]byte
	lenBuf[0] = byte(len(tooLarge) >> 24)
	lenBuf[1] = byte(len(tooLarge) >> 16)
	lenBuf[2] = byte(len(tooLarge) >> 8)
	lenBuf[3] = byte(len(tooLarge))
	buf.Write(lenBuf[:])
	buf.Write(tooLarge)
	if _, err := replicaReadFrame(&buf, maxReplicaMetaBytes); err == nil {
		t.Fatal("expected read oversize error")
	}
}
