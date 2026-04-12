package port

import (
	"context"

	"app/domain"
)

// MessagingService sends and lists group chat messages.
type MessagingService interface {
	SendGroupMessage(ctx context.Context, groupID, text string) error
	GetGroupMessages(ctx context.Context, groupID string) ([]domain.Message, error)
}

// GroupService manages MLS groups, coordinators, and P2P-backed messaging runtime.
type GroupService interface {
	CreateGroupChat(ctx context.Context, groupID string) error
	JoinGroupWithWelcome(ctx context.Context, groupID, welcomeHex, keyPackageBundlePrivateHex string) error
	GetGroups(ctx context.Context) ([]domain.GroupInfo, error)
	GetGroupMembers(ctx context.Context, groupID string) ([]domain.MemberInfo, error)
	GetGroupStatus(ctx context.Context, groupID string) (map[string]interface{}, error)
	AddMemberToGroup(ctx context.Context, groupID, newMemberPeerID, keyPackageHex string) (welcomeHex string, err error)
	GenerateKeyPackage(ctx context.Context) (publicHex, bundlePrivateHex string, err error)
	StartRuntime(ctx context.Context) error
	StopRuntime()
}

// InviteService handles KeyPackage advertisement and group invites.
type InviteService interface {
	InvitePeerToGroup(ctx context.Context, peerIDStr, groupID string) error
	CheckDHTWelcome(ctx context.Context, groupID string) error
	AdvertiseKeyPackage(ctx context.Context) error
	GetKPStatus(ctx context.Context) map[string]interface{}
	OnPeerConnected(ctx context.Context, remotePeerID string)
}

// IdentityService covers onboarding and backup.
type IdentityService interface {
	GetAppState(ctx context.Context) string
	GetOnboardingInfo(ctx context.Context) (*domain.OnboardingInfo, error)
	GenerateKeys(ctx context.Context) (*domain.OnboardingInfo, error)
	ImportBundleData(ctx context.Context, bundleData []byte) error
	ExportBackup(ctx context.Context, passphrase string) ([]byte, error)
	ImportBackup(ctx context.Context, data []byte, passphrase string, force bool) error
}

// AdminService is admin PKI and bundle creation helpers.
type AdminService interface {
	InitAdminKey(ctx context.Context, passphrase string) error
	HasAdminKey(ctx context.Context) (bool, error)
	CreateBundle(ctx context.Context, req domain.CreateBundleRequest, bootstrapAddr string) ([]byte, error)
	CreateAndImportSelfBundle(ctx context.Context, displayName, passphrase, bootstrapAddr string) error
}

// NodeStatusProvider returns connected peers and display info for dashboard.
type NodeStatusProvider interface {
	GetNodeStatus(ctx context.Context) *NodeStatus
}

// NodeStatus is runtime P2P summary for UI.
type NodeStatus struct {
	State          string
	PeerID         string
	DisplayName    string
	IsRunning      bool
	ConnectedPeers []PeerInfo
}

// PeerInfo is one connected peer row.
type PeerInfo struct {
	ID          string
	DisplayName string
	Verified    bool
}
