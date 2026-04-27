import { useCallback } from 'react'
import { service } from '../../../../wailsjs/go/models'
import { useWailsEvent } from '../../../hooks/useWailsEvent'
import { messageInfoToChatMessage } from '../../../lib/chatModel'
import { useChatStore } from '../../../stores/useChatStore'
import { useGroupsStore } from '../../../stores/useGroupsStore'
import { GroupEpochPayload } from './chatTypes'

function uniqueById<T extends { id: string }>(messages: T[]): T[] {
  const seen = new Set<string>()
  return messages.filter((message) => {
    if (seen.has(message.id)) return false
    seen.add(message.id)
    return true
  })
}

interface UseChatEventsOptions {
  activeGroupId: string | null
  refreshGroups: () => Promise<void>
  setActiveGroupId: (groupId: string | null) => void
}

export function useChatEvents({ activeGroupId, refreshGroups, setActiveGroupId }: UseChatEventsOptions) {
  const messagesByGroup = useChatStore((s) => s.messagesByGroup)
  const setMessages = useChatStore((s) => s.setMessages)
  const markGroupRead = useChatStore((s) => s.markGroupRead)
  const incrementUnread = useChatStore((s) => s.incrementUnread)
  const groups = useGroupsStore((s) => s.groups)
  const setGroups = useGroupsStore((s) => s.setGroups)

  const handleGroupMessage = useCallback(
    (payload: service.MessageInfo) => {
      const message = messageInfoToChatMessage(payload)
      const targetGroup = message.groupId
      const existing = messagesByGroup[targetGroup] ?? []
      const deduped = uniqueById([...existing, message])
      setMessages(targetGroup, deduped)
      if (targetGroup !== activeGroupId) {
        incrementUnread(targetGroup)
      } else {
        markGroupRead(targetGroup)
      }
    },
    [activeGroupId, incrementUnread, markGroupRead, messagesByGroup, setMessages],
  )

  const handleGroupEpoch = useCallback(
    (payload: GroupEpochPayload) => {
      setGroups(
        groups.map((group) =>
          group.group_id === payload.group_id ? { ...group, epoch: payload.epoch } : group,
        ),
      )
    },
    [groups, setGroups],
  )

  const handleGroupJoined = useCallback(
    async (payload: { group_id: string }) => {
      await refreshGroups()
      if (payload?.group_id) {
        setActiveGroupId(payload.group_id)
      }
    },
    [refreshGroups, setActiveGroupId],
  )

  useWailsEvent<service.MessageInfo>('group:message', handleGroupMessage)
  useWailsEvent<GroupEpochPayload>('group:epoch', handleGroupEpoch)
  useWailsEvent<{ group_id: string }>('group:joined', handleGroupJoined)
}
