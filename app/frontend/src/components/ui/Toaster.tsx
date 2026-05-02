import { X } from 'lucide-react'
import { useToastStore } from '../../stores/useToastStore'
import { cn } from '@/lib/utils'

export default function Toaster() {
  const toasts = useToastStore((s) => s.toasts)
  const dismissToast = useToastStore((s) => s.dismissToast)

  if (toasts.length === 0) return null

  return (
    <div
      className="pointer-events-none fixed bottom-4 right-4 z-[100] flex max-h-[min(80vh,28rem)] w-full max-w-md flex-col gap-2 p-0 sm:bottom-6 sm:right-6"
      aria-live="polite"
      aria-relevant="additions"
    >
      {toasts.map((t) => (
        <div
          key={t.id}
          className={cn(
            'pointer-events-auto flex gap-3 rounded-xl border px-4 py-3 shadow-2xl backdrop-blur-sm animate-in fade-in slide-in-from-bottom-3 duration-200',
            t.variant === 'destructive'
              ? 'border-rose-500/40 bg-rose-950/95 text-rose-50 ring-1 ring-rose-500/20'
              : 'border-slate-600/80 bg-slate-900/95 text-slate-100 ring-1 ring-slate-500/15',
          )}
          role="status"
        >
          <div className="min-w-0 flex-1">
            <p className="text-sm font-semibold tracking-tight">{t.title}</p>
            {t.description ? (
              <p
                className={cn(
                  'mt-1 text-xs leading-relaxed opacity-95 [text-wrap:pretty]',
                  t.variant === 'destructive' ? 'text-rose-100/90' : 'text-slate-300',
                )}
              >
                {t.description}
              </p>
            ) : null}
          </div>
          <button
            type="button"
            onClick={() => dismissToast(t.id)}
            className={cn(
              'shrink-0 rounded-lg p-1 transition outline-none focus-visible:ring-2 focus-visible:ring-offset-2 focus-visible:ring-offset-transparent',
              t.variant === 'destructive'
                ? 'text-rose-200 hover:bg-rose-900/60 focus-visible:ring-rose-400'
                : 'text-slate-400 hover:bg-slate-800 focus-visible:ring-emerald-500/50',
            )}
            aria-label="Đóng thông báo"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      ))}
    </div>
  )
}
