import { ChatMessage } from '../../stores/useChatStore'
import MessageComposer from './MessageComposer'
import MessageList from './MessageList'
import PostView from './PostView'
import { useContactStore } from '../../stores/useContactStore'
import { Info, Lock, Loader2 } from 'lucide-react'
import { service } from '../../../wailsjs/go/models'
import { useMentions } from '../../features/chat/hooks/useMentions'
import { useEffect, useRef, useState } from 'react'
import { useMessageLimitsStore } from '../../stores/useMessageLimitsStore'

interface ChatViewProps {
  activeGroupId: string | null
  localPeerId: string
  groups: any[]
  messages: ChatMessage[]
  loadingMessages: boolean
  composingMessage: string
  sending: boolean
  onComposingChange: (value: string) => void
  onSend: () => void
  onRetry: (messageId: string) => void
  onRemoveFailed: (messageId: string) => void
  onToggleDetails: () => void
  detailsOpen: boolean
  activeGroupMembers: service.MemberInfo[]
  onLoadMore?: () => Promise<void>
  onLoadComments?: (postId: string) => Promise<void>
  onLoadMoreComments?: (postId: string) => Promise<void>
}

export default function ChatView({
  activeGroupId,
  localPeerId,
  groups,
  messages,
  loadingMessages,
  composingMessage,
  sending,
  onComposingChange,
  onSend,
  onRetry,
  onRemoveFailed,
  onToggleDetails,
  detailsOpen,
  activeGroupMembers,
  onLoadMore,
  onLoadComments,
  onLoadMoreComments,
}: ChatViewProps) {
  const getDisplayName = useContactStore((s) => s.getDisplayName)
  const dmMaxRunes = useMessageLimitsStore((s) => s.dmMaxRunes)
  const activeGroup = groups.find((g) => g.group_id === activeGroupId)
  const isDM = activeGroup?.group_type === 'dm'
  const { mentionCandidates, renderMentionedBody } = useMentions({
    groupMembers: activeGroupMembers,
    localPeerId,
  })

  const scrollRef = useRef<HTMLDivElement>(null)
  const [loadingMore, setLoadingMore] = useState(false)
  const [isAtBottom, setIsAtBottom] = useState(true)

  // Scroll to bottom helper
  const scrollToBottom = (behavior: ScrollBehavior = 'auto') => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }

  // Handle scroll events for infinite loading
  const handleScroll = async () => {
    const el = scrollRef.current
    if (!el) return

    // Check if at bottom (with some threshold)
    const atBottom = el.scrollHeight - el.scrollTop <= el.clientHeight + 100
    setIsAtBottom(atBottom)

    // Check if at top for loading more
    if (el.scrollTop === 0 && onLoadMore && !loadingMore && messages.length > 0) {
      setLoadingMore(true)
      const oldScrollHeight = el.scrollHeight
      await onLoadMore()
      // Use a small timeout to let React render the prepended messages
      setTimeout(() => {
        if (el) {
          const newScrollHeight = el.scrollHeight
          el.scrollTop = newScrollHeight - oldScrollHeight
        }
        setLoadingMore(false)
      }, 0)
    }
  }

  // Scroll to bottom on initial load of a group
  useEffect(() => {
    if (activeGroupId && !loadingMessages) {
      scrollToBottom('auto')
    }
  }, [activeGroupId, loadingMessages])

  // Scroll to bottom on new messages if already at bottom
  useEffect(() => {
    if (isAtBottom) {
      scrollToBottom('smooth')
    }
  }, [messages.length, isAtBottom])

  return (
    <section className="flex min-w-0 flex-1 flex-col bg-[#0f172a]">
      <div className="border-b border-slate-800 px-5 py-4">
        <div className="flex items-center justify-between gap-4">
          <div>
            <p className="text-base font-semibold text-slate-100">
              {activeGroupId ? (isDM ? getDisplayName(activeGroupId) : `# ${activeGroupId}`) : 'Chọn hội thoại'}
            </p>
            <p className="mt-1 flex items-center gap-1 text-xs text-slate-400">
              <Lock className="h-3 w-3" />
              E2EE by OpenMLS
            </p>
          </div>
          <div className="flex items-center gap-1">
            <button
              type="button"
              className={`rounded-lg p-2 transition ${
                detailsOpen
                  ? 'bg-slate-800 text-emerald-300'
                  : 'text-slate-400 hover:bg-slate-800 hover:text-slate-200'
              }`}
              onClick={onToggleDetails}
            >
              <Info className="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>

      {isDM ? (
        <>
          <div 
            ref={scrollRef}
            onScroll={handleScroll}
            className="min-h-0 flex-1 overflow-y-auto px-5 py-4"
          >
            {loadingMore && (
              <div className="flex justify-center py-2">
                <Loader2 className="h-4 w-4 animate-spin text-slate-500" />
              </div>
            )}
            <MessageList
              messages={messages}
              loading={loadingMessages}
              activeGroupId={activeGroupId}
              renderMentionedBody={renderMentionedBody}
              onRetry={onRetry}
              onRemoveFailed={onRemoveFailed}
            />
          </div>

          <div className="border-t border-slate-800 px-5 py-4">
            <MessageComposer
              value={composingMessage}
              disabled={sending || !activeGroupId}
              mentionCandidates={mentionCandidates}
              maxRunes={isDM ? dmMaxRunes : undefined}
              onChange={onComposingChange}
              onSend={() => {
                onSend()
                // Force scroll to bottom after sending
                setTimeout(() => scrollToBottom('smooth'), 100)
              }}
            />
          </div>
        </>
      ) : (
        <PostView
          activeGroupId={activeGroupId}
          messages={messages}
          loadingMessages={loadingMessages}
          mentionCandidates={mentionCandidates}
          renderMentionedBody={renderMentionedBody}
          onLoadMore={onLoadMore}
          onLoadComments={onLoadComments}
          onLoadMoreComments={onLoadMoreComments}
        />
      )}
    </section>
  )
}
