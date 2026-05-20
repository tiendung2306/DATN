/**
 * Maps backend / Wails errors to user-facing toast copy (English).
 */
export function formatOutboundSendError(err: unknown): { title: string; description: string; variant: 'default' | 'destructive' } {
  const raw = err instanceof Error ? err.message : String(err)
  const rawLower = raw.toLowerCase()

  if (raw.includes('TEXT_TOO_LONG')) {
    return {
      title: 'Message too long',
      description:
        'The message exceeds the allowed character limit. In a future update, you will be able to send long content as encrypted files (similar to Discord).',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_MESSAGE_EMPTY')) {
    return {
      title: 'Cannot send',
      description: 'The message body cannot be empty or only contain whitespace.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_CHANNEL_PAYLOAD_INVALID')) {
    if (raw.includes('title exceeds')) {
      return {
        title: 'Title too long',
        description: 'Please shorten the post title.',
        variant: 'destructive',
      }
    }
    if (raw.includes('body exceeds')) {
      return {
        title: 'Content too long',
        description:
          'The post or comment content exceeds the limit. Long content will be sent as encrypted files in a future update.',
        variant: 'destructive',
      }
    }
    return {
      title: 'Invalid content',
      description: 'The post or comment format is incorrect. Please check and try again.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_FILE_TOO_LARGE')) {
    return {
      title: 'File too large',
      description: 'File exceeds the current version limit (max 100MB).',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_USER_CANCELLED')) {
    return {
      title: 'Action cancelled',
      description: 'File selection was cancelled.',
      variant: 'default',
    }
  }

  // File transfer: sender offline / cannot dial peer.
  if (
    rawLower.includes('open file-transfer stream') ||
    rawLower.includes('all dials failed') ||
    rawLower.includes('dial backoff') ||
    rawLower.includes('failed to dial')
  ) {
    return {
      title: 'Cannot connect to sender',
      description:
        'The sender device is currently offline or unavailable. Please try again when the sender is online.',
      variant: 'destructive',
    }
  }

  if (rawLower.includes('err_file_not_downloaded')) {
    return {
      title: 'Not downloaded',
      description: 'This file has not been downloaded to this device. Please download it first.',
      variant: 'destructive',
    }
  }

  if (rawLower.includes('err_file_missing_local')) {
    return {
      title: 'File not found',
      description: 'The file has been moved or deleted from its saved location. Please download it again.',
      variant: 'destructive',
    }
  }

  if (rawLower.includes('err_file_open_failed')) {
    return {
      title: 'Cannot open file',
      description: 'The operating system could not open this file. Check permissions or default software.',
      variant: 'destructive',
    }
  }

  if (rawLower.includes('err_too_many_attachments')) {
    return {
      title: 'Too many attachments',
      description: 'Each post supports a maximum of 10 files. Please remove some before adding more.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_INVITE_REQUEST_STATE_CONFLICT')) {
    return {
      title: 'Request changed',
      description: 'This request has already been processed or is being handled. Please refresh the list.',
      variant: 'default',
    }
  }

  if (raw.includes('ERR_INVITE_PROCESSING_TIMEOUT')) {
    return {
      title: 'Invite timeout',
      description: 'The request timed out during processing. The creator can try again.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_INVITE_MAX_ATTEMPTS_EXCEEDED')) {
    return {
      title: 'Max attempts exceeded',
      description: 'This request has been attempted too many times and cannot proceed automatically.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_CREATOR_UNREACHABLE')) {
    return {
      title: 'Creator unreachable',
      description: 'Cannot contact the group creator. Please try again later.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_GROUP_CREATOR_UNKNOWN')) {
    return {
      title: 'Group not synchronized',
      description: 'Insufficient group information synced yet.',
      variant: 'destructive',
    }
  }

  // File transfer: sender reached but metadata/chunks not ready.
  if (rawLower.includes('read manifest') || rawLower.includes('file-transfer: not available')) {
    return {
      title: 'File not ready',
      description: 'The file might have been removed or is not yet prepared by the sender. Please try again later.',
      variant: 'destructive',
    }
  }

  // File transfer: MLS state mismatch.
  if (rawLower.includes('epoch mismatch')) {
    return {
      title: 'Group out of sync',
      description: 'Your device has not synced the latest group epoch. Wait for sync then try again.',
      variant: 'destructive',
    }
  }

  // File transfer: integrity / decrypt failure.
  if (
    rawLower.includes('decrypt chunk') ||
    rawLower.includes('plaintext sha256 mismatch') ||
    rawLower.includes('size mismatch')
  ) {
    return {
      title: 'File verification failed',
      description:
        'Data integrity check failed or decryption was unsuccessful. Please redownload; if the error persists, ask the sender to resend.',
      variant: 'destructive',
    }
  }

  return {
    title: 'Send failed',
    description: raw || 'An unknown error occurred.',
    variant: 'destructive',
  }
}
