package service

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateOutboundByGroupType_DM(t *testing.T) {
	if err := validateOutboundByGroupType("dm", "hello"); err != nil {
		t.Fatalf("dm valid should pass: %v", err)
	}
	if err := validateOutboundByGroupType("dm", "   "); err == nil || !strings.Contains(err.Error(), "ERR_MESSAGE_EMPTY") {
		t.Fatalf("dm empty should fail with ERR_MESSAGE_EMPTY, got: %v", err)
	}
	tooLong := strings.Repeat("x", MaxDMMessageRunes+1)
	err := validateOutboundByGroupType("dm", tooLong)
	if err == nil {
		t.Fatal("dm over-limit should fail")
	}
	if !errors.Is(err, ErrTextExceedsLimit) {
		t.Fatalf("dm over-limit should wrap ErrTextExceedsLimit, got: %v", err)
	}
}

func TestValidateOutboundByGroupType_Channel(t *testing.T) {
	valid := `{"type":"post","title":"Roadmap","body":"Hello channel"}`
	if err := validateOutboundByGroupType("channel", valid); err != nil {
		t.Fatalf("channel valid should pass: %v", err)
	}
	if err := validateOutboundByGroupType("channel", "   "); err == nil || !strings.Contains(err.Error(), "ERR_CHANNEL_PAYLOAD_INVALID") {
		t.Fatalf("channel empty should fail with ERR_CHANNEL_PAYLOAD_INVALID, got: %v", err)
	}
}

func TestValidateOutboundByGroupType_Group(t *testing.T) {
	err := validateOutboundByGroupType("group", "hello")
	if err != nil {
		t.Fatalf("group valid should pass: %v", err)
	}
	if err := validateOutboundByGroupType("group", "   "); err == nil || !strings.Contains(err.Error(), "ERR_MESSAGE_EMPTY") {
		t.Fatalf("group empty should fail with ERR_MESSAGE_EMPTY, got: %v", err)
	}
}

func TestValidateOutboundByGroupType_UnsupportedType(t *testing.T) {
	err := validateOutboundByGroupType("invalid", "hello")
	if err == nil {
		t.Fatal("unsupported group type should fail")
	}
	if !strings.Contains(err.Error(), "ERR_GROUP_TYPE_INVALID") {
		t.Fatalf("expected ERR_GROUP_TYPE_INVALID, got: %v", err)
	}
}
