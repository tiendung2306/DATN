import { useState } from 'react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import { formatOutboundSendError } from '../../../lib/formatSendError'
import { countUnicodeRunes } from '../../../lib/textLimits'
import { useChatStore } from '../../../stores/useChatStore'
import { useMessageLimitsStore } from '../../../stores/useMessageLimitsStore'
import { useToastStore } from '../../../stores/useToastStore'

interface UseChatActionsOptions {
  activeGroupId: string | null
  localPeerId: string
  refreshGroups: () => Promise<void>
  setActiveGroupId: (groupId: string | null) => void
}

type ConversationCreateType = 'channel' | 'group' | 'dm'

export function useChatActions({
  activeGroupId,
  localPeerId,
  refreshGroups,
  setActiveGroupId,
}: UseChatActionsOptions) {
  const messagesByGroup = useChatStore((s) => s.messagesByGroup)
  const pushMessage = useChatStore((s) => s.pushMessage)
  const pushPost = useChatStore((s) => s.pushPost)
  const updateMessageStatus = useChatStore((s) => s.updateMessageStatus)
  const removeMessage = useChatStore((s) => s.removeMessage)
  const markGroupRead = useChatStore((s) => s.markGroupRead)

  const [creatingGroup, setCreatingGroup] = useState(false)
  const [composingMessage, setComposingMessage] = useState('')
  const [sending, setSending] = useState(false)

  const handleSelectGroup = (groupId: string) => {
    setActiveGroupId(groupId)
    markGroupRead(groupId)
  }

  const handleCreateGroupWithDetails = async (groupId: string, groupType: ConversationCreateType, members: string[]) => {
    groupId = groupId.trim()
    if (groupType !== 'dm' && !groupId) return
    setCreatingGroup(true)
    try {
      if (groupType === 'dm') {
        if (members.length !== 1) {
          throw new Error('Direct message requires exactly one peer.')
        }
        groupId = await runtimeClient.startDirectMessage(members[0])
      } else {
        await runtimeClient.createGroupChat(groupId, groupType)
      }
      
      if (groupType !== 'dm') {
        for (const peerId of members) {
          if (peerId.trim()) {
            try {
              await runtimeClient.invitePeerToGroup(peerId.trim(), groupId)
            } catch (e) {
              console.error(`Failed to invite peer ${peerId}:`, e)
            }
          }
        }
      }

      await refreshGroups()
      setActiveGroupId(groupId)
      const createdAt = Date.now()
      const systemMessage = {
        id: `system:create:${groupId}`,
        groupId,
        sender: 'system',
        content: `Bạn đã tạo ${
          groupType === 'dm' ? 'cuộc trò chuyện trực tiếp' : groupType === 'group' ? 'nhóm chat' : 'kênh'
        } này.`,
        timestamp: createdAt,
        isMine: false,
        status: 'published',
        kind: 'system',
      } as const
      if (groupType === 'channel') {
        pushPost(groupId, systemMessage)
      } else {
        pushMessage(groupId, systemMessage)
      }
    } finally {
      setCreatingGroup(false)
    }
  }

  const handleSendMessage = async () => {
    if (!activeGroupId || !composingMessage.trim()) return
    const text = composingMessage.trim()
    const maxDm = useMessageLimitsStore.getState().dmMaxRunes
    if (countUnicodeRunes(text) > maxDm) {
      const mapped = formatOutboundSendError(new Error('TEXT_TOO_LONG'))
      useToastStore.getState().pushToast({
        title: mapped.title,
        description: mapped.description,
        variant: mapped.variant,
      })
      return
    }
    const pendingId = `local:${Date.now()}`
    pushMessage(activeGroupId, {
      id: pendingId,
      groupId: activeGroupId,
      sender: localPeerId,
      content: text,
      timestamp: Date.now(),
      isMine: true,
      status: 'sending',
      kind: 'user',
    })
    setComposingMessage('')
    setSending(true)
    try {
      await runtimeClient.sendGroupMessage(activeGroupId, text)
      updateMessageStatus(activeGroupId, pendingId, 'published')
      void refreshGroups()
    } catch (err) {
      updateMessageStatus(activeGroupId, pendingId, 'failed')
      const mapped = formatOutboundSendError(err)
      useToastStore.getState().pushToast({
        title: mapped.title,
        description: mapped.description,
        variant: mapped.variant,
      })
    } finally {
      setSending(false)
    }
  }

  const handleRetryMessage = async (messageId: string) => {
    if (!activeGroupId) return
    const messages = messagesByGroup[activeGroupId] ?? []
    const failed = messages.find((message) => message.id === messageId)
    if (!failed) return
    const maxDm = useMessageLimitsStore.getState().dmMaxRunes
    if (countUnicodeRunes(failed.content.trim()) > maxDm) {
      const mapped = formatOutboundSendError(new Error('TEXT_TOO_LONG'))
      useToastStore.getState().pushToast({
        title: mapped.title,
        description: mapped.description,
        variant: mapped.variant,
      })
      return
    }
    updateMessageStatus(activeGroupId, messageId, 'sending')
    try {
      if (messageId.startsWith('local:')) {
        await runtimeClient.sendGroupMessage(activeGroupId, failed.content)
      } else {
        await runtimeClient.retryMessage(activeGroupId, messageId)
      }
      updateMessageStatus(activeGroupId, messageId, 'published')
    } catch (err) {
      updateMessageStatus(activeGroupId, messageId, 'failed')
      const mapped = formatOutboundSendError(err)
      useToastStore.getState().pushToast({
        title: mapped.title,
        description: mapped.description,
        variant: mapped.variant,
      })
    }
  }

  const handleRemoveFailed = async (messageId: string) => {
    if (!activeGroupId) return
    if (!messageId.startsWith('local:')) {
      try {
        await runtimeClient.deleteLocalMessage(activeGroupId, messageId)
      } catch {
        // best-effort local cleanup still proceeds
      }
    }
    removeMessage(activeGroupId, messageId)
  }

  return {
    creatingGroup,
    composingMessage,
    setComposingMessage,
    sending,
    handleSelectGroup,
    handleCreateGroupWithDetails,
    handleSendMessage,
    handleRetryMessage,
    handleRemoveFailed,
  }
}
