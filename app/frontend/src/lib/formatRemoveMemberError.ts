/**
 * Maps backend error codes from RemoveMemberFromGroup / LeaveGroup into
 * Vietnamese, user-friendly toast copy. Backend wire format is
 * "<ERR_CODE>: <english detail>" — see app/service/membership.go.
 */

export type FormattedActionError = {
  title: string
  description: string
  variant: 'default' | 'destructive'
}

const REMOVE_MEMBER_ERROR_MAP: Record<string, FormattedActionError> = {
  ERR_GROUP_NOT_FOUND: {
    title: 'Không tìm thấy nhóm',
    description: 'Nhóm có thể đã bị xóa hoặc bạn không còn là thành viên. Hãy làm mới danh sách nhóm.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_FORBIDDEN: {
    title: 'Bạn không có quyền',
    description: 'Chỉ người tạo nhóm mới có thể loại thành viên khỏi nhóm.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_SELF: {
    title: 'Không thể tự xóa',
    description: 'Bạn không thể tự loại bản thân. Sử dụng "Rời nhóm" thay thế.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_PEER_NOT_VERIFIED: {
    title: 'Chưa thể loại thành viên này',
    description:
      'Hệ thống chưa xác thực được khóa danh tính của thành viên. Hãy chờ kết nối được thiết lập đầy đủ rồi thử lại.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_ACCESS_REVOKED: {
    title: 'Bạn đã bị loại khỏi nhóm',
    description: 'Quyền truy cập của bạn vào nhóm đã bị thu hồi. Bạn không thể thực hiện hành động này nữa.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_INVALID_PEER_ID: {
    title: 'Peer ID không hợp lệ',
    description: 'Định dạng peer ID không đúng. Hãy thử làm mới và chọn lại thành viên.',
    variant: 'destructive',
  },
  ERR_REMOVE_MEMBER_CRYPTO_FAILURE: {
    title: 'Loại thành viên thất bại',
    description:
      'Thao tác mã hóa MLS không hoàn tất. Hãy kiểm tra kết nối mạng và thử lại; nếu vẫn lỗi, xem nhật ký diagnostics.',
    variant: 'destructive',
  },
  ERR_RUNTIME_NOT_INITIALIZED: {
    title: 'Hệ thống chưa sẵn sàng',
    description: 'Runtime chưa khởi tạo xong. Vui lòng đợi vài giây rồi thử lại.',
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
      title: 'Phiên đã bị thay thế',
      description: 'Tài khoản đã đăng nhập trên thiết bị khác. Hãy đăng nhập lại để tiếp tục.',
      variant: 'destructive',
    }
  }

  const code = pickErrorCode(raw)
  if (code && REMOVE_MEMBER_ERROR_MAP[code]) {
    return REMOVE_MEMBER_ERROR_MAP[code]
  }

  return {
    title: 'Không loại được thành viên',
    description: raw.trim() || 'Đã xảy ra lỗi không xác định khi loại thành viên.',
    variant: 'destructive',
  }
}

export function formatLeaveGroupError(err: unknown): FormattedActionError {
  const raw = err instanceof Error ? err.message : String(err ?? '')

  if (raw.toLowerCase().includes(SESSION_REPLACED_CODE)) {
    return {
      title: 'Phiên đã bị thay thế',
      description: 'Tài khoản đã đăng nhập trên thiết bị khác. Hãy đăng nhập lại để tiếp tục.',
      variant: 'destructive',
    }
  }

  const code = pickErrorCode(raw)
  if (code === 'ERR_GROUP_NOT_FOUND') {
    return {
      title: 'Không tìm thấy nhóm',
      description: 'Nhóm có thể đã bị xóa hoặc đã rời trước đó. Hãy làm mới danh sách.',
      variant: 'destructive',
    }
  }

  return {
    title: 'Không rời được nhóm',
    description: raw.trim() || 'Đã xảy ra lỗi không xác định khi rời nhóm.',
    variant: 'destructive',
  }
}
