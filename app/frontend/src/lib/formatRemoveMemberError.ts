/**
 * Maps backend error codes from RemoveMemberFromGroup / LeaveGroup into
 * English, user-friendly toast copy. Backend wire format is
 * "<ERR_CODE>: <english detail>" — see app/service/membership.go.
 */

export type FormattedActionError = {
  title: string
  description: string
  variant: 'default' | 'destructive'
}

const REMOVE_MEMBER_ERROR_MAP: Record<string, FormattedActionError> = {
  ERR_GROUP_NOT_FOUND: {
    title: 'Group not found',
    description: 'The group might have been deleted or you are no longer a member. Please refresh your group list.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_FORBIDDEN: {
    title: 'Permission denied',
    description: 'Only the creator or an admin can remove regular members.',
    variant: 'destructive',
  },
  ERR_REMOVE_CREATOR_FORBIDDEN: {
    title: 'Creator cannot be removed',
    description: 'The group creator is the immutable owner in this version.',
    variant: 'destructive',
  },
  ERR_REMOVE_ADMIN_FORBIDDEN: {
    title: 'Cannot remove admin',
    description: 'Admins cannot remove other admins. The creator must revoke admin first.',
    variant: 'destructive',
  },
  ERR_REMOVE_ADMIN_REQUIRES_DEMOTE: {
    title: 'Revoke admin first',
    description: 'Creator must revoke admin role before removing this member.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_SELF: {
    title: 'Cannot remove self',
    description: 'You cannot remove yourself from the member list. Use "Leave Group" instead.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_PEER_NOT_VERIFIED: {
    title: 'Cannot remove member yet',
    description:
      'The identity key for this member has not been verified. Wait for the connection to be fully established and try again.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_ACCESS_REVOKED: {
    title: 'Access revoked',
    description: 'Your access to this group has been revoked. You can no longer perform this action.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_INVALID_PEER_ID: {
    title: 'Invalid Peer ID',
    description: 'The peer ID format is incorrect. Please refresh and try again.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_CRYPTO_FAILURE: {
    title: 'Removal failed',
    description:
      'MLS cryptographic operation failed. Check your network and try again; if the problem persists, check diagnostics.',
    variant: 'destructive',
  },
  ERR_RUNTIME_NOT_INITIALIZED: {
    title: 'System not ready',
    description: 'The runtime is still initializing. Please wait a few seconds and try again.',
    variant: 'destructive',
  },
}

const SESSION_REPLACED_CODE = 'session has been replaced'

function pickErrorCode(raw: string): string | null {
  const match = raw.match(/ERR_[A-Z0-9_]+/)
  return match ? match[0] : null
}

export function formatRemoveMemberError(err: unknown): FormattedActionError {
  const raw = err instanceof Error ? err.message : String(err ?? '')

  if (raw.toLowerCase().includes(SESSION_REPLACED_CODE)) {
    return {
      title: 'Session Replaced',
      description: 'Your account has been logged in on another device. Please log in again to continue.',
      variant: 'destructive',
    }
  }

  const code = pickErrorCode(raw)
  if (code && REMOVE_MEMBER_ERROR_MAP[code]) {
    return REMOVE_MEMBER_ERROR_MAP[code]
  }

  return {
    title: 'Failed to remove member',
    description: raw.trim() || 'An unknown error occurred while removing the member.',
    variant: 'destructive',
  }
}

export function formatLeaveGroupError(err: unknown): FormattedActionError {
  const raw = err instanceof Error ? err.message : String(err ?? '')

  if (raw.toLowerCase().includes(SESSION_REPLACED_CODE)) {
    return {
      title: 'Session Replaced',
      description: 'Your account has been logged in on another device. Please log in again to continue.',
      variant: 'destructive',
    }
  }

  const code = pickErrorCode(raw)
  if (code === 'ERR_GROUP_NOT_FOUND') {
    return {
      title: 'Group not found',
      description: 'The group might have been deleted or you already left. Please refresh your list.',
      variant: 'destructive',
    }
  }
  if (code === 'ERR_CREATOR_CANNOT_LEAVE') {
    return {
      title: 'Creator cannot leave',
      description: 'The group creator is the immutable owner in this version.',
      variant: 'destructive',
    }
  }

  return {
    title: 'Failed to leave group',
    description: raw.trim() || 'An unknown error occurred while leaving the group.',
    variant: 'destructive',
  }
}
