import { useState } from 'react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'

export default function SettingsScreen() {
  const [passphrase, setPassphrase] = useState('')
  const [bootstrap, setBootstrap] = useState('')
  const [status, setStatus] = useState('')

  const handleExport = async () => {
    await runtimeClient.exportIdentity(passphrase)
    setStatus('Identity backup exported.')
  }

  const handleReconnect = async () => {
    if (bootstrap.trim()) {
      await runtimeClient.validateMultiaddr(bootstrap.trim())
      await runtimeClient.setBootstrapAddress(bootstrap.trim())
    }
    await runtimeClient.reconnectP2P()
    setStatus('P2P reconnected.')
  }

  const handleExportDiagnostics = async () => {
    const path = await runtimeClient.exportDiagnostics()
    setStatus(`Diagnostics exported: ${path}`)
  }

  return (
    <div className="space-y-4 p-4 text-sm text-slate-200">
      <h3 className="font-semibold">Settings & Recovery</h3>
      <div className="rounded-lg border border-slate-700 p-3">
        <p className="mb-2 text-xs text-slate-400">Identity backup export</p>
        <input
          value={passphrase}
          onChange={(event) => setPassphrase(event.target.value)}
          type="password"
          placeholder="Backup passphrase"
          className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
        />
        <button className="btn-secondary mt-2" onClick={() => void handleExport()} disabled={!passphrase}>
          Export backup
        </button>
      </div>
      <div className="rounded-lg border border-slate-700 p-3">
        <p className="mb-2 text-xs text-slate-400">Bootstrap runtime override</p>
        <input
          value={bootstrap}
          onChange={(event) => setBootstrap(event.target.value)}
          placeholder="/ip4/.../tcp/.../p2p/..."
          className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
        />
        <button className="btn-secondary mt-2" onClick={() => void handleReconnect()}>
          Save & reconnect
        </button>
      </div>
      <div className="rounded-lg border border-slate-700 p-3">
        <p className="mb-2 text-xs text-slate-400">Developer diagnostics</p>
        <div className="flex gap-2">
          <button className="btn-secondary text-xs" onClick={() => void handleExportDiagnostics()}>
            Export diagnostics
          </button>
          <button className="btn-ghost text-xs" onClick={() => void runtimeClient.openLogFolder()}>
            Open log folder
          </button>
        </div>
      </div>
      {status ? <p className="text-xs text-emerald-300">{status}</p> : null}
    </div>
  )
}
