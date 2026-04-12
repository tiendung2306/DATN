import { useEffect, useState } from 'react'
import { GetNodeStatus } from '../../wailsjs/go/service/Runtime'
import { service } from '../../wailsjs/go/models'
import StatusBadge from '../components/StatusBadge'
import PeerList from '../components/PeerList'
import AdminPanel from '../components/AdminPanel'
import ChatPanel from '../components/ChatPanel'
import IdentityBackupPanel from '../components/IdentityBackupPanel'

interface DashboardScreenProps {
  isAdmin: boolean
}

function shortID(id: string): string {
  if (id.length <= 20) return id
  return id.slice(0, 10) + '…' + id.slice(-8)
}

export default function DashboardScreen({ isAdmin }: DashboardScreenProps) {
  const [status, setStatus] = useState<service.NodeStatus | null>(null)
  const [copied, setCopied] = useState(false)
  const [showAdmin, setShowAdmin] = useState(false)

  useEffect(() => {
    let cancelled = false

    const refresh = async () => {
      try {
        const s = await GetNodeStatus()
        if (!cancelled) setStatus(s)
      } catch {
        // backend might still be starting
      }
    }

    refresh()
    const interval = setInterval(refresh, 3000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [])

  const copyPeerID = async () => {
    if (!status?.peer_id) return
    await navigator.clipboard.writeText(status.peer_id)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <div className="min-h-screen p-6">
      {/* Top bar */}
      <div className="mb-6 flex items-center justify-between">
        <h1 className="text-lg font-semibold text-gray-200">Secure P2P Node</h1>
        {status && <StatusBadge state={status.state} />}
      </div>

      {/* Main grid */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        {/* Node info card */}
        <div className="card space-y-4">
          <h2 className="text-xs font-semibold uppercase tracking-wide text-gray-500">Node Info</h2>

          {status ? (
            <>
              <div>
                <p className="label">Display Name</p>
                <p className="text-base font-medium text-gray-200">
                  {status.display_name || <span className="text-gray-600 italic">—</span>}
                </p>
              </div>

              <div>
                <p className="label">Peer ID</p>
                <button
                  onClick={copyPeerID}
                  title={status.peer_id}
                  className="font-mono text-xs text-gray-400 hover:text-gray-200 transition-colors text-left break-all"
                >
                  {shortID(status.peer_id)}
                  <span className="ml-2 text-gray-600 text-xs">
                    {copied ? '✓ copied' : '(click to copy)'}
                  </span>
                </button>
              </div>

              <div className="flex items-center gap-2">
                <p className="label mb-0">P2P Node</p>
                {status.is_running ? (
                  <span className="inline-flex items-center gap-1.5 text-xs text-green-400">
                    <span className="h-2 w-2 rounded-full bg-green-400 animate-pulse" />
                    Running
                  </span>
                ) : (
                  <span className="inline-flex items-center gap-1.5 text-xs text-yellow-500">
                    <span className="h-2 w-2 rounded-full bg-yellow-500 animate-pulse" />
                    Starting...
                  </span>
                )}
              </div>
            </>
          ) : (
            <div className="space-y-3">
              <div className="h-5 w-32 rounded bg-gray-800 animate-pulse" />
              <div className="h-5 w-64 rounded bg-gray-800 animate-pulse" />
            </div>
          )}
        </div>

        {/* Peers card */}
        <div className="card">
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
              Connected Peers
            </h2>
            {status && (
              <span className="text-xs text-gray-600">
                {status.connected_peers?.length ?? 0} peer{status.connected_peers?.length !== 1 ? 's' : ''}
              </span>
            )}
          </div>
          <PeerList peers={status?.connected_peers ?? []} />
        </div>
      </div>

      <IdentityBackupPanel />

      {/* Group chat */}
      <ChatPanel />

      {/* Admin panel */}
      {isAdmin && (
        <div className="mt-4">
          <button
            onClick={() => setShowAdmin((v) => !v)}
            className="btn-secondary text-xs"
          >
            {showAdmin ? '▴ Hide Admin Panel' : '▾ Admin Panel'}
          </button>
          {showAdmin && <AdminPanel />}
        </div>
      )}
    </div>
  )
}
