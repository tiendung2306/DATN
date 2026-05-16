import { useEffect, useMemo } from 'react'
import { Bell, AtSign, Reply, UserPlus, CheckCircle, Calendar, MessageSquare } from 'lucide-react'
import { useNotificationStore, Notification } from '../../../stores/useNotificationStore'
import { formatRelativeTime, shortPeerId } from '../../../lib/chatModel'
import { Button } from '../../../components/ui/button'

interface ActivityScreenProps {
  onSelectGroup: (groupId: string) => void
  onSwitchToChat: () => void
}

export default function ActivityScreen({ onSelectGroup, onSwitchToChat }: ActivityScreenProps) {
  const { notifications, isLoading, fetchNotifications, markRead, markAllRead, fetchUnreadCount } = useNotificationStore()

  useEffect(() => {
    fetchNotifications()
    fetchUnreadCount()
  }, [])

  const groupedNotifications = useMemo(() => {
    const groups: Record<string, Notification[]> = {}
    notifications.forEach((n) => {
      const date = new Date(n.created_at).toLocaleDateString(undefined, {
        year: 'numeric',
        month: 'long',
        day: 'numeric',
      })
      if (!groups[date]) groups[date] = []
      groups[date].push(n)
    })
    return Object.entries(groups)
  }, [notifications])

  const handleNotificationClick = async (n: Notification) => {
    if (!n.is_read) {
      await markRead(n.id)
    }
    // Navigate to the group
    onSelectGroup(n.group_id)
    onSwitchToChat()
  }

  const getIcon = (type: string) => {
    switch (type) {
      case 'mention':
        return <AtSign className="h-4 w-4 text-orange-400" />
      case 'reply':
        return <Reply className="h-4 w-4 text-blue-400" />
      case 'group_add':
        return <UserPlus className="h-4 w-4 text-emerald-400" />
      case 'invite_request':
        return <UserPlus className="h-4 w-4 text-amber-400" />
      case 'invite_approved':
        return <CheckCircle className="h-4 w-4 text-violet-400" />
      case 'invite_rejected':
        return <Bell className="h-4 w-4 text-rose-400" />
      default:
        return <Bell className="h-4 w-4 text-slate-400" />
    }
  }

  const getTitle = (n: Notification) => {
    const actor = n.actor_name || shortPeerId(n.actor_id)
    switch (n.type) {
      case 'mention':
        return <><span className="font-bold text-slate-100">{actor}</span> đã nhắc đến bạn</>
      case 'reply':
        return <><span className="font-bold text-slate-100">{actor}</span> đã trả lời bạn</>
      case 'group_add':
        return <><span className="font-bold text-slate-100">{actor}</span> đã thêm bạn vào nhóm</>
      case 'invite_request':
        return <><span className="font-bold text-slate-100">{actor}</span> muốn tham gia nhóm</>
      case 'invite_approved':
        return <>Yêu cầu tham gia nhóm của bạn đã được <span className="font-bold text-slate-100">{actor}</span> phê duyệt</>
      case 'invite_rejected':
        return <>Yêu cầu tham gia nhóm của bạn đã bị <span className="font-bold text-slate-100">{actor}</span> từ chối</>
      default:
        return 'Thông báo mới'
    }
  }

  return (
    <div className="flex h-full flex-col bg-slate-900 overflow-hidden">
      <header className="flex items-center justify-between border-b border-slate-800 px-6 py-4">
        <h2 className="text-lg font-bold text-slate-100">Hoạt động</h2>
        <div className="flex items-center gap-2">
          <Button 
            variant="ghost" 
            size="sm" 
            className="text-xs text-slate-400 hover:text-emerald-400"
            onClick={() => fetchNotifications()}
          >
            Làm mới
          </Button>
          <Button 
            variant="ghost" 
            size="sm" 
            className="text-xs text-slate-400 hover:text-emerald-400"
            onClick={markAllRead}
          >
            Đánh dấu tất cả đã đọc
          </Button>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto px-4 py-4">
        {isLoading && notifications.length === 0 ? (
          <div className="flex h-full items-center justify-center text-slate-500 text-sm">
            Đang tải thông báo...
          </div>
        ) : notifications.length === 0 ? (
          <div className="flex h-full flex-col items-center justify-center gap-4 text-slate-500">
            <Bell className="h-12 w-12 opacity-20" />
            <p className="text-sm">Chưa có hoạt động nào</p>
          </div>
        ) : (
          <div className="max-w-3xl mx-auto space-y-8">
            {groupedNotifications.map(([date, items]) => (
              <div key={date} className="space-y-3">
                <div className="flex items-center gap-2 px-2">
                  <Calendar className="h-3.5 w-3.5 text-slate-500" />
                  <span className="text-[11px] font-bold uppercase tracking-wider text-slate-500">
                    {date}
                  </span>
                </div>
                <div className="space-y-1">
                  {items.map((n) => (
                    <button
                      key={n.id}
                      type="button"
                      onClick={() => handleNotificationClick(n)}
                      className={`group relative flex w-full items-start gap-4 rounded-xl p-4 text-left transition hover:bg-slate-800/50 ${
                        !n.is_read ? 'bg-slate-800/30 ring-1 ring-slate-700/50' : ''
                      }`}
                    >
                      {!n.is_read && (
                        <div className="absolute left-2 top-1/2 -translate-y-1/2 h-2 w-2 rounded-full bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]" />
                      )}
                      
                      <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-slate-950 ring-1 ring-slate-800 group-hover:ring-slate-700 ${!n.is_read ? 'ring-emerald-500/30' : ''}`}>
                        {getIcon(n.type)}
                      </div>

                      <div className="min-w-0 flex-1 space-y-1.5">
                        <div className="flex items-center justify-between gap-2">
                          <p className="truncate text-sm text-slate-400">
                            {getTitle(n)}
                          </p>
                          <span className="shrink-0 text-[11px] text-slate-500">
                            {formatRelativeTime(n.created_at)}
                          </span>
                        </div>
                        
                        {n.group_name && (
                          <div className="flex items-center gap-1.5">
                            <div className="flex h-4 items-center rounded bg-slate-800 px-1.5 text-[10px] font-bold uppercase tracking-tight text-emerald-400 ring-1 ring-slate-700">
                              <MessageSquare className="mr-1 h-2.5 w-2.5" />
                              {n.group_name}
                            </div>
                          </div>
                        )}

                        {n.content && (
                          <div className="rounded-lg bg-slate-950/50 p-2 ring-1 ring-slate-800/50 group-hover:bg-slate-950/80">
                            <p className="line-clamp-2 text-xs leading-relaxed text-slate-400 italic">
                              "{n.content}"
                            </p>
                          </div>
                        )}
                      </div>
                    </button>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
