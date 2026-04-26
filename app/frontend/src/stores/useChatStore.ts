import { create } from 'zustand'

export type ChatMessageStatus = 'sending' | 'published' | 'failed'
export type ChatMessageKind = 'user' | 'system'

export interface ChatMessage {
  id: string
  groupId: string
  sender: string
  content: string
  timestamp: number
  isMine: boolean
  status: ChatMessageStatus
  kind: ChatMessageKind
}

interface ChatState {
  messagesByGroup: Record<string, ChatMessage[]>
  unreadByGroup: Record<string, number>
  pushMessage: (groupId: string, message: ChatMessage) => void
  setMessages: (groupId: string, messages: ChatMessage[]) => void
  markGroupRead: (groupId: string) => void
  incrementUnread: (groupId: string) => void
  updateMessageStatus: (groupId: string, messageId: string, status: ChatMessageStatus) => void
  removeMessage: (groupId: string, messageId: string) => void
  reset: () => void
}

export const useChatStore = create<ChatState>((set) => ({
  messagesByGroup: {},
  unreadByGroup: {},
  pushMessage: (groupId, message) =>
    set((state) => ({
      messagesByGroup: {
        ...state.messagesByGroup,
        [groupId]: [...(state.messagesByGroup[groupId] ?? []), message],
      },
    })),
  setMessages: (groupId, messages) =>
    set((state) => ({
      messagesByGroup: {
        ...state.messagesByGroup,
        [groupId]: messages,
      },
    })),
  markGroupRead: (groupId) =>
    set((state) => ({
      unreadByGroup: {
        ...state.unreadByGroup,
        [groupId]: 0,
      },
    })),
  incrementUnread: (groupId) =>
    set((state) => ({
      unreadByGroup: {
        ...state.unreadByGroup,
        [groupId]: (state.unreadByGroup[groupId] ?? 0) + 1,
      },
    })),
  updateMessageStatus: (groupId, messageId, status) =>
    set((state) => ({
      messagesByGroup: {
        ...state.messagesByGroup,
        [groupId]: (state.messagesByGroup[groupId] ?? []).map((message) =>
          message.id === messageId ? { ...message, status } : message,
        ),
      },
    })),
  removeMessage: (groupId, messageId) =>
    set((state) => ({
      messagesByGroup: {
        ...state.messagesByGroup,
        [groupId]: (state.messagesByGroup[groupId] ?? []).filter((message) => message.id !== messageId),
      },
    })),
  reset: () => set({ messagesByGroup: {}, unreadByGroup: {} }),
}))
