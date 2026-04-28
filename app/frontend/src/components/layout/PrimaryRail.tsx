import { Activity, Files, MessageSquare, Settings } from 'lucide-react'

interface PrimaryRailProps {
  isConnected: boolean
  activeModule: 'chat' | 'invites' | 'settings' | 'admin'
  onSelectModule: (module: 'chat' | 'invites' | 'settings' | 'admin') => void
}

const modules = [
  { id: 'admin', icon: Activity, label: 'Admin' },
  { id: 'chat', icon: MessageSquare, label: 'Chats' },
  { id: 'invites', icon: Files, label: 'Invites' },
  { id: 'settings', icon: Settings, label: 'Settings' },
] as const

export default function PrimaryRail({ isConnected, activeModule, onSelectModule }: PrimaryRailProps) {
  return (
    <aside className="flex w-20 flex-col items-center justify-between border-r border-slate-800 bg-slate-950 py-4">
      <div className="flex h-10 w-10 items-center justify-center rounded-xl bg-slate-800 text-sm font-semibold text-slate-200">
        SW
      </div>

      <nav className="flex flex-1 flex-col items-center justify-center gap-2 py-6">
        {modules.map((module) => {
          const Icon = module.icon
          return (
            <button
              key={module.id}
              className={`relative flex h-11 w-11 items-center justify-center rounded-xl transition ${
                module.id === activeModule
                  ? 'bg-slate-800 text-emerald-400'
                  : 'text-slate-400 hover:bg-slate-900 hover:text-slate-200'
              }`}
              title={module.label}
              aria-label={module.label}
              type="button"
              onClick={() => onSelectModule(module.id)}
            >
              <Icon className="h-5 w-5" />
              {module.id === activeModule ? <span className="absolute -left-3 h-6 w-1 rounded-r bg-emerald-500" /> : null}
            </button>
          )
        })}
      </nav>

      <div className="relative">
        <div className="flex h-10 w-10 items-center justify-center rounded-full bg-slate-700 text-sm font-semibold">
          U
        </div>
        {isConnected ? (
          <span className="absolute bottom-0 right-0 h-2.5 w-2.5 rounded-full border border-slate-950 bg-emerald-500" />
        ) : (
          <span className="absolute bottom-0 right-0 h-2.5 w-2.5 rounded-full border border-slate-950 bg-slate-500" />
        )}
      </div>
    </aside>
  )
}
