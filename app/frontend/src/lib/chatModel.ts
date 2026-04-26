import { service } from '../../wailsjs/go/models'
import { ChatMessage } from '../stores/useChatStore'

export function messageInfoToChatMessage(message: service.MessageInfo): ChatMessage {
  return {
    id: `${message.group_id}:${message.sender}:${message.timestamp}:${message.content}`,
    groupId: message.group_id,
    sender: message.sender,
    content: message.content,
    timestamp: message.timestamp,
    isMine: message.is_mine,
    status: 'published',
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
