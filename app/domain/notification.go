package domain

import "time"

const (
	NotificationTypeMention        = "mention"
	NotificationTypeReply          = "reply"
	NotificationTypeGroupAdd       = "group_add"
	NotificationTypeInviteRequest  = "invite_request"
	NotificationTypeInviteApproved = "invite_approved"
	NotificationTypeInviteRejected = "invite_rejected"
)

type Notification struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	GroupID     string    `json:"group_id"`
	ActorPeerID string    `json:"actor_peer_id"`
	TargetID    string    `json:"target_id"` // message_id, group_id, or invite_id
	Content     string    `json:"content"`
	IsRead      bool      `json:"is_read"`
	CreatedAt   time.Time `json:"created_at"`
}
