import { useState } from 'react'
import { GenerateKeys } from '../../wailsjs/go/service/Runtime'
import WelcomeView from '../components/welcome/WelcomeView'

interface WelcomeScreenProps {
  onIdentityCreated: () => Promise<void>
  onOpenImportBackup: () => void
}

export default function WelcomeScreen({ onIdentityCreated, onOpenImportBackup }: WelcomeScreenProps) {
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const handleCreateIdentity = async () => {
    setLoading(true)
    setError(null)
    try {
      await GenerateKeys()
      await onIdentityCreated()
    } catch (err) {
      setError(String(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <WelcomeView
      loading={loading}
      error={error}
      onCreateIdentity={handleCreateIdentity}
      onOpenImportBackup={onOpenImportBackup}
    />
  )
}
