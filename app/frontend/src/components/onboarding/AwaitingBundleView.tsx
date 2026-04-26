import CopyField from '../CopyField'
import { Button } from '../ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '../ui/card'
import { service } from '../../../wailsjs/go/models'

interface AwaitingBundleViewProps {
  info: service.OnboardingInfo | null
  loadingInfo: boolean
  importing: boolean
  copiedAll: boolean
  error: string | null
  successMessage: string | null
  onCopyAll: () => void
  onDownloadRequest: () => void
  onImportBundle: () => void
}

export default function AwaitingBundleView({
  info,
  loadingInfo,
  importing,
  copiedAll,
  error,
  successMessage,
  onCopyAll,
  onDownloadRequest,
  onImportBundle,
}: AwaitingBundleViewProps) {
  return (
    <div className="flex min-h-[70vh] items-center justify-center p-6">
      <Card className="w-full max-w-2xl border-border/80 bg-[#0a0f16]">
        <CardHeader>
          <CardTitle>Awaiting Identity Authorization</CardTitle>
          <CardDescription>
            Share this identity request with Admin, then import the signed <code>.bundle</code>.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {loadingInfo ? (
            <div className="space-y-2">
              <div className="h-11 animate-pulse rounded-lg bg-muted" />
              <div className="h-11 animate-pulse rounded-lg bg-muted" />
            </div>
          ) : info ? (
            <>
              <CopyField label="Peer ID" value={info.peer_id} />
              <CopyField label="MLS Public Key (hex)" value={info.public_key_hex} />
              <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
                <Button variant="secondary" onClick={onCopyAll}>
                  {copiedAll ? 'Copied' : 'Copy All'}
                </Button>
                <Button variant="secondary" onClick={onDownloadRequest}>
                  Download request.json
                </Button>
                <Button onClick={onImportBundle} disabled={importing}>
                  {importing ? 'Importing...' : 'Import Bundle'}
                </Button>
              </div>
            </>
          ) : null}

          {error && (
            <div className="rounded-lg border border-red-900 bg-red-950/60 px-3 py-2 text-sm text-red-300">
              {error}
            </div>
          )}
          {successMessage && (
            <div className="rounded-lg border border-green-900 bg-green-950/60 px-3 py-2 text-sm text-green-300">
              {successMessage}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
