import { useState } from 'react'
import { CreateBundle, InitAdminKey } from '../../wailsjs/go/service/Runtime'
import { service } from '../../wailsjs/go/models'

type Tab = 'init' | 'create'

export default function AdminPanel() {
  const [activeTab, setActiveTab] = useState<Tab>('create')

  return (
    <div className="card mt-4">
      <h2 className="text-sm font-semibold text-gray-300 mb-4">Admin Panel</h2>

      {/* Tab bar */}
      <div className="flex gap-1 mb-5 bg-gray-800 p-1 rounded-lg w-fit">
        {(['create', 'init'] as Tab[]).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-1.5 rounded-md text-sm font-medium transition-colors ${
              activeTab === tab
                ? 'bg-gray-700 text-gray-100'
                : 'text-gray-500 hover:text-gray-300'
            }`}
          >
            {tab === 'create' ? 'Create Bundle' : 'Init Admin Key'}
          </button>
        ))}
      </div>

      {activeTab === 'init' && <InitAdminKeyTab />}
      {activeTab === 'create' && <CreateBundleTab />}
    </div>
  )
}

function InitAdminKeyTab() {
  const [passphrase, setPassphrase] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  const handleInit = async () => {
    if (!passphrase) return
    setLoading(true)
    setError(null)
    try {
      await InitAdminKey(passphrase)
      setSuccess(true)
      setPassphrase('')
    } catch (e: unknown) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  if (success) {
    return (
      <div className="rounded-lg bg-green-950 border border-green-800 px-4 py-3 text-sm text-green-300">
        Admin key initialized successfully. The node is now in ADMIN_READY state.
      </div>
    )
  }

  return (
    <div className="space-y-4 max-w-sm">
      <p className="text-sm text-gray-400">
        Run once on the Admin machine. Creates the Root Ed25519 key encrypted with your passphrase.
      </p>
      {error && (
        <div className="rounded-lg bg-red-950 border border-red-800 px-4 py-3 text-sm text-red-300">
          {error}
        </div>
      )}
      <div>
        <label className="label">Admin Passphrase</label>
        <input
          type="password"
          className="input"
          placeholder="Strong passphrase..."
          value={passphrase}
          onChange={(e) => setPassphrase(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleInit()}
        />
      </div>
      <button
        className="btn-primary"
        onClick={handleInit}
        disabled={loading || !passphrase}
      >
        {loading ? (
          <>
            <span className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
            Initializing...
          </>
        ) : (
          'Initialize Admin Key'
        )}
      </button>
    </div>
  )
}

function CreateBundleTab() {
  const [form, setForm] = useState<service.CreateBundleRequest>(
    new service.CreateBundleRequest({
      display_name: '',
      peer_id: '',
      public_key_hex: '',
      admin_passphrase: '',
    })
  )
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [outputPath, setOutputPath] = useState<string | null>(null)

  const set = (field: keyof service.CreateBundleRequest) => (
    (e: React.ChangeEvent<HTMLInputElement>) =>
      setForm((prev: service.CreateBundleRequest) => ({ ...prev, [field]: e.target.value }))
  )

  const handleCreate = async () => {
    setLoading(true)
    setError(null)
    setOutputPath(null)
    try {
      const path = await CreateBundle(form)
      if (path) setOutputPath(path)
    } catch (e: unknown) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  const isValid = form.display_name && form.peer_id && form.public_key_hex && form.admin_passphrase

  return (
    <div className="space-y-4 max-w-lg">
      {error && (
        <div className="rounded-lg bg-red-950 border border-red-800 px-4 py-3 text-sm text-red-300">
          {error}
        </div>
      )}
      {outputPath && (
        <div className="rounded-lg bg-green-950 border border-green-800 px-4 py-3 text-sm text-green-300">
          Bundle saved: <code className="font-mono text-xs break-all">{outputPath}</code>
        </div>
      )}

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="label">Display Name</label>
          <input className="input" placeholder="Alice" value={form.display_name} onChange={set('display_name')} />
        </div>
        <div>
          <label className="label">Admin Passphrase</label>
          <input className="input" type="password" placeholder="••••••••" value={form.admin_passphrase} onChange={set('admin_passphrase')} />
        </div>
      </div>

      <div>
        <label className="label">Peer ID</label>
        <input className="input font-mono text-xs" placeholder="12D3KooW..." value={form.peer_id} onChange={set('peer_id')} />
      </div>

      <div>
        <label className="label">MLS Public Key (hex)</label>
        <input className="input font-mono text-xs" placeholder="a3f7c2..." value={form.public_key_hex} onChange={set('public_key_hex')} />
      </div>

      <button
        className="btn-primary"
        onClick={handleCreate}
        disabled={loading || !isValid}
      >
        {loading ? (
          <>
            <span className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
            Creating...
          </>
        ) : (
          'Create Bundle...'
        )}
      </button>
    </div>
  )
}
