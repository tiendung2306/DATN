import { useEffect, useMemo, useState } from 'react'
import { service } from '../../../wailsjs/go/models'
import { getConversationKind, shortPeerId, SidebarConversationItem } from '../../lib/chatModel'
import { useContactStore } from '../../stores/useContactStore'
import { Button } from '../ui/button'
import { NetworkConnectionState } from '../../stores/useNetworkStore'
import { ChevronDown, Plus } from 'lucide-react'
import CreateGroupModal from '../../features/chat/components/CreateGroupModal'
import ChatListAvatar from '../chat/ChatListAvatar'
import { useChatStore } from '../../stores/useChatStore'

interface MainSidebarProps {
  displayName: string
  localPeerId: string
  networkStatus: NetworkConnectionState
  groups: service.GroupInfo[]
  activeGroupId: string | null
  unreadByGroup: Record<string, number>
  peerCount: number
  creatingGroup: boolean
  onCreateGroupWithDetails: (name: string, type: 'channel' | 'group' | 'dm', members: string[]) => Promise<void>
  onSelectGroup: (groupId: string) => void
  showWorkspaceLists: boolean
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
  onCreateGroupWithDetails,
  onSelectGroup,
  showWorkspaceLists,
}: MainSidebarProps) {
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)
  const [isChannelsExpanded, setIsChannelsExpanded] = useState(true)
  const [isConversationsExpanded, setIsConversationsExpanded] = useState(true)
  const getDisplayName = useContactStore((s) => s.getDisplayName)
  const messagesByGroup = useChatStore((s) => s.messagesByGroup)
  const postsByGroup = useChatStore((s) => s.postsByGroup)

  useEffect(() => {
    try {
      const channelsPref = localStorage.getItem('sidebar.channels.expanded')
      const conversationsPref = localStorage.getItem('sidebar.conversations.expanded')
      if (channelsPref !== null) setIsChannelsExpanded(channelsPref === 'true')
      if (conversationsPref !== null) setIsConversationsExpanded(conversationsPref === 'true')
    } catch {
      // ignore persistence read failure
    }
  }, [])

  useEffect(() => {
    try {
      localStorage.setItem('sidebar.channels.expanded', String(isChannelsExpanded))
      localStorage.setItem('sidebar.conversations.expanded', String(isConversationsExpanded))
    } catch {
      // ignore persistence write failure
    }
  }, [isChannelsExpanded, isConversationsExpanded])

  const getLastActivityAt = (groupId: string): number => {
    const messages = messagesByGroup[groupId] ?? []
    const posts = postsByGroup[groupId] ?? []
    const msgTs = messages.length > 0 ? messages[messages.length - 1]?.timestamp ?? 0 : 0
    const postTs = posts.length > 0 ? posts[posts.length - 1]?.timestamp ?? 0 : 0
    return Math.max(msgTs, postTs, 0)
  }

  const channelItems = useMemo(() => {
    const items: SidebarConversationItem[] = groups
      .filter((g) => getConversationKind(g) === 'channel')
      .map((g) => ({
        id: g.group_id,
        kind: 'channel',
        title: String((g as any).conversation_title || g.group_id),
        unreadCount: unreadByGroup[g.group_id] ?? 0,
        lastActivityAt: Math.max(Number((g as any).last_activity_at || 0), getLastActivityAt(g.group_id)),
        isChannel: true,
      }))
    return items.sort((a, b) => {
      if (b.lastActivityAt !== a.lastActivityAt) return b.lastActivityAt - a.lastActivityAt
      if (b.unreadCount !== a.unreadCount) return b.unreadCount - a.unreadCount
      return a.title.localeCompare(b.title)
    })
  }, [groups, unreadByGroup, messagesByGroup, postsByGroup])

  const conversationItems = useMemo(() => {
    const items: SidebarConversationItem[] = groups
      .filter((g) => {
        const kind = getConversationKind(g)
        return kind === 'dm' || kind === 'group'
      })
      .map((g) => {
        const kind = getConversationKind(g)
        const dmPeerId = kind === 'dm' ? String((g as any).counterparty_peer_id || '') : ''
        const backendTitle = String((g as any).conversation_title || '')
        const hasResolvedDmTitle = kind === 'dm' ? backendTitle.length > 0 && backendTitle !== g.group_id : true
        const storeActivity = getLastActivityAt(g.group_id)
        const backendActivity = Number((g as any).last_activity_at || 0)
        return {
          id: g.group_id,
          kind,
          title:
            hasResolvedDmTitle && backendTitle
              ? backendTitle
              : kind === 'dm'
                ? getDisplayName(dmPeerId || g.group_id)
                : g.group_id,
          unreadCount: unreadByGroup[g.group_id] ?? 0,
          isOnline: kind === 'dm' ? Boolean((g as any).is_counterparty_online) : undefined,
          lastActivityAt: Math.max(backendActivity, storeActivity),
          isChannel: false,
        } satisfies SidebarConversationItem
      })
    return items.sort((a, b) => {
      if (b.lastActivityAt !== a.lastActivityAt) return b.lastActivityAt - a.lastActivityAt
      if (b.unreadCount !== a.unreadCount) return b.unreadCount - a.unreadCount
      return a.title.localeCompare(b.title)
    })
  }, [groups, unreadByGroup, getDisplayName, messagesByGroup, postsByGroup])

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

      <div className="px-4 py-3 border-b border-slate-800 space-y-2">
        <Button
          onClick={() => setIsCreateModalOpen(true)}
          disabled={!showWorkspaceLists || creatingGroup}
          className="w-full h-9 bg-violet-600 hover:bg-violet-500 text-slate-50 text-xs font-semibold gap-2 flex items-center justify-center rounded-md transition duration-200"
        >
          <Plus className="h-4 w-4" />
          Tạo hội thoại mới
        </Button>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3 space-y-4">
        <div>
          <button
            type="button"
            className="w-full px-1 text-left text-[11px] font-semibold tracking-[0.16em] text-slate-500 flex items-center justify-between"
            aria-expanded={isChannelsExpanded}
            aria-controls="channels-list"
            onClick={() => setIsChannelsExpanded((v) => !v)}
          >
            <span>CHANNELS</span>
            <ChevronDown className={`h-3.5 w-3.5 transition ${isChannelsExpanded ? '' : '-rotate-90'}`} />
          </button>
          <div id="channels-list" className={`mt-2 space-y-1 ${isChannelsExpanded ? '' : 'hidden'}`}>
            {!showWorkspaceLists ? (
              <div className="rounded-lg border border-dashed border-slate-700 px-3 py-3 text-xs text-slate-500">
                Chuyển sang Trò chuyện
              </div>
            ) : channelItems.length === 0 ? (
              <div className="px-3 py-2 text-xs text-slate-600 italic">Chưa có kênh</div>
            ) : (
              channelItems.map((item) => {
                  const active = item.id === activeGroupId
                  return (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => onSelectGroup(item.id)}
                      className={`flex w-full items-center justify-between gap-2 rounded-lg px-2 py-2 text-left text-sm transition ${
                        active
                          ? 'bg-slate-800 text-slate-100'
                          : 'text-slate-400 hover:bg-slate-800/60 hover:text-slate-200'
                      }`}
                    >
                      <div className="min-w-0 flex flex-1 items-center gap-2">
                        <p className="truncate">
                          <span className="text-emerald-400/90"># </span>
                          {item.title}
                        </p>
                      </div>
                      {item.unreadCount > 0 && (
                        <span className="shrink-0 rounded-full bg-emerald-500/20 px-2 py-0.5 text-[11px] text-emerald-300">
                          {item.unreadCount}
                        </span>
                      )}
                    </button>
                  )
                })
            )}
          </div>
        </div>

        <div>
          <button
            type="button"
            className="w-full px-1 text-left text-[11px] font-semibold tracking-[0.16em] text-slate-500 flex items-center justify-between"
            aria-expanded={isConversationsExpanded}
            aria-controls="conversations-list"
            onClick={() => setIsConversationsExpanded((v) => !v)}
          >
            <span>CONVERSATIONS</span>
            <ChevronDown className={`h-3.5 w-3.5 transition ${isConversationsExpanded ? '' : '-rotate-90'}`} />
          </button>
          <div id="conversations-list" className={`mt-2 space-y-1 ${isConversationsExpanded ? '' : 'hidden'}`}>
            {!showWorkspaceLists ? (
              <div className="rounded-lg border border-dashed border-slate-700 px-3 py-3 text-xs text-slate-500">
                Chuyển sang Trò chuyện
              </div>
            ) : conversationItems.length === 0 ? (
              <div className="px-3 py-2 text-xs text-slate-600 italic">Chưa có hội thoại</div>
            ) : (
              conversationItems.map((item) => {
                  const active = item.id === activeGroupId
                  return (
                    <button
                      key={item.id}
                      type="button"
                      onClick={() => onSelectGroup(item.id)}
                      className={`flex w-full items-center justify-between gap-2 rounded-lg px-2 py-2 text-left text-sm transition ${
                        active
                          ? 'bg-slate-800 text-slate-100'
                          : 'text-slate-400 hover:bg-slate-800/60 hover:text-slate-200'
                      }`}
                    >
                      <div className="min-w-0 flex flex-1 items-center gap-2">
                        <ChatListAvatar variant={item.kind === 'dm' ? 'dm' : 'channel'} displayName={item.title} />
                        <p className="truncate">{item.title}</p>
                      </div>
                      {item.kind === 'dm' ? (
                        <span
                          className={`h-2 w-2 shrink-0 rounded-full ${item.isOnline ? 'bg-emerald-400' : 'bg-slate-500'}`}
                          title={item.isOnline ? 'Trực tuyến' : 'Ngoại tuyến'}
                          aria-label={item.isOnline ? 'Trực tuyến' : 'Ngoại tuyến'}
                        />
                      ) : null}
                      {item.unreadCount > 0 && (
                        <span className="shrink-0 rounded-full bg-emerald-500/20 px-2 py-0.5 text-[11px] text-emerald-300">
                          {item.unreadCount}
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

      <CreateGroupModal
        isOpen={isCreateModalOpen}
        onClose={() => setIsCreateModalOpen(false)}
        onCreate={onCreateGroupWithDetails}
        creating={creatingGroup}
      />
    </aside>
  )
}
