import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  Activity,
  AlertTriangle,
  Bot,
  Boxes,
  Database,
  FolderOpen,
  GitBranch,
  Play,
  Power,
  RefreshCcw,
  RotateCcw,
  Scissors,
  Send,
  ShieldAlert,
  Square,
  Terminal,
  Upload,
  X,
  Zap,
} from 'lucide-react'
import { runtimeClient } from './runtimeClient'
import type {
  ControlSnapshot,
  DemoClusterMember,
  DemoClusterMessage,
  HeadlessLaneSnapshot,
  InstanceStatus,
  PreflightResult,
  ScenarioRunState,
} from './types'

type Tab = 'gui' | 'headless'

function normalizePreflight(raw?: PreflightResult): PreflightResult {
  return {
    ok: raw?.ok ?? true,
    warnings: raw?.warnings ?? [],
    errors: raw?.errors ?? [],
  }
}

function normalizeSnapshot(raw: ControlSnapshot): ControlSnapshot {
  return {
    ...raw,
    gui_lane: {
      ...raw.gui_lane,
      instances: raw.gui_lane?.instances ?? [],
      preflight: normalizePreflight(raw.gui_lane?.preflight),
    },
    headless_lane: {
      ...raw.headless_lane,
      instances: raw.headless_lane?.instances ?? [],
      topology: {
        shared_network: raw.headless_lane?.topology?.shared_network ?? 'datn_p2p_net',
        active: raw.headless_lane?.topology?.active ?? false,
        partition_networks: raw.headless_lane?.topology?.partition_networks ?? [],
        node_networks: raw.headless_lane?.topology?.node_networks ?? [],
      },
      demo_cluster: {
        group_id: raw.headless_lane?.demo_cluster?.group_id ?? 'demo',
        owner_node_id: raw.headless_lane?.demo_cluster?.owner_node_id ?? 'node-01',
        ready: raw.headless_lane?.demo_cluster?.ready ?? false,
        eligible: raw.headless_lane?.demo_cluster?.eligible ?? false,
        last_error: raw.headless_lane?.demo_cluster?.last_error ?? '',
        member_count: raw.headless_lane?.demo_cluster?.member_count ?? 0,
        members: raw.headless_lane?.demo_cluster?.members ?? [],
        recent_messages: raw.headless_lane?.demo_cluster?.recent_messages ?? [],
        group_status_digest: raw.headless_lane?.demo_cluster?.group_status_digest ?? {
          epoch: 0,
          active_members: 0,
          is_healing: false,
        },
      },
      preflight: normalizePreflight(raw.headless_lane?.preflight),
    },
    jobs: raw.jobs ?? [],
    scenarios: raw.scenarios ?? [],
  }
}

export default function App() {
  const [snapshot, setSnapshot] = useState<ControlSnapshot | null>(null)
  const [scenario, setScenario] = useState<ScenarioRunState | null>(null)
  const [activeTab, setActiveTab] = useState<Tab>('gui')
  const [busy, setBusy] = useState<string>('')
  const [error, setError] = useState<string>('')
  const [activeLogNodeId, setActiveLogNodeId] = useState<string | null>(null)
  const [selectedHeadless, setSelectedHeadless] = useState<string[]>([])
  const [messageDraft, setMessageDraft] = useState<string>('Hello from demo-control')

  const refresh = useCallback(async () => {
    try {
      const [snap, sc] = await Promise.all([runtimeClient.getSnapshot(), runtimeClient.getScenarioState()])
      setSnapshot(normalizeSnapshot(snap as unknown as ControlSnapshot))
      setScenario(sc as ScenarioRunState)
      setError('')
    } catch (err) {
      setError(String(err))
    }
  }, [])

  useEffect(() => {
    void refresh()
    const id = window.setInterval(() => void refresh(), 1800)
    return () => window.clearInterval(id)
  }, [refresh])

  const run = useCallback(
    async (label: string, fn: () => Promise<unknown>) => {
      setBusy(label)
      try {
        await fn()
        await refresh()
      } catch (err) {
        setError(String(err))
      } finally {
        setBusy('')
      }
    },
    [refresh],
  )

  const headlessRunning = useMemo(
    () => snapshot?.headless_lane.instances.filter((inst) => inst.running) ?? [],
    [snapshot],
  )

  const selectedHeadlessInstances = useMemo(
    () => headlessRunning.filter((inst) => selectedHeadless.includes(inst.profile.id)),
    [headlessRunning, selectedHeadless],
  )

  return (
    <main className="app-shell">
      <aside className="rail">
        <div className="brand">DC</div>
        <button className={activeTab === 'gui' ? 'rail-btn active' : 'rail-btn'} onClick={() => setActiveTab('gui')} title="GUI Demo">
          <Boxes size={20} />
        </button>
        <button className={activeTab === 'headless' ? 'rail-btn active' : 'rail-btn'} onClick={() => setActiveTab('headless')} title="Headless Demo">
          <Bot size={20} />
        </button>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <div>
            <h1>DATN Demo Control</h1>
            <p>{snapshot?.workspace.repo_root ?? 'Loading workspace'}</p>
          </div>
          <div className="topbar-meta">
            <span className="lane-pill">{activeTab === 'gui' ? 'GUI Demo' : 'Headless Demo'}</span>
            <button className="cmd" disabled={!!busy} onClick={() => void refresh()}>
              <RefreshCcw size={15} /> Refresh
            </button>
          </div>
        </header>

        {error ? <div className="alert danger-alert"><AlertTriangle size={16} /> {error}</div> : null}

        {activeTab === 'gui' && snapshot ? (
          <GuiLaneView
            snapshot={snapshot}
            busy={busy}
            run={run}
            setActiveLogNodeId={setActiveLogNodeId}
          />
        ) : null}

        {activeTab === 'headless' && snapshot ? (
          <HeadlessLaneView
            snapshot={snapshot}
            scenario={scenario}
            busy={busy}
            run={run}
            selected={selectedHeadless}
            setSelected={setSelectedHeadless}
            selectedInstances={selectedHeadlessInstances}
            messageDraft={messageDraft}
            setMessageDraft={setMessageDraft}
            setActiveLogNodeId={setActiveLogNodeId}
          />
        ) : null}
      </section>

      {activeLogNodeId ? <LogViewerModal nodeId={activeLogNodeId} onClose={() => setActiveLogNodeId(null)} /> : null}
    </main>
  )
}

function GuiLaneView({
  snapshot,
  busy,
  run,
  setActiveLogNodeId,
}: {
  snapshot: ControlSnapshot
  busy: string
  run: (label: string, fn: () => Promise<unknown>) => Promise<void>
  setActiveLogNodeId: (id: string | null) => void
}) {
  const gui = snapshot.gui_lane
  const runningCount = gui.instances.filter((inst) => inst.running).length

  return (
    <div className="lane-layout">
      <section className="lane-main">
        <div className="lane-hero">
          <div>
            <h2>GUI Demo</h2>
          </div>
          <div className="toolbar">
            <button className="cmd primary" disabled={!!busy} onClick={() => run('build-gui', runtimeClient.buildGuiDemo)}>
              <Terminal size={15} /> Build EXE
            </button>
            <button className="cmd" disabled={!!busy} onClick={() => run('start-gui', runtimeClient.startGuiLane)}>
              <Play size={15} /> Start All GUI
            </button>
            <button className="cmd" disabled={!!busy} onClick={() => run('stop-gui', runtimeClient.stopGuiLane)}>
              <Square size={15} /> Stop All GUI
            </button>
            <button className="cmd danger" disabled={!!busy} onClick={() => run('reset-gui', runtimeClient.resetGuiLane)}>
              <Database size={15} /> Reset All GUI
            </button>
          </div>
        </div>

        <PreflightBanner preflight={gui.preflight} />

        <section className="instance-grid gui-grid">
          {gui.instances.map((inst) => (
            <InstanceCard
              key={inst.profile.id}
              inst={inst}
              modeHint="GUI"
              actions={
                <>
                  <button className="icon-btn" disabled={!!busy} onClick={() => run(`start-${inst.profile.id}`, () => runtimeClient.startInstance(inst.profile.id))} title="Start">
                    <Play size={15} />
                  </button>
                  <button className="icon-btn" disabled={!!busy} onClick={() => run(`stop-${inst.profile.id}`, () => runtimeClient.stopInstance(inst.profile.id))} title="Stop">
                    <Power size={15} />
                  </button>
                  <button className="icon-btn" disabled={!!busy} onClick={() => run(`restart-${inst.profile.id}`, () => runtimeClient.restartInstance(inst.profile.id))} title="Restart">
                    <RotateCcw size={15} />
                  </button>
                  <button className="icon-btn" disabled={!!busy} onClick={() => run(`reset-${inst.profile.id}`, () => runtimeClient.resetInstance(inst.profile.id))} title="Reset Runtime">
                    <Database size={15} />
                  </button>
                  <button className="icon-btn" disabled={!!busy} onClick={() => run(`capture-${inst.profile.id}`, () => runtimeClient.captureRuntimeAsTemplate(inst.profile.id))} title="Capture Runtime As Template">
                    <Upload size={15} />
                  </button>
                  <button className="icon-btn" disabled={!!busy} onClick={() => run(`folder-${inst.profile.id}`, () => runtimeClient.openRuntimeFolder(inst.profile.id))} title="Open Runtime Folder">
                    <FolderOpen size={15} />
                  </button>
                  <button className="icon-btn" disabled={!inst.running} onClick={() => setActiveLogNodeId(inst.profile.id)} title="View Logs">
                    <Terminal size={15} />
                  </button>
                </>
              }
            />
          ))}
        </section>
      </section>

      <aside className="lane-side">
        <SummaryCard title="Lane Summary">
          <MetricRow label="Running" value={`${runningCount}/${gui.instances.length}`} />
          <MetricRow label="Build Flow" value="opens terminal" />
          <MetricRow label="Artifact" value={snapshot.workspace.app_exe} monospace />
        </SummaryCard>
      </aside>
    </div>
  )
}

function HeadlessLaneView({
  snapshot,
  scenario,
  busy,
  run,
  selected,
  setSelected,
  selectedInstances,
  messageDraft,
  setMessageDraft,
  setActiveLogNodeId,
}: {
  snapshot: ControlSnapshot
  scenario: ScenarioRunState | null
  busy: string
  run: (label: string, fn: () => Promise<unknown>) => Promise<void>
  selected: string[]
  setSelected: (ids: string[]) => void
  selectedInstances: InstanceStatus[]
  messageDraft: string
  setMessageDraft: (next: string) => void
  setActiveLogNodeId: (id: string | null) => void
}) {
  const lane = snapshot.headless_lane
  const running = lane.instances.filter((inst) => inst.running)
  const clusterA = selected
  const clusterB = running.map((inst) => inst.profile.id).filter((id) => !clusterA.includes(id))
  const nodeNetworks = new Map(lane.topology.node_networks?.map((node) => [node.node_id, node]) ?? [])

  const toggleSelected = (id: string) => {
    setSelected(selected.includes(id) ? selected.filter((item) => item !== id) : [...selected, id])
  }

  return (
    <div className="lane-layout">
      <section className="lane-main">
        <div className="lane-hero">
          <div>
            <h2>Headless Demo</h2>
          </div>
          <div className="toolbar">
            <button className="cmd primary" disabled={!!busy} onClick={() => run('build-headless', runtimeClient.buildHeadlessImage)}>
              <Terminal size={15} /> Build Image
            </button>
            <button className="cmd" disabled={!!busy} onClick={() => run('start-headless', runtimeClient.startHeadlessLane)}>
              <Play size={15} /> Start All Headless
            </button>
            <button className="cmd" disabled={!!busy} onClick={() => run('prepare-demo', () => runtimeClient.prepareDemoCluster(''))}>
              <Zap size={15} /> Prepare Demo Cluster
            </button>
            <button className="cmd danger" disabled={!!busy} onClick={() => run('heal-network', runtimeClient.healAll)}>
              <ShieldAlert size={15} /> Heal Network
            </button>
            <button className="cmd danger" disabled={!!busy} onClick={() => run('reset-headless', runtimeClient.resetHeadlessLane)}>
              <Database size={15} /> Reset Headless
            </button>
          </div>
        </div>

        <PreflightBanner preflight={lane.preflight} />

        <section className="grid-two">
          <div className="panel">
            <div className="panel-head">
              <h3>Headless Nodes</h3>
              <span className="muted">{running.length}/{lane.instances.length} running</span>
            </div>
            <div className="instance-grid headless-grid">
              {lane.instances.map((inst) => {
                const network = nodeNetworks.get(inst.profile.id)?.primary_network ?? '-'
                return (
                  <InstanceCard
                    key={inst.profile.id}
                    inst={inst}
                    modeHint={network}
                    selected={selected.includes(inst.profile.id)}
                    onToggle={() => toggleSelected(inst.profile.id)}
                    actions={
                      <>
                        <button className="icon-btn" disabled={!!busy} onClick={() => run(`start-${inst.profile.id}`, () => runtimeClient.startInstance(inst.profile.id))} title="Start">
                          <Play size={15} />
                        </button>
                        <button className="icon-btn" disabled={!!busy} onClick={() => run(`stop-${inst.profile.id}`, () => runtimeClient.stopInstance(inst.profile.id))} title="Stop">
                          <Power size={15} />
                        </button>
                        <button className="icon-btn" disabled={!!busy} onClick={() => run(`restart-${inst.profile.id}`, () => runtimeClient.restartInstance(inst.profile.id))} title="Restart">
                          <RotateCcw size={15} />
                        </button>
                        <button className="icon-btn" disabled={!!busy} onClick={() => run(`isolate-${inst.profile.id}`, () => runtimeClient.isolateNode(inst.profile.id))} title="Isolate Node">
                          <Scissors size={15} />
                        </button>
                        <button className="icon-btn" disabled={!!busy} onClick={() => run(`sync-${inst.profile.id}`, () => runtimeClient.triggerOfflineSync(inst.profile.id))} title="Trigger Offline Sync">
                          <Zap size={15} />
                        </button>
                        <button className="icon-btn" disabled={!!busy} onClick={() => run(`reset-${inst.profile.id}`, () => runtimeClient.resetInstance(inst.profile.id))} title="Reset Runtime">
                          <Database size={15} />
                        </button>
                        <button className="icon-btn" disabled={!!busy} onClick={() => run(`capture-${inst.profile.id}`, () => runtimeClient.captureRuntimeAsTemplate(inst.profile.id))} title="Capture Runtime As Template">
                          <Upload size={15} />
                        </button>
                        <button className="icon-btn" disabled={!inst.running} onClick={() => setActiveLogNodeId(inst.profile.id)} title="View Logs">
                          <Terminal size={15} />
                        </button>
                      </>
                    }
                  />
                )
              })}
            </div>
          </div>

          <div className="stack-panel">
            <SummaryCard title="Demo Group">
              <MetricRow label="Group" value={lane.demo_cluster.group_id} monospace />
              <MetricRow label="Owner" value={lane.demo_cluster.owner_node_id} monospace />
              <MetricRow label="Ready" value={lane.demo_cluster.ready ? 'yes' : 'no'} />
              <MetricRow label="Eligible" value={lane.demo_cluster.eligible ? 'yes' : 'no'} />
              <MetricRow label="Members" value={String(lane.demo_cluster.member_count)} />
              <MetricRow label="Epoch" value={String(lane.demo_cluster.group_status_digest.epoch ?? 0)} />
              <MetricRow label="Token Holder" value={lane.demo_cluster.group_status_digest.token_holder_peer_id || lane.demo_cluster.group_status_digest.token_holder || '-'} monospace />
              <MetricRow label="Tree Hash" value={lane.demo_cluster.group_status_digest.tree_hash_short || '-'} monospace />
              {lane.demo_cluster.last_error ? <p className="error-text">{lane.demo_cluster.last_error}</p> : null}
            </SummaryCard>

            <div className="panel">
              <div className="panel-head">
                <h3>Message Console</h3>
                <span className="muted">{selectedInstances.length ? `${selectedInstances.length} selected` : 'select nodes below'}</span>
              </div>
              <textarea
                className="message-box"
                value={messageDraft}
                onChange={(event) => setMessageDraft(event.target.value)}
                placeholder="Type a Demo group message"
              />
              <div className="toolbar compact">
                <button
                  className="cmd primary"
                  disabled={!!busy || selectedInstances.length === 0 || !messageDraft.trim()}
                  onClick={() => run('send-demo', () => runtimeClient.sendDemoMessage(selectedInstances.map((inst) => inst.profile.id), messageDraft))}
                >
                  <Send size={15} /> Send To Selected
                </button>
                <button className="cmd" disabled={!!busy} onClick={() => setMessageDraft('Network partition checkpoint')}>
                  Quick: Checkpoint
                </button>
                <button className="cmd" disabled={!!busy} onClick={() => setMessageDraft('Replay sync validation after heal')}>
                  Quick: Replay
                </button>
              </div>
            </div>
          </div>
        </section>

        <section className="grid-two">
          <div className="panel">
            <div className="panel-head">
              <h3>Partition Topology</h3>
              <div className="toolbar compact">
                <button
                  className="cmd"
                  disabled={!clusterA.length || clusterA.length === running.length || !!busy}
                  onClick={() => run('partition-selected', () => runtimeClient.applyPartition({ id: 'manual', label: 'Manual split', clusters: [clusterA, clusterB], active: true }))}
                >
                  <GitBranch size={15} /> Split Selected
                </button>
                <button className="cmd danger" disabled={!!busy} onClick={() => run('heal-network', runtimeClient.healAll)}>
                  <ShieldAlert size={15} /> Heal
                </button>
              </div>
            </div>
            <TopologyBoard lane={lane} />
            <div className="cluster-grid">
              <ClusterColumn title="Cluster A" ids={clusterA} />
              <ClusterColumn title="Cluster B" ids={clusterB} />
            </div>
          </div>

          <div className="stack-panel">
            <MemberPanel members={lane.demo_cluster.members ?? []} />
            <TimelinePanel messages={lane.demo_cluster.recent_messages ?? []} />
          </div>
        </section>

        <section className="grid-two">
          <div className="panel">
            <div className="panel-head">
              <h3>Scenarios</h3>
              <span className="muted">Headless lane only</span>
            </div>
            <div className="scenario-list">
              {snapshot.scenarios.map((item) => (
                <article className="scenario-row" key={item.id}>
                  <Activity size={18} />
                  <div>
                    <h4>{item.name}</h4>
                    <p>{item.description}</p>
                  </div>
                  <button className="cmd primary" disabled={!!busy || !!scenario?.running} onClick={() => run(`scenario-${item.id}`, () => runtimeClient.runScenario(item.id))}>
                    <Play size={15} /> Run
                  </button>
                </article>
              ))}
            </div>
          </div>

          <div className="stack-panel">
            <SummaryCard title="Scenario State">
              <MetricRow label="Running" value={scenario?.running ? 'yes' : 'no'} />
              <MetricRow label="Scenario" value={scenario?.scenario_id || '-'} />
              <MetricRow label="Step" value={scenario?.current_step || '-'} />
              {scenario?.last_error ? <p className="error-text">{scenario.last_error}</p> : null}
            </SummaryCard>
            <SummaryCard title="Image Build">
              <MetricRow label="Flow" value="opens terminal" />
              <MetricRow label="Image" value="secure-p2p:latest" monospace />
            </SummaryCard>
          </div>
        </section>
      </section>

      <aside className="lane-side">
        <SummaryCard title="Cluster Summary">
          <MetricRow label="Running" value={`${running.length}/${lane.instances.length}`} />
          <MetricRow label="Active Partitions" value={String(lane.topology.partition_networks?.length ?? 0)} />
          <MetricRow label="Shared Network" value={lane.topology.shared_network} monospace />
          <MetricRow label="Selected Nodes" value={selected.join(', ') || '-'} monospace />
        </SummaryCard>

        <ActionPanel title="Diagnostics">
          <button className="cmd" disabled={!!busy || selectedInstances.length !== 1} onClick={() => run('export-diagnostics', () => runtimeClient.exportDiagnostics(selectedInstances[0].profile.id))}>
            <Database size={15} /> Export Diagnostics
          </button>
          <button className="cmd" disabled={!!busy || selectedInstances.length !== 1} onClick={() => run('open-runtime', () => runtimeClient.openRuntimeFolder(selectedInstances[0].profile.id))}>
            <FolderOpen size={15} /> Open Runtime
          </button>
        </ActionPanel>
      </aside>
    </div>
  )
}

function PreflightBanner({ preflight }: { preflight: PreflightResult }) {
  if (!(preflight.errors?.length || preflight.warnings?.length)) {
    return null
  }
  return (
    <div className={preflight.errors?.length ? 'alert danger-alert' : 'alert'}>
      <AlertTriangle size={16} />
      <span>{[...(preflight.errors ?? []), ...(preflight.warnings ?? [])].join(' | ')}</span>
    </div>
  )
}

function InstanceCard({
  inst,
  modeHint,
  selected,
  onToggle,
  actions,
}: {
  inst: InstanceStatus
  modeHint: string
  selected?: boolean
  onToggle?: () => void
  actions: ReactNode
}) {
  return (
    <article className={selected ? 'node-card selected' : 'node-card'}>
      <header>
        <button className="select-dot" onClick={onToggle} aria-label={`Select ${inst.profile.id}`} disabled={!onToggle} />
        <div>
          <h3>{inst.profile.label}</h3>
          <p>{inst.profile.id} | p2p {inst.profile.p2p_port} | ctrl {inst.profile.control_port}</p>
        </div>
        <span className={inst.running ? 'status running' : 'status stopped'}>{inst.running ? 'RUN' : 'STOP'}</span>
      </header>
      <div className="node-body">
        <MetricRow label="Mode" value={inst.profile.launch_mode} />
        <MetricRow label="App" value={inst.app_state || 'offline'} />
        <MetricRow label="Stage" value={inst.startup_stage || '-'} />
        <MetricRow label="Peers" value={String(inst.peer_count)} />
        <MetricRow label="Groups" value={String(inst.group_count)} />
        <MetricRow label="Lane Hint" value={modeHint} monospace />
      </div>
      <code className="peer-id">{inst.peer_id || inst.profile.runtime_dir}</code>
      {inst.last_error ? <p className="error-text">{inst.last_error}</p> : null}
      <footer>{actions}</footer>
    </article>
  )
}

function SummaryCard({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="panel">
      <div className="panel-head">
        <h3>{title}</h3>
      </div>
      <div className="panel-body">{children}</div>
    </section>
  )
}

function ActionPanel({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="panel">
      <div className="panel-head">
        <h3>{title}</h3>
      </div>
      <div className="action-stack">{children}</div>
    </section>
  )
}

function ClusterColumn({ title, ids }: { title: string; ids: string[] }) {
  return (
    <div className="cluster-box">
      <strong>{title}</strong>
      <span>{ids.length ? ids.join(', ') : 'none'}</span>
    </div>
  )
}

function TopologyBoard({ lane }: { lane: HeadlessLaneSnapshot }) {
  const networks = lane.topology.partition_networks ?? []
  const sharedNodes = (lane.topology.node_networks ?? [])
    .filter((node) => node.primary_network === lane.topology.shared_network)
    .map((node) => node.node_id)

  return (
    <div className="topology-board">
      <div className="topology-network">
        <h4>Shared Network</h4>
        <code>{lane.topology.shared_network}</code>
        <div className="chip-row">
          {sharedNodes.length ? sharedNodes.map((id) => <span className="node-chip" key={id}>{id}</span>) : <span className="muted">No nodes attached</span>}
        </div>
      </div>
      {networks.map((network) => (
        <div className="topology-network warning" key={network.name}>
          <h4>{network.name}</h4>
          <div className="chip-row">
            {(network.nodes ?? []).length ? (network.nodes ?? []).map((id) => <span className="node-chip danger" key={id}>{id}</span>) : <span className="muted">No nodes attached</span>}
          </div>
        </div>
      ))}
    </div>
  )
}

function MemberPanel({ members }: { members: DemoClusterMember[] }) {
  return (
    <section className="panel">
      <div className="panel-head">
        <h3>Member Roster</h3>
        <span className="muted">{members.length} members</span>
      </div>
      <div className="member-list">
        {members.length ? members.map((member) => (
          <div className="member-row" key={member.peer_id}>
            <div>
              <strong>{member.display_name || member.peer_id}</strong>
              <code>{member.peer_id}</code>
            </div>
            <span className={member.is_online ? 'status running' : 'status stopped'}>{member.is_online ? member.role || 'online' : 'offline'}</span>
          </div>
        )) : <p className="muted">No roster data yet.</p>}
      </div>
    </section>
  )
}

function TimelinePanel({ messages }: { messages: DemoClusterMessage[] }) {
  return (
    <section className="panel">
      <div className="panel-head">
        <h3>Timeline Inspector</h3>
        <span className="muted">{messages.length} recent messages</span>
      </div>
      <div className="timeline-list">
        {messages.length ? messages.map((message) => (
          <article className="timeline-item" key={message.message_id}>
            <div className="timeline-meta">
              <strong>{message.sender_display_name || message.sender}</strong>
              <span>{formatTimestamp(message.timestamp)}</span>
            </div>
            <p>{message.content}</p>
          </article>
        )) : <p className="muted">Send messages into the Demo group to inspect them here.</p>}
      </div>
    </section>
  )
}

function MetricRow({ label, value, monospace }: { label: string; value: string; monospace?: boolean }) {
  return (
    <div className="metric-row">
      <span>{label}</span>
      <strong className={monospace ? 'mono' : ''}>{value}</strong>
    </div>
  )
}

function formatTimestamp(timestamp: number) {
  if (!timestamp) return '-'
  const date = new Date(timestamp)
  return date.toLocaleTimeString()
}

function LogViewerModal({
  nodeId,
  onClose,
}: {
  nodeId: string
  onClose: () => void
}) {
  const [logs, setLogs] = useState<string>('Loading logs...')
  const [autoScroll, setAutoScroll] = useState(true)
  const preRef = useRef<HTMLPreElement | null>(null)

  const fetchLogs = useCallback(async () => {
    try {
      const tail = await runtimeClient.readInstanceLogTail(nodeId, 250)
      setLogs(tail as string)
    } catch (err) {
      setLogs(`Error loading logs: ${err}`)
    }
  }, [nodeId])

  useEffect(() => {
    void fetchLogs()
    const intervalId = window.setInterval(() => void fetchLogs(), 1200)
    return () => window.clearInterval(intervalId)
  }, [fetchLogs])

  useEffect(() => {
    if (autoScroll && preRef.current) {
      preRef.current.scrollTop = preRef.current.scrollHeight
    }
  }, [logs, autoScroll])

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal-content" onClick={(event) => event.stopPropagation()}>
        <header className="modal-header">
          <div className="modal-title">
            <Terminal size={18} />
            <h2>Live Logs - {nodeId}</h2>
          </div>
          <div className="toolbar compact">
            <button className="cmd" onClick={() => runtimeClient.openInstanceLog(nodeId)}>
              <FolderOpen size={13} /> Raw File
            </button>
            <button className="icon-btn" onClick={onClose}>
              <X size={16} />
            </button>
          </div>
        </header>
        <div className="modal-body">
          <pre ref={preRef} className="log-container">{logs || 'Waiting for log output...'}</pre>
        </div>
        <footer className="modal-footer">
          <label className="checkbox-row">
            <input type="checkbox" checked={autoScroll} onChange={(event) => setAutoScroll(event.target.checked)} />
            Auto-scroll to bottom
          </label>
          <button className="cmd primary" onClick={onClose}>Close</button>
        </footer>
      </div>
    </div>
  )
}
