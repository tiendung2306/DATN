import { useEffect, useState } from 'react'
import { service } from '../../../../wailsjs/go/models'
import AwaitingBundleView from '../../../components/onboarding/AwaitingBundleView'
import { downloadDeviceRequestJSON } from '../../../lib/onboardingRequest'
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

  const handleDownloadRequest = () => {
    if (!info) return
    try {
      downloadDeviceRequestJSON(info)
      setSuccessMessage('Da tao request.json. Gui file nay cho quan tri vien.')
      setTimeout(() => setSuccessMessage(null), 2200)
    } catch (e) {
      setError(String(e))
    }
  }

  const handleImportBundle = async () => {
    setImporting(true)
    setError(null)
    try {
      await runtimeClient.openAndImportBundle()
      setSuccessMessage('Import thanh cong. Dang khoi dong node...')
      await onImported()
    } catch (e) {
      const message = String(e)
      if (message.includes('expired')) {
        setError('Chung thu da het han. Vui long yeu cau quan tri vien cap lai.')
      } else if (message.includes('signature')) {
        setError('Chu ky khong hop le. Vui long kiem tra lai file cap phep.')
      } else if (message.includes('mismatch')) {
        setError('Bundle khong khop voi dinh danh thiet bi hien tai.')
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
      onDownloadRequest={handleDownloadRequest}
      onImportBundle={handleImportBundle}
    />
  )
}
