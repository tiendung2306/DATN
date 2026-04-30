import { ChangeEventHandler, useEffect, useState } from 'react'
import { downloadBundleFile } from '../../../lib/onboardingRequest'
import { runtimeClient } from '../../../services/runtime/runtimeClient'

export default function AdminPanelScreen() {
  const [adminPasswordInput, setAdminPasswordInput] = useState('')
  const [adminPassphrase, setAdminPassphrase] = useState('')
  const [isAdminUnlocked, setIsAdminUnlocked] = useState(false)
  const [entryMode, setEntryMode] = useState<'manual' | 'file'>('manual')
  const [peerId, setPeerId] = useState('')
  const [mlsPublicKey, setMlsPublicKey] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [adminReady, setAdminReady] = useState<boolean | null>(null)
  const [history, setHistory] = useState<Array<{ id: string; display_name: string; peer_id: string }>>([])

  const loadAdminStatus = async () => {
    try {
      const admin = await runtimeClient.getAdminStatus()
      setAdminReady(admin.has_admin_key)
    } catch {
      setAdminReady(null)
    }
  }

  const loadHistory = async () => {
    try {
      setHistory(await runtimeClient.listIssuanceHistory())
    } catch {
      setHistory([])
    }
  }

  useEffect(() => {
    void loadAdminStatus()
    void loadHistory()
  }, [])

  const handleInit = async () => {
    if (!adminPasswordInput) return
    setStatus('')
    setError('')
    try {
      await runtimeClient.initAdminKey(adminPasswordInput)
      setAdminPassphrase(adminPasswordInput)
      setIsAdminUnlocked(true)
      setStatus('Admin key initialized.')
      await loadAdminStatus()
    } catch (e) {
      setError(String(e))
    }
  }

  const handleUnlock = () => {
    if (!adminPasswordInput.trim()) return
    setAdminPassphrase(adminPasswordInput.trim())
    setIsAdminUnlocked(true)
    setStatus('')
    setError('')
  }

  const handleIssue = async () => {
    if (!adminPassphrase || !displayName || !peerId || !mlsPublicKey) return
    setStatus('')
    setError('')
    try {
      const bundle = await runtimeClient.createBundleFromRequest({
        display_name: displayName,
        peer_id: peerId.trim(),
        public_key_hex: mlsPublicKey.trim(),
        admin_passphrase: adminPassphrase,
      })
      const defaultFileName = `${displayName.trim().replace(/\s+/g, '-') || 'invite'}.bundle`
      const inputName = window.prompt('Nhap ten file bundle (.bundle):', defaultFileName)
      if (inputName !== null) {
        downloadBundleFile(bundle, inputName)
      }
      setStatus(`Bundle created (${bundle.length} bytes).`)
      await loadHistory()
    } catch (e) {
      setError(String(e))
    }
  }

  const handleImportRequestFile: ChangeEventHandler<HTMLInputElement> = async (event) => {
    const file = event.target.files?.[0]
    if (!file) return
    setStatus('')
    setError('')
    try {
      const raw = await file.text()
      const req = await runtimeClient.parseDeviceRequestJson(raw)
      setPeerId(req.peer_id)
      setMlsPublicKey(req.mls_public_key)
      setStatus(`Loaded request file: ${file.name}`)
    } catch (e) {
      setError(`Invalid request file: ${String(e)}`)
    } finally {
      event.target.value = ''
    }
  }

  if (!isAdminUnlocked) {
    return (
      <div className="space-y-4 p-4 text-sm text-slate-200">
        <h3 className="font-semibold">Admin Access</h3>
        <p className="text-xs text-slate-400">
          Nhap admin password de mo khoa man hinh quan tri truoc khi issue bundle.
        </p>
        <input
          type="password"
          value={adminPasswordInput}
          onChange={(event) => setAdminPasswordInput(event.target.value)}
          placeholder="Admin password"
          className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
        />
        <div className="flex gap-2">
          {adminReady === false ? (
            <button
              className="btn-secondary text-xs"
              onClick={() => void handleInit()}
              disabled={!adminPasswordInput.trim()}
            >
              Init admin key
            </button>
          ) : null}
          <button
            className="btn-secondary text-xs"
            onClick={handleUnlock}
            disabled={!adminPasswordInput.trim()}
          >
            Continue
          </button>
        </div>
        {error ? <p className="text-xs text-red-300">{error}</p> : null}
        {status ? <p className="text-xs text-emerald-300">{status}</p> : null}
      </div>
    )
  }

  return (
    <div className="space-y-4 p-4 text-sm text-slate-200">
      <h3 className="font-semibold">Admin Bundle Issuance</h3>
      <div className="rounded border border-slate-700 bg-slate-900/50 px-3 py-2 text-xs text-slate-300">
        Admin passphrase da duoc nhap. Ban co the issue bundle.
      </div>
      <div className="grid grid-cols-2 gap-2 text-xs">
        <button
          className={`rounded border px-2 py-1 ${entryMode === 'manual' ? 'border-emerald-500 text-emerald-300' : 'border-slate-700 text-slate-300'}`}
          onClick={() => setEntryMode('manual')}
          type="button"
        >
          Nhap tay
        </button>
        <button
          className={`rounded border px-2 py-1 ${entryMode === 'file' ? 'border-emerald-500 text-emerald-300' : 'border-slate-700 text-slate-300'}`}
          onClick={() => setEntryMode('file')}
          type="button"
        >
          Import file request
        </button>
      </div>

      <input
        value={displayName}
        onChange={(event) => setDisplayName(event.target.value)}
        placeholder="Display name (admin assigned)"
        className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
      />

      {entryMode === 'manual' ? (
        <>
          <input
            value={peerId}
            onChange={(event) => setPeerId(event.target.value)}
            placeholder="Peer ID"
            className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
          />
          <input
            value={mlsPublicKey}
            onChange={(event) => setMlsPublicKey(event.target.value)}
            placeholder="MLS public key (hex)"
            className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
          />
        </>
      ) : (
        <div className="space-y-2 rounded border border-slate-700 p-3">
          <input type="file" accept=".json,application/json,.bundle" onChange={(event) => void handleImportRequestFile(event)} />
          <p className="text-xs text-slate-400">Import request file de tu dong dien Peer ID va MLS public key.</p>
          <input
            value={peerId}
            onChange={(event) => setPeerId(event.target.value)}
            placeholder="Peer ID"
            className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
          />
          <input
            value={mlsPublicKey}
            onChange={(event) => setMlsPublicKey(event.target.value)}
            placeholder="MLS public key (hex)"
            className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
          />
        </div>
      )}

      <button
        className="btn-secondary text-xs"
        onClick={() => void handleIssue()}
        disabled={!adminPassphrase || !displayName.trim() || !peerId.trim() || !mlsPublicKey.trim()}
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
      {error ? <p className="text-xs text-red-300">{error}</p> : null}
      {status ? <p className="text-xs text-emerald-300">{status}</p> : null}
    </div>
  )
}
