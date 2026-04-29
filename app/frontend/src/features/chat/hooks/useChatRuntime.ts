import { useCallback, useEffect, useMemo, useState } from 'react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import { messageInfoToChatMessage, shortPeerId } from '../../../lib/chatModel'
import { mapNodeStatusToNetworkState } from '../../../lib/networkModel'
import { useGroupsStore } from '../../../stores/useGroupsStore'
import { useNetworkStore } from '../../../stores/useNetworkStore'
import { useChatStore } from '../../../stores/useChatStore'
import { useContactStore } from '../../../stores/useContactStore'
import { service } from '../../../../wailsjs/go/models'

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
        const list = await runtimeClient.getGroupMessages(groupId)
        const mapped = (list ?? []).map(messageInfoToChatMessage)
        setMessages(groupId, mapped)
        markGroupRead(groupId)
      } finally {
        setLoadingMessages(false)
      }
    },
    [markGroupRead, setMessages],
  )

  const loadGroupMembers = useCallback(async (groupId: string) => {
    try {
      const members = await runtimeClient.getGroupMembers(groupId)
      setActiveGroupMembers(members ?? [])
    } catch {
      setActiveGroupMembers([])
    }
  }, [])

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
    void loadGroupMembers(activeGroupId)
  }, [activeGroupId, loadMessages, loadGroupMembers])

  useEffect(() => {
    if (!activeGroupId) {
      setActiveGroupMembers([])
    }
  }, [activeGroupId])

  const activeMessages = useMemo(
    () => (activeGroupId ? messagesByGroup[activeGroupId] ?? [] : []),
    [activeGroupId, messagesByGroup],
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
    activeGroupMembers,
    refreshGroups,
    setGroups,
    setActiveGroupId,
    markGroupRead,
  }
}
