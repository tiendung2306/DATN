import { service } from '../../../../wailsjs/go/models'

interface PendingInvitesPanelProps {
  pending: service.PendingInviteInfo[]
  busyId: string | null
  onAccept: (id: string) => void
  onReject: (id: string) => void
  onRefresh: () => void
}

export default function PendingInvitesPanel({
  pending,
  busyId,
  onAccept,
  onReject,
  onRefresh,
}: PendingInvitesPanelProps) {
  return (
    <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3">
      <div className="flex items-center justify-between">
        <h4 className="text-sm font-semibold text-slate-100">Lời mời đang chờ xử lý</h4>
        <button
          type="button"
          className="text-xs text-slate-400 hover:text-emerald-400 transition font-medium"
          onClick={onRefresh}
        >
          Làm mới
        </button>
      </div>

      <div className="space-y-2 max-h-[min(60vh,24rem)] overflow-y-auto">
        {pending.length === 0 ? (
          <p className="text-xs text-slate-500 italic py-2">Không có lời mời nào đang chờ.</p>
        ) : (
          pending.map((invite) => (
            <div
              key={invite.id}
              className="rounded-lg bg-slate-950/60 border border-slate-800/80 p-3 flex flex-col sm:flex-row justify-between sm:items-center gap-3"
            >
              <div>
                <p className="font-semibold text-slate-100 text-xs">{invite.group_name || invite.group_id}</p>
                <p className="text-[10px] text-slate-400 mt-0.5">Người mời: {invite.inviter_peer || 'Ẩn danh'}</p>
              </div>
              <div className="flex gap-2 shrink-0">
                <button
                  type="button"
                  className="px-3 py-1 bg-emerald-500 hover:bg-emerald-400 disabled:opacity-50 text-slate-950 text-xs font-semibold rounded-lg transition"
                  disabled={busyId === invite.id}
                  onClick={() => void onAccept(invite.id)}
                >
                  Đồng ý
                </button>
                <button
                  type="button"
                  className="px-3 py-1 bg-slate-800 hover:bg-slate-700 disabled:opacity-50 text-slate-300 text-xs font-semibold rounded-lg transition"
                  disabled={busyId === invite.id}
                  onClick={() => void onReject(invite.id)}
                >
                  Từ chối
                </button>
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
