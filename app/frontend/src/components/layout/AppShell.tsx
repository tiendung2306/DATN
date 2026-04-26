import { ReactNode } from 'react'

interface AppShellProps {
  title?: string
  subtitle?: string
  children: ReactNode
}

export default function AppShell({ title, subtitle, children }: AppShellProps) {
  return (
    <div className="min-h-screen bg-[#080b12] text-foreground">
      <header className="border-b border-border/70 bg-[#0b1018]/95 backdrop-blur">
        <div className="mx-auto flex w-full max-w-[1400px] items-center justify-between gap-6 px-6 py-3">
          <div className="flex min-w-0 items-center gap-8">
            <h1 className="text-sm font-semibold tracking-[0.16em] text-foreground/90">
              {title ?? 'Secure P2P'}
            </h1>
            <nav className="hidden items-center gap-6 text-xs font-medium tracking-wide text-muted-foreground md:flex">
              <button className="transition hover:text-foreground">DIRECT</button>
              <button className="border-b border-emerald-500 pb-1 text-foreground">INTERNAL</button>
              <button className="transition hover:text-foreground">GLOBAL</button>
            </nav>
          </div>

          <div className="flex items-center gap-3 text-xs text-muted-foreground">
            {subtitle && <p className="hidden md:block">{subtitle}</p>}
            <span className="h-2 w-2 rounded-full bg-emerald-400" title="Secure session" />
            <span className="h-2 w-2 rounded-full bg-sky-400" title="Connected" />
          </div>
        </div>
      </header>
      <main className="mx-auto w-full max-w-[1400px] px-4 py-6 sm:px-6">{children}</main>
    </div>
  )
}
