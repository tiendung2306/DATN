import { ChatMessage } from '../../stores/useChatStore'
import { useContactStore } from '../../stores/useContactStore'
import { formatMessageTime, parseMessageContent } from '../../lib/chatModel'
import { cn } from '../../lib/utils'
import { Button } from '../ui/button'
import { ReactNode } from 'react'
import FileAttachmentCard from './FileAttachmentCard'
import ChatListAvatar from './ChatListAvatar'

interface MessageListProps {
  messages: ChatMessage[]
  loading: boolean
  activeGroupId: string | null
  /** Avatars for message senders keyed by peer id (from group roster / profile sync). */
  peerAvatarByPeerId?: Record<string, string>
  renderMentionedBody: (body: string) => ReactNode
  onRetry: (messageId: string) => void
  onRemoveFailed: (messageId: string) => void
  onDownloadFile?: (messageId: string) => void
  onOpenFile?: (messageId: string) => void
  onOpenFileLocation?: (messageId: string) => void
  fileTransferStateByMessage?: Record<string, 'idle' | 'downloading' | 'completed' | 'failed'>
  fileLocalPathByMessage?: Record<string, string>
  fileActionDisabled?: boolean
  oldestUnreadMessageId?: string | null
  highlightedMessageId?: string | null
}

function statusLabel(status: ChatMessage['status']): string {
  switch (status) {
    case 'sending':
      return 'Đang gửi'
    case 'failed':
      return 'Lỗi'
    default:
      return 'Đã gửi'
  }
}

export default function MessageList({
  messages,
  loading,
  activeGroupId,
  renderMentionedBody,
  onRetry,
  onRemoveFailed,
  onDownloadFile,
  onOpenFile,
  onOpenFileLocation,
  fileTransferStateByMessage = {},
  fileLocalPathByMessage = {},
  fileActionDisabled = false,
  peerAvatarByPeerId = {},
  oldestUnreadMessageId = null,
  highlightedMessageId = null,
}: MessageListProps) {
  const getDisplayName = useContactStore((s) => s.getDisplayName)

  if (!activeGroupId) {
    return (
      <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-slate-700 text-sm text-slate-400">
        Chọn hội thoại để bắt đầu nhắn tin.
      </div>
    )
  }

  if (loading) {
    return (
      <div className="space-y-2 rounded-xl border border-slate-700/70 p-3">
        <div className="h-14 animate-pulse rounded-md bg-slate-800" />
        <div className="h-14 animate-pulse rounded-md bg-slate-800" />
        <div className="h-14 animate-pulse rounded-md bg-slate-800" />
      </div>
    )
  }

  if (messages.length === 0) {
    return (
      <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-slate-700 text-sm text-slate-400">
        Chưa có tin nhắn. Hãy bắt đầu cuộc trò chuyện.
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {messages.map((message, index) => {
        const previous = messages[index - 1]
        const startsGroup =
          index === 0 ||
          previous.sender !== message.sender ||
          previous.kind !== message.kind ||
          previous.isMine !== message.isMine

        const parsed = parseMessageContent(message.content)
        const file = parsed.file ?? parsed.attachments?.[0]

        if (message.kind === 'system') {
          return (
            <div
              key={message.id}
              className="mx-auto max-w-[70%] rounded-md border border-slate-700 bg-slate-800/60 px-3 py-2 text-center text-xs text-slate-400"
            >
              {message.content}
            </div>
          )
        }

        return (
          <div key={message.id} data-message-id={message.id}>
            {oldestUnreadMessageId === message.id ? (
              <div className="mb-3 flex items-center gap-3">
                <div className="h-px flex-1 bg-amber-500/30" />
                <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-2.5 py-1 text-[10px] font-semibold uppercase tracking-[0.18em] text-amber-200">
                  Chưa đọc từ đây
                </span>
                <div className="h-px flex-1 bg-amber-500/30" />
              </div>
            ) : null}
            <div
              className={`flex ${message.isMine ? 'justify-end' : 'justify-start'} ${startsGroup ? 'mt-3' : 'mt-1'}`}
            >
            <div
              className={`min-w-0 max-w-[78%] ${message.isMine ? 'items-end' : 'items-start'} flex flex-col ${
                startsGroup ? 'gap-1.5' : 'gap-1'
              }`}
            >
              {startsGroup ? (
                <div className="flex items-center gap-2 text-[11px] text-slate-400">
                  {!message.isMine ? (
                    <ChatListAvatar
                      variant="dm"
                      displayName={getDisplayName(message.sender)}
                      imageUrl={peerAvatarByPeerId[message.sender]}
                      size="sm"
                      className="shrink-0"
                    />
                  ) : null}
                  <span>{message.isMine ? 'Bạn' : getDisplayName(message.sender)}</span>
                  <span>{formatMessageTime(message.timestamp)}</span>
                </div>
              ) : null}
              <div
                className={cn(
                  'rounded-2xl px-3 py-2 text-sm shadow-sm transition-all',
                  highlightedMessageId === message.id && 'ring-2 ring-amber-400/60 ring-offset-2 ring-offset-slate-950',
                  message.isMine
                    ? 'border border-emerald-500/30 bg-teal-900/40 text-slate-100'
                    : 'bg-slate-800 text-slate-100',
                )}
              >
                <p className="whitespace-pre-wrap [overflow-wrap:anywhere]">
                  {file ? renderMentionedBody(parsed.body || 'Đã chia sẻ tệp') : renderMentionedBody(message.content)}
                </p>
                {file ? (
                  <FileAttachmentCard
                    file={file}
                    isMine={message.isMine}
                    state={fileTransferStateByMessage[message.id] ?? 'idle'}
                    localPath={fileLocalPathByMessage[message.id]}
                    onDownload={onDownloadFile ? () => onDownloadFile(message.id) : undefined}
                    onOpenFile={onOpenFile ? () => onOpenFile(message.id) : undefined}
                    onOpenFileLocation={onOpenFileLocation ? () => onOpenFileLocation(message.id) : undefined}
                    disabled={fileActionDisabled}
                  />
                ) : null}
                <div className="mt-1 flex items-center justify-end gap-1 text-[11px] text-slate-400">
                  <span>{formatMessageTime(message.timestamp)}</span>
                  <span className={message.status === 'failed' ? 'text-red-300' : ''}>
                    {message.isMine && message.status !== 'failed' ? '✓✓' : null}
                  </span>
                  <span className={message.status === 'failed' ? 'text-red-300' : ''}>
                  {statusLabel(message.status)}
                  </span>
                </div>
              </div>
              {message.status === 'failed' && (
                <div className="mt-2 flex gap-2">
                  <Button size="xs" variant="secondary" onClick={() => onRetry(message.id)}>
                    Gửi lại
                  </Button>
                  <Button size="xs" variant="ghost" onClick={() => onRemoveFailed(message.id)}>
                    Xóa
                  </Button>
                </div>
              )}
            </div>
          </div>
          </div>
        )
      })}
    </div>
  )
}
