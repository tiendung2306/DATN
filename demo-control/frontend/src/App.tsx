import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  Activity,
  AlertTriangle,
  Crown,
  Database,
  FolderOpen,
  GitBranch,
  Network,
  Play,
  Power,
  RefreshCcw,
  RotateCcw,
  Scissors,
  Shield,
  ShieldAlert,
  Square,
  Terminal,
  X,
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
  const [activeLogNodeId, setActiveLogNodeId] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const [snap, sc, pref] = await Promise.all([
        runtimeClient.getSnapshot(),
        runtimeClient.getScenarioState(),
        runtimeClient.preflight(),
      ])
      setSnapshot(normalizeSnapshot(snap as ControlSnapshot))
      setScenario(sc as ScenarioRunState)
      setPreflight(normalizePreflight(pref as PreflightResult))
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
              <ShieldAlert size={16} /> Heal P2P Network
            </button>
            <button className="cmd" disabled={!!busy} onClick={() => run('rebuild-docker', runtimeClient.rebuildDockerImage)} title="Rebuild secure-p2p:latest Docker image">
              <RefreshCcw size={16} /> Rebuild Docker
            </button>
            <button className="cmd" disabled={!!busy} onClick={() => void refresh()}>
              <RefreshCcw size={16} /> Refresh
            </button>
          </div>
        </header>

        {error ? <div className="alert danger-alert"><AlertTriangle size={16} /> {error}</div> : null}
        {preflight && (preflight.errors?.length || preflight.warnings?.length) ? (
          <div className="alert" style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: '16px' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
              <AlertTriangle size={16} style={{ flexShrink: 0 }} />
              <span>{[...(preflight.errors ?? []), ...(preflight.warnings ?? [])].slice(0, 4).join(' | ')}</span>
            </div>
            {preflight.warnings?.some(w => w.includes("secure-p2p:latest")) && (
              <button 
                className="cmd primary" 
                style={{ minHeight: '28px', padding: '0 10px', fontSize: '12px', flexShrink: 0 }}
                disabled={!!busy}
                onClick={() => run('rebuild-docker', runtimeClient.rebuildDockerImage)}
              >
                Build Image Now
              </button>
            )}
          </div>
        ) : null}

        {activeTab === 'instances' && snapshot ? (
          <InstancesView
            snapshot={snapshot}
            selected={selected}
            setSelected={setSelected}
            busy={busy}
            run={run}
            setActiveLogNodeId={setActiveLogNodeId}
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
            setActiveLogNodeId={setActiveLogNodeId}
          />
        ) : null}

        {activeTab === 'scenarios' && snapshot ? (
          <ScenariosView snapshot={snapshot} scenario={scenario} busy={busy} run={run} />
        ) : null}
      </section>

      {activeLogNodeId && (
        <LogViewerModal nodeId={activeLogNodeId} onClose={() => setActiveLogNodeId(null)} />
      )}
    </main>
  )
}

function InstancesView({
  snapshot,
  selected,
  setSelected,
  busy,
  run,
  setActiveLogNodeId,
}: {
  snapshot: ControlSnapshot
  selected: string[]
  setSelected: (next: string[]) => void
  busy: string
  run: (label: string, fn: () => Promise<unknown>) => Promise<void>
  setActiveLogNodeId: (id: string | null) => void
}) {
  const toggle = (id: string) => {
    setSelected(selected.includes(id) ? selected.filter((item) => item !== id) : [...selected, id])
  }
  return (
    <div className="content-grid">
      <section className="instance-grid">
        {snapshot.instances.map((inst) => (
          <InstanceCard key={inst.profile.id} inst={inst} selected={selected.includes(inst.profile.id)} onToggle={() => toggle(inst.profile.id)} busy={busy} run={run} setActiveLogNodeId={setActiveLogNodeId} />
        ))}
      </section>
      <aside className="side-panel">
        <h2>Protocol Summary</h2>
        <div className="metric-row"><span>Running</span><strong>{snapshot.instances.filter((i) => i.running).length}/10</strong></div>
        <div className="metric-row"><span>Active Peer Blocks</span><strong>{snapshot.firewall.length}</strong></div>
        <div className="metric-row"><span>Selected</span><strong>{selected.length}</strong></div>
        <button className="wide cmd" style={{ marginBottom: '8px', backgroundColor: '#3b82f6', color: '#ffffff' }} disabled={!!busy} onClick={() => run('rebuild-docker', runtimeClient.rebuildDockerImage)}>
          <RefreshCcw size={16} /> Rebuild Docker Image
        </button>
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
  setActiveLogNodeId,
}: {
  inst: InstanceStatus
  selected: boolean
  onToggle: () => void
  busy: string
  run: (label: string, fn: () => Promise<unknown>) => Promise<void>
  setActiveLogNodeId: (id: string | null) => void
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
        <button className="icon-btn" disabled={!!busy} onClick={() => run(`isolate-${id}`, () => runtimeClient.isolateNode(id))} title="Isolate (P2P Cut)">
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
        <button className="icon-btn" disabled={!inst.running} onClick={() => setActiveLogNodeId(id)} title="View Live Logs">
          <Terminal size={15} />
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
  setActiveLogNodeId,
}: {
  snapshot: ControlSnapshot
  selected: string[]
  setSelected: (next: string[]) => void
  selectedInstances: InstanceStatus[]
  busy: string
  run: (label: string, fn: () => Promise<unknown>) => Promise<void>
  setActiveLogNodeId: (id: string | null) => void
}) {
  const [activeNodeId, setActiveNodeId] = useState<string | null>(null)

  // Filter running and stopped instances
  const runningInstances = useMemo(() => {
    return snapshot.instances.filter((inst) => inst.running)
  }, [snapshot.instances])

  const stoppedInstances = useMemo(() => {
    return snapshot.instances.filter((inst) => !inst.running)
  }, [snapshot.instances])

  // Auto-select first running node if none active or active node goes offline
  useEffect(() => {
    const isActiveAlive = runningInstances.some((i) => i.profile.id === activeNodeId)
    if (!isActiveAlive && runningInstances.length > 0) {
      setActiveNodeId(runningInstances[0].profile.id)
    } else if (runningInstances.length === 0 && stoppedInstances.length > 0 && !activeNodeId) {
      setActiveNodeId(stoppedInstances[0].profile.id)
    }
  }, [activeNodeId, runningInstances, stoppedInstances])

  const activeInstance = useMemo(() => {
    return snapshot.instances.find((i) => i.profile.id === activeNodeId) || null
  }, [activeNodeId, snapshot.instances])

  const clusterA = selected
  const clusterB = snapshot.instances.map((i) => i.profile.id).filter((id) => !clusterA.includes(id))

  // Coordinates for circle mapping (width=560, height=380, center CX=280, CY=190)
  const CX = 280
  const CY = 190
  const R = 125

  const nodePositions = useMemo(() => {
    const count = runningInstances.length
    const positions: Record<string, { x: number; y: number }> = {}
    runningInstances.forEach((inst, idx) => {
      const theta = (idx * 2 * Math.PI) / count - Math.PI / 2
      positions[inst.profile.id] = {
        x: CX + R * Math.cos(theta),
        y: CY + R * Math.sin(theta),
      }
    })
    return positions
  }, [runningInstances])

  const isBlocked = useCallback(
    (id1: string, id2: string) => {
      const key1 = `${id1.toLowerCase()}-${id2.toLowerCase()}`
      const key2 = `${id2.toLowerCase()}-${id1.toLowerCase()}`
      return snapshot.firewall.some((rule) => {
        const name = rule.name.toLowerCase()
        return name.includes(key1) || name.includes(key2)
      })
    },
    [snapshot.firewall],
  )

  // Generate unique connection links between active nodes only
  const links = useMemo(() => {
    const list: Array<{ id: string; from: string; to: string; x1: number; y1: number; x2: number; y2: number; blocked: boolean; active: boolean }> = []
    for (let i = 0; i < runningInstances.length; i++) {
      for (let j = i + 1; j < runningInstances.length; j++) {
        const from = runningInstances[i]
        const to = runningInstances[j]
        const pos1 = nodePositions[from.profile.id]
        const pos2 = nodePositions[to.profile.id]
        if (pos1 && pos2) {
          const blocked = isBlocked(from.profile.id, to.profile.id)
          const active = from.p2p_ready && to.p2p_ready && !blocked
          list.push({
            id: `${from.profile.id}-${to.profile.id}`,
            from: from.profile.id,
            to: to.profile.id,
            x1: pos1.x,
            y1: pos1.y,
            x2: pos2.x,
            y2: pos2.y,
            blocked,
            active,
          })
        }
      }
    }
    return list
  }, [runningInstances, nodePositions, isBlocked])

  return (
    <div className="topology-layout">
      <section>
        <div className="section-head">
          <h2>Network Topology Map</h2>
          <div className="toolbar">
            <button className="cmd" disabled={!selected.length || !!busy} onClick={() => run('partition-selected', () => runtimeClient.applyPartition({ id: 'manual', label: 'Manual split', clusters: [clusterA, clusterB], active: true }))}>
              <GitBranch size={16} /> Split Selected
            </button>
            <button className="cmd danger" disabled={!!busy} onClick={() => run('heal', runtimeClient.healAll)}>
              <ShieldAlert size={16} /> Heal Network
            </button>
          </div>
        </div>

        <div className="topology-container">
          {runningInstances.length > 0 ? (
            <svg className="topology-svg" viewBox="0 0 560 380">
              {/* Draw connection link lines */}
              {links.map((link) => {
                if (link.blocked) {
                  // Midpoint for block indicator
                  const mx = (link.x1 + link.x2) / 2
                  const my = (link.y1 + link.y2) / 2
                  return (
                    <g key={link.id}>
                      <line x1={link.x1} y1={link.y1} x2={link.x2} y2={link.y2} className="svg-link blocked" />
                      <circle cx={mx} cy={my} r="8" fill="#f43f5e" />
                      <text x={mx} y={my} fill="#ffffff" fontSize="8" fontWeight="bold" textAnchor="middle" dominantBaseline="central">✕</text>
                    </g>
                  )
                }
                if (link.active) {
                  return <line key={link.id} x1={link.x1} y1={link.y1} x2={link.x2} y2={link.y2} className="svg-link healthy active" />
                }
                // Inactive Potential P2P connections (drawn very faintly)
                return <line key={link.id} x1={link.x1} y1={link.y1} x2={link.x2} y2={link.y2} stroke="rgba(148,163,184,0.06)" strokeWidth="1" />
              })}

              {/* Draw active nodes only */}
              {runningInstances.map((inst, idx) => {
                const pos = nodePositions[inst.profile.id]
                if (!pos) return null

                const isSelected = inst.profile.id === activeNodeId
                const label = inst.profile.id.replace('node-', '')

                // Node status classes
                let statusClass = 'stopped'
                if (inst.running) {
                  if (inst.p2p_ready) {
                    statusClass = 'p2p-ready'
                  } else if (inst.app_state === 'offline') {
                    statusClass = 'running'
                  } else {
                    statusClass = 'p2p-cut' // App alive, but P2P disconnected (operator Soft P2P Cut)
                  }
                }

                return (
                  <g
                    key={inst.profile.id}
                    className={`svg-node ${statusClass} ${isSelected ? 'selected' : ''}`}
                    onClick={() => setActiveNodeId(inst.profile.id)}
                    transform={`translate(${pos.x}, ${pos.y})`}
                  >
                    <circle r="26" className="node-outer-ring" />
                    <circle r="20" className="node-circle" />
                    <text className="node-text">{label}</text>
                    <text y="32" className="node-label-sub">{inst.profile.label}</text>
                  </g>
                )
              })}
            </svg>
          ) : (
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: '12px', color: '#94a3b8' }}>
              <Network size={36} style={{ strokeWidth: 1.5, opacity: 0.7 }} />
              <p style={{ fontSize: '13px' }}>Không có node nào đang hoạt động. Vui lòng bật các node để xem sơ đồ.</p>
            </div>
          )}
        </div>
      </section>

      <aside className="side-panel">
        <h2>Node Details & Control</h2>
        {activeInstance ? (
          <div className="active-node-card">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <h3>{activeInstance.profile.label}</h3>
              <span className={activeInstance.running ? 'status running' : 'status stopped'}>
                {activeInstance.running ? 'RUNNING' : 'STOPPED'}
              </span>
            </div>

            {activeInstance.running ? (
              <>
                <div style={{ display: 'flex', gap: '6px', flexWrap: 'wrap', marginTop: '4px' }}>
                  <span className={`node-card-tag ${activeInstance.app_state === 'active' ? 'ready' : 'not-ready'}`}>
                    App: {activeInstance.app_state || 'offline'}
                  </span>
                  <span className={`node-card-tag ${activeInstance.p2p_ready ? 'ready' : 'not-ready'}`}>
                    P2P: {activeInstance.p2p_ready ? 'READY' : 'OFFLINE'}
                  </span>
                  <span className={`node-card-tag ${activeInstance.crypto_ready ? 'ready' : 'not-ready'}`}>
                    MLS: {activeInstance.crypto_ready ? 'READY' : 'OFFLINE'}
                  </span>
                </div>

                <div className="node-body" style={{ marginTop: '8px' }}>
                  <div className="metric-row"><span>P2P Port</span><strong>{activeInstance.profile.p2p_port}</strong></div>
                  <div className="metric-row"><span>Control Port</span><strong>{activeInstance.profile.control_port}</strong></div>
                  <div className="metric-row"><span>Startup Stage</span><strong>{activeInstance.startup_stage || '-'}</strong></div>
                  <div className="metric-row"><span>Peers Connected</span><strong>{activeInstance.peer_count}</strong></div>
                  <div className="metric-row"><span>MLS Groups</span><strong>{activeInstance.group_count}</strong></div>
                  {activeInstance.peer_id && (
                    <div style={{ marginTop: '4px' }}>
                      <span style={{ fontSize: '10px', color: '#94a3b8' }}>Peer ID:</span>
                      <code className="peer-id" style={{ marginTop: '2px' }}>{activeInstance.peer_id}</code>
                    </div>
                  )}
                </div>
              </>
            ) : (
              <p className="muted" style={{ margin: '8px 0 12px 0' }}>Node này hiện đang ngoại tuyến. Vui lòng bấm khởi chạy bên dưới.</p>
            )}

            {/* Node Lifecycle Actions */}
            <div className="panel-actions" style={{ marginTop: '10px' }}>
              <span style={{ fontSize: '11px', color: '#94a3b8', fontWeight: 600 }}>Instance Lifecycle</span>
              <div className="btn-grid">
                <button className="cmd" disabled={!!busy || activeInstance.running} onClick={() => run(`start-${activeInstance.profile.id}`, () => runtimeClient.startInstance(activeInstance.profile.id))} title="Start">
                  <Play size={14} /> Start
                </button>
                <button className="cmd" disabled={!!busy || !activeInstance.running} onClick={() => run(`stop-${activeInstance.profile.id}`, () => runtimeClient.stopInstance(activeInstance.profile.id))} title="Stop">
                  <Power size={14} /> Stop
                </button>
                <button className="cmd" disabled={!!busy} onClick={() => run(`restart-${activeInstance.profile.id}`, () => runtimeClient.restartInstance(activeInstance.profile.id))} title="Restart">
                  <RotateCcw size={14} /> Restart
                </button>
              </div>
            </div>

            {/* Direct P2P & Firewall Actions */}
            {activeInstance.running && (
              <div className="panel-actions" style={{ marginTop: '12px', borderTop: '1px solid rgba(148, 163, 184, 0.08)', paddingTop: '8px' }}>
                <span style={{ fontSize: '11px', color: '#94a3b8', fontWeight: 600 }}>Simulated Network Actions</span>
                <div className="btn-grid-wide">
                  {/* Application-layer P2P connection cut */}
                  <button className="cmd danger" disabled={!!busy} onClick={() => run(`isolate-${activeInstance.profile.id}`, () => runtimeClient.isolateNode(activeInstance.profile.id))} title="Isolate Node (P2P Cut)">
                    <Scissors size={14} /> Isolate P2P
                  </button>
                  <button className="cmd" disabled={!!busy} onClick={() => run(`sync-${activeInstance.profile.id}`, () => runtimeClient.triggerOfflineSync(activeInstance.profile.id))} title="Trigger Offline Sync">
                    <Zap size={14} /> Offline Sync
                  </button>
                </div>
                
                <div className="btn-grid-wide" style={{ marginTop: '6px' }}>
                  <button className="cmd" disabled={!!busy} onClick={() => run(`folder-${activeInstance.profile.id}`, () => runtimeClient.openRuntimeFolder(activeInstance.profile.id))} title="Open Runtime Directory">
                    <FolderOpen size={14} /> Open Folder
                  </button>
                  <button className="cmd" onClick={() => setActiveLogNodeId(activeInstance.profile.id)} title="View Live Logs">
                    <Terminal size={14} /> View Logs
                  </button>
                </div>
              </div>
            )}

            {/* Partition Cluster Assignment */}
            {activeInstance.running && (
              <div className="cluster-assignment" style={{ marginTop: '12px', borderTop: '1px solid rgba(148, 163, 184, 0.08)', paddingTop: '8px', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <span style={{ fontSize: '11px', color: '#94a3b8' }}>Include in Partition A:</span>
                <input
                  type="checkbox"
                  style={{ width: '16px', height: '16px', cursor: 'pointer', accentColor: '#10b981' }}
                  checked={selected.includes(activeInstance.profile.id)}
                  onChange={() => setSelected(
                    selected.includes(activeInstance.profile.id)
                      ? selected.filter((id) => id !== activeInstance.profile.id)
                      : [...selected, activeInstance.profile.id]
                  )}
                />
              </div>
            )}
          </div>
        ) : (
          <p className="muted" style={{ marginTop: '20px', textAlign: 'center' }}>Click a node on the map to control it</p>
        )}

        {/* Stopped Nodes quick control panel */}
        {stoppedInstances.length > 0 && (
          <div style={{ marginTop: '16px', borderTop: '1px solid rgba(148, 163, 184, 0.12)', paddingTop: '10px' }}>
            <h3>Stopped Nodes ({stoppedInstances.length})</h3>
            <div className="event-list" style={{ maxHeight: '130px', overflowY: 'auto' }}>
              {stoppedInstances.map((inst) => (
                <div className="event-row" key={inst.profile.id} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', padding: '6px 8px' }}>
                  <span style={{ fontSize: '11px', color: '#94a3b8' }}>{inst.profile.label}</span>
                  <button className="cmd primary" style={{ minHeight: '26px', padding: '0 8px', fontSize: '11px' }} disabled={!!busy} onClick={() => run(`start-${inst.profile.id}`, () => runtimeClient.startInstance(inst.profile.id))}>
                    <Play size={10} /> Start
                  </button>
                </div>
              ))}
            </div>
          </div>
        )}

        <div style={{ marginTop: '16px', borderTop: '1px solid rgba(148, 163, 184, 0.12)', paddingTop: '10px' }}>
          <h3>Active Partition Groups</h3>
          <div className="cluster-box">
            <strong>Cluster A</strong>
            <span>{selectedInstances.map((i) => i.profile.id).join(', ') || 'none'}</span>
          </div>
          <div className="cluster-box">
            <strong>Cluster B</strong>
            <span>{clusterB.join(', ') || 'none'}</span>
          </div>
        </div>

        {snapshot.firewall.length > 0 && (
          <div style={{ marginTop: '12px' }}>
            <span style={{ fontSize: '11px', color: '#f43f5e', fontWeight: 600 }}>Active Connection Blocks ({snapshot.firewall.length})</span>
            <div className="event-list" style={{ maxHeight: '110px', overflowY: 'auto' }}>
              {snapshot.firewall.slice(0, 5).map((rule) => (
                <div className="event-row" key={rule.name} style={{ background: 'rgba(244, 63, 94, 0.08)', border: '1px solid rgba(244, 63, 94, 0.15)' }}>
                  <Shield size={12} style={{ color: '#f43f5e', flexShrink: 0 }} />
                  <span style={{ fontSize: '10px', color: '#fca5a5' }}>{rule.name.replace('DATN-DEMO-', '').replace('-', ' ✕ ')}</span>
                </div>
              ))}
            </div>
          </div>
        )}
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

function LogViewerModal({
  nodeId,
  onClose,
}: {
  nodeId: string
  onClose: () => void
}) {
  const [logs, setLogs] = useState<string>('Loading logs...')
  const [autoScroll, setAutoScroll] = useState<boolean>(true)
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
      <div className="modal-content" onClick={(e) => e.stopPropagation()}>
        <header className="modal-header">
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <Terminal size={18} style={{ color: '#10b981' }} />
            <h2>Live Logs - {nodeId}</h2>
          </div>
          <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
            <button className="cmd" style={{ minHeight: '28px', padding: '0 8px', fontSize: '11px' }} onClick={() => runtimeClient.openInstanceLog(nodeId)} title="Open in System Editor">
              <FolderOpen size={13} /> Raw File
            </button>
            <button className="icon-btn" onClick={onClose} style={{ width: '28px', height: '28px' }}>
              <X size={16} />
            </button>
          </div>
        </header>
        <div className="modal-body">
          <pre ref={preRef} className="log-container">{logs || 'Waiting for log output...'}</pre>
        </div>
        <footer className="modal-footer" style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: '6px', fontSize: '12px', color: '#94a3b8', cursor: 'pointer' }}>
            <input type="checkbox" checked={autoScroll} onChange={(e) => setAutoScroll(e.target.checked)} style={{ cursor: 'pointer', accentColor: '#10b981' }} />
            Auto-scroll to bottom
          </label>
          <button className="cmd primary" onClick={onClose}>Close</button>
        </footer>
      </div>
    </div>
  )
}
