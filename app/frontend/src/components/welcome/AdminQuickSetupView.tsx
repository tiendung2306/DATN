import { useState } from 'react'
import { Button } from '../ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'
import { Input } from '../ui/input'
import { Label } from '../ui/label'

interface AdminQuickSetupViewProps {
  loading: boolean
  error: string | null
  onSubmit: (displayName: string, passphrase: string) => void
  onBack: () => void
}

export default function AdminQuickSetupView({
  loading,
  error,
  onSubmit,
  onBack,
}: AdminQuickSetupViewProps) {
  const [displayName, setDisplayName] = useState('')
  const [passphrase, setPassphrase] = useState('')

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    if (!displayName.trim() || !passphrase.trim()) return
    onSubmit(displayName.trim(), passphrase)
  }

  return (
    <div className="flex min-h-[70vh] items-center justify-center p-6">
      <Card className="w-full max-w-xl border-border/80 bg-[#0a0f16]">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">Create New Organization</CardTitle>
          <CardDescription>
            Initialize the Root Admin key and create your official administrator identity.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="displayName">Admin Display Name</Label>
              <Input
                id="displayName"
                placeholder="e.g., Alice (Admin)"
                value={displayName}
                onChange={(e) => setDisplayName(e.target.value)}
                disabled={loading}
                required
                className="bg-background/50"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="passphrase">Admin Passphrase</Label>
              <Input
                id="passphrase"
                type="password"
                placeholder="Enter a strong passphrase"
                value={passphrase}
                onChange={(e) => setPassphrase(e.target.value)}
                disabled={loading}
                required
                className="bg-background/50"
              />
              <p className="text-xs text-muted-foreground">
                This passphrase encrypts the Root Admin key on this device. Do not lose it.
              </p>
            </div>

            {error && (
              <div className="rounded-lg border border-red-900 bg-red-950/60 px-3 py-2 text-sm text-red-300">
                {error}
              </div>
            )}

            <div className="flex flex-col gap-2 pt-2">
              <Button type="submit" className="w-full" size="lg" disabled={loading || !displayName.trim() || !passphrase.trim()}>
                {loading ? 'Initializing Organization...' : 'Create Organization'}
              </Button>
              <Button type="button" variant="secondary" onClick={onBack} disabled={loading}>
                Back
              </Button>
            </div>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}
