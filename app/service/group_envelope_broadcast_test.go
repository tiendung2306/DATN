package service

import (
	"bytes"
	"testing"
	"time"

	"app/coordination"
)

func TestAsyncEnvelopeBroadcast_ReturnsWithoutWaitingForPublisher(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})

	publisher := func(coordination.MessageType, string, []byte) {
		close(started)
		<-release
	}

	start := time.Now()
	asyncEnvelopeBroadcast(publisher, coordination.MsgApplication, "g1", []byte("payload"))
	elapsed := time.Since(start)

	select {
	case <-started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("publisher goroutine did not start")
	}

	if elapsed > 50*time.Millisecond {
		t.Fatalf("asyncEnvelopeBroadcast blocked too long: %v", elapsed)
	}

	close(release)
}

func TestAsyncEnvelopeBroadcast_CopiesWireBeforeDispatch(t *testing.T) {
	done := make(chan []byte, 1)
	publisher := func(_ coordination.MessageType, _ string, wire []byte) {
		done <- wire
	}

	wire := []byte("payload")
	asyncEnvelopeBroadcast(publisher, coordination.MsgApplication, "g1", wire)
	wire[0] = 'X'

	select {
	case got := <-done:
		if !bytes.Equal(got, []byte("payload")) {
			t.Fatalf("publisher saw mutated wire: got %q", string(got))
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("publisher did not receive wire")
	}
}
