import { Button } from '../ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'
import { Input } from '../ui/input'

interface ImportBackupViewProps {
  passphrase: string
  forceReplace: boolean
  loading: boolean
  error: string | null
  success: string | null
  onPassphraseChange: (value: string) => void
  onForceReplaceChange: (value: boolean) => void
  onImport: () => void
  onBack: () => void
}

export default function ImportBackupView({
  passphrase,
  forceReplace,
  loading,
  error,
  success,
  onPassphraseChange,
  onForceReplaceChange,
  onImport,
  onBack,
}: ImportBackupViewProps) {
  return (
    <div className="flex min-h-[70vh] items-center justify-center p-6">
      <Card className="w-full max-w-xl">
        <CardHeader>
          <CardTitle>Khoi Phuc Dinh Danh Tu Backup</CardTitle>
          <CardDescription>
            Import se khoi phuc danh tinh va co the thay the phien hien tai tren thiet bi khac.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="rounded-lg border border-amber-900 bg-amber-950/40 px-3 py-2 text-sm text-amber-300">
            Neu khoi phuc tren thiet bi moi, thiet bi cu co the bi mat phien hoat dong.
          </div>
          <div className="space-y-1.5">
            <label className="text-xs font-medium text-muted-foreground">Passphrase</label>
            <Input
              type="password"
              value={passphrase}
              onChange={(e) => onPassphraseChange(e.target.value)}
              placeholder="Nhap passphrase de giai ma .backup"
              disabled={loading}
            />
          </div>
          <label className="flex items-center gap-2 text-sm text-muted-foreground">
            <input
              type="checkbox"
              checked={forceReplace}
              onChange={(e) => onForceReplaceChange(e.target.checked)}
              disabled={loading}
            />
            Ghi de danh tinh hien tai tren may nay
          </label>
          {error && (
            <div className="rounded-lg border border-red-900 bg-red-950/60 px-3 py-2 text-sm text-red-300">
              {error}
            </div>
          )}
          {success && (
            <div className="rounded-lg border border-green-900 bg-green-950/60 px-3 py-2 text-sm text-green-300">
              {success}
            </div>
          )}
          <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
            <Button variant="secondary" onClick={onBack} disabled={loading}>
              Quay lai
            </Button>
            <Button onClick={onImport} disabled={loading || !passphrase.trim()}>
              {loading ? 'Dang import...' : 'Chon file .backup va khoi phuc'}
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
