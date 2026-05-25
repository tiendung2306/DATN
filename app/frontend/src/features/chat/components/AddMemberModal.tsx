import { useState, useEffect } from 'react'
import { Button } from '../../../components/ui/button'
import { Input } from '../../../components/ui/input'
import { Label } from '../../../components/ui/label'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '../../../components/ui/dialog'
import { Search, Check, UserPlus } from 'lucide-react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import { formatOutboundSendError } from '../../../lib/formatSendError'
import { useToastStore } from '../../../stores/useToastStore'

interface AddMemberModalProps {
  isOpen: boolean
  onClose: () => void
  groupId: string
  onSuccess?: () => void
}

interface PeerInfo {
  id: string
  display_name: string
  verified?: boolean
}

export default function AddMemberModal({
  isOpen,
  onClose,
  groupId,
  onSuccess,
}: AddMemberModalProps) {
  const [searchQuery, setSearchQuery] = useState('')
  const [knownPeers, setKnownPeers] = useState<PeerInfo[]>([])
  const [selectedPeers, setSelectedPeers] = useState<string[]>([])
  const [inviting, setInviting] = useState(false)
  const [error, setError] = useState('')
  const pushToast = useToastStore((s) => s.pushToast)

  useEffect(() => {
    if (isOpen && groupId) {
      const loadPeers = async () => {
        try {
          // Fetch all peers known to the system
          const peers = await runtimeClient.getKnownPeers()
          
          // Fetch existing members of this group to filter them out
          const members = await runtimeClient.getGroupMembers(groupId)
          const memberIds = members.map((m: any) => m.peer_id)

          setKnownPeers(
            peers
              .map((p: any) => ({
                id: p.id,
                display_name: p.display_name || p.id.slice(0, 10),
                verified: p.verified,
              }))
              .filter((p: any) => !memberIds.includes(p.id)) // Only show non-members
          )
        } catch (e) {
          console.error("Failed to load appropriate candidate peers", e)
        }
      }
      loadPeers()
    }
  }, [isOpen, groupId])

  const toggleSelection = (peerId: string) => {
    if (selectedPeers.includes(peerId)) {
      setSelectedPeers(selectedPeers.filter(id => id !== peerId))
    } else {
      setSelectedPeers([...selectedPeers, peerId])
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (selectedPeers.length === 0) return

    const peers = selectedPeers
      .map((peerId) => peerId.trim())
      .filter((peerId) => peerId.length > 0)
    setInviting(true)
    setError('')
    setSelectedPeers([])
    if (onSuccess) onSuccess()
    onClose()
    void (async () => {
      const failures: string[] = []
      for (const peerId of peers) {
        try {
          await runtimeClient.requestGroupInvite(groupId, peerId)
        } catch (err) {
          const mapped = formatOutboundSendError(err)
          failures.push(mapped.description ? `${mapped.title}: ${mapped.description}` : mapped.title)
        }
      }
      setInviting(false)
      if (failures.length > 0) {
        pushToast({
          title: 'Some invite requests failed',
          description: failures.slice(0, 4).join(' · '),
          variant: 'destructive',
        })
      }
    })()
  }

  const getInitials = (name: string) => {
    if (!name) return '?'
    const parts = name.trim().split(' ')
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
    }
    return name.slice(0, 2).toUpperCase()
  }

  const filteredPeers = knownPeers.filter(peer => 
    peer.display_name.toLowerCase().includes(searchQuery.toLowerCase()) ||
    peer.id.toLowerCase().includes(searchQuery.toLowerCase())
  )

  return (
    <Dialog open={isOpen} onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="sm:max-w-md bg-slate-900 border-slate-800 text-slate-100 ring-1 ring-slate-800 shadow-2xl">
        <DialogHeader>
          <DialogTitle className="text-xl font-bold text-slate-100 flex items-center gap-2">
            <UserPlus className="h-5 w-5 text-emerald-400" />
            Add Members to Group
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-4 mt-2">
          <div className="space-y-2">
            <Label className="text-xs text-slate-400 font-semibold uppercase tracking-wider flex justify-between items-center">
              <span>Available Users ({selectedPeers.length})</span>
            </Label>
            
            <div className="relative">
              <Search className="absolute left-3 top-2.5 h-4 w-4 text-slate-500" />
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search by name or Peer ID..."
                className="pl-9 bg-slate-950 border-slate-800 text-slate-100 placeholder:text-slate-600 focus-visible:ring-emerald-500 text-xs"
              />
            </div>

            <div className="mt-2 border border-slate-800 bg-slate-950 rounded-lg max-h-56 overflow-y-auto divide-y divide-slate-800/50">
              {filteredPeers.length === 0 ? (
                <p className="text-xs text-slate-600 italic text-center py-4">No new members available</p>
              ) : (
                filteredPeers.map((peer) => {
                  const isSelected = selectedPeers.includes(peer.id)
                  const initials = getInitials(peer.display_name)
                  return (
                    <div 
                      key={peer.id}
                      onClick={() => toggleSelection(peer.id)}
                      className={`flex items-center justify-between p-2.5 hover:bg-slate-800/40 cursor-pointer transition duration-150 ${
                        isSelected ? 'bg-slate-800/60' : ''
                      }`}
                    >
                      <div className="flex items-center gap-3 min-w-0">
                        <div className="flex h-8 w-8 items-center justify-center rounded-full bg-slate-800 border border-slate-700 text-xs font-bold text-emerald-400 select-none flex-shrink-0">
                          {initials}
                        </div>
                        <div className="min-w-0 flex flex-col">
                          <span className="text-xs font-medium text-slate-200 truncate">{peer.display_name}</span>
                          <span className="text-[10px] font-mono text-slate-500 truncate">{peer.id.slice(0, 16)}...</span>
                        </div>
                      </div>
                      <div className={`h-4 w-4 rounded border flex items-center justify-center transition ${
                        isSelected 
                          ? 'bg-emerald-500 border-emerald-500 text-slate-950' 
                          : 'border-slate-700 hover:border-slate-500'
                      }`}>
                        {isSelected && <Check className="h-3 w-3 stroke-[3]" />}
                      </div>
                    </div>
                  )
                })
              )}
            </div>
          </div>

          {error && <p className="text-xs text-red-400 bg-red-500/10 border border-red-500/20 rounded px-2.5 py-1.5">{error}</p>}

          <DialogFooter className="pt-2">
            <Button
              type="button"
              variant="ghost"
              onClick={onClose}
              className="text-slate-400 hover:text-slate-200 hover:bg-slate-800"
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={inviting || selectedPeers.length === 0}
              className="bg-emerald-500 hover:bg-emerald-400 text-slate-900 font-semibold"
            >
              {inviting ? 'Adding...' : 'Invite to Group'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
