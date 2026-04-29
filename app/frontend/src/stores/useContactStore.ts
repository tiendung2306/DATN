import { create } from 'zustand'

export interface Contact {
  displayName: string
  isOnline: boolean
}

interface ContactState {
  contacts: Record<string, Contact>
  setContact: (peerId: string, contact: Partial<Contact>) => void
  setContacts: (contacts: Record<string, Partial<Contact>>) => void
  getDisplayName: (peerId: string) => string
}

export const useContactStore = create<ContactState>((set, get) => ({
  contacts: {},
  setContact: (peerId, contact) =>
    set((state) => ({
      contacts: {
        ...state.contacts,
        [peerId]: {
          displayName: contact.displayName ?? state.contacts[peerId]?.displayName ?? '',
          isOnline: contact.isOnline ?? state.contacts[peerId]?.isOnline ?? false,
        },
      },
    })),
  setContacts: (newContacts) =>
    set((state) => {
      const merged = { ...state.contacts }
      for (const [peerId, data] of Object.entries(newContacts)) {
        merged[peerId] = {
          displayName: data.displayName ?? state.contacts[peerId]?.displayName ?? '',
          isOnline: data.isOnline ?? state.contacts[peerId]?.isOnline ?? false,
        };
      }
      return { contacts: merged }
    }),
  getDisplayName: (peerId) => {
    const contact = get().contacts[peerId]
    if (contact?.displayName && contact.displayName.trim() !== '') {
      return contact.displayName
    }
    // Fallback to short peerId
    return peerId.length > 12 ? `${peerId.slice(0, 6)}...${peerId.slice(-6)}` : peerId
  },
}))
