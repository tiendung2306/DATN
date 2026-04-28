import { useEffect, useState } from 'react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'

export default function AdminPanelScreen() {
  const [passphrase, setPassphrase] = useState('')
  const [requestJson, setRequestJson] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [status, setStatus] = useState('')
  const [history, setHistory] = useState<Array<{ id: string; display_name: string; peer_id: string }>>([])

  const loadHistory = async () => {
    try {
      setHistory(await runtimeClient.listIssuanceHistory())
    } catch {
      setHistory([])
    }
  }

  useEffect(() => {
    void loadHistory()
  }, [])

  const handleInit = async () => {
    await runtimeClient.initAdminKey(passphrase)
    setStatus('Admin key initialized.')
  }

  const handleIssue = async () => {
    const req = await runtimeClient.parseDeviceRequestJson(requestJson)
    const bundle = await runtimeClient.createBundleFromRequest({
      display_name: displayName,
      peer_id: req.peer_id,
      public_key_hex: req.mls_public_key,
      admin_passphrase: passphrase,
    })
    setStatus(`Bundle created (${bundle.length} bytes).`)
    await loadHistory()
  }

  return (
    <div className="space-y-4 p-4 text-sm text-slate-200">
      <h3 className="font-semibold">Admin Bundle Issuance</h3>
      <input
        type="password"
        value={passphrase}
        onChange={(event) => setPassphrase(event.target.value)}
        placeholder="Admin passphrase"
        className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
      />
      <div className="flex gap-2">
        <button className="btn-secondary text-xs" onClick={() => void handleInit()} disabled={!passphrase}>
          Init admin key
        </button>
      </div>
      <input
        value={displayName}
        onChange={(event) => setDisplayName(event.target.value)}
        placeholder="Display name (admin assigned)"
        className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
      />
      <textarea
        value={requestJson}
        onChange={(event) => setRequestJson(event.target.value)}
        placeholder='Paste request.json payload: {"version":1,"peer_id":"...","mls_public_key":"..."}'
        className="min-h-[100px] w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
      />
      <button
        className="btn-secondary text-xs"
        onClick={() => void handleIssue()}
        disabled={!passphrase || !displayName || !requestJson}
      >
        Issue bundle
      </button>
      <div className="rounded-lg border border-slate-700 p-3">
        <p className="mb-2 text-xs text-slate-400">Issuance history</p>
        {history.length === 0 ? <p className="text-xs text-slate-500">No issuance records.</p> : null}
        {history.map((entry) => (
          <div key={entry.id} className="text-xs text-slate-300">
            {entry.display_name} - {entry.peer_id}
          </div>
        ))}
      </div>
      {status ? <p className="text-xs text-emerald-300">{status}</p> : null}
    </div>
  )
}
