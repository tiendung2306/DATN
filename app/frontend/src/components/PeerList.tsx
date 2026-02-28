import { main } from '../../wailsjs/go/models'

interface PeerListProps {
  peers: main.PeerInfo[]
}

function shortID(id: string): string {
  if (id.length <= 16) return id
  return id.slice(0, 8) + '…' + id.slice(-6)
}

export default function PeerList({ peers }: PeerListProps) {
  if (peers.length === 0) {
    return (
      <div className="flex items-center justify-center h-24 text-sm text-gray-600 italic">
        No peers connected
      </div>
    )
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-gray-800 text-left text-xs text-gray-500 uppercase tracking-wide">
            <th className="pb-2 pr-4 font-medium">Peer ID</th>
            <th className="pb-2 pr-4 font-medium">Display Name</th>
            <th className="pb-2 font-medium text-center">Auth</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-800/50">
          {peers.map((peer) => (
            <tr key={peer.id} className="hover:bg-gray-800/30 transition-colors">
              <td className="py-2 pr-4 font-mono text-xs text-gray-400" title={peer.id}>
                {shortID(peer.id)}
              </td>
              <td className="py-2 pr-4 text-gray-300">
                {peer.display_name || <span className="text-gray-600 italic">unknown</span>}
              </td>
              <td className="py-2 text-center">
                {peer.verified ? (
                  <span className="text-green-400 text-base" title="Verified">✓</span>
                ) : (
                  <span className="text-red-400 text-base" title="Not verified">✗</span>
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
