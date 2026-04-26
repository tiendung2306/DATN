import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  CreateGroupChat,
  GetGroupMessages,
  GetGroups,
  GetNodeStatus,
  SendGroupMessage,
} from '../../wailsjs/go/service/Runtime'
import { service } from '../../wailsjs/go/models'
import AppShell from '../components/layout/AppShell'
import MainSidebar from '../components/layout/MainSidebar'
import ChatView from '../components/chat/ChatView'
import { useGroupsStore } from '../stores/useGroupsStore'
import { useNetworkStore } from '../stores/useNetworkStore'
import { messageInfoToChatMessage, shortPeerId } from '../lib/chatModel'
import { mapNodeStatusToNetworkState } from '../lib/networkModel'
import { ChatMessage, useChatStore } from '../stores/useChatStore'
import { useWailsEvent } from '../hooks/useWailsEvent'
import RoomPanel from '../components/chat/RoomPanel'

interface MainAppScreenProps {
  isAdmin: boolean
}

interface GroupEpochPayload {
  group_id: string
  epoch: number
}

function uniqueById(messages: ChatMessage[]): ChatMessage[] {
  const seen = new Set<string>()
  return messages.filter((message) => {
    if (seen.has(message.id)) return false
    seen.add(message.id)
    return true
  })
}

export default function MainAppScreen({ isAdmin }: MainAppScreenProps) {
  const groups = useGroupsStore((s) => s.groups)
  const activeGroupId = useGroupsStore((s) => s.activeGroupId)
  const setGroups = useGroupsStore((s) => s.setGroups)
  const setActiveGroupId = useGroupsStore((s) => s.setActiveGroupId)
  const setGroupsLoading = useGroupsStore((s) => s.setLoading)
  const setGroupsError = useGroupsStore((s) => s.setError)

  const networkStatus = useNetworkStore((s) => s.status)
  const connectedPeers = useNetworkStore((s) => s.connectedPeers)
  const localPeerId = useNetworkStore((s) => s.localPeerId)
  const setNetworkStatus = useNetworkStore((s) => s.setStatus)
  const setConnectedPeers = useNetworkStore((s) => s.setConnectedPeers)
  const setLocalPeerId = useNetworkStore((s) => s.setLocalPeerId)

  const messagesByGroup = useChatStore((s) => s.messagesByGroup)
  const unreadByGroup = useChatStore((s) => s.unreadByGroup)
  const pushMessage = useChatStore((s) => s.pushMessage)
  const setMessages = useChatStore((s) => s.setMessages)
  const markGroupRead = useChatStore((s) => s.markGroupRead)
  const incrementUnread = useChatStore((s) => s.incrementUnread)
  const updateMessageStatus = useChatStore((s) => s.updateMessageStatus)
  const removeMessage = useChatStore((s) => s.removeMessage)

  const [displayName, setDisplayName] = useState('')
  const [createGroupValue, setCreateGroupValue] = useState('')
  const [creatingGroup, setCreatingGroup] = useState(false)
  const [loadingMessages, setLoadingMessages] = useState(false)
  const [composingMessage, setComposingMessage] = useState('')
  const [sending, setSending] = useState(false)

  const refreshNodeStatus = useCallback(async () => {
    try {
      const status = await GetNodeStatus()
      setDisplayName(status.display_name || shortPeerId(status.peer_id))
      setLocalPeerId(status.peer_id)
      setConnectedPeers(status.connected_peers ?? [])
      setNetworkStatus(mapNodeStatusToNetworkState(status))
    } catch {
      setNetworkStatus('offline')
    }
  }, [setConnectedPeers, setLocalPeerId, setNetworkStatus])

  const refreshGroups = useCallback(async () => {
    setGroupsLoading(true)
    setGroupsError(null)
    try {
      const list = await GetGroups()
      setGroups(list ?? [])
      if (!activeGroupId && list.length > 0) {
        setActiveGroupId(list[0].group_id)
      }
    } catch (error) {
      setGroupsError(String(error))
    } finally {
      setGroupsLoading(false)
    }
  }, [activeGroupId, setActiveGroupId, setGroups, setGroupsError, setGroupsLoading])

  const loadMessages = useCallback(
    async (groupId: string) => {
      setLoadingMessages(true)
      try {
        const list = await GetGroupMessages(groupId)
        const mapped = (list ?? []).map(messageInfoToChatMessage)
        setMessages(groupId, mapped)
        markGroupRead(groupId)
      } finally {
        setLoadingMessages(false)
      }
    },
    [markGroupRead, setMessages],
  )

  useEffect(() => {
    void refreshNodeStatus()
    void refreshGroups()
    const interval = setInterval(() => {
      void refreshNodeStatus()
      void refreshGroups()
    }, 5000)
    return () => clearInterval(interval)
  }, [refreshGroups, refreshNodeStatus])

  useEffect(() => {
    if (!activeGroupId) return
    void loadMessages(activeGroupId)
  }, [activeGroupId, loadMessages])

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

  const handleSelectGroup = (groupId: string) => {
    setActiveGroupId(groupId)
    markGroupRead(groupId)
  }

  const handleCreateGroup = async () => {
    const groupId = createGroupValue.trim()
    if (!groupId) return
    setCreatingGroup(true)
    try {
      await CreateGroupChat(groupId)
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
      await SendGroupMessage(activeGroupId, text)
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
      await SendGroupMessage(activeGroupId, failed.content)
      updateMessageStatus(activeGroupId, messageId, 'published')
    } catch {
      updateMessageStatus(activeGroupId, messageId, 'failed')
    }
  }

  const handleRemoveFailed = (messageId: string) => {
    if (!activeGroupId) return
    removeMessage(activeGroupId, messageId)
  }

  const activeMessages = useMemo(
    () => (activeGroupId ? messagesByGroup[activeGroupId] ?? [] : []),
    [activeGroupId, messagesByGroup],
  )

  return (
    <AppShell
      title="Secure P2P"
      subtitle={isAdmin ? 'Admin capability enabled' : 'Authorized device'}
    >
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-[290px_minmax(0,1fr)_280px]">
        <MainSidebar
          displayName={displayName}
          localPeerId={localPeerId}
          networkStatus={networkStatus}
          groups={groups}
          activeGroupId={activeGroupId}
          unreadByGroup={unreadByGroup}
          peerCount={connectedPeers.length}
          creatingGroup={creatingGroup}
          createGroupValue={createGroupValue}
          onCreateGroupValueChange={setCreateGroupValue}
          onCreateGroup={handleCreateGroup}
          onSelectGroup={handleSelectGroup}
        />

        <ChatView
          activeGroupId={activeGroupId}
          messages={activeMessages}
          loadingMessages={loadingMessages}
          composingMessage={composingMessage}
          sending={sending}
          onComposingChange={setComposingMessage}
          onSend={handleSendMessage}
          onRetry={handleRetryMessage}
          onRemoveFailed={handleRemoveFailed}
        />

        <RoomPanel activeGroupId={activeGroupId} isAdmin={isAdmin} peers={connectedPeers} />
      </div>
    </AppShell>
  )
}
