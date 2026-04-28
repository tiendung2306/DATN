import { service } from '../../wailsjs/go/models'
import { ChatMessage } from '../stores/useChatStore'

export function messageInfoToChatMessage(message: service.MessageInfo): ChatMessage {
  const fallbackId = `${message.group_id}:${message.sender}:${message.timestamp}:${message.content}`
  return {
    id: (message as { message_id?: string }).message_id || fallbackId,
    groupId: message.group_id,
    sender: message.sender,
    content: message.content,
    timestamp: message.timestamp,
    isMine: message.is_mine,
    status: ((message as { status?: ChatMessage['status'] }).status ?? 'published') as ChatMessage['status'],
    kind: 'user',
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
