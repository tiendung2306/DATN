package service

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateDMOutboundMessage_WithinLimit(t *testing.T) {
	s := strings.Repeat("a", MaxDMMessageRunes)
	if err := validateDMOutboundMessage(s); err != nil {
		t.Fatalf("expected nil: %v", err)
	}
}

func TestValidateDMOutboundMessage_ExceedsLimit(t *testing.T) {
	s := strings.Repeat("b", MaxDMMessageRunes+1)
	err := validateDMOutboundMessage(s)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrTextExceedsLimit) {
		t.Fatalf("expected ErrTextExceedsLimit wrap: %v", err)
	}
}

func TestValidateDMOutboundMessage_EmptyTrimmed(t *testing.T) {
	if err := validateDMOutboundMessage("   "); err == nil {
		t.Fatal("expected empty error")
	}
}

func TestRuntime_GetMessageLimits(t *testing.T) {
	r := NewRuntime(nil)
	lim := r.GetMessageLimits()
	if lim.DMMaxRunes != MaxDMMessageRunes || lim.ChannelTitleMaxRunes != MaxChannelPostTitleRunes {
		t.Fatalf("unexpected limits: %+v", lim)
	}
}
