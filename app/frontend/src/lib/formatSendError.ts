/**
 * Maps backend / Wails errors to user-facing toast copy (Vietnamese).
 */
export function formatOutboundSendError(err: unknown): { title: string; description: string; variant: 'default' | 'destructive' } {
  const raw = err instanceof Error ? err.message : String(err)

  if (raw.includes('TEXT_TOO_LONG')) {
    return {
      title: 'Nội dung quá dài',
      description:
        'Tin nhắn vượt quá giới hạn cho phép. Trong bản cập nhật sau, bạn sẽ có thể gửi nội dung dài dưới dạng file đã mã hóa (giống Discord).',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_MESSAGE_EMPTY')) {
    return {
      title: 'Không thể gửi',
      description: 'Nội dung không được để trống hoặc chỉ gồm khoảng trắng.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_CHANNEL_PAYLOAD_INVALID')) {
    if (raw.includes('title exceeds')) {
      return {
        title: 'Tiêu đề quá dài',
        description: 'Vui lòng rút ngắn tiêu đề bài viết.',
        variant: 'destructive',
      }
    }
    if (raw.includes('body exceeds')) {
      return {
        title: 'Nội dung quá dài',
        description:
          'Nội dung bài viết hoặc bình luận vượt quá giới hạn. Nội dung rất dài sẽ được gửi dưới dạng file đã mã hóa trong bản cập nhật sau.',
        variant: 'destructive',
      }
    }
    return {
      title: 'Nội dung không hợp lệ',
      description: 'Định dạng bài viết hoặc bình luận không đúng. Vui lòng kiểm tra lại.',
      variant: 'destructive',
    }
  }

  return {
    title: 'Không gửi được',
    description: raw || 'Đã xảy ra lỗi không xác định.',
    variant: 'destructive',
  }
}
