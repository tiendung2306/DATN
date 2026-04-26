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

export function downloadDeviceRequestJSON(info: service.OnboardingInfo) {
  const payload = buildDeviceRequestPayload(info)
  const content = JSON.stringify(payload, null, 2)
  const blob = new Blob([content], { type: 'application/json' })
  const url = URL.createObjectURL(blob)

  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = 'request.json'
  anchor.click()

  URL.revokeObjectURL(url)
}
