import { useState } from 'react'
import { service } from '../../../wailsjs/go/models'
import { shortPeerId } from '../../lib/chatModel'
import { useContactStore } from '../../stores/useContactStore'
import { Button } from '../ui/button'
import { LogOut, Shield, UserPlus, Users, X } from 'lucide-react'
import { runtimeClient } from '../../services/runtime/runtimeClient'
import AddMemberModal from '../../features/chat/components/AddMemberModal'
import { ConversationKind } from '../../lib/chatModel'

interface RoomPanelProps {
  activeGroupId: string | null
  activeKind: ConversationKind
  peers: service.MemberInfo[]
  onClose: () => void
  setActiveGroupId?: (id: string | null) => void
  refreshGroups?: () => Promise<void>
}

export default function RoomPanel({
  activeGroupId,
  activeKind,
  peers,
  onClose,
  setActiveGroupId,
  refreshGroups,
}: RoomPanelProps) {
  const getDisplayName = useContactStore((s) => s.getDisplayName)
  const [isAddModalOpen, setIsAddModalOpen] = useState(false)
  const [isLeaving, setIsLeaving] = useState(false)

  const handleLeaveGroup = async () => {
    if (!activeGroupId) return
    if (!confirm('Bạn có chắc chắn muốn rời nhóm này?')) return
    setIsLeaving(true)
    try {
      await runtimeClient.leaveGroup(activeGroupId)
      if (setActiveGroupId) setActiveGroupId(null)
      if (refreshGroups) await refreshGroups()
    } catch (e) {
      console.error('Failed to leave group', e)
      alert('Lỗi khi rời nhóm: ' + e)
    } finally {
      setIsLeaving(false)
    }
  }

  return (
    <aside className="flex w-80 shrink-0 flex-col border-l border-slate-800 bg-slate-950">
      <div className="mb-4 flex items-center justify-between border-b border-slate-800 px-4 py-4">
        <div>
          <p className="text-sm font-semibold text-slate-100">
            {activeKind === 'dm' ? 'Direct message details' : 'Group details'}
          </p>
          <p className="text-xs text-slate-400">{activeGroupId || 'No group selected'}</p>
        </div>
        <button
          type="button"
          aria-label="Đóng chi tiết nhóm"
          className="flex h-8 w-8 items-center justify-center rounded-md text-slate-400 hover:bg-slate-800 hover:text-slate-100"
          onClick={onClose}
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      <div className="px-4">
        <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
          <Users className="h-3.5 w-3.5" />
          <span>Members</span>
        </div>
        <div className="space-y-2 pb-4">
          {peers.length === 0 ? (
            <p className="text-xs text-slate-500">No members available.</p>
          ) : (
            peers.map((peer) => (
              <div
                key={peer.peer_id}
                className="flex items-center justify-between rounded-md border border-slate-800 bg-slate-900/60 px-2 py-2"
              >
                <div>
                  <p className="text-xs font-medium text-slate-200">{getDisplayName(peer.peer_id)}</p>
                  <p className="text-[11px] text-slate-500">{shortPeerId(peer.peer_id)}</p>
                </div>
                <span
                  className={`h-2 w-2 rounded-full ${
                    peer.is_online ? 'bg-emerald-400' : 'bg-slate-500'
                  }`}
                  title={peer.is_online ? 'online' : 'offline'}
                />
              </div>
            ))
          )}
        </div>
      </div>

      <div className="mt-auto space-y-2 border-t border-slate-800 p-4">
        <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
          <Shield className="h-3.5 w-3.5" />
          <span>{activeKind === 'dm' ? 'DM actions' : 'Group actions'}</span>
        </div>
        {activeKind !== 'dm' ? (
          <Button
            className="w-full bg-slate-800 hover:bg-slate-700 text-slate-100 border border-slate-700 gap-2"
            variant="secondary"
            disabled={!activeGroupId}
            onClick={() => setIsAddModalOpen(true)}
          >
            <UserPlus className="h-4 w-4" />
            Add Member
          </Button>
        ) : null}
        <Button
          className="w-full text-slate-400 hover:text-red-400 hover:bg-red-500/10 gap-2"
          variant="ghost"
          disabled={!activeGroupId || isLeaving}
          onClick={() => void handleLeaveGroup()}
        >
          <LogOut className="h-4 w-4" />
          {isLeaving ? 'Leaving...' : 'Leave Group'}
        </Button>
      </div>

      <AddMemberModal
        isOpen={isAddModalOpen}
        onClose={() => setIsAddModalOpen(false)}
        groupId={activeGroupId || ''}
        onSuccess={() => {
          if (refreshGroups) refreshGroups()
        }}
      />
    </aside>
  )
}
