package main

type DemoWorkspace struct {
	Name       string                `json:"name"`
	RepoRoot   string                `json:"repo_root"`
	AppDir     string                `json:"app_dir"`
	AppExe     string                `json:"app_exe"`
	Token      string                `json:"token"`
	Instances  []DemoInstanceProfile `json:"instances"`
	Partitions []PartitionSpec       `json:"partitions,omitempty"`
}

type DemoInstanceProfile struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	LaunchMode  string `json:"launch_mode"`
	RuntimeDir  string `json:"runtime_dir"`
	TemplateDir string `json:"template_dir,omitempty"`
	DBPath      string `json:"db_path"`
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
	Workspace DemoWorkspace    `json:"workspace"`
	Instances []InstanceStatus `json:"instances"`
	Firewall  []FirewallRule   `json:"firewall"`
	Scenarios []ScenarioSpec   `json:"scenarios"`
}

type PartitionSpec struct {
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Clusters [][]string `json:"clusters"`
	Active   bool       `json:"active"`
}

type FirewallRule struct {
	Name string `json:"name"`
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
