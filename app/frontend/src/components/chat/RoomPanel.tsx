import { service } from '../../../wailsjs/go/models'
import { shortPeerId } from '../../lib/chatModel'
import { Button } from '../ui/button'

interface RoomPanelProps {
  activeGroupId: string | null
  isAdmin: boolean
  peers: service.PeerInfo[]
}

export default function RoomPanel({ activeGroupId, isAdmin, peers }: RoomPanelProps) {
  return (
    <aside className="h-full min-h-[78vh] rounded-xl border border-border bg-[#070b12] p-4">
      <div className="mb-4 flex items-center justify-between border-b border-border/60 pb-3">
        <div>
          <p className="text-sm font-semibold">Room Intelligence</p>
          <p className="text-xs text-muted-foreground">{activeGroupId || 'No room selected'}</p>
        </div>
      </div>

      <div className="mb-4 flex items-center gap-3 border-b border-border/60 pb-3 text-xs">
        <button className="border-b border-emerald-500 pb-1 font-medium text-emerald-300">Members</button>
        <button className="pb-1 text-muted-foreground">Media</button>
        <button className="pb-1 text-muted-foreground">Security Audit</button>
      </div>

      <div className="space-y-3">
        <p className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">Members</p>
        <div className="space-y-2">
          {peers.length === 0 ? (
            <p className="text-xs text-muted-foreground">No connected peers</p>
          ) : (
            peers.map((peer) => (
              <div
                key={peer.id}
                className="flex items-center justify-between rounded-md border border-border/70 bg-black/20 px-2 py-2"
              >
                <div>
                  <p className="text-xs font-medium">{peer.display_name || shortPeerId(peer.id)}</p>
                  <p className="text-[11px] text-muted-foreground">{shortPeerId(peer.id)}</p>
                </div>
                <span
                  className={`h-2 w-2 rounded-full ${
                    peer.verified ? 'bg-emerald-400' : 'bg-amber-400'
                  }`}
                  title={peer.verified ? 'verified' : 'unverified'}
                />
              </div>
            ))
          )}
        </div>
      </div>

      <div className="mt-6 space-y-2 border-t border-border/60 pt-4">
        <Button className="w-full" variant="secondary" disabled={!activeGroupId}>
          Add Member
        </Button>
        <Button className="w-full" variant="ghost" disabled={!activeGroupId || !isAdmin}>
          Leave Group
        </Button>
      </div>
    </aside>
  )
}
