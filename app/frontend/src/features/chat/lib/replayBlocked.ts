export interface ReplayBlockedPayloadLike {
  reason?: string
  state?: string
  user_visible?: boolean
}

export function isSilentReplayBlocked(payload: ReplayBlockedPayloadLike | null | undefined): boolean {
  if (payload?.user_visible === false) {
    return true
  }
  const reason = String(payload?.reason ?? payload?.state ?? 'unknown')
  return reason === 'stale_epoch_requires_recovery_snapshot'
}
