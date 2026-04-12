import { useState } from 'react'
import { ExportIdentity, ImportIdentityFromFile } from '../../wailsjs/go/service/Runtime'
import { Quit } from '../../wailsjs/runtime/runtime'

type StatusKind = 'ok' | 'err' | 'info'

export default function IdentityBackupPanel() {
  const [passphrase, setPassphrase] = useState('')
  const [forceImport, setForceImport] = useState(false)
  const [busy, setBusy] = useState(false)
  const [status, setStatus] = useState<{ kind: StatusKind; text: string } | null>(null)

  const setErr = (e: unknown) => {
    const msg = e instanceof Error ? e.message : String(e)
    setStatus({ kind: 'err', text: msg })
  }

  const handleExport = async () => {
    if (!passphrase.trim()) {
      setStatus({ kind: 'err', text: 'Enter a passphrase first.' })
      return
    }
    setBusy(true)
    setStatus(null)
    try {
      await ExportIdentity(passphrase)
      setStatus({
        kind: 'ok',
        text: 'Export finished. File path was chosen in the save dialog.',
      })
      setPassphrase('')
    } catch (e) {
      setErr(e)
    } finally {
      setBusy(false)
    }
  }

  const handleImport = async () => {
    if (!passphrase.trim()) {
      setStatus({ kind: 'err', text: 'Enter a passphrase first.' })
      return
    }
    setBusy(true)
    setStatus(null)
    try {
      await ImportIdentityFromFile(passphrase, forceImport)
      setStatus({
        kind: 'info',
        text: 'Import succeeded. Restart the app so the new identity and P2P stack load correctly.',
      })
      setPassphrase('')
      setForceImport(false)
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e)
      if (msg.includes('force=true') || msg.includes('force')) {
        setStatus({
          kind: 'err',
          text: `${msg} Enable “Force replace” if you intend to overwrite this device’s identity.`,
        })
      } else {
        setErr(e)
      }
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="card mt-4 space-y-4">
      <div>
        <h2 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
          Identity backup <span className="text-amber-600/90">(dev / test)</span>
        </h2>
        <p className="mt-1 text-xs text-gray-600">
          Encrypted .backup includes libp2p + MLS keys and invitation data. Keep passphrase and file offline.
        </p>
      </div>

      <div>
        <label className="label" htmlFor="backup-passphrase">
          Passphrase
        </label>
        <input
          id="backup-passphrase"
          type="password"
          autoComplete="off"
          className="input font-mono"
          placeholder="Passphrase for encrypt / decrypt"
          value={passphrase}
          onChange={(e) => setPassphrase(e.target.value)}
          disabled={busy}
        />
      </div>

      <label className="flex cursor-pointer items-center gap-2 text-sm text-gray-400">
        <input
          type="checkbox"
          className="rounded border-gray-600 bg-gray-800"
          checked={forceImport}
          onChange={(e) => setForceImport(e.target.checked)}
          disabled={busy}
        />
        Force replace existing identity (import only)
      </label>

      <div className="flex flex-wrap gap-2">
        <button type="button" className="btn-secondary text-xs" disabled={busy} onClick={handleExport}>
          Export encrypted backup…
        </button>
        <button type="button" className="btn-secondary text-xs" disabled={busy} onClick={handleImport}>
          Import from backup…
        </button>
        <button
          type="button"
          className="btn-primary text-xs"
          disabled={busy}
          onClick={() => Quit()}
          title="Close app after import so identity reloads on next start"
        >
          Quit app
        </button>
      </div>

      {status && (
        <p
          className={
            status.kind === 'ok'
              ? 'text-sm text-green-400'
              : status.kind === 'info'
                ? 'text-sm text-amber-400/90'
                : 'text-sm text-red-400'
          }
        >
          {status.text}
        </p>
      )}
    </div>
  )
}
