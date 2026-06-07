package main

type DemoWorkspace struct {
	Name         string                `json:"name"`
	RepoRoot     string                `json:"repo_root"`
	AppDir       string                `json:"app_dir"`
	AppExe       string                `json:"app_exe"`
	Token        string                `json:"token"`
	GuiLane      DemoLaneConfig        `json:"gui_lane"`
	HeadlessLane DemoLaneConfig        `json:"headless_lane"`
	Instances    []DemoInstanceProfile `json:"instances,omitempty"`
	Partitions   []PartitionSpec       `json:"partitions,omitempty"`
}

type DemoLaneConfig struct {
	ID            string                `json:"id"`
	Label         string                `json:"label"`
	Description   string                `json:"description,omitempty"`
	RuntimeRoot   string                `json:"runtime_root"`
	TemplateRoot  string                `json:"template_root"`
	Instances     []DemoInstanceProfile `json:"instances"`
	DemoGroupID   string                `json:"demo_group_id,omitempty"`
	DemoOwnerNode string                `json:"demo_owner_node_id,omitempty"`
}

type DemoInstanceProfile struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	LaunchMode  string `json:"launch_mode"`
	RuntimeDir  string `json:"runtime_dir"`
	TemplateDir string `json:"template_dir,omitempty"`
	DBPath      string `json:"db_path"`
	BindIP      string `json:"bind_ip"`
	P2PPort     int    `json:"p2p_port"`
	ControlPort int    `json:"control_port"`
	Bootstrap   string `json:"bootstrap,omitempty"`
	Headless    bool   `json:"headless"`
	StoreNode   bool   `json:"store_node,omitempty"`
}

type InstanceStatus struct {
	Profile      DemoInstanceProfile `json:"profile"`
	Running      bool                `json:"running"`
	PID          int                 `json:"pid,omitempty"`
	LastError    string              `json:"last_error,omitempty"`
	LastWarning  string              `json:"last_warning,omitempty"`
	AppState     string              `json:"app_state,omitempty"`
	StartupStage string              `json:"startup_stage,omitempty"`
	P2PReady     bool                `json:"p2p_ready"`
	CryptoReady  bool                `json:"crypto_ready"`
	PeerID       string              `json:"peer_id,omitempty"`
	PeerCount    int                 `json:"peer_count"`
	GroupCount   int                 `json:"group_count"`
	Groups       []InstanceGroup     `json:"groups,omitempty"`
	LastSeenMs   int64               `json:"last_seen_ms,omitempty"`
}

type InstanceGroup struct {
	GroupID         string   `json:"group_id"`
	Epoch           uint64   `json:"epoch"`
	TokenHolder     string   `json:"token_holder"`
	TokenHolderPeer string   `json:"token_holder_peer_id,omitempty"`
	ActiveMembers   int      `json:"active_members"`
	ActiveView      []string `json:"active_view,omitempty"`
	TreeHashShort   string   `json:"tree_hash_short,omitempty"`
	IsHealing       bool     `json:"is_healing"`
}

type ControlSnapshot struct {
	Workspace    DemoWorkspace        `json:"workspace"`
	GuiLane      GuiLaneSnapshot      `json:"gui_lane"`
	HeadlessLane HeadlessLaneSnapshot `json:"headless_lane"`
	Jobs         []JobStatus          `json:"jobs"`
	Scenarios    []ScenarioSpec       `json:"scenarios"`
}

type GuiLaneSnapshot struct {
	Instances  []InstanceStatus `json:"instances"`
	Preflight  PreflightResult  `json:"preflight"`
	BuildJobID string           `json:"build_job_id,omitempty"`
}

type HeadlessLaneSnapshot struct {
	Instances   []InstanceStatus `json:"instances"`
	Topology    NetworkTopology  `json:"topology"`
	DemoCluster DemoClusterState `json:"demo_cluster"`
	Preflight   PreflightResult  `json:"preflight"`
	Warnings    []string         `json:"warnings,omitempty"`
	BuildJobID  string           `json:"build_job_id,omitempty"`
}

type DemoClusterState struct {
	GroupID           string               `json:"group_id"`
	OwnerNodeID       string               `json:"owner_node_id"`
	Ready             bool                 `json:"ready"`
	Eligible          bool                 `json:"eligible"`
	LastError         string               `json:"last_error,omitempty"`
	TargetNodeIDs     []string             `json:"target_node_ids,omitempty"`
	TargetPeerIDs     []string             `json:"target_peer_ids,omitempty"`
	TargetCount       int                  `json:"target_count"`
	EligibleCount     int                  `json:"eligible_count"`
	BlockingNodes     []string             `json:"blocking_nodes,omitempty"`
	MemberCount       int                  `json:"member_count"`
	Members           []DemoClusterMember  `json:"members,omitempty"`
	RecentMessages    []DemoClusterMessage `json:"recent_messages,omitempty"`
	GroupStatusDigest DemoGroupStatus      `json:"group_status_digest"`
}

type DemoClusterMember struct {
	PeerID      string `json:"peer_id"`
	DisplayName string `json:"display_name"`
	IsOnline    bool   `json:"is_online"`
	Role        string `json:"role,omitempty"`
}

type DemoClusterMessage struct {
	MessageID         string `json:"message_id"`
	Sender            string `json:"sender"`
	SenderDisplayName string `json:"sender_display_name"`
	Content           string `json:"content"`
	Timestamp         int64  `json:"timestamp"`
	IsMine            bool   `json:"is_mine"`
}

type DemoGroupStatus struct {
	GroupID           string   `json:"group_id,omitempty"`
	Epoch             uint64   `json:"epoch"`
	TokenHolder       string   `json:"token_holder,omitempty"`
	TokenHolderPeerID string   `json:"token_holder_peer_id,omitempty"`
	ActiveMembers     int      `json:"active_members"`
	ActiveView        []string `json:"active_view,omitempty"`
	TreeHashShort     string   `json:"tree_hash_short,omitempty"`
	IsHealing         bool     `json:"is_healing"`
}

type JobStatus struct {
	ID          string   `json:"id"`
	Lane        string   `json:"lane"`
	Kind        string   `json:"kind"`
	Title       string   `json:"title"`
	State       string   `json:"state"`
	Summary     string   `json:"summary,omitempty"`
	LogTail     []string `json:"log_tail,omitempty"`
	StartedAtMs int64    `json:"started_at_ms,omitempty"`
	EndedAtMs   int64    `json:"ended_at_ms,omitempty"`
}

type PartitionSpec struct {
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Clusters [][]string `json:"clusters"`
	Active   bool       `json:"active"`
}

type NetworkTopology struct {
	SharedNetwork     string             `json:"shared_network"`
	Active            bool               `json:"active"`
	PartitionNetworks []PartitionNetwork `json:"partition_networks,omitempty"`
	NodeNetworks      []NodeNetworkState `json:"node_networks,omitempty"`
}

type PartitionNetwork struct {
	Name  string   `json:"name"`
	Nodes []string `json:"nodes,omitempty"`
}

type NodeNetworkState struct {
	NodeID          string   `json:"node_id"`
	Networks        []string `json:"networks,omitempty"`
	PrimaryNetwork  string   `json:"primary_network,omitempty"`
	IsDockerManaged bool     `json:"is_docker_managed"`
}

type ScenarioSpec struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Steps       []ScenarioStep `json:"steps"`
}

type ScenarioStep struct {
	Action       string     `json:"action"`
	InstanceID   string     `json:"instance_id,omitempty"`
	Partition    [][]string `json:"partition,omitempty"`
	Message      string     `json:"message,omitempty"`
	Milliseconds int        `json:"milliseconds,omitempty"`
}

type ScenarioRunState struct {
	Running     bool   `json:"running"`
	ScenarioID  string `json:"scenario_id,omitempty"`
	StepIndex   int    `json:"step_index"`
	CurrentStep string `json:"current_step,omitempty"`
	LastError   string `json:"last_error,omitempty"`
	StartedAtMs int64  `json:"started_at_ms,omitempty"`
	EndedAtMs   int64  `json:"ended_at_ms,omitempty"`
}

type PreflightResult struct {
	OK       bool     `json:"ok"`
	Warnings []string `json:"warnings,omitempty"`
	Errors   []string `json:"errors,omitempty"`
}
