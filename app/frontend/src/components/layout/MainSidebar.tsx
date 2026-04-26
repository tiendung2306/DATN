import { service } from '../../../wailsjs/go/models'
import { shortPeerId } from '../../lib/chatModel'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import NetworkStatusIndicator from '../network/NetworkStatusIndicator'
import { NetworkConnectionState } from '../../stores/useNetworkStore'

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
  return (
    <aside className="flex h-full min-h-[78vh] flex-col rounded-xl border border-emerald-900/30 bg-[#070b12] p-4 shadow-[0_20px_50px_rgba(0,0,0,0.35)]">
      <div className="border-b border-border/60 pb-4">
        <p className="text-sm font-semibold text-foreground">{displayName || 'Operator'}</p>
        <p className="mt-1 text-xs text-muted-foreground">ID: {shortPeerId(localPeerId)}</p>
      </div>
      <div className="py-4">
        <NetworkStatusIndicator status={networkStatus} peerCount={peerCount} />
      </div>

      <nav className="mb-4 space-y-1 border-b border-border/60 pb-4 text-sm">
        <button className="w-full rounded-md bg-emerald-500/10 px-3 py-2 text-left text-emerald-300">
          Messages
        </button>
        <button className="w-full rounded-md px-3 py-2 text-left text-muted-foreground hover:bg-muted/50">
          Invites
        </button>
        <button className="w-full rounded-md px-3 py-2 text-left text-muted-foreground hover:bg-muted/50">
          Vault
        </button>
      </nav>

      <div className="space-y-2 border-b border-border/60 pb-4">
        <p className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
          New secure channel
        </p>
        <div className="flex gap-2">
          <Input
            value={createGroupValue}
            onChange={(event) => onCreateGroupValueChange(event.target.value)}
            placeholder="core-intelligence"
            className="h-9 bg-black/30 text-xs"
          />
          <Button
            onClick={onCreateGroup}
            disabled={creatingGroup || !createGroupValue.trim()}
            className="h-9 px-3 text-xs"
          >
            {creatingGroup ? '...' : 'New'}
          </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1 space-y-2 pt-4">
        <p className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
          Active groups
        </p>
        <div className="max-h-[40vh] space-y-1 overflow-y-auto pr-1">
          {groups.length === 0 ? (
            <div className="rounded-lg border border-dashed border-border px-3 py-4 text-xs text-muted-foreground">
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
                      ? 'bg-emerald-500/15 text-emerald-200'
                      : 'text-muted-foreground hover:bg-muted/50 hover:text-foreground'
                  }`}
                >
                  <div className="min-w-0">
                    <p className="truncate">{group.group_id}</p>
                    <p className="text-[11px] opacity-70">Epoch {group.epoch}</p>
                  </div>
                  {unread > 0 && (
                    <span className="rounded-full bg-emerald-500/20 px-2 py-0.5 text-[11px] text-emerald-200">
                      {unread}
                    </span>
                  )}
                </button>
              )
            })
          )}
        </div>
      </div>

      <div className="mt-3 border-t border-border/60 pt-4 text-xs text-emerald-300/80">
        P2P Network: {networkStatus === 'connected' ? 'Connected' : networkStatus}
      </div>
    </aside>
  )
}
