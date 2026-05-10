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
import ChatListAvatar from './ChatListAvatar'
import { ConversationKind } from '../../lib/chatModel'

interface ChatViewProps {
  activeGroupId: string | null
  localPeerId: string
  groups: any[]
  messages: ChatMessage[]
  loadingMessages: boolean
  composingMessage: string
  sending: boolean
  attachingFile: boolean
  onComposingChange: (value: string) => void
  onSend: () => void
  onAttachFile: () => void
  onRetry: (messageId: string) => void
  onRemoveFailed: (messageId: string) => void
  onDownloadFile: (messageId: string) => void
  onOpenDownloadedFile: (messageId: string) => void
  fileTransferStateByMessage: Record<string, 'idle' | 'downloading' | 'completed' | 'failed'>
  fileLocalPathByMessage: Record<string, string>
  onToggleDetails: () => void
  detailsOpen: boolean
  activeGroupMembers: service.MemberInfo[]
  activeKind: ConversationKind
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
  attachingFile,
  onComposingChange,
  onSend,
  onAttachFile,
  onRetry,
  onRemoveFailed,
  onDownloadFile,
  onOpenDownloadedFile,
  fileTransferStateByMessage,
  fileLocalPathByMessage,
  onToggleDetails,
  detailsOpen,
  activeGroupMembers,
  activeKind,
  onLoadMore,
  onLoadComments,
  onLoadMoreComments,
}: ChatViewProps) {
  const getDisplayName = useContactStore((s) => s.getDisplayName)
  const dmMaxRunes = useMessageLimitsStore((s) => s.dmMaxRunes)
  const activeGroup = groups.find((g) => g.group_id === activeGroupId)
  const isDM = activeKind === 'dm'
  const isChannel = activeKind === 'channel'
  const { mentionCandidates, renderMentionedBody } = useMentions({
    groupMembers: activeGroupMembers,
    localPeerId,
  })

  const scrollRef = useRef<HTMLDivElement>(null)
  const [loadingMore, setLoadingMore] = useState(false)
  const [isAtBottom, setIsAtBottom] = useState(true)

  const scrollToBottom = (behavior: ScrollBehavior = 'auto') => {
    if (scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight
    }
  }

  const handleScroll = async () => {
    const el = scrollRef.current
    if (!el) return

    const atBottom = el.scrollHeight - el.scrollTop <= el.clientHeight + 100
    setIsAtBottom(atBottom)

    if (el.scrollTop === 0 && onLoadMore && !loadingMore && messages.length > 0) {
      setLoadingMore(true)
      const oldScrollHeight = el.scrollHeight
      await onLoadMore()
      setTimeout(() => {
        if (el) {
          const newScrollHeight = el.scrollHeight
          el.scrollTop = newScrollHeight - oldScrollHeight
        }
        setLoadingMore(false)
      }, 0)
    }
  }

  useEffect(() => {
    if (activeGroupId && !loadingMessages) {
      scrollToBottom('auto')
    }
  }, [activeGroupId, loadingMessages])

  useEffect(() => {
    if (isAtBottom) {
      scrollToBottom('smooth')
    }
  }, [messages.length, isAtBottom])

  const dmPeerId = activeGroupId ? String((activeGroup as any)?.counterparty_peer_id || '') : ''
  const backendTitle = activeGroupId ? String((activeGroup as any)?.conversation_title || '') : ''
  const isPlaceholderDmTitle =
    isDM && activeGroupId
      ? backendTitle === activeGroupId || /^dm-[0-9a-f]{8,}$/i.test(backendTitle)
      : false
  const titleLabel = activeGroupId
    ? isDM
      ? !isPlaceholderDmTitle && backendTitle
        ? backendTitle
        : getDisplayName(dmPeerId || activeGroupId)
      : backendTitle || `# ${activeGroupId}`
    : 'Chọn hội thoại'

  return (
    <section className="flex min-w-0 flex-1 flex-col bg-[#0f172a]">
      <div className="border-b border-slate-800 px-5 py-4">
        <div className="flex items-center justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            {activeGroupId ? (
              <ChatListAvatar
                variant={isDM ? 'dm' : 'channel'}
                displayName={isDM ? titleLabel : activeGroupId}
                size="md"
                className="mt-0.5"
              />
            ) : null}
            <div className="min-w-0">
              <p className="text-base font-semibold text-slate-100">{titleLabel}</p>
              <p className="mt-1 flex items-center gap-1 text-xs text-slate-400">
                <Lock className="h-3 w-3" />
                E2EE by OpenMLS
              </p>
            </div>
          </div>
          <div className="flex shrink-0 items-center gap-1">
            <button
              type="button"
              className={`rounded-lg p-2 transition ${
                detailsOpen
                  ? 'bg-slate-800 text-emerald-300'
                  : 'text-slate-400 hover:bg-slate-800 hover:text-slate-200'
              }`}
              onClick={onToggleDetails}
              aria-label="Chi tiết nhóm"
            >
              <Info className="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>

      {!isChannel ? (
        <>
          <div
            ref={scrollRef}
            onScroll={() => void handleScroll()}
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
              onDownloadFile={onDownloadFile}
              onOpenDownloadedFile={onOpenDownloadedFile}
              fileTransferStateByMessage={fileTransferStateByMessage}
              fileLocalPathByMessage={fileLocalPathByMessage}
              fileActionDisabled={attachingFile || sending}
            />
          </div>

          <div className="border-t border-slate-800 px-5 py-4">
            <MessageComposer
              value={composingMessage}
              disabled={sending || attachingFile || !activeGroupId}
              attachingFile={attachingFile}
              mentionCandidates={mentionCandidates}
              maxRunes={isDM ? dmMaxRunes : undefined}
              onChange={onComposingChange}
              onAttachFile={onAttachFile}
              onSend={() => {
                onSend()
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
