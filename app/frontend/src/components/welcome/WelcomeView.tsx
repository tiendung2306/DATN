import { Button } from '../ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'

interface WelcomeViewProps {
  loading: boolean
  error: string | null
  onCreateIdentity: () => void
  onOpenImportBackup: () => void
}

export default function WelcomeView({
  loading,
  error,
  onCreateIdentity,
  onOpenImportBackup,
}: WelcomeViewProps) {
  return (
    <div className="flex min-h-[70vh] items-center justify-center p-6">
      <Card className="w-full max-w-xl border-border/80 bg-[#0a0f16]">
        <CardHeader className="text-center">
          <CardTitle className="text-2xl">Security Starts On Your Device</CardTitle>
          <CardDescription>
            Generate a local cryptographic identity to begin secure peer-to-peer communication.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-sm text-muted-foreground">
            Your official display name will be assigned by Admin through a signed{' '}
            <code>.bundle</code>.
          </p>
          {error && (
            <div className="rounded-lg border border-red-900 bg-red-950/60 px-3 py-2 text-sm text-red-300">
              {error}
            </div>
          )}
          <Button className="w-full" size="lg" onClick={onCreateIdentity} disabled={loading}>
            {loading ? 'Creating identity...' : 'Create Device Identity'}
          </Button>
          <Button className="w-full" variant="secondary" onClick={onOpenImportBackup} disabled={loading}>
            Import Backup
          </Button>
        </CardContent>
      </Card>
    </div>
  )
}
