import AppShell from '../components/layout/AppShell'
import { Button } from '../components/ui/button'

interface MainShellPlaceholderProps {
  isAdmin: boolean
}

export default function MainShellPlaceholder({ isAdmin }: MainShellPlaceholderProps) {
  return (
    <AppShell
      title="Secure P2P"
      subtitle={isAdmin ? 'Admin mode available - FE-3 will build full dashboard.' : 'Authorized mode'}
    >
      <div className="flex min-h-[55vh] items-center justify-center">
        <div className="w-full max-w-xl rounded-xl border border-border bg-card p-6 text-center">
          <h2 className="text-lg font-semibold">Onboarding completed</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            FE-2 da hoan tat luong nguoi dung moi. Main application shell chat/settings se duoc mo
            rong o FE-3.
          </p>
          <div className="mt-4">
            <Button variant="secondary" disabled>
              Main shell placeholder
            </Button>
          </div>
        </div>
      </div>
    </AppShell>
  )
}
