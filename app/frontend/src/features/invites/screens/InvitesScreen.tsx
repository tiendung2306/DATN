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
    await runtimeClient.invitePeerToGroup(activeGroupId, invitePeerId.trim())
    setInvitePeerId('')
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
    <div className="space-y-4 p-4 text-sm text-slate-200">
      <h3 className="font-semibold">Invites & Join</h3>
      <div className="rounded-lg border border-slate-700 p-3">
        <p className="mb-2 text-xs text-slate-400">Generate join code</p>
        <button className="btn-secondary" onClick={handleGenerateJoinCode}>
          Generate
        </button>
        {joinCode ? <p className="mt-2 break-all text-xs text-slate-300">{joinCode}</p> : null}
      </div>
      <div className="rounded-lg border border-slate-700 p-3">
        <p className="mb-2 text-xs text-slate-400">Invite peer into active group</p>
        <input
          className="w-full rounded border border-slate-700 bg-slate-900 px-2 py-1 text-xs"
          value={invitePeerId}
          onChange={(e) => setInvitePeerId(e.target.value)}
          placeholder="PeerID"
          disabled={!activeGroupId}
        />
        <button className="btn-secondary mt-2" onClick={handleInvitePeer} disabled={!activeGroupId}>
          Invite
        </button>
      </div>
      <div className="rounded-lg border border-slate-700 p-3">
        <div className="mb-2 flex items-center justify-between">
          <p className="text-xs text-slate-400">Pending invites</p>
          <button className="btn-ghost text-xs" onClick={refresh}>
            Refresh
          </button>
        </div>
        {error ? <p className="text-xs text-red-300">{error}</p> : null}
        <div className="space-y-2">
          {pending.length === 0 ? <p className="text-xs text-slate-500">No pending invites.</p> : null}
          {pending.map((invite) => (
            <div key={invite.id} className="rounded border border-slate-700 p-2">
              <p className="font-medium">{invite.group_name || invite.group_id}</p>
              <p className="text-xs text-slate-400">{invite.inviter_peer || 'unknown inviter'}</p>
              <div className="mt-2 flex gap-2">
                <button
                  className="btn-secondary text-xs"
                  disabled={busyId === invite.id}
                  onClick={() => void handleAccept(invite.id)}
                >
                  Accept
                </button>
                <button
                  className="btn-ghost text-xs"
                  disabled={busyId === invite.id}
                  onClick={() => void handleReject(invite.id)}
                >
                  Reject
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>
    </div>
  )
}
