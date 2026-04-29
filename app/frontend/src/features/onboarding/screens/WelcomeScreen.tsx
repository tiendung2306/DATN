import { useState } from 'react'
import WelcomeView from '../../../components/welcome/WelcomeView'
import AdminQuickSetupView from '../../../components/welcome/AdminQuickSetupView'
import { runtimeClient } from '../../../services/runtime/runtimeClient'

interface WelcomeScreenProps {
  onIdentityCreated: () => Promise<void>
  onOpenImportBackup: () => void
}

export default function WelcomeScreen({ onIdentityCreated, onOpenImportBackup }: WelcomeScreenProps) {
  const [view, setView] = useState<'welcome' | 'admin-setup'>('welcome')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleCreateIdentity = async () => {
    setLoading(true)
    setError(null)
    try {
      await runtimeClient.generateKeys()
      await onIdentityCreated()
    } catch (err) {
      setError(String(err))
    } finally {
      setLoading(false)
    }
  }

  const handleAdminSetup = async (displayName: string, passphrase: string) => {
    setLoading(true)
    setError(null)
    try {
      // 1. Check if identity keys already exist, generate if not
      let hasKeys = false
      try {
        const info = await runtimeClient.getOnboardingInfo()
        if (info && info.public_key_hex) {
          hasKeys = true
        }
      } catch (e) {
        // Not generated yet
      }

      if (!hasKeys) {
        await runtimeClient.generateKeys()
      }

      // 2. Initialize Root Admin Key
      await runtimeClient.initAdminKey(passphrase)

      // 3. Create and Import Self Bundle
      await runtimeClient.createAndImportSelfBundle(displayName, passphrase)

      // 4. Notify parent to refresh app state
      await onIdentityCreated()
    } catch (err) {
      setError(String(err))
      setLoading(false)
    }
  }

  if (view === 'admin-setup') {
    return (
      <AdminQuickSetupView
        loading={loading}
        error={error}
        onSubmit={handleAdminSetup}
        onBack={() => {
          setError(null)
          setView('welcome')
        }}
      />
    )
  }

  return (
    <WelcomeView
      loading={loading}
      error={error}
      onCreateIdentity={handleCreateIdentity}
      onOpenImportBackup={onOpenImportBackup}
      onOpenAdminSetup={() => {
        setError(null)
        setView('admin-setup')
      }}
    />
  )
}
