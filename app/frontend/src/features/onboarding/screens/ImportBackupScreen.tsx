import { useState } from 'react'
import ImportBackupView from '../../../components/backup/ImportBackupView'
import { runtimeClient } from '../../../services/runtime/runtimeClient'

interface ImportBackupScreenProps {
  onBack: () => void
  onImported: () => Promise<void>
}

export default function ImportBackupScreen({ onBack, onImported }: ImportBackupScreenProps) {
  const [passphrase, setPassphrase] = useState('')
  const [forceReplace, setForceReplace] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [success, setSuccess] = useState<string | null>(null)

  const handleImport = async () => {
    setLoading(true)
    setError(null)
    setSuccess(null)
    try {
      await runtimeClient.importIdentityFromFile(passphrase, forceReplace)
      setSuccess('Khoi phuc thanh cong. Ung dung se tai lai trang thai.')
      await onImported()
    } catch (err) {
      setError(String(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <ImportBackupView
      passphrase={passphrase}
      forceReplace={forceReplace}
      loading={loading}
      error={error}
      success={success}
      onPassphraseChange={setPassphrase}
      onForceReplaceChange={setForceReplace}
      onImport={handleImport}
      onBack={onBack}
    />
  )
}
