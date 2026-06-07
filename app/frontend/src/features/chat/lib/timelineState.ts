import type { ChatMessage } from '../../../stores/useChatStore'

export function compareMessagesByTimeline(a: ChatMessage, b: ChatMessage): number {
  if (a.timestamp !== b.timestamp) return a.timestamp - b.timestamp
  return a.id.localeCompare(b.id)
}

export function reconcileTimelineMessages(messages: ChatMessage[]): ChatMessage[] {
  const next = [...messages]
  next.sort(compareMessagesByTimeline)
  const supersededIds = new Set(
    next
      .map((message) => message.supersedesMessageId)
      .filter((messageId): messageId is string => typeof messageId === 'string' && messageId.length > 0),
  )
  return next.filter((message) => !supersededIds.has(message.id))
}

export function mergeTimelineMessages(existing: ChatMessage[], incoming: ChatMessage[]): ChatMessage[] {
  const merged = [...existing]
  for (const message of incoming) {
    const existingIdx = merged.findIndex((candidate) => candidate.id === message.id)
    if (existingIdx >= 0) {
      merged[existingIdx] = { ...merged[existingIdx], ...message }
    } else {
      merged.push(message)
    }
  }
  return reconcileTimelineMessages(merged)
}

export function insertSortedUniqueMessage(existing: ChatMessage[], message: ChatMessage): ChatMessage[] {
  return mergeTimelineMessages(existing, [message])
}

function normalizeMessageContent(content: string): string {
  return content.trim()
}

function findOptimisticByLocalEchoToken(existing: ChatMessage[], canonicalMessage: ChatMessage): number {
  const token = canonicalMessage.localEchoToken?.trim()
  if (!canonicalMessage.isMine || !token) {
    return -1
  }

  return existing.findIndex((message) => {
    if (!message.id.startsWith('local:')) return false
    if (!message.isMine) return false
    if (message.status === 'failed') return false
    return message.localEchoToken?.trim() === token
  })
}

function chooseOptimisticCandidateIndex(existing: ChatMessage[], canonicalMessage: ChatMessage): number {
  if (!canonicalMessage.isMine) {
    return -1
  }

  const canonicalContent = normalizeMessageContent(canonicalMessage.content)
  if (!canonicalContent) {
    return -1
  }

  const candidates = existing
    .map((message, index) => ({ message, index }))
    .filter(({ message }) => {
      if (!message.id.startsWith('local:')) return false
      if (!message.isMine) return false
      if (message.status === 'failed') return false
      if (message.kind !== 'user') return false
      if (message.groupId !== canonicalMessage.groupId) return false
      return normalizeMessageContent(message.content) === canonicalContent
    })

  if (candidates.length === 0) {
    return -1
  }

  const notAfterCanonical = candidates.filter(({ message }) => message.timestamp <= canonicalMessage.timestamp)
  const preferredPool = notAfterCanonical.length > 0 ? notAfterCanonical : candidates

  preferredPool.sort((a, b) => {
    if (a.message.timestamp !== b.message.timestamp) {
      return a.message.timestamp - b.message.timestamp
    }
    return a.index - b.index
  })

  return preferredPool[0]?.index ?? -1
}

export function reconcileCanonicalWithOptimistic(
  existing: ChatMessage[],
  canonicalMessage: ChatMessage,
): ChatMessage[] {
  const optimisticIdxByToken = findOptimisticByLocalEchoToken(existing, canonicalMessage)
  const optimisticIdx =
    optimisticIdxByToken >= 0 ? optimisticIdxByToken : chooseOptimisticCandidateIndex(existing, canonicalMessage)
  if (optimisticIdx < 0) {
    return insertSortedUniqueMessage(existing, canonicalMessage)
  }

  const optimisticMessage = existing[optimisticIdx]
  const next = existing.filter((_, index) => index !== optimisticIdx)
  next.push({
    ...optimisticMessage,
    ...canonicalMessage,
    id: canonicalMessage.id,
    status: 'published',
  })
  return reconcileTimelineMessages(next)
}

export interface UnreadAnchorState {
  anchorId: string | null
  count: number
}

export interface ComputeUnreadAnchorParams {
  previousMessages: ChatMessage[]
  nextMessages: ChatMessage[]
  current: UnreadAnchorState
  isAtBottom: boolean
  suppressTracking?: boolean
}

export function computeUnreadAnchorUpdate({
  previousMessages,
  nextMessages,
  current,
  isAtBottom,
  suppressTracking = false,
}: ComputeUnreadAnchorParams): UnreadAnchorState {
  if (suppressTracking) return current

  const previousIds = new Set(previousMessages.map((message) => message.id))
  const newMessages = nextMessages.filter((message) => !previousIds.has(message.id))
  if (newMessages.length === 0) return current

  const relevantIncomingMessages = newMessages.filter(
    (message) => !message.isMine && message.kind === 'user',
  )

  const oldLength = previousMessages.length
  const oldestInsertedIndex = newMessages.reduce((min, message) => {
    const index = nextMessages.findIndex((candidate) => candidate.id === message.id)
    return index >= 0 ? Math.min(min, index) : min
  }, Number.POSITIVE_INFINITY)
  const insertedIntoHistory = oldestInsertedIndex < oldLength

  if (relevantIncomingMessages.length === 0) {
    if (isAtBottom && !insertedIntoHistory) {
      return { anchorId: null, count: 0 }
    }
    return current
  }

  if (isAtBottom && !insertedIntoHistory) {
    return { anchorId: null, count: 0 }
  }

  const unreadIds = new Set<string>()
  if (current.anchorId) unreadIds.add(current.anchorId)
  for (const message of relevantIncomingMessages) unreadIds.add(message.id)

  const orderedUnread = nextMessages.filter((message) => unreadIds.has(message.id))
  return {
    anchorId: orderedUnread[0]?.id ?? null,
    count: orderedUnread.length,
  }
}

export function shouldPerformInitialScroll(
  activeGroupId: string | null,
  pendingInitialScrollGroupId: string | null,
  loadingMessages: boolean,
): boolean {
  return Boolean(
    activeGroupId &&
      pendingInitialScrollGroupId === activeGroupId &&
      !loadingMessages,
  )
}
