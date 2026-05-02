import { create } from 'zustand'
import { runtimeClient } from '../services/runtime/runtimeClient'

export type MessageLimitsState = {
  dmMaxRunes: number
  channelTitleMaxRunes: number
  channelBodyMaxRunes: number
  channelCommentMaxRunes: number
  loaded: boolean
  fetchLimits: () => Promise<void>
}

const fallback: Pick<MessageLimitsState, 'dmMaxRunes' | 'channelTitleMaxRunes' | 'channelBodyMaxRunes' | 'channelCommentMaxRunes'> = {
  dmMaxRunes: 4000,
  channelTitleMaxRunes: 160,
  channelBodyMaxRunes: 4000,
  channelCommentMaxRunes: 4000,
}

export const useMessageLimitsStore = create<MessageLimitsState>((set) => ({
  ...fallback,
  loaded: false,
  fetchLimits: async () => {
    try {
      const lim = await runtimeClient.getMessageLimits()
      set({
        dmMaxRunes: lim.dm_max_runes,
        channelTitleMaxRunes: lim.channel_title_max_runes,
        channelBodyMaxRunes: lim.channel_body_max_runes,
        channelCommentMaxRunes: lim.channel_comment_max_runes,
        loaded: true,
      })
    } catch {
      set({ ...fallback, loaded: true })
    }
  },
}))
