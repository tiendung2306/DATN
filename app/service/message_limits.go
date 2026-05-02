package service

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Product-level text limits (Unicode code point / Go rune count after TrimSpace).
// GossipSub still caps wire frames (~1 MiB); these limits keep UX and MLS payloads predictable.
const (
	MaxDMMessageRunes            = 4000
	MaxChannelPostTitleRunes   = 160
	MaxChannelPostBodyRunes    = 4000
	MaxChannelCommentBodyRunes = MaxChannelPostBodyRunes
)

// ErrTextExceedsLimit marks outbound text longer than the configured product maximum.
var ErrTextExceedsLimit = errors.New("TEXT_TOO_LONG")

// validateDMOutboundMessage enforces the DM plaintext cap before MLS encrypt.
func validateDMOutboundMessage(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("ERR_MESSAGE_EMPTY: nội dung không được để trống")
	}
	n := utf8.RuneCountInString(trimmed)
	if n > MaxDMMessageRunes {
		return fmt.Errorf("%w: tin nhắn tối đa %d ký tự Unicode (hiện có %d). Nội dung rất dài sẽ được gửi dưới dạng file đã mã hóa trong bản cập nhật sau",
			ErrTextExceedsLimit, MaxDMMessageRunes, n)
	}
	return nil
}

// MessageLimitsDTO is exposed to the frontend via Wails so limits stay in sync with the backend.
type MessageLimitsDTO struct {
	DMMaxRunes             int `json:"dm_max_runes"`
	ChannelTitleMaxRunes   int `json:"channel_title_max_runes"`
	ChannelBodyMaxRunes    int `json:"channel_body_max_runes"`
	ChannelCommentMaxRunes int `json:"channel_comment_max_runes"`
}

// GetMessageLimits returns current product limits for chat and channel payloads.
func (r *Runtime) GetMessageLimits() MessageLimitsDTO {
	_ = r // no mutable state required
	return MessageLimitsDTO{
		DMMaxRunes:             MaxDMMessageRunes,
		ChannelTitleMaxRunes:   MaxChannelPostTitleRunes,
		ChannelBodyMaxRunes:    MaxChannelPostBodyRunes,
		ChannelCommentMaxRunes: MaxChannelCommentBodyRunes,
	}
}
