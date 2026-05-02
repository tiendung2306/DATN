import { create } from 'zustand'

export type ToastVariant = 'default' | 'destructive'

export type ToastItem = {
  id: string
  title: string
  description?: string
  variant: ToastVariant
}

type ToastStore = {
  toasts: ToastItem[]
  pushToast: (input: Omit<ToastItem, 'id'>) => void
  dismissToast: (id: string) => void
}

function nextId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID()
  }
  return `toast-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`
}

export const useToastStore = create<ToastStore>((set) => ({
  toasts: [],
  pushToast: (input) => {
    const id = nextId()
    const item: ToastItem = { id, ...input }
    set((s) => ({ toasts: [...s.toasts, item] }))
    window.setTimeout(() => {
      set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) }))
    }, 6500)
  },
  dismissToast: (id) => set((s) => ({ toasts: s.toasts.filter((t) => t.id !== id) })),
}))
