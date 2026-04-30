import { service } from '../../wailsjs/go/models'

export interface DeviceRequestPayload {
  version: number
  peer_id: string
  mls_public_key: string
}

export function buildDeviceRequestPayload(info: service.OnboardingInfo): DeviceRequestPayload {
  return {
    version: 1,
    peer_id: info.peer_id,
    mls_public_key: info.public_key_hex,
  }
}

export function downloadDeviceRequestJSON(info: service.OnboardingInfo, filename = 'request.json') {
  const payload = buildDeviceRequestPayload(info)
  downloadTextFile(JSON.stringify(payload, null, 2), ensureExtension(filename, '.json'), 'application/json')
}

export function downloadBundleFile(bundleContent: string, suggestedName: string) {
  downloadTextFile(bundleContent, ensureExtension(suggestedName, '.bundle'), 'application/octet-stream')
}

function downloadTextFile(content: string, filename: string, mimeType: string) {
  const blob = new Blob([content], { type: mimeType })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = filename
  document.body.appendChild(anchor)
  anchor.click()
  document.body.removeChild(anchor)
  // Delay revoke slightly so WebView has enough time to start the download.
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}

function ensureExtension(raw: string, extension: string): string {
  const trimmed = (raw || '').trim()
  if (!trimmed) return `download${extension}`
  if (trimmed.toLowerCase().endsWith(extension)) return trimmed
  return `${trimmed}${extension}`
}

export function askFileName(defaultValue: string, requiredExtension: string): string | null {
  const input = window.prompt(`Nhap ten file (se them ${requiredExtension} neu thieu):`, defaultValue)
  if (input === null) return null
  return ensureExtension(input, requiredExtension)
}

export function parseDeviceRequestFromText(raw: string): service.DeviceAccessRequest {
  return service.DeviceAccessRequest.createFrom(JSON.parse(raw))
}
