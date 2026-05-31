export interface DemoWorkspace {
  name: string
  repo_root: string
  app_dir: string
  app_exe: string
  token: string
  instances: DemoInstanceProfile[]
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
  tree_hash_short?: string
  is_healing: boolean
}

export interface InstanceStatus {
  profile: DemoInstanceProfile
  running: boolean
  pid?: number
  last_error?: string
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

export interface FirewallRule {
  name: string
}

export interface ScenarioSpec {
  id: string
  name: string
  description: string
}

export interface ControlSnapshot {
  workspace: DemoWorkspace
  instances: InstanceStatus[]
  firewall: FirewallRule[]
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
