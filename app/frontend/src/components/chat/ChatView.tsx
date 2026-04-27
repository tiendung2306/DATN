import { ChatMessage } from '../../stores/useChatStore'
import MessageComposer from './MessageComposer'
import MessageList from './MessageList'
import { Info, Lock, Phone, Video } from 'lucide-react'

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
    <section className="flex min-w-0 flex-1 flex-col bg-[#0f172a]">
      <div className="border-b border-slate-800 px-5 py-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-base font-semibold text-slate-100">{activeGroupId ? `# ${activeGroupId}` : 'Select chat'}</p>
            <p className="mt-1 flex items-center gap-1 text-xs text-slate-400">
              <Lock className="h-3 w-3" />
              E2EE by OpenMLS
            </p>
          </div>
          <div className="flex items-center gap-1">
            <button
              type="button"
              className="rounded-lg p-2 text-slate-400 transition hover:bg-slate-800 hover:text-slate-200"
            >
              <Phone className="h-4 w-4" />
            </button>
            <button
              type="button"
              className="rounded-lg p-2 text-slate-400 transition hover:bg-slate-800 hover:text-slate-200"
            >
              <Video className="h-4 w-4" />
            </button>
            <button
              type="button"
              className="rounded-lg p-2 text-slate-400 transition hover:bg-slate-800 hover:text-slate-200"
            >
              <Info className="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        <MessageList
          messages={messages}
          loading={loadingMessages}
          activeGroupId={activeGroupId}
          onRetry={onRetry}
          onRemoveFailed={onRemoveFailed}
        />
      </div>

      <div className="border-t border-slate-800 px-5 py-4">
        <MessageComposer
          value={composingMessage}
          disabled={sending || !activeGroupId}
          onChange={onComposingChange}
          onSend={onSend}
        />
      </div>
    </section>
  )
}
