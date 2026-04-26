import { ChatMessage } from '../../stores/useChatStore'
import { formatMessageTime, shortPeerId } from '../../lib/chatModel'
import { Button } from '../ui/button'

interface MessageListProps {
  messages: ChatMessage[]
  loading: boolean
  activeGroupId: string | null
  onRetry: (messageId: string) => void
  onRemoveFailed: (messageId: string) => void
}

function statusLabel(status: ChatMessage['status']): string {
  switch (status) {
    case 'sending':
      return 'Sending'
    case 'failed':
      return 'Failed'
    default:
      return 'Published'
  }
}

export default function MessageList({
  messages,
  loading,
  activeGroupId,
  onRetry,
  onRemoveFailed,
}: MessageListProps) {
  if (!activeGroupId) {
    return (
      <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-border text-sm text-muted-foreground">
        Select a group to start messaging.
      </div>
    )
  }

  if (loading) {
    return (
      <div className="space-y-2 rounded-xl border border-border p-3">
        <div className="h-14 animate-pulse rounded-md bg-muted" />
        <div className="h-14 animate-pulse rounded-md bg-muted" />
        <div className="h-14 animate-pulse rounded-md bg-muted" />
      </div>
    )
  }

  if (messages.length === 0) {
    return (
      <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-border text-sm text-muted-foreground">
        No messages yet. Start the conversation.
      </div>
    )
  }

  return (
    <div className="space-y-3 rounded-xl border border-border/80 bg-[#090d15] p-4">
      {messages.map((message) => {
        if (message.kind === 'system') {
          return (
            <div
              key={message.id}
              className="mx-auto max-w-[70%] rounded-md border border-border bg-muted/40 px-3 py-2 text-center text-xs text-muted-foreground"
            >
              {message.content}
            </div>
          )
        }

        return (
          <div key={message.id} className={`flex ${message.isMine ? 'justify-end' : 'justify-start'}`}>
            <div
              className={`max-w-[78%] rounded-lg px-3 py-2 text-sm shadow-sm ${
                message.isMine
                  ? 'border border-emerald-700/60 bg-emerald-600/14 text-emerald-100'
                  : 'border border-border bg-[#101622] text-foreground'
              }`}
            >
              <div className="mb-1 flex items-center gap-2 text-[11px] opacity-80">
                <span>{message.isMine ? 'You' : shortPeerId(message.sender)}</span>
                <span>{formatMessageTime(message.timestamp)}</span>
                <span className={message.status === 'failed' ? 'text-red-300' : ''}>
                  {statusLabel(message.status)}
                </span>
              </div>
              <p className="whitespace-pre-wrap break-words">{message.content}</p>
              {message.status === 'failed' && (
                <div className="mt-2 flex gap-2">
                  <Button size="xs" variant="secondary" onClick={() => onRetry(message.id)}>
                    Retry
                  </Button>
                  <Button size="xs" variant="ghost" onClick={() => onRemoveFailed(message.id)}>
                    Remove
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
