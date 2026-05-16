import { useCallback, useEffect, useRef } from 'react'
import { service } from '../../../../wailsjs/go/models'
import { useWailsEvent } from '../../../hooks/useWailsEvent'
import { messageInfoToChatMessage } from '../../../lib/chatModel'
import { useChatStore } from '../../../stores/useChatStore'
import { useContactStore } from '../../../stores/useContactStore'
import { useGroupsStore } from '../../../stores/useGroupsStore'
import { useToastStore } from '../../../stores/useToastStore'
import { GroupEpochPayload, GroupLeftPayload, GroupMembersChangedPayload } from './chatTypes'

interface InviteAutoJoinedPayload {
  id?: string
  group_id?: string
  group_type?: string
  inviter_peer?: string
}

interface UseChatEventsOptions {
  activeGroupId: string | null
  localPeerId: string
  refreshGroups: () => Promise<void>
  refreshNodeStatus: () => Promise<void>
  refreshGroupMembers: (groupId: string) => Promise<void>
  setActiveGroupId: (groupId: string | null) => void
}

export function useChatEvents({
  activeGroupId,
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
      if (reason === 'profile_push' || reason === 'group_avatar' || reason === 'presence') {
        scheduleGroupsVisualRefresh()
      }
      if (payload.group_id === activeGroupId) {
        await refreshGroupMembers(payload.group_id)
      }
      if (payload.reason === 'removed' && payload.target_peer_id === localPeerId) {
        await refreshGroups()
        setActiveGroupId(null)
        useToastStore.getState().pushToast({
          title: 'Bạn đã bị xóa khỏi nhóm',
          description: `Không còn quyền truy cập nhóm ${payload.group_id}.`,
          variant: 'destructive',
        })
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
          title: 'Quyền truy cập nhóm đã bị thu hồi',
          description: `Bạn đã bị xóa khỏi nhóm ${payload.group_id}.`,
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
      const inviter = (payload.inviter_peer ?? '').trim()
      const inviterName = inviter ? useContactStore.getState().getDisplayName(inviter) : ''
      const groupKind = payload.group_type === 'dm' ? 'cuộc trò chuyện' : 'nhóm'
      useToastStore.getState().pushToast({
        title: 'Bạn vừa được thêm vào nhóm',
        description: inviterName
          ? `${inviterName} đã thêm bạn vào ${groupKind} ${payload.group_id}.`
          : `Bạn đã được thêm vào ${groupKind} ${payload.group_id}.`,
        variant: 'default',
      })
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
      title: 'Tải tệp thành công',
      description: payload.path ? `Đã lưu vào ${payload.path}` : `Đã tải xong tệp ${payload.file_id}.`,
      variant: 'default',
    })
  }, [])

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
}
