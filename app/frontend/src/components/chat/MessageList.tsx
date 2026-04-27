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
  const feed = messages.length > 0 ? messages : mockMessages(activeGroupId)

  if (!activeGroupId) {
    return (
      <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-slate-700 text-sm text-slate-400">
        Select a group to start messaging.
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

  if (feed.length === 0) {
    return (
      <div className="flex h-full items-center justify-center rounded-xl border border-dashed border-slate-700 text-sm text-slate-400">
        No messages yet. Start the conversation.
      </div>
    )
  }

  return (
    <div className="space-y-3">
      {feed.map((message, index) => {
        const previous = feed[index - 1]
        const startsGroup =
          index === 0 ||
          previous.sender !== message.sender ||
          previous.kind !== message.kind ||
          previous.isMine !== message.isMine

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
              className={`max-w-[78%] ${message.isMine ? 'items-end' : 'items-start'} flex flex-col ${
                startsGroup ? 'gap-1.5' : 'gap-1'
              }`}
            >
              {startsGroup ? (
                <div className="flex items-center gap-2 text-[11px] text-slate-400">
                  {!message.isMine ? <div className="h-7 w-7 rounded-full bg-slate-700" /> : null}
                  <span>{message.isMine ? 'You' : shortPeerId(message.sender)}</span>
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
                <p className="whitespace-pre-wrap break-words">{message.content}</p>
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

function mockMessages(groupId: string | null): ChatMessage[] {
  if (!groupId) return []
  return [
    {
      id: 'mock-1',
      groupId,
      sender: '12D3KooW-Alice',
      content: 'Morning team, security review starts at 10:00.',
      timestamp: Date.now() - 1000 * 60 * 20,
      isMine: false,
      status: 'published',
      kind: 'user',
    },
    {
      id: 'mock-2',
      groupId,
      sender: '12D3KooW-Alice',
      content: 'Please keep this channel for release blockers only.',
      timestamp: Date.now() - 1000 * 60 * 19,
      isMine: false,
      status: 'published',
      kind: 'user',
    },
    {
      id: 'mock-3',
      groupId,
      sender: 'local-user',
      content: 'Acknowledged. We are validating offline sync scenarios now.',
      timestamp: Date.now() - 1000 * 60 * 12,
      isMine: true,
      status: 'published',
      kind: 'user',
    },
  ]
}
