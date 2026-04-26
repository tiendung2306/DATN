import { service } from '../../wailsjs/go/models'
import { NetworkConnectionState } from '../stores/useNetworkStore'

export function mapNodeStatusToNetworkState(status: service.NodeStatus): NetworkConnectionState {
  if (!status.is_running) {
    return 'starting'
  }
  const peers = status.connected_peers ?? []
  if (peers.length === 0) {
    return 'authorized_no_peers'
  }
  return 'connected'
}

export function getNetworkStatusLabel(status: NetworkConnectionState, peerCount: number): string {
  switch (status) {
    case 'connected':
      return `Connected: ${peerCount} peers`
    case 'authorized_no_peers':
      return 'Authorized: no peers'
    case 'syncing':
      return 'Syncing...'
    case 'reconnecting':
      return 'Reconnecting...'
    case 'offline':
      return 'Offline'
    default:
      return 'Starting...'
  }
}
