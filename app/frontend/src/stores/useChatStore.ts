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
  commentCount?: number
}

interface ChatState {
  messagesByGroup: Record<string, ChatMessage[]> // For DMs or legacy
  postsByGroup: Record<string, ChatMessage[]>
  commentsByPost: Record<string, ChatMessage[]>
  unreadByGroup: Record<string, number>
  pushMessage: (groupId: string, message: ChatMessage) => void
  pushPost: (groupId: string, message: ChatMessage) => void
  pushComment: (postId: string, message: ChatMessage) => void
  setMessages: (groupId: string, messages: ChatMessage[]) => void
  setPosts: (groupId: string, posts: ChatMessage[]) => void
  setComments: (postId: string, comments: ChatMessage[]) => void
  markGroupRead: (groupId: string) => void
  incrementUnread: (groupId: string) => void
  updateMessageStatus: (groupId: string, messageId: string, status: ChatMessageStatus) => void
  removeMessage: (groupId: string, messageId: string) => void
  prependMessages: (groupId: string, messages: ChatMessage[]) => void
  prependPosts: (groupId: string, posts: ChatMessage[]) => void
  prependComments: (postId: string, comments: ChatMessage[]) => void
  reset: () => void
}

export const useChatStore = create<ChatState>((set) => ({
  messagesByGroup: {},
  postsByGroup: {},
  commentsByPost: {},
  unreadByGroup: {},
  pushMessage: (groupId, message) =>
    set((state) => ({
      messagesByGroup: {
        ...state.messagesByGroup,
        [groupId]: [...(state.messagesByGroup[groupId] ?? []), message],
      },
    })),
  pushPost: (groupId, message) =>
    set((state) => ({
      postsByGroup: {
        ...state.postsByGroup,
        [groupId]: [...(state.postsByGroup[groupId] ?? []), message],
      },
    })),
  pushComment: (postId, message) =>
    set((state) => {
      const newComments = {
        ...state.commentsByPost,
        [postId]: [...(state.commentsByPost[postId] ?? []), message],
      }
      // Update comment count in posts list if present
      const newPostsByGroup = { ...state.postsByGroup }
      Object.keys(newPostsByGroup).forEach((groupId) => {
        newPostsByGroup[groupId] = newPostsByGroup[groupId].map((p) =>
          p.id === postId ? { ...p, commentCount: (p.commentCount ?? 0) + 1 } : p,
        )
      })
      return {
        commentsByPost: newComments,
        postsByGroup: newPostsByGroup,
      }
    }),
  setMessages: (groupId, messages) =>
    set((state) => ({
      messagesByGroup: {
        ...state.messagesByGroup,
        [groupId]: messages,
      },
    })),
  setPosts: (groupId, posts) =>
    set((state) => ({
      postsByGroup: {
        ...state.postsByGroup,
        [groupId]: posts,
      },
    })),
  setComments: (postId, comments) =>
    set((state) => ({
      commentsByPost: {
        ...state.commentsByPost,
        [postId]: comments,
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
  prependMessages: (groupId, messages) =>
    set((state) => {
      const existing = state.messagesByGroup[groupId] ?? []
      const existingIds = new Set(existing.map((m) => m.id))
      const newUnique = messages.filter((m) => !existingIds.has(m.id))
      return {
        messagesByGroup: {
          ...state.messagesByGroup,
          [groupId]: [...newUnique, ...existing],
        },
      }
    }),
  prependPosts: (groupId, posts) =>
    set((state) => {
      const existing = state.postsByGroup[groupId] ?? []
      const existingIds = new Set(existing.map((m) => m.id))
      const newUnique = posts.filter((m) => !existingIds.has(m.id))
      return {
        postsByGroup: {
          ...state.postsByGroup,
          [groupId]: [...newUnique, ...existing],
        },
      }
    }),
  prependComments: (postId, comments) =>
    set((state) => {
      const existing = state.commentsByPost[postId] ?? []
      const existingIds = new Set(existing.map((m) => m.id))
      const newUnique = comments.filter((m) => !existingIds.has(m.id))
      return {
        commentsByPost: {
          ...state.commentsByPost,
          [postId]: [...newUnique, ...existing],
        },
      }
    }),
  reset: () => set({ messagesByGroup: {}, postsByGroup: {}, commentsByPost: {}, unreadByGroup: {} }),
}))
