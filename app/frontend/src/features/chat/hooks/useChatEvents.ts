import { useCallback, useEffect, useRef } from 'react'
import { service } from '../../../../wailsjs/go/models'
import { useWailsEvent } from '../../../hooks/useWailsEvent'
import { messageInfoToChatMessage, shortPeerId } from '../../../lib/chatModel'
import { useChatStore } from '../../../stores/useChatStore'
import { useContactStore } from '../../../stores/useContactStore'
import { useGroupsStore } from '../../../stores/useGroupsStore'
import { useToastStore } from '../../../stores/useToastStore'
import { useNotificationStore } from '../../../stores/useNotificationStore'
import { GroupEpochPayload, GroupLeftPayload, GroupMembersChangedPayload } from './chatTypes'

interface InviteAutoJoinedPayload {
	id?: string
	group_id?: string
	group_type?: string
	inviter_peer?: string
}

interface NotificationNewPayload {
	id: string
	type: string
	group_id: string
	actor_id: string
	actor_name: string
	content: string
}

interface UseChatEventsOptions {
  activeGroupId: string | null
  activeModule: string
  localPeerId: string
  refreshGroups: () => Promise<void>
  refreshNodeStatus: () => Promise<void>
  refreshGroupMembers: (groupId: string) => Promise<void>
  setActiveGroupId: (groupId: string | null) => void
}

export function useChatEvents({
  activeGroupId,
  activeModule,
  localPeerId,
  refreshGroups,
  refreshNodeStatus,
  refreshGroupMembers,
  setActiveGroupId,
}: UseChatEventsOptions) {
  const pushMessage = useChatStore((s) => s.pushMessage)
  const upsertPublishedMessage = useChatStore((s) => s.upsertPublishedMessage)
  const pushPost = useChatStore((s) => s.pushPost)
  const pushComment = useChatStore((s) => s.pushComment)
  const markGroupRead = useChatStore((s) => s.markGroupRead)
  const incrementUnread = useChatStore((s) => s.incrementUnread)
  const groups = useGroupsStore((s) => s.groups)
  const setGroups = useGroupsStore((s) => s.setGroups)
  const lastGroupsRefreshAtRef = useRef(0)
  const groupsVisualRefreshTimerRef = useRef<number | null>(null)

  const scheduleGroupsVisualRefresh = useCallback(() => {
    if (groupsVisualRefreshTimerRef.current != null) {
      window.clearTimeout(groupsVisualRefreshTimerRef.current)
    }
    groupsVisualRefreshTimerRef.current = window.setTimeout(() => {
      groupsVisualRefreshTimerRef.current = null
      void refreshGroups()
    }, 120)
  }, [refreshGroups])

  useEffect(
    () => () => {
      if (groupsVisualRefreshTimerRef.current != null) {
        window.clearTimeout(groupsVisualRefreshTimerRef.current)
      }
    },
    [],
  )

  const handleGroupMessage = useCallback(
    (payload: service.MessageInfo) => {
      const message = messageInfoToChatMessage(payload)
      const targetGroup = message.groupId
      
      try {
        const parsed = JSON.parse(message.content)
        if (parsed.type === 'post') {
          pushPost(targetGroup, message)
        } else if (parsed.type === 'comment' || parsed.type === 'reply') {
          const postId = parsed.post_id || parsed.parent_id
          if (postId) {
            pushComment(postId, message)
          } else {
            pushMessage(targetGroup, message)
          }
        } else {
          upsertPublishedMessage(targetGroup, message)
        }
      } catch {
        upsertPublishedMessage(targetGroup, message)
      }

      if (targetGroup !== activeGroupId) {
        incrementUnread(targetGroup)
      } else {
        markGroupRead(targetGroup)
      }
      const now = Date.now()
      if (now - lastGroupsRefreshAtRef.current > 800) {
        lastGroupsRefreshAtRef.current = now
        void refreshGroups()
      }
    },
    [activeGroupId, incrementUnread, markGroupRead, pushPost, pushComment, upsertPublishedMessage, refreshGroups],
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
        await refreshGroupMembers(payload.group_id)
      }
    },
    [refreshGroupMembers, refreshGroups, setActiveGroupId],
  )

  const handleMembersChanged = useCallback(
    async (payload: GroupMembersChangedPayload) => {
      if (!payload?.group_id) return
      const reason = String(payload.reason ?? '')
      const isVisualOnly =
        reason === 'profile_push' || reason === 'group_avatar' || reason === 'presence'
      const affectsGroupTopology =
        reason === 'created' ||
        reason === 'joined' ||
        reason === 'joined_ack' ||
        reason === 'invited' ||
        reason === 'removed' ||
        reason === 'removed_self' ||
        reason === 'epoch_reconcile'
      if (isVisualOnly) {
        scheduleGroupsVisualRefresh()
      }
      if (affectsGroupTopology) {
        await refreshGroups()
      }
      if (payload.group_id === activeGroupId) {
        await refreshGroupMembers(payload.group_id)
      }
      // Local removal is handled by handleGroupLeft to avoid duplicate toasts
      if (payload.reason === 'removed' && payload.target_peer_id === localPeerId) {
        await refreshGroups()
        setActiveGroupId(null)
      }
    },
    [activeGroupId, localPeerId, refreshGroupMembers, refreshGroups, scheduleGroupsVisualRefresh, setActiveGroupId],
  )

  const handleGroupLeft = useCallback(
    async (payload: GroupLeftPayload) => {
      await refreshGroups()
      if (payload?.group_id && payload.group_id === activeGroupId) {
        setActiveGroupId(null)
      }
      if (payload?.reason === 'removed') {
        useToastStore.getState().pushToast({
          title: 'Group access revoked',
          description: `You have been removed from group ${payload.group_id}.`,
          variant: 'destructive',
        })
      }
    },
    [activeGroupId, refreshGroups, setActiveGroupId],
  )

  const handleInviteAutoJoined = useCallback(
    async (payload: InviteAutoJoinedPayload) => {
      if (!payload?.group_id) return
      await refreshGroups()
      await refreshGroupMembers(payload.group_id)
      
      // Toast notification is now handled exclusively by handleNotificationNew 
      // to avoid duplicate popups (event-driven vs notification-driven).
    },
    [refreshGroupMembers, refreshGroups],
  )

  const handleNodeStatusChanged = useCallback(async () => {
    await refreshNodeStatus()
    if (activeGroupId) {
      await refreshGroupMembers(activeGroupId)
    }
  }, [activeGroupId, refreshGroupMembers, refreshNodeStatus])

  const handleFilePrepare = useCallback((payload: { group_id?: string; file_id?: string; bytes?: number }) => {
    // Deliberately silent: local UI already shows attaching state, avoid noisy duplicate toasts.
    void payload
  }, [])

  const handleFileSent = useCallback((payload: { group_id?: string; file_id?: string; peer?: string }) => {
    // Deliberately silent: metadata send is expected and frequent.
    void payload
  }, [])

  const handleFileReceived = useCallback((payload: { group_id?: string; file_id?: string; path?: string }) => {
    if (!payload?.file_id) return
    useToastStore.getState().pushToast({
      title: 'File download successful',
      description: payload.path ? `Saved to ${payload.path}` : `Download finished for ${payload.file_id}.`,
      variant: 'default',
    })
  }, [])

  const handleNotificationNew = useCallback(
    async (payload: NotificationNewPayload) => {
      await useNotificationStore.getState().fetchUnreadCount()
      
      if (activeModule === 'activity') {
        await useNotificationStore.getState().fetchNotifications()
      } else {
        // Show Toast for the new notification
        const actor = payload.actor_name || shortPeerId(payload.actor_id)
        let title = 'New notification'
        if (payload.type === 'mention') title = `${actor} mentioned you`
        else if (payload.type === 'reply') title = `${actor} replied to you`
        else if (payload.type === 'group_add') title = `${actor} added you to a group`
        else if (payload.type === 'invite_request') title = `${actor} requested to join group`
        else if (payload.type === 'invite_approved') title = `Join request approved`
        else if (payload.type === 'invite_rejected') title = `Join request rejected`

        useToastStore.getState().pushToast({
          title,
          description: payload.content || 'Click to view details',
          variant: 'default',
        })
      }
    },
    [activeModule],
  )

  useWailsEvent<service.MessageInfo>('group:message', handleGroupMessage)
  useWailsEvent<GroupEpochPayload>('group:epoch', handleGroupEpoch)
  useWailsEvent<{ group_id: string }>('group:joined', handleGroupJoined)
  useWailsEvent<GroupLeftPayload>('group:left', handleGroupLeft)
  useWailsEvent<GroupMembersChangedPayload>('group:members_changed', handleMembersChanged)
  useWailsEvent<InviteAutoJoinedPayload>('invite:auto_joined', handleInviteAutoJoined)
  useWailsEvent('node:status', handleNodeStatusChanged)
  useWailsEvent('p2p:status', handleNodeStatusChanged)
  useWailsEvent<{ group_id?: string; file_id?: string; bytes?: number }>('file:prepare', handleFilePrepare)
  useWailsEvent<{ group_id?: string; file_id?: string; peer?: string }>('file:sent', handleFileSent)
  useWailsEvent<{ group_id?: string; file_id?: string; path?: string }>('file:received', handleFileReceived)
  useWailsEvent<NotificationNewPayload>('notification:new', handleNotificationNew)
}
