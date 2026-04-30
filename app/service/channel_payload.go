package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	maxChannelTitleLen = 160
	maxChannelBodyLen  = 4000
)

type channelMention struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name"`
	Start       int    `json:"start"`
	End         int    `json:"end"`
}

type channelPostPayload struct {
	Type  string `json:"type"`
	Title string `json:"title,omitempty"`
	Body  string `json:"body"`
}

type channelCommentPayload struct {
	Type             string           `json:"type"`
	PostID           string           `json:"post_id"`
	Body             string           `json:"body"`
	Mentions         []channelMention `json:"mentions,omitempty"`
	ReplyToCommentID string           `json:"reply_to_comment_id,omitempty"`
}

type legacyReplyPayload struct {
	Type     string `json:"type"`
	ParentID string `json:"parent_id"`
	Content  string `json:"content"`
}

func validateChannelOutboundMessage(raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: empty message body")
	}

	// Backward compatibility: allow plain text payloads during migration window.
	if !strings.HasPrefix(trimmed, "{") {
		if utf8.RuneCountInString(trimmed) > maxChannelBodyLen {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: body exceeds %d characters", maxChannelBodyLen)
		}
		return nil
	}

	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err != nil {
		return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: malformed JSON payload")
	}

	switch envelope.Type {
	case "post":
		var payload channelPostPayload
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: malformed post payload")
		}
		return validatePostPayload(payload)
	case "comment":
		var payload channelCommentPayload
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: malformed comment payload")
		}
		return validateCommentPayload(payload)
	case "reply":
		var payload legacyReplyPayload
		if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: malformed legacy reply payload")
		}
		if strings.TrimSpace(payload.ParentID) == "" {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: legacy reply missing parent_id")
		}
		bodyLen := utf8.RuneCountInString(strings.TrimSpace(payload.Content))
		if bodyLen == 0 {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: legacy reply body is required")
		}
		if bodyLen > maxChannelBodyLen {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: body exceeds %d characters", maxChannelBodyLen)
		}
		return nil
	default:
		return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: unsupported payload type %q", envelope.Type)
	}
}

func validatePostPayload(payload channelPostPayload) error {
	bodyLen := utf8.RuneCountInString(strings.TrimSpace(payload.Body))
	if bodyLen == 0 {
		return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: post body is required")
	}
	if bodyLen > maxChannelBodyLen {
		return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: body exceeds %d characters", maxChannelBodyLen)
	}
	if titleLen := utf8.RuneCountInString(strings.TrimSpace(payload.Title)); titleLen > maxChannelTitleLen {
		return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: title exceeds %d characters", maxChannelTitleLen)
	}
	return nil
}

func validateCommentPayload(payload channelCommentPayload) error {
	if strings.TrimSpace(payload.PostID) == "" {
		return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: comment requires post_id")
	}
	body := payload.Body
	bodyLen := utf8.RuneCountInString(strings.TrimSpace(body))
	if bodyLen == 0 {
		return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: comment body is required")
	}
	if bodyLen > maxChannelBodyLen {
		return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: body exceeds %d characters", maxChannelBodyLen)
	}
	if strings.TrimSpace(payload.ReplyToCommentID) != "" && strings.TrimSpace(payload.ReplyToCommentID) == strings.TrimSpace(payload.PostID) {
		return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: reply_to_comment_id must not equal post_id")
	}
	return validateMentionRanges(body, payload.Mentions)
}

func validateMentionRanges(body string, mentions []channelMention) error {
	if len(mentions) == 0 {
		return nil
	}
	runeLen := utf8.RuneCountInString(body)
	lastEnd := -1
	for idx, m := range mentions {
		if strings.TrimSpace(m.UserID) == "" {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: mention[%d] missing user_id", idx)
		}
		if strings.TrimSpace(m.DisplayName) == "" {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: mention[%d] missing display_name", idx)
		}
		if m.Start < 0 || m.End <= m.Start {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: mention[%d] has invalid range", idx)
		}
		if m.End > runeLen {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: mention[%d] exceeds body length", idx)
		}
		if lastEnd > m.Start {
			return fmt.Errorf("ERR_CHANNEL_PAYLOAD_INVALID: mention[%d] overlaps previous mention", idx)
		}
		lastEnd = m.End
	}
	return nil
}
