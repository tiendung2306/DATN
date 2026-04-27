import { ReactNode } from 'react'

interface AppShellProps {
  title?: string
  subtitle?: string
  children: ReactNode
}

export default function AppShell({ title, subtitle, children }: AppShellProps) {
  return (
    <div className="h-screen w-full bg-slate-950 text-slate-200">
      <main className="h-full w-full" aria-label={title ?? 'Secure workspace'}>
        {subtitle ? <span className="sr-only">{subtitle}</span> : null}
        {children}
      </main>
    </div>
  )
}
