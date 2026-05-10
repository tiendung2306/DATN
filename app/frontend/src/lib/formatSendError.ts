/**
 * Maps backend / Wails errors to user-facing toast copy (Vietnamese).
 */
export function formatOutboundSendError(err: unknown): { title: string; description: string; variant: 'default' | 'destructive' } {
  const raw = err instanceof Error ? err.message : String(err)
  const rawLower = raw.toLowerCase()

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

  if (raw.includes('ERR_FILE_TOO_LARGE')) {
    return {
      title: 'File quá lớn',
      description: 'File vượt giới hạn cho phiên bản hiện tại (tối đa 100MB).',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_USER_CANCELLED')) {
    return {
      title: 'Đã hủy thao tác',
      description: 'Bạn đã hủy chọn file.',
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
      title: 'Không thể kết nối tới thiết bị gửi',
      description:
        'Thiết bị gửi hiện đang ngoại tuyến hoặc chưa sẵn sàng kết nối. Vui lòng thử lại khi thiết bị gửi online.',
      variant: 'destructive',
    }
  }

  if (rawLower.includes('err_file_not_downloaded')) {
    return {
      title: 'Chưa có bản tải cục bộ',
      description: 'Tệp này chưa được tải trên thiết bị này. Vui lòng bấm Tải xuống trước.',
      variant: 'destructive',
    }
  }

  if (rawLower.includes('err_file_missing_local')) {
    return {
      title: 'Không tìm thấy tệp trên máy',
      description: 'Tệp đã bị di chuyển hoặc xóa khỏi vị trí đã lưu. Vui lòng tải lại.',
      variant: 'destructive',
    }
  }

  if (rawLower.includes('err_file_open_failed')) {
    return {
      title: 'Không mở được tệp',
      description: 'Hệ điều hành không thể mở tệp này. Hãy kiểm tra quyền truy cập hoặc phần mềm mặc định.',
      variant: 'destructive',
    }
  }

  if (rawLower.includes('err_too_many_attachments')) {
    return {
      title: 'Vuot gioi han tep dinh kem',
      description: 'Moi bai viet chi ho tro toi da 10 tep. Hay xoa bot tep truoc khi them moi.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_INVITE_REQUEST_STATE_CONFLICT')) {
    return {
      title: 'Yêu cầu đã thay đổi',
      description: 'Yêu cầu này đã được xử lý hoặc đang được xử lý. Vui lòng tải lại danh sách.',
      variant: 'default',
    }
  }

  if (raw.includes('ERR_INVITE_PROCESSING_TIMEOUT')) {
    return {
      title: 'Xử lý lời mời bị quá hạn',
      description: 'Yêu cầu đã bị timeout khi xử lý. Người tạo có thể thử lại.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_INVITE_MAX_ATTEMPTS_EXCEEDED')) {
    return {
      title: 'Đã vượt quá số lần thử',
      description: 'Yêu cầu này đã thử quá nhiều lần và không thể tự xử lý tiếp.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_CREATOR_UNREACHABLE')) {
    return {
      title: 'Không liên hệ được người tạo nhóm',
      description: 'Không thể liên hệ người tạo nhóm, thử lại sau.',
      variant: 'destructive',
    }
  }

  if (raw.includes('ERR_GROUP_CREATOR_UNKNOWN')) {
    return {
      title: 'Chưa đồng bộ nhóm',
      description: 'Chưa đồng bộ đủ thông tin nhóm.',
      variant: 'destructive',
    }
  }

  // File transfer: sender reached but metadata/chunks not ready.
  if (rawLower.includes('read manifest') || rawLower.includes('file-transfer: not available')) {
    return {
      title: 'Tệp chưa sẵn sàng để tải',
      description: 'Tệp có thể đã bị gỡ khỏi máy gửi hoặc chưa được chuẩn bị xong. Vui lòng thử lại sau.',
      variant: 'destructive',
    }
  }

  // File transfer: MLS state mismatch.
  if (rawLower.includes('epoch mismatch')) {
    return {
      title: 'Nhóm chưa đồng bộ',
      description: 'Thiết bị của bạn chưa đồng bộ epoch mới nhất của nhóm. Hãy đợi đồng bộ rồi tải lại.',
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
      title: 'Không thể xác minh tệp',
      description:
        'Dữ liệu tệp không toàn vẹn hoặc không giải mã được. Vui lòng tải lại; nếu vẫn lỗi, yêu cầu người gửi gửi lại tệp.',
      variant: 'destructive',
    }
  }

  return {
    title: 'Không gửi được',
    description: raw || 'Đã xảy ra lỗi không xác định.',
    variant: 'destructive',
  }
}
