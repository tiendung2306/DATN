import { create } from 'zustand'
import { service } from '../../wailsjs/go/models'
import { GetNotifications, GetUnreadNotificationCount, MarkNotificationRead, MarkAllNotificationsRead } from '../../wailsjs/go/service/Runtime'

export interface Notification extends service.NotificationDTO {}

interface NotificationState {
  notifications: Notification[]
  unreadCount: number
  isLoading: boolean
  
  fetchNotifications: (limit?: number, offset?: number) => Promise<void>
  fetchUnreadCount: () => Promise<void>
  markRead: (id: string) => Promise<void>
  markAllRead: () => Promise<void>
  addNotification: (n: Notification) => void
}

export const useNotificationStore = create<NotificationState>((set, get) => ({
  notifications: [],
  unreadCount: 0,
  isLoading: false,

  fetchNotifications: async (limit = 50, offset = 0) => {
    set({ isLoading: true })
    try {
      const list = await GetNotifications(limit, offset)
      set({ notifications: list as Notification[] })
    } catch (err) {
      console.error('Failed to fetch notifications:', err)
    } finally {
      set({ isLoading: false })
    }
  },

  fetchUnreadCount: async () => {
    try {
      const count = await GetUnreadNotificationCount()
      set({ unreadCount: count })
    } catch (err) {
      console.error('Failed to fetch unread count:', err)
    }
  },

  markRead: async (id: string) => {
    try {
      await MarkNotificationRead(id)
      set((state) => ({
        notifications: state.notifications.map((n) =>
          n.id === id ? { ...n, is_read: true } : n
        ),
        unreadCount: Math.max(0, state.unreadCount - 1),
      }))
    } catch (err) {
      console.error('Failed to mark notification read:', err)
    }
  },

  markAllRead: async () => {
    try {
      await MarkAllNotificationsRead()
      set((state) => ({
        notifications: state.notifications.map((n) => ({ ...n, is_read: true })),
        unreadCount: 0,
      }))
    } catch (err) {
      console.error('Failed to mark all read:', err)
    }
  },

  addNotification: (n: Notification) => {
    set((state) => {
      // Avoid duplicates
      if (state.notifications.some(existing => existing.id === n.id)) {
        return state
      }
      return {
        notifications: [n, ...state.notifications],
        unreadCount: state.unreadCount + 1,
      }
    })
  },
}))
