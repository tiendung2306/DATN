import { useState } from 'react'
import { GenerateKeys } from '../../wailsjs/go/service/Runtime'
import { service } from '../../wailsjs/go/models'
import CopyField from '../components/CopyField'

interface SetupScreenProps {
  onDone: () => void
}

export default function SetupScreen({ onDone }: SetupScreenProps) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [info, setInfo] = useState<service.OnboardingInfo | null>(null)
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

  const handleGenerate = async () => {
    setLoading(true)
    setError(null)
    try {
      const result = await GenerateKeys()
      setInfo(result)
    } catch (e: unknown) {
      setError(String(e))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-6">
      <div className="w-full max-w-lg">
        {/* Header */}
        <div className="mb-8 text-center">
          <div className="mb-3 inline-flex h-14 w-14 items-center justify-center rounded-2xl bg-blue-900/40 text-3xl">
            🔑
          </div>
          <h1 className="text-2xl font-semibold text-gray-100">First Launch Setup</h1>
          <p className="mt-2 text-sm text-gray-500">
            Generate your cryptographic identity to get started.
          </p>
        </div>

        <div className="card space-y-5">
          {!info ? (
            <>
              <p className="text-sm text-gray-400 leading-relaxed">
                This will generate a <span className="text-gray-200 font-medium">Libp2p PeerID</span> and
                an <span className="text-gray-200 font-medium">MLS public key</span> on this machine.
                The private keys never leave your device.
              </p>

              {error && (
                <div className="rounded-lg bg-red-950 border border-red-800 px-4 py-3 text-sm text-red-300">
                  {error}
                </div>
              )}

              <button
                className="btn-primary w-full py-3 text-base"
                onClick={handleGenerate}
                disabled={loading}
              >
                {loading ? (
                  <>
                    <span className="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white" />
                    Generating...
                  </>
                ) : (
                  'Generate Keys'
                )}
              </button>
            </>
          ) : (
            <>
              <div className="rounded-lg bg-green-950 border border-green-800 px-4 py-3 text-sm text-green-300">
                Keys generated successfully. Send both values to Admin out-of-band (Zalo, email, etc.).
              </div>

              <div className="space-y-3">
                <CopyField label="Your Peer ID" value={info.peer_id} />
                <CopyField label="Your MLS Public Key (hex)" value={info.public_key_hex} />
                <button
                  onClick={handleCopyBoth}
                  className="btn-secondary w-full text-xs"
                >
                  {copiedBoth ? '✓ Both Copied' : 'Copy Both (Peer ID + Public Key)'}
                </button>
              </div>

              <p className="text-xs text-gray-600 leading-relaxed">
                Admin will assign your display name and send back a <code className="text-gray-500">.bundle</code> file.
                Import it on the next screen.
              </p>

              <button className="btn-secondary w-full" onClick={onDone}>
                Continue →
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  )
}
