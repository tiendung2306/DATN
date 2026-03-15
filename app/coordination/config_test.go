package coordination

import (
	"errors"
	"testing"
	"time"
)

func TestDefaultConfig_IsValid(t *testing.T) {
	if err := DefaultConfig().Validate(); err != nil {
		t.Errorf("DefaultConfig should be valid: %v", err)
	}
}

func TestTestConfig_IsValid(t *testing.T) {
	if err := TestConfig().Validate(); err != nil {
		t.Errorf("TestConfig should be valid: %v", err)
	}
}

func TestValidate_RejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name   string
		modify func(*CoordinatorConfig)
	}{
		{"zero timeout", func(c *CoordinatorConfig) { c.TokenHolderTimeout = 0 }},
		{"negative timeout", func(c *CoordinatorConfig) { c.TokenHolderTimeout = -1 * time.Second }},
		{"zero heartbeat", func(c *CoordinatorConfig) { c.HeartbeatInterval = 0 }},
		{"zero PeerDeadAfter", func(c *CoordinatorConfig) { c.PeerDeadAfter = 0 }},
		{"zero MaxBatchedProposals", func(c *CoordinatorConfig) { c.MaxBatchedProposals = 0 }},
		{"negative rotation", func(c *CoordinatorConfig) { c.KeyRotationInterval = -1 * time.Second }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if err == nil {
				t.Error("expected validation error")
			}
			if !errors.Is(err, ErrInvalidConfig) {
				t.Errorf("error should wrap ErrInvalidConfig, got: %v", err)
			}
		})
	}
}

func TestValidate_AllowsZeroKeyRotation(t *testing.T) {
	cfg := DefaultConfig()
	cfg.KeyRotationInterval = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("zero KeyRotationInterval should be allowed (disables auto-rotation): %v", err)
	}
}
