import { useCallback, useEffect, useState } from 'react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import { service } from '../../../../wailsjs/go/models'

interface InvitesScreenProps {
  activeGroupId: string | null
}

export default function InvitesScreen({ activeGroupId }: InvitesScreenProps) {
  const [pending, setPending] = useState<service.PendingInviteInfo[]>([])
  const [joinCode, setJoinCode] = useState('')
  const [invitePeerId, setInvitePeerId] = useState('')
  const [inviteJoinCode, setInviteJoinCode] = useState('')
  const [inviteCodePeerId, setInviteCodePeerId] = useState('')
  const [invitingWithCode, setInvitingWithCode] = useState(false)
  const [busyId, setBusyId] = useState<string | null>(null)
  const [error, setError] = useState('')

  const refresh = useCallback(async () => {
    try {
      setPending(await runtimeClient.listPendingInvites())
    } catch (err) {
      setError(String(err))
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const handleGenerateJoinCode = async () => {
    const result = await runtimeClient.generateJoinCode()
    setJoinCode(result.code_hex || '')
  }

  const handleInvitePeer = async () => {
    if (!activeGroupId || !invitePeerId.trim()) return
    try {
      await runtimeClient.invitePeerToGroup(activeGroupId, invitePeerId.trim())
      setInvitePeerId('')
    } catch (err) {
      setError(String(err))
    }
  }

  const handleAddMemberWithCode = async () => {
    if (!activeGroupId || !inviteCodePeerId.trim() || !inviteJoinCode.trim()) return
    setInvitingWithCode(true)
    setError('')
    try {
      await runtimeClient.addMemberToGroup(activeGroupId, inviteCodePeerId.trim(), inviteJoinCode.trim())
      setInviteJoinCode('')
      setInviteCodePeerId('')
    } catch (err) {
      setError(String(err))
    } finally {
      setInvitingWithCode(false)
    }
  }

  const handleAccept = async (id: string) => {
    setBusyId(id)
    try {
      await runtimeClient.acceptInvite(id)
      await refresh()
    } finally {
      setBusyId(null)
    }
  }

  const handleReject = async (id: string) => {
    setBusyId(id)
    try {
      await runtimeClient.rejectInvite(id)
      await refresh()
    } finally {
      setBusyId(null)
    }
  }

  return (
    <div className="space-y-6 p-6 max-w-xl text-slate-200">
      <h3 className="text-lg font-bold text-slate-100 tracking-tight">Quản lý lời mời & Kết nối</h3>
      
      {error && (
        <div className="rounded-lg bg-red-500/10 border border-red-500/20 p-3 text-xs text-red-400">
          {error}
        </div>
      )}

      {/* 1. Generate Join Code (Alice generates this for Bob) */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3">
        <div>
          <h4 className="text-sm font-semibold text-slate-100">Mã kết nối của bạn</h4>
          <p className="text-xs text-slate-400 mt-1 leading-relaxed">
            Tạo mã kết nối (KeyPackage) và gửi cho bạn bè để họ có thể thêm bạn vào các kênh thảo luận riêng tư.
          </p>
        </div>
        <button 
          className="px-3 py-1.5 bg-slate-800 text-slate-200 hover:bg-slate-700 text-xs font-semibold rounded-lg transition" 
          onClick={handleGenerateJoinCode}
        >
          Tạo mã kết nối
        </button>
        {joinCode && (
          <div className="mt-2 rounded-lg bg-slate-950 p-3 border border-slate-800">
            <p className="text-[10px] font-semibold text-slate-500 tracking-wider uppercase mb-1">Hãy sao chép mã này:</p>
            <p className="break-all text-xs font-mono text-emerald-400 selection:bg-emerald-500/20">{joinCode}</p>
          </div>
        )}
      </div>

      {/* 2. Invite Peer by PeerID (Fast-path using Kademlia) */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3">
        <div>
          <h4 className="text-sm font-semibold text-slate-100">Mời thành viên (Qua PeerID)</h4>
          <p className="text-xs text-slate-400 mt-1 leading-relaxed">
            Thêm trực tiếp một người dùng đang trực tuyến bằng mã định danh PeerID của họ.
          </p>
        </div>
        <div className="flex gap-2">
          <input
            className="flex-1 rounded-lg border border-slate-800 bg-slate-950 px-3 py-2 text-xs text-slate-100 placeholder:text-slate-600 outline-none focus:border-emerald-500/40 transition"
            value={invitePeerId}
            onChange={(e) => setInvitePeerId(e.target.value)}
            placeholder="Nhập PeerID của thành viên..."
            disabled={!activeGroupId}
          />
          <button 
            className="px-4 py-2 bg-emerald-500 text-slate-950 hover:bg-emerald-400 disabled:opacity-50 text-xs font-semibold rounded-lg transition shrink-0" 
            onClick={handleInvitePeer} 
            disabled={!activeGroupId || !invitePeerId.trim()}
          >
            Mời
          </button>
        </div>
        {!activeGroupId && (
          <p className="text-[11px] text-yellow-500/70 italic">Chọn một nhóm trước khi gửi lời mời.</p>
        )}
      </div>

      {/* 3. Add Member with manual Join Code */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3">
        <div>
          <h4 className="text-sm font-semibold text-slate-100">Thêm thành viên thủ công (Qua Mã kết nối)</h4>
          <p className="text-xs text-slate-400 mt-1 leading-relaxed">
            Nếu PeerID không thể tra cứu trên mạng, dán Mã kết nối và PeerID của thành viên để thực hiện gán ghép bảo mật.
          </p>
        </div>
        <div className="space-y-2">
          <input
            className="w-full rounded-lg border border-slate-800 bg-slate-950 px-3 py-2 text-xs text-slate-100 placeholder:text-slate-600 outline-none focus:border-emerald-500/40 transition"
            value={inviteCodePeerId}
            onChange={(e) => setInviteCodePeerId(e.target.value)}
            placeholder="Nhập PeerID của thành viên..."
            disabled={!activeGroupId}
          />
          <textarea
            className="w-full rounded-lg border border-slate-800 bg-slate-950 px-3 py-2 text-xs text-slate-100 placeholder:text-slate-600 outline-none focus:border-emerald-500/40 transition resize-none"
            value={inviteJoinCode}
            onChange={(e) => setInviteJoinCode(e.target.value)}
            placeholder="Dán mã kết nối (Mã dài ngoằng)..."
            rows={2}
            disabled={!activeGroupId}
          />
          <div className="flex justify-end">
            <button 
              className="px-4 py-2 bg-slate-800 text-slate-200 hover:bg-slate-700 disabled:opacity-50 text-xs font-semibold rounded-lg transition" 
              onClick={handleAddMemberWithCode} 
              disabled={invitingWithCode || !activeGroupId || !inviteJoinCode.trim() || !inviteCodePeerId.trim()}
            >
              {invitingWithCode ? 'Đang thêm...' : 'Xác nhận Thêm'}
            </button>
          </div>
        </div>
      </div>

      {/* 4. Pending Invites List */}
      <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3">
        <div className="flex items-center justify-between">
          <h4 className="text-sm font-semibold text-slate-100">Lời mời đang chờ xử lý</h4>
          <button 
            className="text-xs text-slate-400 hover:text-emerald-400 transition font-medium" 
            onClick={refresh}
          >
            Làm mới
          </button>
        </div>
        
        <div className="space-y-2">
          {pending.length === 0 ? (
            <p className="text-xs text-slate-500 italic py-2">Không có lời mời nào đang chờ.</p>
          ) : (
            pending.map((invite) => (
              <div key={invite.id} className="rounded-lg bg-slate-950/60 border border-slate-800/80 p-3 flex flex-col sm:flex-row justify-between sm:items-center gap-3">
                <div>
                  <p className="font-semibold text-slate-100 text-xs">{invite.group_name || invite.group_id}</p>
                  <p className="text-[10px] text-slate-400 mt-0.5">Người mời: {invite.inviter_peer || 'Ẩn danh'}</p>
                </div>
                <div className="flex gap-2">
                  <button
                    className="px-3 py-1 bg-emerald-500 hover:bg-emerald-400 disabled:opacity-50 text-slate-950 text-xs font-semibold rounded-lg transition"
                    disabled={busyId === invite.id}
                    onClick={() => void handleAccept(invite.id)}
                  >
                    Đồng ý
                  </button>
                  <button
                    className="px-3 py-1 bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-slate-300 text-xs font-semibold rounded-lg transition"
                    disabled={busyId === invite.id}
                    onClick={() => void handleReject(invite.id)}
                  >
                    Từ chối
                  </button>
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
