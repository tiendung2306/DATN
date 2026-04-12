package domain

// Message is a decrypted chat line for UI display.
type Message struct {
	GroupID   string
	Sender    string
	Content   string
	Timestamp int64
	IsMine    bool
}
