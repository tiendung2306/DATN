import { useEffect, useState } from 'react'
import {
  GetOnboardingInfo,
  OpenAndImportBundle,
  HasAdminKey,
  CreateAndImportSelfBundle,
} from '../../wailsjs/go/service/Runtime'
import { service } from '../../wailsjs/go/models'
import CopyField from '../components/CopyField'

interface AwaitingBundleScreenProps {
  onImported: () => void
}

export default function AwaitingBundleScreen({ onImported }: AwaitingBundleScreenProps) {
  const [info, setInfo] = useState<service.OnboardingInfo | null>(null)
  const [isAdmin, setIsAdmin] = useState(false)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [copiedBoth, setCopiedBoth] = useState(false)

  const handleCopyBoth = async () => {
    if (!info) return
    try {
      await navigator.clipboard.writeText(`Peer ID: ${info.peer_id}\nPublic Key (hex): ${info.public_key_hex}`)
      setCopiedBoth(true)
      setTimeout(() => setCopiedBoth(false), 1500)
    } catch {
      // clipboard not available
    }
  }

  useEffect(() => {
    Promise.all([GetOnboardingInfo(), HasAdminKey()])
      .then(([inf, hasKey]) => {
        setInfo(inf)
        setIsAdmin(hasKey)
      })
      .catch((e) => setLoadError(String(e)))
  }, [])

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <div className="w-full max-w-lg space-y-4">
        {/* Header */}
        <div className="mb-2 text-center">
          <div className="mb-3 inline-flex h-14 w-14 items-center justify-center rounded-2xl bg-yellow-900/40 text-3xl">
            📦
          </div>
          <h1 className="text-2xl font-semibold text-gray-100">Awaiting Invitation Bundle</h1>
          <p className="mt-2 text-sm text-gray-500">
            This node needs an InvitationBundle signed by the Root Admin to join the network.
          </p>
        </div>

        {loadError && (
          <div className="rounded-lg bg-red-950 border border-red-800 px-4 py-3 text-sm text-red-300">
            {loadError}
          </div>
        )}

        {/* Admin shortcut — visible only when admin key is present */}
        {isAdmin && (
          <AdminSelfSetup info={info} onDone={onImported} />
        )}

        {/* Divider */}
        {isAdmin && (
          <div className="flex items-center gap-3">
            <div className="flex-1 border-t border-gray-800" />
            <span className="text-xs text-gray-600">or import a bundle manually</span>
            <div className="flex-1 border-t border-gray-800" />
          </div>
        )}

        {/* Standard flow — send info to admin + import bundle */}
        <div className="card space-y-5">
          <div>
            <p className="text-xs font-semibold uppercase tracking-wide text-gray-500 mb-3">
              {isAdmin ? 'Manual: send identity to another admin' : 'Step 1 — Send these to Admin'}
            </p>
            <div className="space-y-3">
              {info ? (
                <>
                  <CopyField label="Your Peer ID" value={info.peer_id} />
                  <CopyField label="Your MLS Public Key (hex)" value={info.public_key_hex} />
                  <button
                    onClick={handleCopyBoth}
                    className="btn-secondary w-full text-xs"
                  >
                    {copiedBoth ? '✓ Both Copied' : 'Copy Both (Peer ID + Public Key)'}
                  </button>
                </>
              ) : (
                <div className="h-20 rounded-lg bg-gray-800/50 animate-pulse" />
              )}
            </div>
          </div>

          <div className="border-t border-gray-800" />

          <ImportBundleSection onImported={onImported} isAdmin={isAdmin} />
        </div>
      </div>
    </div>
  )
}

// ─── Admin self-setup card ────────────────────────────────────────────────────

function AdminSelfSetup({
  info,
  onDone,
}: {
  info: service.OnboardingInfo | null
  onDone: () => void
}) {
  const [displayName, setDisplayName] = useState('Admin')
  const [passphrase, setPassphrase] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleCreate = async () => {
    if (!passphrase) return
    setLoading(true)
    setError(null)
    try {
      await CreateAndImportSelfBundle(displayName || 'Admin', passphrase)
      onDone()
    } catch (e: unknown) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="card border-blue-900/60 bg-blue-950/20 space-y-4">
      <div className="flex items-center gap-2">
        <span className="text-base">🛡️</span>
        <div>
          <p className="text-sm font-semibold text-blue-300">Admin Quick Setup</p>
          <p className="text-xs text-gray-500">
            You have an admin key — create and import your own bundle instantly.
          </p>
        </div>
      </div>

      {error && (
        <div className="rounded-lg bg-red-950 border border-red-800 px-4 py-3 text-sm text-red-300">
          {error}
        </div>
      )}

      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="label">Display Name</label>
          <input
            className="input"
            placeholder="Admin"
            value={displayName}
            onChange={(e) => setDisplayName(e.target.value)}
          />
        </div>
        <div>
          <label className="label">Admin Passphrase</label>
          <input
            type="password"
            className="input"
            placeholder="••••••••"
            value={passphrase}
            onChange={(e) => setPassphrase(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
          />
        </div>
      </div>

      {info && (
        <p className="text-xs text-gray-600">
          Will create a bundle for <span className="font-mono text-gray-500">{info.peer_id.slice(0, 20)}…</span>
        </p>
      )}

      <button
        className="btn-primary w-full"
        onClick={handleCreate}
        disabled={loading || !passphrase}
      >
        {loading ? (
          <>
            <span className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
            Creating & importing...
          </>
        ) : (
          'Create & Import My Bundle'
        )}
      </button>
    </div>
  )
}

// ─── Standard import section ──────────────────────────────────────────────────

function ImportBundleSection({
  onImported,
  isAdmin,
}: {
  onImported: () => void
  isAdmin: boolean
}) {
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState(false)

  const handleImport = async () => {
    setImporting(true)
    setError(null)
    try {
      await OpenAndImportBundle()
      setSuccess(true)
      setTimeout(onImported, 1200)
    } catch (e: unknown) {
      const msg = String(e)
      if (msg && msg !== 'undefined') setError(msg)
    } finally {
      setImporting(false)
    }
  }

  return (
    <div>
      <p className="text-xs font-semibold uppercase tracking-wide text-gray-500 mb-3">
        {isAdmin ? 'Import a bundle file' : 'Step 2 — Import bundle received from Admin'}
      </p>

      {success && (
        <div className="mb-3 rounded-lg bg-green-950 border border-green-800 px-4 py-3 text-sm text-green-300">
          Bundle imported successfully! Starting node...
        </div>
      )}

      {error && (
        <div className="mb-3 rounded-lg bg-red-950 border border-red-800 px-4 py-3 text-sm text-red-300">
          {error}
        </div>
      )}

      <button
        className="btn-secondary w-full py-2.5"
        onClick={handleImport}
        disabled={importing || success}
      >
        {importing ? (
          <>
            <span className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
            Importing...
          </>
        ) : (
          'Import Bundle...'
        )}
      </button>
      <p className="mt-2 text-xs text-gray-600 text-center">
        Accepts <code className="text-gray-500">.bundle</code> files created by Admin
      </p>
    </div>
  )
}
