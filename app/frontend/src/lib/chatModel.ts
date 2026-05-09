import { service } from '../../wailsjs/go/models'
import { ChatMessage } from '../stores/useChatStore'
import { useContactStore } from '../stores/useContactStore'

export type ConversationKind = 'dm' | 'group' | 'channel'

export function getConversationKind(group: { group_type?: string } | null | undefined): ConversationKind {
  const raw = String(group?.group_type ?? '').trim().toLowerCase()
  if (raw === 'dm') return 'dm'
  if (raw === 'channel') return 'channel'
  return 'group'
}

export interface SidebarConversationItem {
  id: string
  kind: ConversationKind
  title: string
  unreadCount: number
  isOnline?: boolean
  lastActivityAt: number
  isChannel: boolean
}

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
  type: 'post' | 'comment' | 'file' | 'legacy'
  title?: string
  body: string
  postId?: string
  replyToCommentId?: string
  mentions?: MentionEntity[]
  file?: FileAttachment
  attachments?: FileAttachment[]
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
  attachments?: FileAttachment[]
}

export interface CommentPayload {
  type: 'comment'
  post_id: string
  body: string
  mentions?: MentionEntity[]
  reply_to_comment_id?: string
}

export interface FileAttachment {
  type: 'file'
  file_id: string
  name: string
  ext?: string
  mime_type: string
  size: number
  sha256: string
  chunk_count: number
  chunk_size: number
  export_epoch: number
  sender_peer_id: string
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
        const attachments = normalizeAttachments(data.attachments)
        return { type: 'post', title, body, mentions, attachments }
      }
      if (data.type === 'file') {
        const file = normalizeFileAttachment(data)
        if (file) {
          return {
            type: 'file',
            body: typeof data.body === 'string' ? data.body : '',
            file,
            attachments: [file],
          }
        }
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

function normalizeAttachments(raw: unknown): FileAttachment[] | undefined {
  if (!Array.isArray(raw)) return undefined
  const mapped = raw
    .map((item) => normalizeFileAttachment(item))
    .filter((item): item is FileAttachment => item !== null)
  return mapped.length > 0 ? mapped : undefined
}

function normalizeFileAttachment(raw: unknown): FileAttachment | null {
  if (!raw || typeof raw !== 'object') return null
  const entry = raw as Record<string, unknown>
  const type = String(entry.type ?? '').trim().toLowerCase()
  if (type !== 'file') return null
  const fileID = String(entry.file_id ?? '').trim()
  const name = String(entry.name ?? '').trim()
  const mimeType = String(entry.mime_type ?? '').trim()
  const size = Number(entry.size ?? 0)
  const sha256 = String(entry.sha256 ?? '').trim()
  const chunkCount = Number(entry.chunk_count ?? 0)
  const chunkSize = Number(entry.chunk_size ?? 0)
  const exportEpoch = Number(entry.export_epoch ?? 0)
  const senderPeerID = String(entry.sender_peer_id ?? '').trim()
  if (!fileID || !name || !mimeType || !sha256 || !senderPeerID) return null
  return {
    type: 'file',
    file_id: fileID,
    name,
    ext: String(entry.ext ?? '').trim() || undefined,
    mime_type: mimeType,
    size: Number.isFinite(size) ? size : 0,
    sha256,
    chunk_count: Number.isFinite(chunkCount) ? chunkCount : 0,
    chunk_size: Number.isFinite(chunkSize) ? chunkSize : 0,
    export_epoch: Number.isFinite(exportEpoch) ? exportEpoch : 0,
    sender_peer_id: senderPeerID,
  }
}

export function formatFileSize(bytes: number): string {
  const n = Number.isFinite(bytes) && bytes > 0 ? bytes : 0
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let value = n / 1024
  let idx = 0
  while (value >= 1024 && idx < units.length - 1) {
    value /= 1024
    idx += 1
  }
  return `${value.toFixed(value >= 10 || idx === 0 ? 0 : 1)} ${units[idx]}`
}

export function fileIconByMimeOrExt(file: Pick<FileAttachment, 'mime_type' | 'ext' | 'name'>): 'pdf' | 'doc' | 'sheet' | 'archive' | 'image' | 'video' | 'audio' | 'file' {
  const mime = String(file.mime_type || '').toLowerCase()
  const extRaw = String(file.ext || '').toLowerCase().replace(/^\./, '')
  const ext = extRaw || String(file.name || '').toLowerCase().split('.').pop() || ''
  if (mime.includes('pdf') || ext === 'pdf') return 'pdf'
  if (mime.includes('word') || ['doc', 'docx'].includes(ext)) return 'doc'
  if (mime.includes('sheet') || ['xls', 'xlsx', 'csv'].includes(ext)) return 'sheet'
  if (mime.startsWith('image/') || ['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg'].includes(ext)) return 'image'
  if (mime.startsWith('video/') || ['mp4', 'mov', 'mkv', 'webm'].includes(ext)) return 'video'
  if (mime.startsWith('audio/') || ['mp3', 'wav', 'ogg', 'm4a'].includes(ext)) return 'audio'
  if (
    mime.includes('zip') ||
    mime.includes('compressed') ||
    ['zip', 'rar', '7z', 'tar', 'gz'].includes(ext)
  ) {
    return 'archive'
  }
  return 'file'
}

export function isFilePayload(message: { content: string }): boolean {
  const parsed = parseMessageContent(message.content)
  return parsed.type === 'file' || Boolean(parsed.attachments?.length)
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

export function serializePostPayload(input: { title?: string; body: string; mentions?: MentionEntity[]; attachments?: FileAttachment[] }): string {
  const payload: PostPayload = {
    type: 'post',
    title: input.title?.trim() || undefined,
    body: input.body,
    mentions: input.mentions && input.mentions.length > 0 ? input.mentions : undefined,
    attachments: input.attachments && input.attachments.length > 0 ? input.attachments : undefined,
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
