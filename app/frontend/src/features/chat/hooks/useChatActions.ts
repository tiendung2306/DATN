import { useState } from 'react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import { useChatStore } from '../../../stores/useChatStore'

interface UseChatActionsOptions {
  activeGroupId: string | null
  localPeerId: string
  refreshGroups: () => Promise<void>
  setActiveGroupId: (groupId: string | null) => void
}

export function useChatActions({
  activeGroupId,
  localPeerId,
  refreshGroups,
  setActiveGroupId,
}: UseChatActionsOptions) {
  const messagesByGroup = useChatStore((s) => s.messagesByGroup)
  const pushMessage = useChatStore((s) => s.pushMessage)
  const updateMessageStatus = useChatStore((s) => s.updateMessageStatus)
  const removeMessage = useChatStore((s) => s.removeMessage)
  const markGroupRead = useChatStore((s) => s.markGroupRead)

  const [createGroupValue, setCreateGroupValue] = useState('')
  const [creatingGroup, setCreatingGroup] = useState(false)
  const [composingMessage, setComposingMessage] = useState('')
  const [sending, setSending] = useState(false)

  const handleSelectGroup = (groupId: string) => {
    setActiveGroupId(groupId)
    markGroupRead(groupId)
  }

  const handleCreateGroup = async () => {
    const groupId = createGroupValue.trim()
    if (!groupId) return
    setCreatingGroup(true)
    try {
      await runtimeClient.createGroupChat(groupId)
      setCreateGroupValue('')
      await refreshGroups()
      setActiveGroupId(groupId)
      pushMessage(groupId, {
        id: `system:create:${groupId}`,
        groupId,
        sender: 'system',
        content: 'You created this secure group.',
        timestamp: Date.now(),
        isMine: false,
        status: 'published',
        kind: 'system',
      })
    } finally {
      setCreatingGroup(false)
    }
  }

  const handleSendMessage = async () => {
    if (!activeGroupId || !composingMessage.trim()) return
    const text = composingMessage.trim()
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
    } catch {
      updateMessageStatus(activeGroupId, pendingId, 'failed')
    } finally {
      setSending(false)
    }
  }

  const handleRetryMessage = async (messageId: string) => {
    if (!activeGroupId) return
    const messages = messagesByGroup[activeGroupId] ?? []
    const failed = messages.find((message) => message.id === messageId)
    if (!failed) return
    updateMessageStatus(activeGroupId, messageId, 'sending')
    try {
      await runtimeClient.sendGroupMessage(activeGroupId, failed.content)
      updateMessageStatus(activeGroupId, messageId, 'published')
    } catch {
      updateMessageStatus(activeGroupId, messageId, 'failed')
    }
  }

  const handleRemoveFailed = (messageId: string) => {
    if (!activeGroupId) return
    removeMessage(activeGroupId, messageId)
  }

  return {
    createGroupValue,
    setCreateGroupValue,
    creatingGroup,
    composingMessage,
    setComposingMessage,
    sending,
    handleSelectGroup,
    handleCreateGroup,
    handleSendMessage,
    handleRetryMessage,
    handleRemoveFailed,
  }
}
