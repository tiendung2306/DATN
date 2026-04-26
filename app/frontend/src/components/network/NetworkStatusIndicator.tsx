import { getNetworkStatusLabel } from '../../lib/networkModel'
import { NetworkConnectionState } from '../../stores/useNetworkStore'

interface NetworkStatusIndicatorProps {
  status: NetworkConnectionState
  peerCount: number
}

function dotClass(status: NetworkConnectionState): string {
  switch (status) {
    case 'connected':
      return 'bg-green-500'
    case 'syncing':
    case 'reconnecting':
      return 'bg-amber-500'
    case 'authorized_no_peers':
      return 'bg-slate-400'
    case 'offline':
      return 'bg-red-500'
    default:
      return 'bg-blue-500'
  }
}

export default function NetworkStatusIndicator({ status, peerCount }: NetworkStatusIndicatorProps) {
  return (
    <div className="inline-flex items-center gap-2 rounded-md border border-emerald-900/40 bg-emerald-950/20 px-3 py-1.5 text-xs text-emerald-200">
      <span className={`h-2 w-2 rounded-full ${dotClass(status)} shadow-[0_0_8px_rgba(16,185,129,0.45)]`} />
      {getNetworkStatusLabel(status, peerCount)}
    </div>
  )
}
