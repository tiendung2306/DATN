import { ChatMessage } from '../../stores/useChatStore'
import MessageComposer from './MessageComposer'
import MessageList from './MessageList'
import PostView from './PostView'
import { useContactStore } from '../../stores/useContactStore'
import { ArrowUp, Info, Lock, Loader2 } from 'lucide-react'
import { service } from '../../../wailsjs/go/models'
import { useMentions } from '../../features/chat/hooks/useMentions'
import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useMessageLimitsStore } from '../../stores/useMessageLimitsStore'
import ChatListAvatar from './ChatListAvatar'
import { ConversationKind } from '../../lib/chatModel'
import { Button } from '../ui/button'
import { computeUnreadAnchorUpdate, shouldPerformInitialScroll } from '../../features/chat/lib/timelineState'

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

  const peerAvatarByPeerId = useMemo(() => {
    const m: Record<string, string> = {}
    for (const mem of activeGroupMembers) {
      const u = String(mem.avatar_data_url ?? '').trim()
      if (u) m[mem.peer_id] = u
    }
    return m
  }, [activeGroupMembers])

  const scrollRef = useRef<HTMLDivElement>(null)
  const [loadingMore, setLoadingMore] = useState(false)
  const [isAtBottom, setIsAtBottom] = useState(true)
  const [unreadAnchorMessageId, setUnreadAnchorMessageId] = useState<string | null>(null)
  const [unreadJumpCount, setUnreadJumpCount] = useState(0)
  const [highlightedMessageId, setHighlightedMessageId] = useState<string | null>(null)
  const previousMessagesRef = useRef<ChatMessage[]>([])
  const previousGroupRef = useRef<string | null>(null)
  const previousScrollHeightRef = useRef(0)
  const suppressScrollCompensationRef = useRef(false)
  const pendingInitialScrollGroupRef = useRef<string | null>(null)
  const highlightTimerRef = useRef<number | null>(null)

  const scrollToBottom = (behavior: ScrollBehavior = 'auto') => {
    if (scrollRef.current) {
      scrollRef.current.scrollTo({ top: scrollRef.current.scrollHeight, behavior })
    }
  }

  const focusUnreadAnchor = (behavior: ScrollBehavior = 'smooth') => {
    if (!unreadAnchorMessageId) return
    const el = scrollRef.current?.querySelector<HTMLElement>(`[data-message-id="${CSS.escape(unreadAnchorMessageId)}"]`)
    if (!el) return
    el.scrollIntoView({ behavior, block: 'center' })
    setHighlightedMessageId(unreadAnchorMessageId)
    setUnreadAnchorMessageId(null)
    setUnreadJumpCount(0)
    if (highlightTimerRef.current != null) {
      window.clearTimeout(highlightTimerRef.current)
    }
    highlightTimerRef.current = window.setTimeout(() => {
      setHighlightedMessageId((current) => (current === unreadAnchorMessageId ? null : current))
      highlightTimerRef.current = null
    }, 2200)
  }

  const handleScroll = async () => {
    const el = scrollRef.current
    if (!el) return

    const atBottom = el.scrollHeight - el.scrollTop <= el.clientHeight + 100
    setIsAtBottom(atBottom)
    if (atBottom && unreadAnchorMessageId == null && unreadJumpCount === 0 && highlightedMessageId) {
      setHighlightedMessageId(null)
    }

    if (el.scrollTop === 0 && onLoadMore && !loadingMore && messages.length > 0) {
      setLoadingMore(true)
      suppressScrollCompensationRef.current = true
      const oldScrollHeight = el.scrollHeight
      await onLoadMore()
      setTimeout(() => {
        if (el) {
          const newScrollHeight = el.scrollHeight
          el.scrollTop = newScrollHeight - oldScrollHeight
        }
        setLoadingMore(false)
        suppressScrollCompensationRef.current = false
      }, 0)
    }
  }

  useEffect(() => {
    return () => {
      if (highlightTimerRef.current != null) {
        window.clearTimeout(highlightTimerRef.current)
      }
    }
  }, [])

  useEffect(() => {
    if (previousGroupRef.current !== activeGroupId) {
      pendingInitialScrollGroupRef.current = activeGroupId
      previousGroupRef.current = activeGroupId
      previousMessagesRef.current = messages
      previousScrollHeightRef.current = scrollRef.current?.scrollHeight ?? 0
      setUnreadAnchorMessageId(null)
      setUnreadJumpCount(0)
      setHighlightedMessageId(null)
    }
  }, [activeGroupId, messages])

  useLayoutEffect(() => {
    const el = scrollRef.current
    if (!el) return

    const previousMessages = previousMessagesRef.current
    const previousGroup = previousGroupRef.current
    const currentGroup = activeGroupId
    const previousIds = new Set(previousMessages.map((message) => message.id))
    const newMessages = messages.filter((message) => !previousIds.has(message.id))

    if (
      previousGroup === currentGroup &&
      newMessages.length > 0 &&
      !isAtBottom &&
      !suppressScrollCompensationRef.current
    ) {
      const oldHeight = previousScrollHeightRef.current
      const newHeight = el.scrollHeight
      const delta = newHeight - oldHeight
      if (delta > 0) {
        el.scrollTop += delta
      }
    }

    if (
      previousGroup === currentGroup &&
      newMessages.length > 0 &&
      !suppressScrollCompensationRef.current
    ) {
      const unreadUpdate = computeUnreadAnchorUpdate({
        previousMessages,
        nextMessages: messages,
        current: {
          anchorId: unreadAnchorMessageId,
          count: unreadJumpCount,
        },
        isAtBottom,
        suppressTracking: suppressScrollCompensationRef.current,
      })
      if (unreadUpdate.anchorId !== unreadAnchorMessageId || unreadUpdate.count !== unreadJumpCount) {
        setUnreadAnchorMessageId(unreadUpdate.anchorId)
        setUnreadJumpCount(unreadUpdate.count)
      }
    }

    previousScrollHeightRef.current = el.scrollHeight
    previousMessagesRef.current = messages
    previousGroupRef.current = currentGroup
  }, [activeGroupId, isAtBottom, messages, unreadAnchorMessageId])

  useEffect(() => {
    if (shouldPerformInitialScroll(activeGroupId, pendingInitialScrollGroupRef.current, loadingMessages)) {
      scrollToBottom('auto')
      pendingInitialScrollGroupRef.current = null
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

  const dmAvatarUrl =
    isDM && activeGroup ? String((activeGroup as { counterparty_avatar_data_url?: string }).counterparty_avatar_data_url || '').trim() : ''

  const groupAvatarUrl =
    activeKind === 'group' && activeGroup
      ? String((activeGroup as { group_avatar_data_url?: string }).group_avatar_data_url || '').trim()
      : ''

  const headerAvatarVariant = isDM ? 'dm' : activeKind === 'group' ? 'group' : 'channel'
  const headerAvatarImage = isDM ? dmAvatarUrl : activeKind === 'group' ? groupAvatarUrl : undefined

  return (
    <section className="flex min-w-0 flex-1 flex-col bg-[#0f172a]">
      <div className="border-b border-slate-800 px-5 py-4">
        <div className="flex items-center justify-between gap-4">
          <div className="flex min-w-0 items-start gap-3">
            {activeGroupId ? (
              <ChatListAvatar
                variant={headerAvatarVariant}
                displayName={titleLabel}
                imageUrl={headerAvatarImage}
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
          <div className="relative min-h-0 flex-1">
            <div
              ref={scrollRef}
              onScroll={() => void handleScroll()}
              className="min-h-0 h-full overflow-y-auto px-5 py-4"
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
              peerAvatarByPeerId={peerAvatarByPeerId}
              renderMentionedBody={renderMentionedBody}
              onRetry={onRetry}
              onRemoveFailed={onRemoveFailed}
              onDownloadFile={onDownloadFile}
              onOpenDownloadedFile={onOpenDownloadedFile}
              fileTransferStateByMessage={fileTransferStateByMessage}
              fileLocalPathByMessage={fileLocalPathByMessage}
              fileActionDisabled={attachingFile || sending}
              oldestUnreadMessageId={unreadAnchorMessageId}
              highlightedMessageId={highlightedMessageId}
            />
            </div>
            {unreadAnchorMessageId ? (
              <div className="pointer-events-none absolute inset-x-0 bottom-4 flex justify-center px-4">
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  className="pointer-events-auto rounded-full border border-amber-400/30 bg-slate-900/95 px-4 text-amber-100 shadow-lg shadow-black/30 backdrop-blur"
                  onClick={() => focusUnreadAnchor('smooth')}
                >
                  <ArrowUp className="h-3.5 w-3.5" />
                  {unreadJumpCount > 1
                    ? `${unreadJumpCount} tin chưa đọc`
                    : '1 tin chưa đọc'}
                  <span className="text-amber-300/80">Nhảy tới</span>
                </Button>
              </div>
            ) : null}
          </div>

          <div className="border-t border-slate-800 px-5 py-4">
            <MessageComposer
              value={composingMessage}
              disabled={sending || attachingFile || !activeGroupId}
              inputDisabled={!activeGroupId}
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
