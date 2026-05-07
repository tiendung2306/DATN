import { useCallback, useEffect, useMemo, useState } from 'react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import { getConversationKind, messageInfoToChatMessage, shortPeerId } from '../../../lib/chatModel'
import { mapNodeStatusToNetworkState } from '../../../lib/networkModel'
import { useGroupsStore } from '../../../stores/useGroupsStore'
import { useNetworkStore } from '../../../stores/useNetworkStore'
import { useChatStore } from '../../../stores/useChatStore'
import { useContactStore } from '../../../stores/useContactStore'
import { useMessageLimitsStore } from '../../../stores/useMessageLimitsStore'
import { service } from '../../../../wailsjs/go/models'

const EMPTY_ARRAY: any[] = []

export function useChatRuntime() {
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
  const postsByGroup = useChatStore((s) => s.postsByGroup)
  const unreadByGroup = useChatStore((s) => s.unreadByGroup)
  const setMessages = useChatStore((s) => s.setMessages)
  const markGroupRead = useChatStore((s) => s.markGroupRead)


  const [displayName, setDisplayName] = useState('')
  const [loadingMessages, setLoadingMessages] = useState(false)
  const [activeGroupMembers, setActiveGroupMembers] = useState<service.MemberInfo[]>([])

  const refreshNodeStatus = useCallback(async () => {
    try {
      const status = await runtimeClient.getNodeStatus()
      setDisplayName(status.display_name || shortPeerId(status.peer_id))
      setLocalPeerId(status.peer_id)
      setConnectedPeers(status.connected_peers ?? [])
      setNetworkStatus(mapNodeStatusToNetworkState(status))

      if (status.connected_peers) {
        const contactMap: Record<string, { displayName: string; isOnline: boolean }> = {}
        
        // Hydrate local user
        if (status.peer_id) {
          contactMap[status.peer_id] = {
            displayName: status.display_name || '',
            isOnline: true,
          }
        }

        for (const peer of status.connected_peers) {
          contactMap[peer.id] = {
            displayName: peer.display_name || '',
            isOnline: true,
          }
        }
        useContactStore.getState().setContacts(contactMap)
      }
    } catch {
      setNetworkStatus('offline')
    }
  }, [setConnectedPeers, setLocalPeerId, setNetworkStatus])

  const refreshGroups = useCallback(async () => {
    setGroupsLoading(true)
    setGroupsError(null)
    try {
      const list = await runtimeClient.getGroups()
      setGroups(list ?? [])
      const currentActiveId = useGroupsStore.getState().activeGroupId
      if (!currentActiveId && list.length > 0) {
        setActiveGroupId(list[0].group_id)
      }
    } catch (error) {
      setGroupsError(String(error))
    } finally {
      setGroupsLoading(false)
    }
  }, [setActiveGroupId, setGroups, setGroupsError, setGroupsLoading])

  const loadMessages = useCallback(
    async (groupId: string) => {
      setLoadingMessages(true)
      try {
        const list = await runtimeClient.getGroupMessages(groupId, 50, 0)
        const mapped = (list ?? []).map(messageInfoToChatMessage)
        const sorted = mapped.sort((a, b) => a.timestamp - b.timestamp)
        setMessages(groupId, sorted)
        markGroupRead(groupId)
      } finally {
        setLoadingMessages(false)
      }
    },
    [markGroupRead, setMessages],
  )

  const loadMoreMessages = useCallback(
    async (groupId: string) => {
      const existing = useChatStore.getState().messagesByGroup[groupId] ?? []
      const offset = existing.length
      try {
        const list = await runtimeClient.getGroupMessages(groupId, 50, offset)
        const mapped = (list ?? []).map(messageInfoToChatMessage)
        const sorted = mapped.sort((a, b) => a.timestamp - b.timestamp)
        useChatStore.getState().prependMessages(groupId, sorted)
      } catch (err) {
        console.error('Failed to load more messages:', err)
      }
    },
    [],
  )

  const loadPosts = useCallback(
    async (groupId: string) => {
      setLoadingMessages(true)
      try {
        const list = await runtimeClient.getGroupPosts(groupId, 20, 0)
        const mapped = (list ?? []).map(messageInfoToChatMessage)
        const sorted = mapped.sort((a, b) => a.timestamp - b.timestamp)
        useChatStore.getState().setPosts(groupId, sorted)
        markGroupRead(groupId)
      } finally {
        setLoadingMessages(false)
      }
    },
    [markGroupRead],
  )

  const loadMorePosts = useCallback(
    async (groupId: string) => {
      const existing = useChatStore.getState().postsByGroup[groupId] ?? []
      const offset = existing.length
      try {
        const list = await runtimeClient.getGroupPosts(groupId, 20, offset)
        const mapped = (list ?? []).map(messageInfoToChatMessage)
        const sorted = mapped.sort((a, b) => a.timestamp - b.timestamp)
        useChatStore.getState().prependPosts(groupId, sorted)
      } catch (err) {
        console.error('Failed to load more posts:', err)
      }
    },
    [],
  )

  const loadComments = useCallback(
    async (groupId: string, postId: string) => {
      try {
        // Load latest 3 comments initially for each post
        const list = await runtimeClient.getPostComments(groupId, postId, 3, 0)
        const mapped = (list ?? []).map(messageInfoToChatMessage)
        const sorted = mapped.sort((a, b) => a.timestamp - b.timestamp)
        useChatStore.getState().setComments(postId, sorted)
      } catch (err) {
        console.error('Failed to load comments:', err)
      }
    },
    [],
  )

  const loadMoreComments = useCallback(
    async (groupId: string, postId: string) => {
      const existing = useChatStore.getState().commentsByPost[postId] ?? []
      const offset = existing.length
      try {
        const list = await runtimeClient.getPostComments(groupId, postId, 20, offset)
        const mapped = (list ?? []).map(messageInfoToChatMessage)
        const sorted = mapped.sort((a, b) => a.timestamp - b.timestamp)
        useChatStore.getState().prependComments(postId, sorted)
      } catch (err) {
        console.error('Failed to load more comments:', err)
      }
    },
    [],
  )

  const loadGroupMembers = useCallback(async (groupId: string) => {
    try {
      const members = await runtimeClient.getGroupMembers(groupId)
      setActiveGroupMembers(members ?? [])
      if (members && members.length > 0) {
        const contactMap: Record<string, { displayName: string; isOnline: boolean }> = {}
        for (const m of members) {
          if (m.display_name) {
            contactMap[m.peer_id] = {
              displayName: m.display_name,
              isOnline: m.is_online,
            }
          }
        }
        useContactStore.getState().setContacts(contactMap)
      }
    } catch {
      setActiveGroupMembers([])
    }
  }, [])

  useEffect(() => {
    void useMessageLimitsStore.getState().fetchLimits()
  }, [])

  useEffect(() => {
    void refreshNodeStatus()
    void refreshGroups()
    // Startup race guard: when main chat mounts before backend stack is fully ready,
    // a short one-shot retry prevents empty sidebar until next manual action.
    const retry = setTimeout(() => {
      void refreshNodeStatus()
      void refreshGroups()
    }, 1500)
    return () => clearTimeout(retry)
  }, [refreshGroups, refreshNodeStatus])

  useEffect(() => {
    if (!activeGroupId) return
    const group = groups.find((g) => g.group_id === activeGroupId)
    const kind = getConversationKind(group)
    if (kind === 'channel') {
      void loadPosts(activeGroupId)
    } else {
      void loadMessages(activeGroupId)
    }
    void loadGroupMembers(activeGroupId)
  }, [activeGroupId, loadMessages, loadPosts, loadGroupMembers, groups])

  useEffect(() => {
    if (!activeGroupId) {
      setActiveGroupMembers([])
    }
  }, [activeGroupId])

  const activeMessages = useMemo(
    () => (activeGroupId ? messagesByGroup[activeGroupId] ?? EMPTY_ARRAY : EMPTY_ARRAY),
    [activeGroupId, messagesByGroup],
  )

  const activePosts = useMemo(
    () => (activeGroupId ? postsByGroup[activeGroupId] ?? EMPTY_ARRAY : EMPTY_ARRAY),
    [activeGroupId, postsByGroup],
  )


  return {
    displayName,
    groups,
    activeGroupId,
    networkStatus,
    connectedPeers,
    localPeerId,
    unreadByGroup,
    loadingMessages,
    activeMessages,
    activePosts,
    activeGroupMembers,
    refreshGroups,
    refreshNodeStatus,
    setGroups,
    setActiveGroupId,
    markGroupRead,
    loadMessages,
    loadMoreMessages,
    loadPosts,
    loadMorePosts,
    loadComments,
    loadMoreComments,
    loadGroupMembers,
  }
}

