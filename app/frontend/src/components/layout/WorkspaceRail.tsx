import { Bell, MessageSquare, Settings, Shield } from 'lucide-react'

export type WorkspaceModule = 'activity' | 'chat' | 'settings' | 'admin'

interface WorkspaceRailProps {
  activeModule: WorkspaceModule
  onSelectModule: (module: WorkspaceModule) => void
  isAdmin: boolean
  pendingInviteCount: number
}

export default function WorkspaceRail({
  activeModule,
  onSelectModule,
  isAdmin,
  pendingInviteCount,
}: WorkspaceRailProps) {
  const itemClass = (id: WorkspaceModule) =>
    `relative flex w-full flex-col items-center gap-1 rounded-xl px-1 py-2 text-[10px] font-medium transition ${
      activeModule === id
        ? 'bg-slate-800 text-emerald-400'
        : 'text-slate-400 hover:bg-slate-900 hover:text-slate-200'
    }`

  return (
    <aside className="flex w-[72px] shrink-0 flex-col items-stretch border-r border-slate-800 bg-slate-950 py-3">
      <div className="flex h-9 items-center justify-center text-[11px] font-bold tracking-tight text-slate-300">SW</div>

      <nav className="mt-2 flex flex-col items-stretch gap-1 px-1.5">
        <button
          type="button"
          className={itemClass('activity')}
          onClick={() => onSelectModule('activity')}
          title="Hoạt động"
        >
          <span className="relative flex h-6 w-6 items-center justify-center">
            <Bell className="h-5 w-5" />
            {pendingInviteCount > 0 ? (
              <span className="absolute -right-1 -top-1 flex h-4 min-w-4 items-center justify-center rounded-full bg-red-500 px-0.5 text-[9px] font-bold text-white">
                {pendingInviteCount > 9 ? '9+' : pendingInviteCount}
              </span>
            ) : null}
          </span>
          <span className="leading-tight text-center">Hoạt động</span>
        </button>

        <button
          type="button"
          className={itemClass('chat')}
          onClick={() => onSelectModule('chat')}
          title="Trò chuyện"
        >
          <MessageSquare className="h-5 w-5" />
          <span className="leading-tight text-center">Trò chuyện</span>
        </button>

        {isAdmin ? (
          <button
            type="button"
            className={itemClass('admin')}
            onClick={() => onSelectModule('admin')}
            title="Quản trị"
          >
            <Shield className="h-5 w-5" />
            <span className="leading-tight text-center">Quản trị</span>
          </button>
        ) : null}
      </nav>

      <div className="flex-1 min-h-2" />

      <div className="px-1.5">
        <button
          type="button"
          className={itemClass('settings')}
          onClick={() => onSelectModule('settings')}
          title="Cài đặt"
        >
          <Settings className="h-5 w-5" />
          <span className="leading-tight text-center">Cài đặt</span>
        </button>
      </div>
    </aside>
  )
}
