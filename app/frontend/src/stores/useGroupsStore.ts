import { create } from 'zustand'
import { service } from '../../wailsjs/go/models'

interface GroupsState {
  groups: service.GroupInfo[]
  activeGroupId: string | null
  loading: boolean
  error: string | null
  setGroups: (groups: service.GroupInfo[]) => void
  setActiveGroupId: (groupId: string | null) => void
  setLoading: (loading: boolean) => void
  setError: (error: string | null) => void
  reset: () => void
}

export const useGroupsStore = create<GroupsState>((set) => ({
  groups: [],
  activeGroupId: null,
  loading: false,
  error: null,
  setGroups: (groups) => set({ groups }),
  setActiveGroupId: (groupId) => set({ activeGroupId: groupId }),
  setLoading: (loading) => set({ loading }),
  setError: (error) => set({ error }),
  reset: () => set({ groups: [], activeGroupId: null, loading: false, error: null }),
}))
