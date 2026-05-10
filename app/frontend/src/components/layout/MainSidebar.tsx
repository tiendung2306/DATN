import { useEffect, useMemo, useState } from 'react'
import { service } from '../../../wailsjs/go/models'
import { getConversationKind, shortPeerId, SidebarConversationItem } from '../../lib/chatModel'
import { useContactStore } from '../../stores/useContactStore'
import { Button } from '../ui/button'
import { NetworkConnectionState } from '../../stores/useNetworkStore'
import { ChevronDown, ChevronRight, FolderPlus, Plus, Trash2 } from 'lucide-react'
import CreateGroupModal from '../../features/chat/components/CreateGroupModal'
import ChatListAvatar from '../chat/ChatListAvatar'
import { useChatStore } from '../../stores/useChatStore'
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from '../ui/dialog'
import { Input } from '../ui/input'
import { Label } from '../ui/label'
import { useToastStore } from '../../stores/useToastStore'
import { ChannelCategory } from '../../features/chat/hooks/useChannelCategories'

interface MainSidebarProps {
  displayName: string
  localPeerId: string
  networkStatus: NetworkConnectionState
  groups: service.GroupInfo[]
  activeGroupId: string | null
  unreadByGroup: Record<string, number>
  peerCount: number
  creatingGroup: boolean
  onCreateGroupWithDetails: (name: string, type: 'channel' | 'group' | 'dm', members: string[], categoryId?: string) => Promise<void>
  channelCategories: ChannelCategory[]
  onCreateCategory: (name: string) => Promise<void>
  onDeleteCategory: (categoryID: string) => Promise<void>
  onSelectGroup: (groupId: string) => void
  showWorkspaceLists: boolean
}

const CATEGORY_COLLAPSE_STORAGE_KEY = 'sidebar.channel.category-collapse.v2'

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
  channelCategories,
  onCreateCategory,
  onDeleteCategory,
  onSelectGroup,
  showWorkspaceLists,
}: MainSidebarProps) {
  const [isCreateModalOpen, setIsCreateModalOpen] = useState(false)
  const [isCreateCategoryOpen, setIsCreateCategoryOpen] = useState(false)
  const [categoryDraft, setCategoryDraft] = useState('')
  const [categoryIdForQuickCreate, setCategoryIdForQuickCreate] = useState<string | null>(null)
  const [isChannelsExpanded, setIsChannelsExpanded] = useState(true)
  const [isConversationsExpanded, setIsConversationsExpanded] = useState(true)
  const [collapsedCategories, setCollapsedCategories] = useState<Record<string, boolean>>({})
  const getDisplayName = useContactStore((s) => s.getDisplayName)
  const messagesByGroup = useChatStore((s) => s.messagesByGroup)
  const postsByGroup = useChatStore((s) => s.postsByGroup)
  const pushToast = useToastStore((s) => s.pushToast)

  useEffect(() => {
    try {
      const channelsPref = localStorage.getItem('sidebar.channels.expanded')
      const conversationsPref = localStorage.getItem('sidebar.conversations.expanded')
      const collapsedPref = localStorage.getItem(CATEGORY_COLLAPSE_STORAGE_KEY)
      if (channelsPref !== null) setIsChannelsExpanded(channelsPref === 'true')
      if (conversationsPref !== null) setIsConversationsExpanded(conversationsPref === 'true')
      if (collapsedPref) {
        const parsed = JSON.parse(collapsedPref) as Record<string, boolean>
        setCollapsedCategories(parsed || {})
      }
    } catch {
      // ignore persistence read failure
    }
  }, [])

  useEffect(() => {
    try {
      localStorage.setItem('sidebar.channels.expanded', String(isChannelsExpanded))
      localStorage.setItem('sidebar.conversations.expanded', String(isConversationsExpanded))
      localStorage.setItem(CATEGORY_COLLAPSE_STORAGE_KEY, JSON.stringify(collapsedCategories))
    } catch {
      // ignore persistence write failure
    }
  }, [collapsedCategories, isChannelsExpanded, isConversationsExpanded])

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

  const categorizedChannels = useMemo(() => {
    const categoryMap = new Map<string, SidebarConversationItem[]>()
    for (const item of channelItems) {
      const g = groups.find((entry) => entry.group_id === item.id)
      const categoryID = String((g as any)?.category_id || '').trim()
      if (!categoryID) continue
      const list = categoryMap.get(categoryID) ?? []
      list.push(item)
      categoryMap.set(categoryID, list)
    }
    return channelCategories.map((category) => ({
      ...category,
      channels: (categoryMap.get(category.category_id) ?? []).sort((a, b) => a.title.localeCompare(b.title)),
    }))
  }, [channelCategories, channelItems, groups])

  const uncategorizedChannels = useMemo(() => {
    const known = new Set(channelCategories.map((c) => c.category_id))
    return channelItems.filter((item) => {
      const g = groups.find((entry) => entry.group_id === item.id)
      const categoryID = String((g as any)?.category_id || '').trim()
      return !categoryID || !known.has(categoryID)
    })
  }, [channelCategories, channelItems, groups])

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

  const handleCreateCategory = async () => {
    const name = categoryDraft.trim()
    if (!name) return
    try {
      await onCreateCategory(name)
      setCategoryDraft('')
      setIsCreateCategoryOpen(false)
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err)
      pushToast({
        title: 'Tạo danh mục thất bại',
        description: raw,
        variant: 'destructive',
      })
    }
  }

  const handleDeleteCategory = async (categoryID: string) => {
    try {
      await onDeleteCategory(categoryID)
    } catch (err) {
      const raw = err instanceof Error ? err.message : String(err)
      pushToast({
        title: 'Không thể xóa danh mục',
        description: raw.includes('ERR_CATEGORY_NOT_EMPTY')
          ? 'Danh mục vẫn còn ít nhất một kênh bên trong.'
          : raw,
        variant: 'destructive',
      })
    }
  }

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
            <div className="flex items-center gap-1">
              <button
                type="button"
                className="rounded p-0.5 text-slate-500 hover:bg-slate-800 hover:text-slate-200"
                onClick={(event) => {
                  event.stopPropagation()
                  setIsCreateCategoryOpen(true)
                }}
                title="Tạo danh mục"
              >
                <FolderPlus className="h-3.5 w-3.5" />
              </button>
              <ChevronDown className={`h-3.5 w-3.5 transition ${isChannelsExpanded ? '' : '-rotate-90'}`} />
            </div>
          </button>
          <div id="channels-list" className={`mt-2 space-y-1 ${isChannelsExpanded ? '' : 'hidden'}`}>
            {!showWorkspaceLists ? (
              <div className="rounded-lg border border-dashed border-slate-700 px-3 py-3 text-xs text-slate-500">
                Chuyển sang Trò chuyện
              </div>
            ) : channelItems.length === 0 && channelCategories.length === 0 ? (
              <div className="px-3 py-2 text-xs text-slate-600 italic">Chưa có kênh</div>
            ) : (
              <>
                {categorizedChannels.map((category) => {
                  const isCollapsed = Boolean(collapsedCategories[category.category_id])
                  return (
                    <div key={category.category_id} className="space-y-1">
                      <div className="flex items-center justify-between rounded px-1">
                        <button
                          type="button"
                          className="flex min-w-0 flex-1 items-center gap-1 text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-500 hover:text-slate-300"
                          onClick={() =>
                            setCollapsedCategories((prev) => ({ ...prev, [category.category_id]: !prev[category.category_id] }))
                          }
                        >
                          {isCollapsed ? <ChevronRight className="h-3 w-3" /> : <ChevronDown className="h-3 w-3" />}
                          <span className="truncate">{category.name}</span>
                        </button>
                        <div className="flex items-center">
                          <button
                            type="button"
                            className="rounded p-1 text-slate-500 hover:bg-slate-800 hover:text-slate-200"
                            title="Tạo kênh trong danh mục"
                            onClick={() => {
                              setCategoryIdForQuickCreate(category.category_id)
                              setIsCreateModalOpen(true)
                            }}
                          >
                            <Plus className="h-3.5 w-3.5" />
                          </button>
                          <button
                            type="button"
                            className="rounded p-1 text-slate-500 hover:bg-slate-800 hover:text-rose-300"
                            title="Xóa danh mục"
                            onClick={() => handleDeleteCategory(category.category_id)}
                          >
                            <Trash2 className="h-3.5 w-3.5" />
                          </button>
                        </div>
                      </div>
                      {!isCollapsed &&
                        category.channels.map((item) => {
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
                        })}
                    </div>
                  )
                })}
                {uncategorizedChannels.length > 0 && (
                  <div className="space-y-1">
                    <div className="px-1 text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-600">
                      Chưa gán danh mục
                    </div>
                    {uncategorizedChannels.map((item) => {
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
                    })}
                  </div>
                )}
              </>
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
        onClose={() => {
          setIsCreateModalOpen(false)
          setCategoryIdForQuickCreate(null)
        }}
        onCreate={onCreateGroupWithDetails}
        creating={creatingGroup}
        initialType={categoryIdForQuickCreate ? 'channel' : 'group'}
        forcedType={categoryIdForQuickCreate ? 'channel' : undefined}
        forcedCategoryId={categoryIdForQuickCreate ?? undefined}
        channelCategories={channelCategories}
        title={
          categoryIdForQuickCreate
            ? `Tạo kênh trong ${channelCategories.find((c) => c.category_id === categoryIdForQuickCreate)?.name ?? 'danh mục'}`
            : undefined
        }
      />

      <Dialog open={isCreateCategoryOpen} onOpenChange={(open) => !open && setIsCreateCategoryOpen(false)}>
        <DialogContent className="sm:max-w-md bg-slate-900 border-slate-800 text-slate-100 ring-1 ring-slate-800 shadow-2xl">
          <DialogHeader>
            <DialogTitle className="text-lg font-semibold text-slate-100">Tạo danh mục mới</DialogTitle>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="category-name" className="text-xs text-slate-400 font-semibold uppercase tracking-wider">
              Tên danh mục
            </Label>
            <Input
              id="category-name"
              value={categoryDraft}
              onChange={(event) => setCategoryDraft(event.target.value)}
              placeholder="Ví dụ: Phòng ban kỹ thuật"
              className="bg-slate-950 border-slate-800 text-slate-100 placeholder:text-slate-600 focus-visible:ring-emerald-500"
            />
          </div>
          <DialogFooter>
            <Button
              variant="ghost"
              className="text-slate-400 hover:text-slate-200 hover:bg-slate-800"
              onClick={() => setIsCreateCategoryOpen(false)}
            >
              Hủy
            </Button>
            <Button
              onClick={handleCreateCategory}
              className="bg-emerald-500 hover:bg-emerald-400 text-slate-900 font-semibold"
              disabled={!categoryDraft.trim()}
            >
              Tạo danh mục
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </aside>
  )
}
