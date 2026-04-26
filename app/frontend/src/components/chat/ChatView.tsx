import { ChatMessage } from '../../stores/useChatStore'
import MessageComposer from './MessageComposer'
import MessageList from './MessageList'

interface ChatViewProps {
  activeGroupId: string | null
  messages: ChatMessage[]
  loadingMessages: boolean
  composingMessage: string
  sending: boolean
  onComposingChange: (value: string) => void
  onSend: () => void
  onRetry: (messageId: string) => void
  onRemoveFailed: (messageId: string) => void
}

export default function ChatView({
  activeGroupId,
  messages,
  loadingMessages,
  composingMessage,
  sending,
  onComposingChange,
  onSend,
  onRetry,
  onRemoveFailed,
}: ChatViewProps) {
  return (
    <section className="flex h-full min-h-[70vh] flex-col gap-3">
      <div className="rounded-xl border border-border/80 bg-[#0a0f16] p-3">
        <div className="flex items-center justify-between">
          <div>
            <p className="text-lg font-semibold">{activeGroupId || 'No group selected'}</p>
            <p className="text-xs text-muted-foreground">End-to-end encrypted by OpenMLS</p>
          </div>
          <span className="rounded-full border border-emerald-500/30 bg-emerald-500/10 px-2 py-1 text-[11px] text-emerald-300">
            Secure
          </span>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <MessageList
          messages={messages}
          loading={loadingMessages}
          activeGroupId={activeGroupId}
          onRetry={onRetry}
          onRemoveFailed={onRemoveFailed}
        />
      </div>

      <MessageComposer
        value={composingMessage}
        disabled={sending || !activeGroupId}
        onChange={onComposingChange}
        onSend={onSend}
      />
    </section>
  )
}
