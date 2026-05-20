import { useEffect, useState } from 'react'
import { service } from '../../../../wailsjs/go/models'
import AwaitingBundleView from '../../../components/onboarding/AwaitingBundleView'
import { runtimeClient } from '../../../services/runtime/runtimeClient'

interface AwaitingBundleScreenProps {
  onImported: () => Promise<void>
}

export default function AwaitingBundleScreen({ onImported }: AwaitingBundleScreenProps) {
  const [info, setInfo] = useState<service.OnboardingInfo | null>(null)
  const [loadingInfo, setLoadingInfo] = useState(true)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [successMessage, setSuccessMessage] = useState<string | null>(null)
  const [copiedAll, setCopiedAll] = useState(false)

  useEffect(() => {
    const load = async () => {
      setLoadingInfo(true)
      setError(null)
      try {
        const onboarding = await runtimeClient.getOnboardingInfo()
        setInfo(onboarding)
      } catch (e) {
        setError(String(e))
      } finally {
        setLoadingInfo(false)
      }
    }
    void load()
  }, [])

  const handleCopyAll = async () => {
    if (!info) return
    try {
      await navigator.clipboard.writeText(
        `Peer ID: ${info.peer_id}\nMLS Public Key: ${info.public_key_hex}`,
      )
      setCopiedAll(true)
      setTimeout(() => setCopiedAll(false), 1200)
    } catch (e) {
      setError(String(e))
    }
  }

  const handleDownloadRequest = async () => {
    if (!info) return
    try {
      const savedPath = await runtimeClient.exportDeviceRequestJson()
      if (!savedPath) return // User cancelled
      setSuccessMessage(`Request file saved to: ${savedPath}`)
      setTimeout(() => setSuccessMessage(null), 2500)
    } catch (e) {
      setError(String(e))
    }
  }

  const handleImportBundle = async () => {
    setError(null)
    try {
      const imported = await runtimeClient.openAndImportBundle()
      if (!imported) return // User cancelled
      
      setSuccessMessage('Import successful. Launching node...')
      await onImported()
    } catch (e) {
      const message = String(e)
      if (message.includes('expired')) {
        setError('The invitation bundle has expired. Please request a new one from Admin.')
      } else if (message.includes('signature')) {
        setError('Invalid Admin signature. The file may have been tampered with.')
      } else if (message.includes('mismatch')) {
        setError('Identity mismatch. This bundle was issued for a different device.')
      } else {
        setError(message)
      }
    } finally {
      setImporting(false)
    }
  }

  return (
    <AwaitingBundleView
      info={info}
      loadingInfo={loadingInfo}
      importing={importing}
      copiedAll={copiedAll}
      error={error}
      successMessage={successMessage}
      onCopyAll={handleCopyAll}
      onDownloadRequest={() => void handleDownloadRequest()}
      onImportBundle={handleImportBundle}
    />
  )
}
