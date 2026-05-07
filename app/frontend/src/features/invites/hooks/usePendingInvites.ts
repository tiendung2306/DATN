import { useCallback, useEffect, useRef, useState } from 'react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import { service } from '../../../../wailsjs/go/models'

export function usePendingInvites() {
  const [pending, setPending] = useState<service.PendingInviteInfo[]>([])
  const [busyId, setBusyId] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const refreshSeqRef = useRef(0)

  const refresh = useCallback(async () => {
    const seq = ++refreshSeqRef.current
    try {
      const list = await runtimeClient.listPendingInvites()
      if (seq !== refreshSeqRef.current) return
      setPending(list ?? [])
      setError(null)
    } catch (err) {
      if (seq !== refreshSeqRef.current) return
      setError(String(err))
    }
  }, [])

  const accept = useCallback(
    async (id: string) => {
      setBusyId(id)
      try {
        await runtimeClient.acceptInvite(id)
        await refresh()
      } finally {
        setBusyId(null)
      }
    },
    [refresh],
  )

  const reject = useCallback(
    async (id: string) => {
      setBusyId(id)
      try {
        await runtimeClient.rejectInvite(id)
        await refresh()
      } finally {
        setBusyId(null)
      }
    },
    [refresh],
  )

  useEffect(() => {
    void refresh()
  }, [refresh])

  return { pending, refresh, accept, reject, busyId, error }
}
