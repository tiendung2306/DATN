import { ChatMessage } from '../../stores/useChatStore'
import { useContactStore } from '../../stores/useContactStore'
import { formatMessageTime, parseMessageContent, shortPeerId } from '../../lib/chatModel'
import { Button } from '../ui/button'
import { ReactNode } from 'react'
import FileAttachmentCard from './FileAttachmentCard'

interface MessageListProps {
  messages: ChatMessage[]
  loading: boolean
  activeGroupId: string | null
  renderMentionedBody: (body: string) => ReactNode
  onRetry: (messageId: string) => void
  onRemoveFailed: (messageId: string) => void
  onDownloadFile?: (messageId: string) => void
  onOpenDownloadedFile?: (messageId: string) => void
  fileTransferStateByMessage?: Record<string, 'idle' | 'downloading' | 'completed' | 'failed'>
  fileLocalPathByMessage?: Record<string, string>
  fileActionDisabled?: boolean
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
  onOpenDownloadedFile,
  fileTransferStateByMessage = {},
  fileLocalPathByMessage = {},
  fileActionDisabled = false,
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
          <div
            key={message.id}
            className={`flex ${message.isMine ? 'justify-end' : 'justify-start'} ${startsGroup ? 'mt-3' : 'mt-1'}`}
          >
            <div
              className={`min-w-0 max-w-[78%] ${message.isMine ? 'items-end' : 'items-start'} flex flex-col ${
                startsGroup ? 'gap-1.5' : 'gap-1'
              }`}
            >
              {startsGroup ? (
                <div className="flex items-center gap-2 text-[11px] text-slate-400">
                  {!message.isMine ? <div className="h-7 w-7 rounded-full bg-slate-700" /> : null}
                  <span>{message.isMine ? 'Bạn' : getDisplayName(message.sender)}</span>
                  <span>{formatMessageTime(message.timestamp)}</span>
                </div>
              ) : null}
              <div
                className={`rounded-2xl px-3 py-2 text-sm shadow-sm ${
                  message.isMine
                    ? 'border border-emerald-500/30 bg-teal-900/40 text-slate-100'
                    : 'bg-slate-800 text-slate-100'
                }`}
              >
                <p className="whitespace-pre-wrap [overflow-wrap:anywhere]">
                  {file ? renderMentionedBody(parsed.body || `Đã chia sẻ tệp: ${file.name}`) : renderMentionedBody(message.content)}
                </p>
                {file ? (
                  <FileAttachmentCard
                    file={file}
                    isMine={message.isMine}
                    state={fileTransferStateByMessage[message.id] ?? 'idle'}
                    localPath={fileLocalPathByMessage[message.id]}
                    onDownload={onDownloadFile ? () => onDownloadFile(message.id) : undefined}
                    onOpenFile={onOpenDownloadedFile ? () => onOpenDownloadedFile(message.id) : undefined}
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
        )
      })}
    </div>
  )
}
