import { useState, useEffect } from 'react'
import { Button } from '../../../components/ui/button'
import { Input } from '../../../components/ui/input'
import { Label } from '../../../components/ui/label'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '../../../components/ui/dialog'
import { Hash, MessageSquare, Plus, Search, Check, Users } from 'lucide-react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'

interface CreateGroupModalProps {
  isOpen: boolean
  onClose: () => void
  onCreate: (name: string, type: 'channel' | 'group' | 'dm', members: string[]) => Promise<void>
  creating: boolean
  initialType?: 'channel' | 'group' | 'dm'
}

interface PeerInfo {
  id: string
  display_name: string
  verified?: boolean
}

export default function CreateGroupModal({
  isOpen,
  onClose,
  onCreate,
  creating,
  initialType = 'group',
}: CreateGroupModalProps) {
  const [groupName, setGroupName] = useState('')
  const [groupType, setGroupType] = useState<'channel' | 'group' | 'dm'>('group')
  const [searchQuery, setSearchQuery] = useState('')
  const [knownPeers, setKnownPeers] = useState<PeerInfo[]>([])
  const [members, setMembers] = useState<string[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    if (isOpen) {
      setGroupType(initialType)
      setGroupName('')
      setSearchQuery('')
      setMembers([])
      setError('')
      const loadPeers = async () => {
        try {
          const peers = await runtimeClient.getKnownPeers()
          setKnownPeers(peers.map((p: any) => ({
            id: p.id,
            display_name: p.display_name || p.id.slice(0, 10),
            verified: p.verified
          })))
        } catch (e) {
          console.error("Failed to load known peers", e)
        }
      }
      loadPeers()
    }
  }, [isOpen, initialType])

  useEffect(() => {
    if (!isOpen) return
    setMembers([])
    setError('')
  }, [groupType, isOpen])

  const toggleMember = (peerId: string) => {
    if (groupType === 'dm') {
      setMembers([peerId])
      return
    }
    if (members.includes(peerId)) {
      setMembers(members.filter(id => id !== peerId))
    } else {
      setMembers([...members, peerId])
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    setError('')
    const trimmedName = groupName.trim()
    if (groupType !== 'dm' && !trimmedName) {
      setError('Tên nhóm không được để trống.')
      return
    }
    if (groupType === 'dm' && members.length !== 1) {
      setError('Hãy chọn đúng 1 người để bắt đầu nhắn tin trực tiếp.')
      return
    }

    try {
      const autoName = groupType === 'dm' ? members[0] : trimmedName
      await onCreate(autoName, groupType, members)
      setGroupName('')
      setMembers([])
      onClose()
    } catch (err) {
      setError(String(err))
    }
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
            <Plus className="h-5 w-5 text-emerald-400" />
            Tạo hội thoại mới
          </DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="space-y-5 mt-2">
          {/* Group Type Selector */}
          <div className="space-y-2">
            <Label className="text-xs text-slate-400 font-semibold uppercase tracking-wider">
              Loại hội thoại
            </Label>
            <div className="grid grid-cols-3 gap-2 p-1 bg-slate-950 rounded-lg border border-slate-800">
              <button
                type="button"
                onClick={() => setGroupType('channel')}
                className={`flex items-center justify-center gap-2 py-2 px-3 text-xs font-medium rounded-md transition duration-150 ${
                  groupType === 'channel'
                    ? 'bg-slate-800 text-emerald-400 border border-slate-700 shadow-sm'
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                <Hash className="h-4 w-4" />
                Kênh
              </button>
              <button
                type="button"
                onClick={() => setGroupType('group')}
                className={`flex items-center justify-center gap-2 py-2 px-3 text-xs font-medium rounded-md transition duration-150 ${
                  groupType === 'group'
                    ? 'bg-slate-800 text-emerald-400 border border-slate-700 shadow-sm'
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                <Users className="h-4 w-4" />
                Nhóm chat
              </button>
              <button
                type="button"
                onClick={() => setGroupType('dm')}
                className={`flex items-center justify-center gap-2 py-2 px-3 text-xs font-medium rounded-md transition duration-150 ${
                  groupType === 'dm'
                    ? 'bg-slate-800 text-sky-400 border border-slate-700 shadow-sm'
                    : 'text-slate-400 hover:text-slate-200'
                }`}
              >
                <MessageSquare className="h-4 w-4" />
                DM 1-1
              </button>
            </div>
          </div>

          {/* Group Name Input */}
          {groupType !== 'dm' ? (
            <div className="space-y-2">
              <Label htmlFor="group-name" className="text-xs text-slate-400 font-semibold uppercase tracking-wider">
                Tên {groupType === 'channel' ? 'Kênh' : 'Nhóm'}
              </Label>
              <Input
                id="group-name"
                value={groupName}
                onChange={(e) => setGroupName(e.target.value)}
                placeholder={groupType === 'channel' ? 'e.g. thong-bao' : 'e.g. Nhom Du an'}
                className="bg-slate-950 border-slate-800 text-slate-100 placeholder:text-slate-600 focus-visible:ring-emerald-500"
              />
            </div>
          ) : null}

          {/* Members section with search and list */}
          <div className="space-y-2">
            <Label className="text-xs text-slate-400 font-semibold uppercase tracking-wider flex justify-between items-center">
              <span>{groupType === 'dm' ? 'Chọn 1 người' : `Mời thành viên (${members.length})`}</span>
            </Label>
            
            <div className="relative">
              <Search className="absolute left-3 top-2.5 h-4 w-4 text-slate-500" />
              <Input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Tim theo ten hoac Peer ID..."
                className="pl-9 bg-slate-950 border-slate-800 text-slate-100 placeholder:text-slate-600 focus-visible:ring-emerald-500 text-xs"
              />
            </div>

            <div className="mt-2 border border-slate-800 bg-slate-950 rounded-lg max-h-48 overflow-y-auto divide-y divide-slate-800/50">
              {filteredPeers.length === 0 ? (
                <p className="text-xs text-slate-600 italic text-center py-4">Khong tim thay nguoi dung nao</p>
              ) : (
                filteredPeers.map((peer) => {
                  const isSelected = members.includes(peer.id)
                  const initials = getInitials(peer.display_name)
                  return (
                    <div 
                      key={peer.id}
                      onClick={() => toggleMember(peer.id)}
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
              Hủy
            </Button>
            <Button
              type="submit"
              disabled={creating || (groupType !== 'dm' && !groupName.trim()) || (groupType === 'dm' && members.length !== 1)}
              className="bg-emerald-500 hover:bg-emerald-400 text-slate-900 font-semibold"
            >
              {creating ? 'Đang tạo...' : 'Khởi tạo'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
