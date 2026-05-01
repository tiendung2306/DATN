import { service } from '../../wailsjs/go/models'
import { ChatMessage } from '../stores/useChatStore'
import { useContactStore } from '../stores/useContactStore'

export function messageInfoToChatMessage(message: service.MessageInfo): ChatMessage {
  const fallbackId = `${message.group_id}:${message.sender}:${message.timestamp}:${message.content}`
  
  if ((message as any).sender_display_name) {
    useContactStore.getState().setContact(message.sender, {
      displayName: (message as any).sender_display_name,
    })
  }
  return {
    id: (message as { message_id?: string }).message_id || fallbackId,
    groupId: message.group_id,
    sender: message.sender,
    content: message.content,
    timestamp: message.timestamp,
    isMine: message.is_mine,
    status: ((message as { status?: ChatMessage['status'] }).status ?? 'published') as ChatMessage['status'],
    kind: 'user',
    commentCount: (message as any).comment_count,
  }
}

export function formatMessageTime(timestampMs: number): string {
  if (!timestampMs) return ''
  return new Date(timestampMs).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
  })
}

export function shortPeerId(peerId: string): string {
  if (!peerId) return ''
  if (peerId.length <= 14) return peerId
  return `${peerId.slice(0, 6)}...${peerId.slice(-6)}`
}
export interface ParsedPayload {
  type: 'post' | 'comment' | 'legacy'
  title?: string
  body: string
  postId?: string
  replyToCommentId?: string
  mentions?: MentionEntity[]
}

export interface MentionEntity {
  user_id: string
  display_name: string
  start: number
  end: number
}

export interface PostPayload {
  type: 'post'
  title?: string
  body: string
  mentions?: MentionEntity[]
}

export interface CommentPayload {
  type: 'comment'
  post_id: string
  body: string
  mentions?: MentionEntity[]
  reply_to_comment_id?: string
}

export function parseMessageContent(content: string): ParsedPayload {
  try {
    const data = JSON.parse(content) as Record<string, unknown>
    if (data && typeof data === 'object') {
      if (data.type === 'post') {
        const title = typeof data.title === 'string' ? data.title : undefined
        const body = typeof data.body === 'string'
          ? data.body
          : typeof data.content === 'string'
            ? data.content
            : ''
        const mentions = normalizeMentions(data.mentions)
        return { type: 'post', title, body, mentions }
      }
      if (data.type === 'comment') {
        const postId = typeof data.post_id === 'string' ? data.post_id : ''
        const body = typeof data.body === 'string' ? data.body : ''
        const replyToCommentId =
          typeof data.reply_to_comment_id === 'string' ? data.reply_to_comment_id : undefined
        const mentions = normalizeMentions(data.mentions)
        return { type: 'comment', postId, body, mentions, replyToCommentId }
      }
      // Backward compatibility with old schema.
      if (data.type === 'reply') {
        const postId = typeof data.parent_id === 'string' ? data.parent_id : ''
        const body = typeof data.content === 'string' ? data.content : ''
        return { type: 'comment', postId, body, mentions: normalizeMentions(data.mentions) }
      }
    }
  } catch {
    // Ignore JSON parse failure
  }
  return { type: 'legacy', body: content }
}

function normalizeMentions(raw: unknown): MentionEntity[] | undefined {
  if (!Array.isArray(raw)) return undefined
  const mentions = raw
    .map((item) => {
      if (!item || typeof item !== 'object') return null
      const entry = item as Record<string, unknown>
      const userId = typeof entry.user_id === 'string' ? entry.user_id : ''
      const displayName = typeof entry.display_name === 'string' ? entry.display_name : ''
      const start = typeof entry.start === 'number' ? entry.start : -1
      const end = typeof entry.end === 'number' ? entry.end : -1
      if (!userId || !displayName || start < 0 || end <= start) return null
      return {
        user_id: userId,
        display_name: displayName,
        start,
        end,
      } satisfies MentionEntity
    })
    .filter((item): item is MentionEntity => item !== null)
  return mentions.length > 0 ? mentions : undefined
}

export function serializePostPayload(input: { title?: string; body: string; mentions?: MentionEntity[] }): string {
  const payload: PostPayload = {
    type: 'post',
    title: input.title?.trim() || undefined,
    body: input.body,
    mentions: input.mentions && input.mentions.length > 0 ? input.mentions : undefined,
  }
  return JSON.stringify(payload)
}

export function serializeCommentPayload(input: {
  postId: string
  body: string
  mentions?: MentionEntity[]
  replyToCommentId?: string
}): string {
  const payload: CommentPayload = {
    type: 'comment',
    post_id: input.postId,
    body: input.body,
    mentions: input.mentions && input.mentions.length > 0 ? input.mentions : undefined,
    reply_to_comment_id: input.replyToCommentId?.trim() || undefined,
  }
  return JSON.stringify(payload)
}

export interface MentionCandidate {
  userId: string
  displayName: string
}

export function extractMentionsFromBody(body: string, candidates: MentionCandidate[]): MentionEntity[] {
  if (!body || candidates.length === 0) return []
  const sorted = [...candidates]
    .filter((item) => item.userId && item.displayName)
    .sort((a, b) => b.displayName.length - a.displayName.length)

  const bodyLower = body.toLowerCase()
  const mentions: MentionEntity[] = []
  for (let i = 0; i < body.length; i += 1) {
    if (body[i] !== '@') continue
    const afterAt = i + 1
    for (const candidate of sorted) {
      const label = candidate.displayName
      const labelLower = label.toLowerCase()
      if (bodyLower.slice(afterAt, afterAt + label.length) !== labelLower) continue
      const end = afterAt + label.length
      const nextChar = body[end]
      if (nextChar && /[a-zA-Z0-9_-]/.test(nextChar)) {
        continue
      }
      mentions.push({
        user_id: candidate.userId,
        display_name: candidate.displayName,
        start: i,
        end,
      })
      i = end - 1
      break
    }
  }
  return mentions
}

export function renderBodyWithMentions(body: string, mentions?: MentionEntity[]): string {
  if (!mentions || mentions.length === 0) return body
  return body
}
