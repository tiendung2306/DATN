import { service } from '../../../wailsjs/go/models'
import { shortPeerId } from '../../lib/chatModel'
import { Button } from '../ui/button'
import { ChevronRight, Shield, Users } from 'lucide-react'

interface RoomPanelProps {
  activeGroupId: string | null
  isAdmin: boolean
  peers: service.MemberInfo[]
  collapsed: boolean
  onToggleCollapsed: () => void
}

export default function RoomPanel({
  activeGroupId,
  isAdmin,
  peers,
  collapsed,
  onToggleCollapsed,
}: RoomPanelProps) {
  if (collapsed) {
    return (
      <aside className="flex w-12 border-l border-slate-800 bg-slate-950">
        <button
          type="button"
          aria-label="Open group details"
          className="m-2 flex h-8 w-8 items-center justify-center rounded-md text-slate-400 hover:bg-slate-800 hover:text-slate-100"
          onClick={onToggleCollapsed}
        >
          <ChevronRight className="h-4 w-4" />
        </button>
      </aside>
    )
  }

  return (
    <aside className="flex w-80 flex-col border-l border-slate-800 bg-slate-950">
      <div className="mb-4 flex items-center justify-between border-b border-slate-800 px-4 py-4">
        <div>
          <p className="text-sm font-semibold text-slate-100">Group details</p>
          <p className="text-xs text-slate-400">{activeGroupId || 'No group selected'}</p>
        </div>
        <button
          type="button"
          aria-label="Collapse group details"
          className="flex h-8 w-8 items-center justify-center rounded-md text-slate-400 hover:bg-slate-800 hover:text-slate-100"
          onClick={onToggleCollapsed}
        >
          <ChevronRight className="h-4 w-4 rotate-180" />
        </button>
      </div>

      <div className="px-4">
        <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
          <Users className="h-3.5 w-3.5" />
          <span>Members</span>
        </div>
        <div className="space-y-2 pb-4">
          {peers.length === 0 ? (
            <p className="text-xs text-slate-500">No members available.</p>
          ) : (
            peers.map((peer) => (
              <div
                key={peer.peer_id}
                className="flex items-center justify-between rounded-md border border-slate-800 bg-slate-900/60 px-2 py-2"
              >
                <div>
                  <p className="text-xs font-medium text-slate-200">{shortPeerId(peer.peer_id)}</p>
                  <p className="text-[11px] text-slate-500">{shortPeerId(peer.peer_id)}</p>
                </div>
                <span
                  className={`h-2 w-2 rounded-full ${
                    peer.is_online ? 'bg-emerald-400' : 'bg-slate-500'
                  }`}
                  title={peer.is_online ? 'online' : 'offline'}
                />
              </div>
            ))
          )}
        </div>
      </div>

      <div className="mt-auto space-y-2 border-t border-slate-800 p-4">
        <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
          <Shield className="h-3.5 w-3.5" />
          <span>Group actions</span>
        </div>
        <Button className="w-full" variant="secondary" disabled={!activeGroupId}>
          Add Member
        </Button>
        <Button className="w-full" variant="ghost" disabled={!activeGroupId || !isAdmin}>
          Leave Group
        </Button>
      </div>
    </aside>
  )
}
