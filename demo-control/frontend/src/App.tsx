import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  Database,
  FolderOpen,
  GitBranch,
  Network,
  Play,
  Power,
  RefreshCcw,
  RotateCcw,
  Scissors,
  ShieldAlert,
  Square,
  Terminal,
  Zap,
} from 'lucide-react'
import { runtimeClient } from './runtimeClient'
import type { ControlSnapshot, InstanceStatus, PreflightResult, ScenarioRunState } from './types'

type Tab = 'instances' | 'topology' | 'scenarios'

function normalizeSnapshot(raw: ControlSnapshot): ControlSnapshot {
  return {
    ...raw,
    workspace: {
      ...raw.workspace,
      instances: raw.workspace?.instances ?? [],
    },
    instances: (raw.instances ?? []).map((inst) => ({
      ...inst,
      groups: inst.groups ?? [],
    })),
    firewall: raw.firewall ?? [],
    scenarios: raw.scenarios ?? [],
  }
}

function normalizePreflight(raw: PreflightResult): PreflightResult {
  return {
    ...raw,
    warnings: raw.warnings ?? [],
    errors: raw.errors ?? [],
  }
}

export default function App() {
  const [snapshot, setSnapshot] = useState<ControlSnapshot | null>(null)
  const [scenario, setScenario] = useState<ScenarioRunState | null>(null)
  const [preflight, setPreflight] = useState<PreflightResult | null>(null)
  const [activeTab, setActiveTab] = useState<Tab>('instances')
  const [selected, setSelected] = useState<string[]>([])
  const [busy, setBusy] = useState<string>('')
  const [error, setError] = useState<string>('')

  const refresh = useCallback(async () => {
    try {
      const [snap, sc] = await Promise.all([runtimeClient.getSnapshot(), runtimeClient.getScenarioState()])
      setSnapshot(normalizeSnapshot(snap as ControlSnapshot))
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

  useEffect(() => {
    void runtimeClient.preflight().then((result) => setPreflight(normalizePreflight(result as PreflightResult))).catch((err) => setError(String(err)))
  }, [])

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

  const selectedInstances = useMemo(() => {
    if (!snapshot) return []
    return snapshot.instances.filter((inst) => selected.includes(inst.profile.id))
  }, [selected, snapshot])

  return (
    <main className="app-shell">
      <aside className="rail">
        <div className="brand">SW</div>
        <button className={activeTab === 'instances' ? 'rail-btn active' : 'rail-btn'} onClick={() => setActiveTab('instances')} title="Instances">
          <Terminal size={20} />
        </button>
        <button className={activeTab === 'topology' ? 'rail-btn active' : 'rail-btn'} onClick={() => setActiveTab('topology')} title="Topology">
          <Network size={20} />
        </button>
        <button className={activeTab === 'scenarios' ? 'rail-btn active' : 'rail-btn'} onClick={() => setActiveTab('scenarios')} title="Scenarios">
          <GitBranch size={20} />
        </button>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <div>
            <h1>DATN Demo Control</h1>
            <p>{snapshot?.workspace.repo_root ?? 'Loading workspace'}</p>
          </div>
          <div className="toolbar">
            <button className="cmd primary" disabled={!!busy} onClick={() => run('start-all', runtimeClient.startAll)}>
              <Play size={16} /> Start All
            </button>
            <button className="cmd" disabled={!!busy} onClick={() => run('stop-all', runtimeClient.stopAll)}>
              <Square size={16} /> Stop All
            </button>
            <button className="cmd danger" disabled={!!busy} onClick={() => run('heal', runtimeClient.healAll)}>
              <ShieldAlert size={16} /> Heal Firewall
            </button>
            <button className="cmd" disabled={!!busy} onClick={() => void refresh()}>
              <RefreshCcw size={16} /> Refresh
            </button>
          </div>
        </header>

        {error ? <div className="alert danger-alert"><AlertTriangle size={16} /> {error}</div> : null}
        {preflight && (preflight.errors?.length || preflight.warnings?.length) ? (
          <div className="alert">
            <AlertTriangle size={16} />
            {[...(preflight.errors ?? []), ...(preflight.warnings ?? [])].slice(0, 4).join(' | ')}
          </div>
        ) : null}

        {activeTab === 'instances' && snapshot ? (
          <InstancesView
            snapshot={snapshot}
            selected={selected}
            setSelected={setSelected}
            busy={busy}
            run={run}
          />
        ) : null}

        {activeTab === 'topology' && snapshot ? (
          <TopologyView
            snapshot={snapshot}
            selected={selected}
            setSelected={setSelected}
            selectedInstances={selectedInstances}
            busy={busy}
            run={run}
          />
        ) : null}

        {activeTab === 'scenarios' && snapshot ? (
          <ScenariosView snapshot={snapshot} scenario={scenario} busy={busy} run={run} />
        ) : null}
      </section>
    </main>
  )
}

function InstancesView({
  snapshot,
  selected,
  setSelected,
  busy,
  run,
}: {
  snapshot: ControlSnapshot
  selected: string[]
  setSelected: (next: string[]) => void
  busy: string
  run: (label: string, fn: () => Promise<unknown>) => Promise<void>
}) {
  const toggle = (id: string) => {
    setSelected(selected.includes(id) ? selected.filter((item) => item !== id) : [...selected, id])
  }
  return (
    <div className="content-grid">
      <section className="instance-grid">
        {snapshot.instances.map((inst) => (
          <InstanceCard key={inst.profile.id} inst={inst} selected={selected.includes(inst.profile.id)} onToggle={() => toggle(inst.profile.id)} busy={busy} run={run} />
        ))}
      </section>
      <aside className="side-panel">
        <h2>Protocol Summary</h2>
        <div className="metric-row"><span>Running</span><strong>{snapshot.instances.filter((i) => i.running).length}/10</strong></div>
        <div className="metric-row"><span>Firewall Rules</span><strong>{snapshot.firewall.length}</strong></div>
        <div className="metric-row"><span>Selected</span><strong>{selected.length}</strong></div>
        <button className="wide cmd danger" disabled={!!busy} onClick={() => run('reset-all', runtimeClient.resetAll)}>
          <Database size={16} /> Reset All Runtimes
        </button>
        <div className="event-list">
          {snapshot.instances.flatMap((inst) => inst.groups ?? []).slice(0, 12).map((group, idx) => (
            <div className="event-row" key={`${group.group_id}-${idx}`}>
              <span>{group.group_id}</span>
              <strong>E{group.epoch}</strong>
              <code>{group.tree_hash_short || 'no-hash'}</code>
            </div>
          ))}
        </div>
      </aside>
    </div>
  )
}

function InstanceCard({
  inst,
  selected,
  onToggle,
  busy,
  run,
}: {
  inst: InstanceStatus
  selected: boolean
  onToggle: () => void
  busy: string
  run: (label: string, fn: () => Promise<unknown>) => Promise<void>
}) {
  const id = inst.profile.id
  return (
    <article className={selected ? 'node-card selected' : 'node-card'}>
      <header>
        <button className="select-dot" onClick={onToggle} aria-label={`Select ${id}`} />
        <div>
          <h3>{inst.profile.label}</h3>
          <p>{id} | p2p {inst.profile.p2p_port} | ctrl {inst.profile.control_port}</p>
        </div>
        <span className={inst.running ? 'status running' : 'status stopped'}>{inst.running ? 'RUN' : 'STOP'}</span>
      </header>
      <div className="node-body">
        <div className="metric-row"><span>App</span><strong>{inst.app_state || 'offline'}</strong></div>
        <div className="metric-row"><span>Stage</span><strong>{inst.startup_stage || '-'}</strong></div>
        <div className="metric-row"><span>Peers</span><strong>{inst.peer_count}</strong></div>
        <div className="metric-row"><span>Groups</span><strong>{inst.group_count}</strong></div>
        <code className="peer-id">{inst.peer_id || inst.profile.runtime_dir}</code>
      </div>
      {inst.last_error ? <p className="error-text">{inst.last_error}</p> : null}
      <footer>
        <button className="icon-btn" disabled={!!busy} onClick={() => run(`start-${id}`, () => runtimeClient.startInstance(id))} title="Start">
          <Play size={15} />
        </button>
        <button className="icon-btn" disabled={!!busy} onClick={() => run(`stop-${id}`, () => runtimeClient.stopInstance(id))} title="Stop">
          <Power size={15} />
        </button>
        <button className="icon-btn" disabled={!!busy} onClick={() => run(`restart-${id}`, () => runtimeClient.restartInstance(id))} title="Restart">
          <RotateCcw size={15} />
        </button>
        <button className="icon-btn" disabled={!!busy} onClick={() => run(`isolate-${id}`, () => runtimeClient.isolateNode(id))} title="Isolate">
          <Scissors size={15} />
        </button>
        <button className="icon-btn" disabled={!!busy} onClick={() => run(`sync-${id}`, () => runtimeClient.triggerOfflineSync(id))} title="Offline Sync">
          <Zap size={15} />
        </button>
        <button className="icon-btn" disabled={!!busy} onClick={() => run(`reset-${id}`, () => runtimeClient.resetInstance(id))} title="Reset DB">
          <Database size={15} />
        </button>
        <button className="icon-btn" disabled={!!busy} onClick={() => run(`folder-${id}`, () => runtimeClient.openRuntimeFolder(id))} title="Open Folder">
          <FolderOpen size={15} />
        </button>
      </footer>
    </article>
  )
}

function TopologyView({
  snapshot,
  selected,
  setSelected,
  selectedInstances,
  busy,
  run,
}: {
  snapshot: ControlSnapshot
  selected: string[]
  setSelected: (next: string[]) => void
  selectedInstances: InstanceStatus[]
  busy: string
  run: (label: string, fn: () => Promise<unknown>) => Promise<void>
}) {
  const clusterA = selected
  const clusterB = snapshot.instances.map((i) => i.profile.id).filter((id) => !clusterA.includes(id))
  return (
    <div className="topology-layout">
      <section>
        <div className="section-head">
          <h2>Connectivity Matrix</h2>
          <div className="toolbar">
            <button className="cmd" disabled={!selected.length || !!busy} onClick={() => run('partition-selected', () => runtimeClient.applyPartition({ id: 'manual', label: 'Manual split', clusters: [clusterA, clusterB], active: true }))}>
              <GitBranch size={16} /> Split Selected
            </button>
            <button className="cmd danger" disabled={!!busy} onClick={() => run('heal', runtimeClient.healAll)}>
              <ShieldAlert size={16} /> Heal All
            </button>
          </div>
        </div>
        <div className="matrix">
          {snapshot.instances.map((row) => (
            <button
              key={row.profile.id}
              className={selected.includes(row.profile.id) ? 'matrix-cell selected' : 'matrix-cell'}
              onClick={() => setSelected(selected.includes(row.profile.id) ? selected.filter((id) => id !== row.profile.id) : [...selected, row.profile.id])}
            >
              <span>{row.profile.id}</span>
              <strong>{row.peer_count}</strong>
            </button>
          ))}
        </div>
      </section>
      <aside className="side-panel">
        <h2>Partition Editor</h2>
        <p className="muted">Selected nodes become cluster A. All other nodes become cluster B.</p>
        <div className="cluster-box">
          <strong>Cluster A</strong>
          <span>{selectedInstances.map((i) => i.profile.id).join(', ') || 'none'}</span>
        </div>
        <div className="cluster-box">
          <strong>Cluster B</strong>
          <span>{clusterB.join(', ') || 'none'}</span>
        </div>
        <div className="event-list">
          {snapshot.firewall.map((rule) => (
            <div className="event-row" key={rule.name}>
              <ShieldAlert size={13} />
              <span>{rule.name}</span>
            </div>
          ))}
        </div>
      </aside>
    </div>
  )
}

function ScenariosView({
  snapshot,
  scenario,
  busy,
  run,
}: {
  snapshot: ControlSnapshot
  scenario: ScenarioRunState | null
  busy: string
  run: (label: string, fn: () => Promise<unknown>) => Promise<void>
}) {
  return (
    <div className="scenario-layout">
      <section className="scenario-list">
        {snapshot.scenarios.map((item) => (
          <article className="scenario-row" key={item.id}>
            <Activity size={20} />
            <div>
              <h3>{item.name}</h3>
              <p>{item.description}</p>
            </div>
            <button className="cmd primary" disabled={!!busy || scenario?.running} onClick={() => run(`scenario-${item.id}`, () => runtimeClient.runScenario(item.id))}>
              <Play size={16} /> Run
            </button>
          </article>
        ))}
      </section>
      <aside className="side-panel">
        <h2>Scenario State</h2>
        <div className="metric-row"><span>Running</span><strong>{scenario?.running ? 'yes' : 'no'}</strong></div>
        <div className="metric-row"><span>Scenario</span><strong>{scenario?.scenario_id || '-'}</strong></div>
        <div className="metric-row"><span>Step</span><strong>{scenario?.current_step || '-'}</strong></div>
        {scenario?.last_error ? <p className="error-text">{scenario.last_error}</p> : null}
      </aside>
    </div>
  )
}
