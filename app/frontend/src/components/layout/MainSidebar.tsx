import { service } from '../../../wailsjs/go/models'
import { shortPeerId } from '../../lib/chatModel'
import { useContactStore } from '../../stores/useContactStore'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { NetworkConnectionState } from '../../stores/useNetworkStore'
import { Hash, MessageSquare, Plus, Settings, Shield, UserPlus } from 'lucide-react'

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
  activeModule: 'chat' | 'invites' | 'settings' | 'admin'
  onSelectModule: (module: 'chat' | 'invites' | 'settings' | 'admin') => void
  isAdmin: boolean
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
  activeModule,
  onSelectModule,
  isAdmin,
}: MainSidebarProps) {
  const getDisplayName = useContactStore((s) => s.getDisplayName)
  const modules = [
    { id: 'chat' as const, label: 'Chats', icon: MessageSquare },
    { id: 'invites' as const, label: 'Loi moi', icon: UserPlus },
    ...(isAdmin ? [{ id: 'admin' as const, label: 'Quan tri', icon: Shield }] : []),
    { id: 'settings' as const, label: 'Cai dat', icon: Settings },
  ]
  const showChatGroups = activeModule === 'chat'

  return (
    <aside className="flex w-80 flex-col border-r border-slate-800 bg-slate-900">
      <div className="flex items-center justify-between border-b border-slate-800 px-4 py-4">
        <div>
          <p className="text-sm font-semibold text-slate-100">Secure Workspace</p>
          <p className="mt-0.5 text-xs text-slate-400">{displayName || shortPeerId(localPeerId)}</p>
        </div>
        <Button
          size="icon-sm"
          variant="ghost"
          className="text-slate-300 hover:text-slate-100"
          title={networkStatus === 'connected' ? 'Connected' : 'Disconnected'}
        >
          <Plus className="h-4 w-4" />
        </Button>
      </div>

      <div className="grid grid-cols-2 gap-2 border-b border-slate-800 px-4 py-3">
        {modules.map((module) => {
          const Icon = module.icon
          const active = activeModule === module.id
          return (
            <button
              key={module.id}
              type="button"
              onClick={() => onSelectModule(module.id)}
              className={`flex items-center gap-2 rounded-md px-2 py-2 text-xs font-medium transition ${
                active
                  ? 'bg-slate-800 text-emerald-300'
                  : 'text-slate-400 hover:bg-slate-800/60 hover:text-slate-100'
              }`}
            >
              <Icon className="h-3.5 w-3.5" />
              {module.label}
            </button>
          )
        })}
      </div>

      <div className="space-y-2 border-b border-slate-800 px-4 py-3">
        <div className="flex gap-2">
          <Input
            value={createGroupValue}
            onChange={(event) => onCreateGroupValueChange(event.target.value)}
            placeholder="Create secure group"
            className="h-9 border-slate-700 bg-slate-800 text-xs text-slate-200 placeholder:text-slate-500"
            disabled={!showChatGroups}
          />
          <Button
            onClick={onCreateGroup}
            disabled={!showChatGroups || creatingGroup || !createGroupValue.trim()}
            className="h-9 bg-emerald-500 px-3 text-xs text-slate-900 hover:bg-emerald-400"
          >
            {creatingGroup ? '...' : 'New'}
          </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3 space-y-4">
        {/* GROUP CHANNELS */}
        <div>
          <p className="px-1 text-[11px] font-semibold tracking-[0.16em] text-slate-500">
            GROUP CHANNELS
          </p>
          <div className="mt-2 space-y-1">
            {!showChatGroups ? (
              <div className="rounded-lg border border-dashed border-slate-700 px-3 py-3 text-xs text-slate-500">
                Switch to Chats
              </div>
            ) : groups.filter((g) => (g as any).group_type !== 'dm').length === 0 ? (
              <div className="px-3 py-2 text-xs text-slate-600 italic">No channels</div>
            ) : (
              groups
                .filter((g) => (g as any).group_type !== 'dm')
                .map((group) => {
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
                        <Hash className="h-3.5 w-3.5 opacity-80 text-emerald-400" />
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
        </div>

        {/* DIRECT MESSAGES */}
        <div>
          <p className="px-1 text-[11px] font-semibold tracking-[0.16em] text-slate-500">
            DIRECT MESSAGES
          </p>
          <div className="mt-2 space-y-1">
            {!showChatGroups ? (
              <div className="rounded-lg border border-dashed border-slate-700 px-3 py-3 text-xs text-slate-500">
                Switch to Chats
              </div>
            ) : groups.filter((g) => (g as any).group_type === 'dm').length === 0 ? (
              <div className="px-3 py-2 text-xs text-slate-600 italic">No direct messages</div>
            ) : (
              groups
                .filter((g) => (g as any).group_type === 'dm')
                .map((group) => {
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
                        <MessageSquare className="h-3.5 w-3.5 opacity-80 text-sky-400" />
                        <p className="truncate">{getDisplayName(group.group_id)}</p>
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
        </div>
      </div>

      <div className="border-t border-slate-800 px-4 py-3 text-xs text-slate-400">
        {networkStatus === 'connected' ? `${peerCount} peers connected` : `Status: ${networkStatus}`}
      </div>
    </aside>
  )
}
