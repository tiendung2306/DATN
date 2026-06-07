export interface DemoWorkspace {
  name: string
  repo_root: string
  app_dir: string
  app_exe: string
  token: string
  gui_lane: DemoLaneConfig
  headless_lane: DemoLaneConfig
}

export interface DemoLaneConfig {
  id: string
  label: string
  description?: string
  runtime_root: string
  template_root: string
  instances: DemoInstanceProfile[]
  demo_group_id?: string
  demo_owner_node_id?: string
}

export interface DemoInstanceProfile {
  id: string
  label: string
  launch_mode: string
  runtime_dir: string
  template_dir?: string
  db_path: string
  bind_ip: string
  p2p_port: number
  control_port: number
  bootstrap?: string
  headless: boolean
  store_node?: boolean
}

export interface InstanceGroup {
  group_id: string
  epoch: number
  token_holder: string
  token_holder_peer_id?: string
  active_members: number
  active_view?: string[]
  tree_hash_short?: string
  is_healing: boolean
}

export interface InstanceStatus {
  profile: DemoInstanceProfile
  running: boolean
  pid?: number
  last_error?: string
  last_warning?: string
  app_state?: string
  startup_stage?: string
  p2p_ready: boolean
  crypto_ready: boolean
  peer_id?: string
  peer_count: number
  group_count: number
  groups?: InstanceGroup[]
  last_seen_ms?: number
}

export interface PartitionSpec {
  id: string
  label: string
  clusters: string[][]
  active: boolean
}

export interface PartitionNetwork {
  name: string
  nodes?: string[]
}

export interface NodeNetworkState {
  node_id: string
  networks?: string[]
  primary_network?: string
  is_docker_managed: boolean
}

export interface NetworkTopology {
  shared_network: string
  active: boolean
  partition_networks?: PartitionNetwork[]
  node_networks?: NodeNetworkState[]
}

export interface DemoClusterMember {
  peer_id: string
  display_name: string
  is_online: boolean
  role?: string
}

export interface DemoClusterMessage {
  message_id: string
  sender: string
  sender_display_name: string
  content: string
  timestamp: number
  is_mine: boolean
}

export interface DemoGroupStatus {
  group_id?: string
  epoch: number
  token_holder?: string
  token_holder_peer_id?: string
  active_members: number
  active_view?: string[]
  tree_hash_short?: string
  is_healing: boolean
}

export interface DemoClusterState {
  group_id: string
  owner_node_id: string
  ready: boolean
  eligible: boolean
  last_error?: string
  target_node_ids?: string[]
  target_peer_ids?: string[]
  target_count: number
  eligible_count: number
  blocking_nodes?: string[]
  member_count: number
  members?: DemoClusterMember[]
  recent_messages?: DemoClusterMessage[]
  group_status_digest: DemoGroupStatus
}

export interface JobStatus {
  id: string
  lane: string
  kind: string
  title: string
  state: string
  summary?: string
  log_tail?: string[]
  started_at_ms?: number
  ended_at_ms?: number
}

export interface GuiLaneSnapshot {
  instances: InstanceStatus[]
  preflight: PreflightResult
  build_job_id?: string
}

export interface HeadlessLaneSnapshot {
  instances: InstanceStatus[]
  topology: NetworkTopology
  demo_cluster: DemoClusterState
  preflight: PreflightResult
  warnings?: string[]
  build_job_id?: string
}

export interface ScenarioSpec {
  id: string
  name: string
  description: string
}

export interface ControlSnapshot {
  workspace: DemoWorkspace
  gui_lane: GuiLaneSnapshot
  headless_lane: HeadlessLaneSnapshot
  jobs: JobStatus[]
  scenarios: ScenarioSpec[]
}

export interface ScenarioRunState {
  running: boolean
  scenario_id?: string
  step_index: number
  current_step?: string
  last_error?: string
  started_at_ms?: number
  ended_at_ms?: number
}

export interface PreflightResult {
  ok: boolean
  warnings?: string[]
  errors?: string[]
}
