package domain

// GroupRole indicates the local node's relationship to a group.
type GroupRole string

const (
	RoleCreator GroupRole = "creator"
	RoleMember  GroupRole = "member"
)

// GroupInfo is a summary of a joined group for UI / API.
type GroupInfo struct {
	GroupID string
	Epoch   uint64
	MyRole  string
}

// GroupRecord is persisted MLS group state (opaque blob to Go).
type GroupRecord struct {
	GroupID    string
	GroupState []byte
	Epoch      uint64
	TreeHash   []byte
	MyRole     GroupRole
}

// MemberInfo describes a peer in the coordination active view.
type MemberInfo struct {
	PeerID   string
	IsOnline bool
}

// GroupStatus is live coordination metrics for a group.
type GroupStatus struct {
	GroupID           string
	Epoch             uint64
	IsTokenHolder     bool
	ActiveMembers     int
	CommitsIssued     int64
	ProposalsReceived int64
	CommitBytesTotal  int64
	PartitionsDetected int64
}
