import { service } from '../../../wailsjs/go/models'
import { shortPeerId } from '../../lib/chatModel'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { NetworkConnectionState } from '../../stores/useNetworkStore'
import { Hash, Plus } from 'lucide-react'

interface MainSidebarProps {
  displayName: string
  localPeerId: string
  networkStatus: NetworkConnectionState
  groups: service.GroupInfo[]
  activeGroupId: string | null
  unreadByGroup: Record<string, number>
  peerCount: number
  creatingGroup: boolean
  createGroupValue: string
  onCreateGroupValueChange: (value: string) => void
  onCreateGroup: () => void
  onSelectGroup: (groupId: string) => void
}

export default function MainSidebar({
  displayName,
  localPeerId,
  networkStatus,
  groups,
  activeGroupId,
  unreadByGroup,
  peerCount,
  creatingGroup,
  createGroupValue,
  onCreateGroupValueChange,
  onCreateGroup,
  onSelectGroup,
}: MainSidebarProps) {
  const activeDirects = peersToDirectMessages(groups)

  return (
    <aside className="flex w-72 flex-col border-r border-slate-800 bg-slate-900">
      <div className="flex items-center justify-between border-b border-slate-800 px-4 py-4">
        <div>
          <p className="text-sm font-semibold text-slate-100">Chats</p>
          <p className="mt-0.5 text-xs text-slate-400">{displayName || shortPeerId(localPeerId)}</p>
        </div>
        <Button size="icon-sm" variant="ghost" className="text-slate-300 hover:text-slate-100">
          <Plus className="h-4 w-4" />
        </Button>
      </div>

      <div className="space-y-2 border-b border-slate-800 px-4 py-3">
        <div className="flex gap-2">
          <Input
            value={createGroupValue}
            onChange={(event) => onCreateGroupValueChange(event.target.value)}
            placeholder="Create or find channel"
            className="h-9 border-slate-700 bg-slate-800 text-xs text-slate-200 placeholder:text-slate-500"
          />
          <Button
            onClick={onCreateGroup}
            disabled={creatingGroup || !createGroupValue.trim()}
            className="h-9 bg-emerald-500 px-3 text-xs text-slate-900 hover:bg-emerald-400"
          >
            {creatingGroup ? '...' : 'New'}
          </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
        <p className="px-1 text-[11px] font-semibold tracking-[0.16em] text-slate-500">
          CHANNELS
        </p>
        <div className="mt-2 space-y-1">
          {groups.length === 0 ? (
            <div className="rounded-lg border border-dashed border-slate-700 px-3 py-4 text-xs text-slate-400">
              You have not joined any group yet.
            </div>
          ) : (
            groups.map((group) => {
              const active = group.group_id === activeGroupId
              const unread = unreadByGroup[group.group_id] ?? 0
              return (
                <button
                  key={group.group_id}
                  onClick={() => onSelectGroup(group.group_id)}
                  className={`flex w-full items-center justify-between rounded-lg px-2 py-2 text-left text-sm transition ${
                    active
                      ? 'bg-slate-800 text-slate-100'
                      : 'text-slate-400 hover:bg-slate-800/60 hover:text-slate-200'
                  }`}
                >
                  <div className="min-w-0 flex items-center gap-2">
                    <Hash className="h-3.5 w-3.5 opacity-80" />
                    <p className="truncate">{group.group_id}</p>
                  </div>
                  {unread > 0 && (
                    <span className="rounded-full bg-emerald-500/20 px-2 py-0.5 text-[11px] text-emerald-300">
                      {unread}
                    </span>
                  )}
                </button>
              )
            })
          )}
        </div>

        <p className="mt-5 px-1 text-[11px] font-semibold tracking-[0.16em] text-slate-500">
          DIRECT MESSAGES
        </p>
        <div className="mt-2 space-y-1">
          {activeDirects.map((direct) => (
            <button
              key={direct.id}
              className="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-left text-sm text-slate-300 hover:bg-slate-800/60"
              type="button"
            >
              <div className="relative h-6 w-6 rounded-full bg-slate-700">
                <span className="absolute bottom-0 right-0 h-2 w-2 rounded-full border border-slate-900 bg-emerald-500" />
              </div>
              <span className="truncate">{direct.name}</span>
            </button>
          ))}
        </div>
      </div>

      <div className="border-t border-slate-800 px-4 py-3 text-xs text-slate-400">
        {networkStatus === 'connected' ? `${peerCount} peers connected` : `Status: ${networkStatus}`}
      </div>
    </aside>
  )
}

function peersToDirectMessages(groups: service.GroupInfo[]): Array<{ id: string; name: string }> {
  if (groups.length === 0) {
    return [
      { id: 'dm-security', name: 'Security Team' },
      { id: 'dm-admin', name: 'System Admin' },
      { id: 'dm-ops', name: 'Network Ops' },
    ]
  }

  return groups.slice(0, 4).map((group) => ({
    id: `dm-${group.group_id}`,
    name: group.group_id.replace(/[-_]/g, ' '),
  }))
}
