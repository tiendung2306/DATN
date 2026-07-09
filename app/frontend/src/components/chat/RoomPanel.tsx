import { useCallback, useEffect, useRef, useState, type ChangeEvent } from 'react'
import { service } from '../../../wailsjs/go/models'
import { shortPeerId } from '../../lib/chatModel'
import { formatLeaveGroupError, formatRemoveMemberError } from '../../lib/formatRemoveMemberError'
import { useContactStore } from '../../stores/useContactStore'
import { useAppRuntimeStore } from '../../stores/useAppRuntimeStore'
import { useToastStore } from '../../stores/useToastStore'
import { Button } from '../ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../ui/dialog'
import { LogOut, Settings, Shield, UserMinus, UserPlus, Users, X, ImageIcon, Terminal, Activity, ChevronDown, ChevronUp, RefreshCw, CheckCircle, XCircle, Clock, FileText, Hash, ShieldAlert } from 'lucide-react'
import { runtimeClient } from '../../services/runtime/runtimeClient'
import AddMemberModal from '../../features/chat/components/AddMemberModal'
import { ConversationKind } from '../../lib/chatModel'
import { formatOutboundSendError } from '../../lib/formatSendError'
import { cn } from '@/lib/utils'
import { useWailsEvent } from '../../hooks/useWailsEvent'
import ChatListAvatar from './ChatListAvatar'
import {
  AVATAR_INPUT_MAX_BYTES,
  AVATAR_OUTPUT_MAX_BYTES,
  AvatarImageError,
  compressAvatarFile,
  formatBytesShort,
  type CompressedAvatarResult,
} from '../../lib/avatarImage'

interface RoomPanelProps {
  activeGroupId: string | null
  activeKind: ConversationKind
  myRole?: string
  localPeerId?: string
  peers: service.MemberInfo[]
  onClose: () => void
  setActiveGroupId?: (id: string | null) => void
  refreshGroups?: () => Promise<void>
  /** Current group chat image (data URL) when `activeKind === 'group'`. */
  groupAvatarDataUrl?: string
  conversationTitle?: string
}

type InviteListItem = {
  request_id?: string
  target_peer_id?: string
  requester_peer_id?: string
  status?: string
  created_at?: number
  updated_at?: number
}

type GroupEventPayload = Record<string, any>

const parseGroupEventPayload = (payloadJson?: string): GroupEventPayload => {
  if (!payloadJson) return {}
  try {
    const parsed = JSON.parse(payloadJson)
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

const formatGroupEventLabel = (eventType: string) => {
  switch (eventType) {
    case 'group_created':
      return 'Group created'
    case 'member_joined':
      return 'Member joined'
    case 'member_left':
      return 'Member left'
    case 'proposal_received':
      return 'Proposal observed'
    case 'commit_issued':
      return 'Commit issued'
    case 'add_commit_observed':
      return 'Add commit observed'
    case 'invite_request_created':
      return 'Invite request created'
    case 'invite_request_approved':
      return 'Invite request approved'
    case 'invite_request_rejected':
      return 'Invite request rejected'
    case 'fork_heal_started':
      return 'Fork heal started'
    case 'fork_heal_completed':
      return 'Fork heal completed'
    case 'fork_heal_failed':
      return 'Fork heal failed'
    default:
      return eventType.split('_').join(' ')
  }
}

const eventToneClass = (eventType: string) => {
  switch (eventType) {
    case 'group_created':
    case 'member_joined':
    case 'invite_request_approved':
    case 'fork_heal_completed':
      return 'bg-emerald-500/10 text-emerald-400 border-emerald-500/20'
    case 'commit_issued':
    case 'proposal_received':
    case 'add_commit_observed':
      return 'bg-sky-500/10 text-sky-400 border-sky-500/20'
    case 'fork_heal_started':
    case 'invite_request_created':
      return 'bg-amber-500/10 text-amber-400 border-amber-500/20'
    case 'member_left':
    case 'invite_request_rejected':
    case 'fork_heal_failed':
      return 'bg-rose-500/10 text-rose-400 border-rose-500/20'
    default:
      return 'bg-slate-800 text-slate-300 border-slate-700'
  }
}

export default function RoomPanel({
  activeGroupId,
  activeKind,
  myRole,
  localPeerId,
  peers,
  onClose,
  setActiveGroupId,
  refreshGroups,
  groupAvatarDataUrl = '',
  conversationTitle = '',
}: RoomPanelProps) {
  const getDisplayName = useContactStore((s) => s.getDisplayName)
  const pushToast = useToastStore((s) => s.pushToast)
  const isDevMode = useAppRuntimeStore((s) => s.isDevMode)
  
  const [isAddModalOpen, setIsAddModalOpen] = useState(false)
  const [isLeaving, setIsLeaving] = useState(false)
  const [removingPeerId, setRemovingPeerId] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'members' | 'invites' | 'diagnostics'>('members')
  
  const [diagSnapshot, setDiagSnapshot] = useState<any | null>(null)
  const [healHistory, setHealHistory] = useState<any[]>([])
  const [groupEvents, setGroupEvents] = useState<service.GroupEventLogEntry[]>([])
  const [loadingDiag, setLoadingDiag] = useState(false)
  const [expandedHealTrace, setExpandedHealTrace] = useState<string | null>(null)
  const [expandedEventId, setExpandedEventId] = useState<number | null>(null)
  const diagRequestSeq = useRef(0)

  const resetDiagnosticsView = useCallback(() => {
    setDiagSnapshot(null)
    setHealHistory([])
    setGroupEvents([])
    setExpandedHealTrace(null)
    setExpandedEventId(null)
  }, [])

  const loadGroupDiagData = useCallback(async () => {
    if (!activeGroupId || !isDevMode) {
      resetDiagnosticsView()
      return
    }
    const requestId = ++diagRequestSeq.current
    const groupID = activeGroupId
    setLoadingDiag(true)
    try {
      const snap = await runtimeClient.getDiagnosticsSnapshot()
      if (diagRequestSeq.current !== requestId || groupID !== activeGroupId) return
      const groupSnap = snap.groups?.find((g: any) => g.group_id === groupID)
      setDiagSnapshot(groupSnap || null)
      
      const history = await runtimeClient.getForkHealHistory(groupID, 10)
      if (diagRequestSeq.current !== requestId || groupID !== activeGroupId) return
      setHealHistory(history || [])

      const events = await runtimeClient.getGroupEventLog(groupID, 50)
      if (diagRequestSeq.current !== requestId || groupID !== activeGroupId) return
      setGroupEvents(events || [])
    } catch (err) {
      if (diagRequestSeq.current === requestId) {
        resetDiagnosticsView()
      }
      console.error('Failed to load group diagnostics', err)
    } finally {
      if (diagRequestSeq.current === requestId) {
        setLoadingDiag(false)
      }
    }
  }, [activeGroupId, isDevMode, resetDiagnosticsView])

  useEffect(() => {
    if (activeTab === 'diagnostics' && activeGroupId) {
      resetDiagnosticsView()
      void loadGroupDiagData()
      
      const interval = setInterval(() => {
        void loadGroupDiagData()
      }, 4000)
      return () => clearInterval(interval)
    }
    diagRequestSeq.current += 1
    resetDiagnosticsView()
    setLoadingDiag(false)
  }, [activeTab, activeGroupId, loadGroupDiagData, resetDiagnosticsView])
  const [inviteItems, setInviteItems] = useState<InviteListItem[]>([])
  const [loadingInvites, setLoadingInvites] = useState(false)
  const [changingInviteId, setChangingInviteId] = useState<string | null>(null)
  const [changingAdminPeerId, setChangingAdminPeerId] = useState<string | null>(null)
  const [invitePolicy, setInvitePolicy] = useState<'creator_approval' | 'any_member' | null>(null)
  const [loadingPolicy, setLoadingPolicy] = useState(false)
  const [savingPolicy, setSavingPolicy] = useState(false)
  const [settingsDraftPolicy, setSettingsDraftPolicy] = useState<'creator_approval' | 'any_member'>(
    'creator_approval',
  )
  const [isGroupSettingsOpen, setIsGroupSettingsOpen] = useState(false)
  const localMember = localPeerId ? peers.find((peer) => peer.peer_id === localPeerId) : undefined
  const localIsCreator = Boolean(localMember?.is_creator || myRole === 'creator')
  const localIsAdmin = Boolean(localMember?.is_admin || localIsCreator)
  const canManageGroupSettings = activeKind !== 'dm' && localIsAdmin
  const canManageAdmins = activeKind !== 'dm' && localIsCreator
  const canLeaveGroup = activeKind !== 'dm' && !localIsCreator

  const groupAvatarFileRef = useRef<HTMLInputElement>(null)
  const [pendingGroupCompressedAvatar, setPendingGroupCompressedAvatar] = useState<CompressedAvatarResult | null>(null)
  const [groupAvatarPreviewUrl, setGroupAvatarPreviewUrl] = useState<string | null>(null)
  const [removeGroupAvatarOnSave, setRemoveGroupAvatarOnSave] = useState(false)
  const [groupAvatarProcessing, setGroupAvatarProcessing] = useState(false)

  const revokeGroupAvatarPreview = useCallback(() => {
    setGroupAvatarPreviewUrl((prev) => {
      if (prev) URL.revokeObjectURL(prev)
      return null
    })
  }, [])

  useEffect(() => {
    if (!isGroupSettingsOpen) return
    setPendingGroupCompressedAvatar(null)
    setRemoveGroupAvatarOnSave(false)
    revokeGroupAvatarPreview()
  }, [isGroupSettingsOpen, activeGroupId, revokeGroupAvatarPreview])

  useEffect(() => {
    return () => {
      revokeGroupAvatarPreview()
    }
  }, [revokeGroupAvatarPreview])

  useEffect(() => {
    if (!activeGroupId || activeKind === 'dm') {
      setInvitePolicy(null)
      return
    }
    let alive = true
    ;(async () => {
      setLoadingPolicy(true)
      try {
        const p = await runtimeClient.getGroupInvitePolicy(activeGroupId)
        if (alive) setInvitePolicy(p)
      } catch {
        if (alive) setInvitePolicy('creator_approval')
      } finally {
        if (alive) setLoadingPolicy(false)
      }
    })()
    return () => {
      alive = false
    }
  }, [activeGroupId, activeKind])

  const sortInviteRequestsNewestFirst = (items: InviteListItem[]) =>
    [...items].sort(
      (a, b) =>
        Number(b.created_at ?? b.updated_at ?? 0) - Number(a.created_at ?? a.updated_at ?? 0),
    )

  const loadPendingInviteRequests = async () => {
    if (!activeGroupId || activeKind === 'dm') {
      setInviteItems([])
      return
    }
    setLoadingInvites(true)
    try {
      const statuses = ['pending', 'processing']
      let result = await runtimeClient.listGroupInviteRequests(activeGroupId, statuses, '', 100)
      let raw: InviteListItem[] = Array.isArray(result.items) ? (result.items as InviteListItem[]) : []

      if (invitePolicy === 'creator_approval' && localPeerId) {
        const syncTargets = raw.filter(
          (row) =>
            String(row.requester_peer_id ?? '') === localPeerId &&
            (String(row.status ?? '') === 'pending' || String(row.status ?? '') === 'processing'),
        )
        for (const row of syncTargets) {
          const rid = String(row.request_id ?? '')
          if (!rid) continue
          try {
            await runtimeClient.syncInviteRequestFromCreator(rid)
          } catch {
            /* creator offline — keep cached mirror */
          }
        }
        result = await runtimeClient.listGroupInviteRequests(activeGroupId, statuses, '', 100)
        raw = Array.isArray(result.items) ? (result.items as InviteListItem[]) : []
      }

      setInviteItems(sortInviteRequestsNewestFirst(raw))
    } catch (err) {
      pushToast(formatOutboundSendError(err))
    } finally {
      setLoadingInvites(false)
    }
  }

  useEffect(() => {
    if (!activeGroupId || activeKind === 'dm') {
      setInviteItems([])
      return
    }
    let cancelled = false
    void loadPendingInviteRequests().finally(() => {
      if (cancelled) return
    })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeGroupId, activeKind])

  useEffect(() => {
    if (!activeGroupId || activeKind === 'dm' || activeTab !== 'invites') return
    void loadPendingInviteRequests()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab, invitePolicy, localPeerId])

  useEffect(() => {
    if (!isGroupSettingsOpen || invitePolicy === null) return
    setSettingsDraftPolicy(invitePolicy)
  }, [isGroupSettingsOpen, invitePolicy])

  useWailsEvent<{ group_id?: string; policy?: string }>('group:invite_policy_changed', (payload) => {
    if (!activeGroupId || payload?.group_id !== activeGroupId) return
    const nextPolicy = payload?.policy === 'any_member' ? 'any_member' : 'creator_approval'
    setInvitePolicy(nextPolicy)
    setSettingsDraftPolicy((prev) => (prev === 'any_member' || prev === 'creator_approval' ? nextPolicy : prev))
  })

  const hasStoredAvatar = Boolean(String(groupAvatarDataUrl || '').trim())
  const avatarDirty =
    activeKind === 'group' &&
    canManageGroupSettings &&
    (pendingGroupCompressedAvatar !== null || (removeGroupAvatarOnSave && hasStoredAvatar))
  const policyDirty = invitePolicy !== null && settingsDraftPolicy !== invitePolicy
  const settingsDirty = policyDirty || avatarDirty

  const handlePickGroupAvatarClick = () => groupAvatarFileRef.current?.click()

  const handleGroupAvatarFileChange = (e: ChangeEvent<HTMLInputElement>) => {
    const f = e.target.files?.[0]
    e.target.value = ''
    if (!f) return

    void (async () => {
      setGroupAvatarProcessing(true)
      try {
        const out = await compressAvatarFile(f)
        if (out.outputBytes > AVATAR_OUTPUT_MAX_BYTES) {
          throw new AvatarImageError(`Processed image still exceeds ${formatBytesShort(AVATAR_OUTPUT_MAX_BYTES)}.`)
        }
        setRemoveGroupAvatarOnSave(false)
        revokeGroupAvatarPreview()
        setGroupAvatarPreviewUrl(URL.createObjectURL(out.blob))
        setPendingGroupCompressedAvatar(out)
        if (out.wasCompressed) {
          pushToast({
            title: 'Avatar optimized',
            description: `${formatBytesShort(out.originalBytes)} → ${formatBytesShort(out.outputBytes)} (${out.width}×${out.height}). Click Save to apply.`,
            variant: 'default',
          })
        }
      } catch (err) {
        const msg = err instanceof AvatarImageError ? err.message : err instanceof Error ? err.message : String(err)
        pushToast({ title: 'Failed to process image', description: msg, variant: 'destructive' })
        setPendingGroupCompressedAvatar(null)
        revokeGroupAvatarPreview()
      } finally {
        setGroupAvatarProcessing(false)
      }
    })()
  }

  const handleDiscardGroupAvatarDraft = () => {
    setPendingGroupCompressedAvatar(null)
    revokeGroupAvatarPreview()
  }

  const handleMarkRemoveGroupAvatar = () => {
    setPendingGroupCompressedAvatar(null)
    revokeGroupAvatarPreview()
    setRemoveGroupAvatarOnSave(true)
  }

  const displayGroupAvatarSrc =
    groupAvatarPreviewUrl || (removeGroupAvatarOnSave ? '' : String(groupAvatarDataUrl || '').trim()) || ''

  const handleCloseGroupSettings = () => {
    if (invitePolicy !== null) {
      setSettingsDraftPolicy(invitePolicy)
    }
    setPendingGroupCompressedAvatar(null)
    setRemoveGroupAvatarOnSave(false)
    revokeGroupAvatarPreview()
    setIsGroupSettingsOpen(false)
  }

  const handleSaveGroupSettings = async () => {
    if (!activeGroupId) return
    const dirty = policyDirty || avatarDirty

    if (!dirty) {
      setIsGroupSettingsOpen(false)
      return
    }

    const pendingGroupSnap = pendingGroupCompressedAvatar
    setSavingPolicy(true)
    try {
      if (avatarDirty) {
        let avatarChange = 0
        let avatarBytes: number[] = []
        if (removeGroupAvatarOnSave) {
          avatarChange = 2
        } else if (pendingGroupSnap) {
          if (pendingGroupSnap.outputBytes > AVATAR_OUTPUT_MAX_BYTES) {
            pushToast({
              title: 'Image too large',
              description: `Must be ≤ ${formatBytesShort(AVATAR_OUTPUT_MAX_BYTES)} after compression.`,
              variant: 'destructive',
            })
            return
          }
          avatarBytes = pendingGroupSnap.bytes
          avatarChange = 1
        }
        await runtimeClient.saveGroupChatAvatar(activeGroupId, avatarBytes, avatarChange)
        setPendingGroupCompressedAvatar(null)
        setRemoveGroupAvatarOnSave(false)
        revokeGroupAvatarPreview()
        const desc =
          avatarChange === 1 && pendingGroupSnap?.wasCompressed
            ? `Group avatar saved (${formatBytesShort(pendingGroupSnap.outputBytes)}). Shown on sidebar and header (stored locally on this device).`
            : 'Avatar updated and stored locally on this device.'
        pushToast({
          title: 'Avatar updated',
          description: desc,
          variant: 'default',
        })
        if (refreshGroups) await refreshGroups()
      }

      if (policyDirty) {
        if (invitePolicy === null) {
          pushToast({
            title: 'Policy not loaded',
            description: 'Wait for loading to complete or try reopening settings.',
            variant: 'destructive',
          })
          return
        }
        await runtimeClient.setGroupInvitePolicy(activeGroupId, settingsDraftPolicy)
        setInvitePolicy(settingsDraftPolicy)
        pushToast({
          title: 'Settings saved',
          description:
            settingsDraftPolicy === 'any_member'
              ? 'Members can now invite new people directly.'
              : 'Only the creator can approve new member requests.',
          variant: 'default',
        })
        await loadPendingInviteRequests()
      }

      setIsGroupSettingsOpen(false)
    } catch (err) {
      pushToast(formatOutboundSendError(err))
    } finally {
      setSavingPolicy(false)
    }
  }

  const handleLeaveGroup = async () => {
    if (!activeGroupId) return
    if (!confirm('Are you sure you want to leave this group?')) return
    setIsLeaving(true)
    try {
      await runtimeClient.leaveGroup(activeGroupId)
      if (setActiveGroupId) setActiveGroupId(null)
      if (refreshGroups) await refreshGroups()
      pushToast({ title: 'Left group', description: 'You have left the group successfully.', variant: 'default' })
    } catch (e) {
      console.error('Failed to leave group', e)
      pushToast(formatLeaveGroupError(e))
    } finally {
      setIsLeaving(false)
    }
  }

  const handleRemoveMember = async (peerId: string) => {
    const target = peers.find((peer) => peer.peer_id === peerId)
    if (!activeGroupId || !target?.can_remove || !peerId || peerId === localPeerId) return
    const displayName = getDisplayName(peerId)
    if (!confirm(`Are you sure you want to remove member ${displayName} from group?`)) return
    setRemovingPeerId(peerId)
    try {
      await runtimeClient.removeMemberFromGroup(activeGroupId, peerId)
      if (refreshGroups) await refreshGroups()
      pushToast({
        title: 'Member removed',
        description: `${displayName} has been removed from the group.`,
        variant: 'default',
      })
    } catch (e) {
      console.error('Failed to remove member', e)
      pushToast(formatRemoveMemberError(e))
    } finally {
      setRemovingPeerId(null)
    }
  }

  const handleSetMemberAdmin = async (peerId: string, isAdmin: boolean) => {
    if (!activeGroupId || !canManageAdmins || !peerId || peerId === localPeerId) return
    const target = peers.find((peer) => peer.peer_id === peerId)
    if (!target || target.is_creator) return
    const displayName = getDisplayName(peerId)
    const verb = isAdmin ? 'make admin' : 'revoke admin from'
    if (!confirm(`Are you sure you want to ${verb} ${displayName}?`)) return
    setChangingAdminPeerId(peerId)
    try {
      await runtimeClient.setGroupMemberAdmin(activeGroupId, peerId, isAdmin)
      if (refreshGroups) await refreshGroups()
      pushToast({
        title: isAdmin ? 'Admin granted' : 'Admin revoked',
        description: isAdmin
          ? `${displayName} can now approve invites, change group settings, and remove regular members.`
          : `${displayName} is now a regular member.`,
        variant: 'default',
      })
    } catch (err) {
      pushToast(formatOutboundSendError(err))
    } finally {
      setChangingAdminPeerId(null)
    }
  }

  const handleInviteAction = async (requestId: string, action: 'approve' | 'reject') => {
    if (!requestId) return
    setChangingInviteId(requestId)
    try {
      if (action === 'approve') await runtimeClient.approveGroupInviteRequest(requestId)
      if (action === 'reject') await runtimeClient.rejectGroupInviteRequest(requestId, '')
      await loadPendingInviteRequests()
    } catch (err) {
      pushToast(formatOutboundSendError(err))
    } finally {
      setChangingInviteId(null)
    }
  }

  const pendingInviteBadgeCount =
    activeKind !== 'dm' ? inviteItems.length : 0

  return (
    <aside className="flex w-80 shrink-0 flex-col border-l border-slate-800 bg-slate-950">
      <div className="mb-4 flex items-center justify-between border-b border-slate-800 px-4 py-4">
        <div className="min-w-0 flex-1 pr-2">
          <p className="text-sm font-semibold text-slate-100 truncate">
            {activeKind === 'dm' ? 'Direct message details' : 'Group details'}
          </p>
          <p className="text-xs text-slate-400 truncate">
            {conversationTitle || activeGroupId || 'No group selected'}
          </p>
        </div>
        <button
          type="button"
          aria-label="Close details"
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-slate-400 hover:bg-slate-800 hover:text-slate-100"
          onClick={onClose}
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      <div className="px-4">
        {activeKind !== 'dm' ? (
          <div className={cn(
            "mb-3 grid gap-2 rounded-lg border border-slate-800 p-1",
            isDevMode ? "grid-cols-3" : "grid-cols-2"
          )}>
            <Button
              type="button"
              variant={activeTab === 'members' ? 'secondary' : 'ghost'}
              className="h-8 min-h-0 gap-0 px-1.5 text-xs"
              onClick={() => setActiveTab('members')}
            >
              Members
            </Button>
            <Button
              type="button"
              variant={activeTab === 'invites' ? 'secondary' : 'ghost'}
              className="h-8 min-h-0 gap-1.5 px-1.5 text-xs"
              onClick={() => setActiveTab('invites')}
            >
              <span className="truncate">Join Requests</span>
              {pendingInviteBadgeCount > 0 ? (
                <span className="flex h-5 min-w-5 shrink-0 items-center justify-center rounded-full bg-emerald-600 px-1 text-[10px] font-semibold tabular-nums text-white">
                  {pendingInviteBadgeCount > 99 ? '99+' : pendingInviteBadgeCount}
                </span>
              ) : null}
            </Button>
            {isDevMode && (
              <Button
                type="button"
                variant={activeTab === 'diagnostics' ? 'secondary' : 'ghost'}
                className="h-8 min-h-0 gap-1.5 px-1.5 text-xs text-amber-400 hover:text-amber-300"
                onClick={() => setActiveTab('diagnostics')}
              >
                Dev Mode
              </Button>
            )}
          </div>
        ) : null}
        {activeTab === 'members' || activeKind === 'dm' ? (
          <>
            <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
              <Users className="h-3.5 w-3.5" />
              <span>Members</span>
            </div>
            <div className="space-y-2 pb-4">
              {peers.length === 0 ? (
                <p className="text-xs text-slate-500">No members available.</p>
              ) : (
                peers.map((peer) => (
                  <div
                    key={peer.peer_id}
                    className="flex items-center justify-between rounded-md border border-slate-800 bg-slate-900/60 px-2 py-2"
                  >
                    <div className="flex min-w-0 flex-1 items-center gap-2">
                      <ChatListAvatar
                        variant="dm"
                        displayName={getDisplayName(peer.peer_id)}
                        imageUrl={peer.avatar_data_url}
                        size="sm"
                      />
                      <div className="min-w-0">
                        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
                          <p className="truncate text-xs font-medium text-slate-200">{getDisplayName(peer.peer_id)}</p>
                          {peer.is_creator ? (
                            <span className="rounded-full border border-amber-500/30 bg-amber-500/10 px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide text-amber-300">
                              Creator
                            </span>
                          ) : peer.is_admin ? (
                            <span className="rounded-full border border-sky-500/30 bg-sky-500/10 px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide text-sky-300">
                              Admin
                            </span>
                          ) : null}
                        </div>
                        <p className="text-[11px] text-slate-500">{shortPeerId(peer.peer_id)}</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      {peer.can_remove ? (
                        <button
                          type="button"
                          aria-label={`Remove ${getDisplayName(peer.peer_id)} from group`}
                          disabled={removingPeerId === peer.peer_id}
                          className="rounded p-1 text-slate-400 transition hover:bg-red-500/10 hover:text-red-300 disabled:cursor-not-allowed disabled:opacity-50"
                          onClick={() => void handleRemoveMember(peer.peer_id)}
                        >
                          <UserMinus className="h-3.5 w-3.5" />
                        </button>
                      ) : null}
                      <span
                        className={`h-2 w-2 rounded-full ${
                          peer.is_online ? 'bg-emerald-400' : 'bg-slate-500'
                        }`}
                        title={peer.is_online ? 'online' : 'offline'}
                      />
                    </div>
                  </div>
                ))
              )}
            </div>
          </>
        ) : activeTab === 'invites' ? (
          <>
            <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
              <Users className="h-3.5 w-3.5" />
              <span>Join Requests</span>
            </div>
            <div className="space-y-2 pb-4">
              {loadingInvites ? (
                <p className="text-xs text-slate-500">Loading...</p>
              ) : null}
              {!loadingInvites && inviteItems.length === 0 ? (
                <p className="text-xs text-slate-500">No pending requests.</p>
              ) : null}
              {inviteItems.map((item) => {
                const requestId = String(item.request_id ?? '')
                const targetPeer = String(item.target_peer_id ?? '')
                const requesterPeer = String(item.requester_peer_id ?? '')
                const isBusy = changingInviteId === requestId
                const canReviewInvite = canManageGroupSettings
                return (
                  <div key={requestId} className="rounded-md border border-slate-800 bg-slate-900/60 px-2 py-2">
                    <p className="text-xs font-medium text-slate-200">{getDisplayName(targetPeer)}</p>
                    <p className="text-[11px] text-slate-500">{shortPeerId(targetPeer)}</p>
                    {requesterPeer ? (
                      <p className="mt-1 text-[11px] text-slate-400">
                        Requested by: {getDisplayName(requesterPeer)}
                      </p>
                    ) : null}
                    {canReviewInvite ? (
                      <div className="mt-2 flex flex-wrap gap-2">
                        <Button
                          size="sm"
                          className="h-7 text-[11px]"
                          disabled={isBusy}
                          onClick={() => void handleInviteAction(requestId, 'approve')}
                        >
                          Approve
                        </Button>
                        <Button
                          size="sm"
                          variant="secondary"
                          className="h-7 text-[11px]"
                          disabled={isBusy}
                          onClick={() => void handleInviteAction(requestId, 'reject')}
                        >
                          Reject
                        </Button>
                      </div>
                    ) : (
                      <p className="mt-2 text-[11px] text-slate-500">Awaiting admin approval.</p>
                    )}
                  </div>
                )
              })}
            </div>
          </>
        ) : activeTab === 'diagnostics' && isDevMode ? (
          <div className="flex-1 overflow-y-auto pr-1 space-y-4 pb-6" style={{ maxHeight: 'calc(100vh - 24rem)' }}>
            {/* MLS Metadata */}
            <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-3.5 space-y-3">
              <div className="flex items-center gap-2 border-b border-slate-800/80 pb-2 text-[11px] font-semibold uppercase tracking-wider text-amber-400">
                <Terminal className="h-3.5 w-3.5" />
                <span>MLS Group Cryptographics</span>
              </div>
              
              <div className="grid grid-cols-2 gap-2 text-xs">
                <div className="rounded-lg bg-slate-950 p-2 border border-slate-800/50">
                  <span className="block text-[10px] text-slate-500 uppercase tracking-tight">Current Epoch</span>
                  <span className="font-mono text-sm font-semibold text-emerald-400 tracking-wide">
                    e{diagSnapshot?.epoch ?? 0}
                  </span>
                </div>
                <div className="rounded-lg bg-slate-950 p-2 border border-slate-800/50">
                  <span className="block text-[10px] text-slate-500 uppercase tracking-tight">Consensus Size</span>
                  <span className="font-mono text-sm font-semibold text-sky-400">
                    {diagSnapshot?.active_members ?? peers.length} nodes
                  </span>
                </div>
              </div>

              <div className="rounded-lg bg-slate-950 p-2 border border-slate-800/50 text-xs">
                <span className="block text-[10px] text-slate-500 uppercase tracking-tight mb-1">Tree Hash</span>
                {diagSnapshot?.tree_hash_hex ? (
                  <div className="space-y-1">
                    <span className="font-mono font-bold text-amber-500 px-1 py-0.5 rounded bg-amber-950/20 text-[10px] border border-amber-900/20">
                      {diagSnapshot.tree_hash_short}
                    </span>
                    <span className="block font-mono text-[9px] text-slate-400 select-all break-all leading-normal pt-1 border-t border-slate-900">
                      {diagSnapshot.tree_hash_hex}
                    </span>
                  </div>
                ) : (
                  <span className="text-slate-500 font-mono text-[10px]">No tree hash generated</span>
                )}
              </div>

              <div className="rounded-lg bg-slate-950 p-2 border border-slate-800/50 text-xs">
                <span className="block text-[10px] text-slate-500 uppercase tracking-tight mb-1">Token Holder</span>
                {diagSnapshot?.token_holder_peer_id ? (
                  <div className="flex flex-col gap-0.5 min-w-0">
                    <div className="flex items-center gap-1.5 min-w-0">
                      <span className="truncate font-semibold text-slate-200">
                        {getDisplayName(diagSnapshot.token_holder_peer_id)}
                      </span>
                      {diagSnapshot.token_holder_peer_id === localPeerId && (
                        <span className="shrink-0 text-[9px] font-semibold bg-emerald-500/10 text-emerald-400 border border-emerald-500/20 px-1 py-px rounded">
                          Me
                        </span>
                      )}
                    </div>
                    <span className="font-mono text-[9px] text-slate-400 select-all truncate">
                      {diagSnapshot.token_holder_peer_id}
                    </span>
                  </div>
                ) : (
                  <span className="text-slate-500 font-mono text-[10px]">Electing Token Holder...</span>
                )}
              </div>

              {/* Liveness Overlay */}
              <div className="space-y-1 text-xs">
                <span className="block text-[10px] text-slate-500 uppercase tracking-tight">Consensus Online View</span>
                <div className="flex flex-wrap gap-1 pt-1 max-h-24 overflow-y-auto">
                  {peers.map((peer) => {
                    const isConsensusActive = diagSnapshot?.active_view?.includes(peer.peer_id) ?? true;
                    return (
                      <span
                        key={peer.peer_id}
                        className={cn(
                          "inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] font-medium border",
                          peer.is_online
                            ? isConsensusActive
                              ? "bg-emerald-500/10 text-emerald-400 border-emerald-500/20"
                              : "bg-amber-500/10 text-amber-400 border-amber-500/20"
                            : "bg-slate-900 text-slate-500 border-slate-800"
                        )}
                        title={`${getDisplayName(peer.peer_id)}: ${
                          peer.is_online ? (isConsensusActive ? "Online & Consensus Active" : "Online but Out-of-sync") : "Offline"
                        }`}
                      >
                        <span className={cn("h-1 w-1 rounded-full", peer.is_online ? "bg-emerald-400" : "bg-slate-500")} />
                        {getDisplayName(peer.peer_id).substring(0, 10)}
                      </span>
                    );
                  })}
                </div>
              </div>

              {diagSnapshot?.is_healing && (
                <div className="flex items-center gap-2 p-2 rounded bg-amber-500/10 text-amber-400 border border-amber-500/20 animate-pulse text-[11px] font-medium">
                  <RefreshCw className="h-3.5 w-3.5 animate-spin" />
                  <span>Fork healing active. Recovering consistency...</span>
                </div>
              )}
            </div>

            <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-3.5 space-y-3">
              <div className="flex items-center gap-2 border-b border-slate-800/80 pb-2 text-[11px] font-semibold uppercase tracking-wider text-sky-400">
                <Activity className="h-3.5 w-3.5" />
                <span>Group Event Log</span>
              </div>

              {groupEvents.length === 0 ? (
                <div className="rounded-lg border border-slate-800 bg-slate-950 px-3 py-4 text-center">
                  <p className="text-xs font-medium text-slate-300">Chua co su kien coordination nao</p>
                  <p className="mt-1 text-[10px] text-slate-500">Log se hien thi proposal, commit, invite request va fork-heal moi nhat.</p>
                </div>
              ) : (
                <div className="space-y-2">
                  {groupEvents.map((event) => {
                    const payload = parseGroupEventPayload(event.payload_json)
                    const isCommit = event.event_type === 'commit_issued'
                    const traceId = typeof payload.trace_id === 'string' ? payload.trace_id : ''
                    const linkedHeal = traceId ? healHistory.find((item: any) => item.trace_id === traceId) : null
                    const expandable = isCommit || Boolean(linkedHeal)
                    const isExpanded = expandedEventId === event.id
                    const actorName = event.actor_peer_id ? getDisplayName(event.actor_peer_id) : ''
                    const targetName = event.target_peer_id ? getDisplayName(event.target_peer_id) : ''
                    let summary = formatGroupEventLabel(event.event_type)

                    if (event.event_type === 'group_created') {
                      summary = `${actorName || 'Creator'} initialized MLS state`
                    } else if (event.event_type === 'member_joined') {
                      summary = `${targetName || actorName || 'Member'} joined the group`
                    } else if (event.event_type === 'member_left') {
                      summary = `${targetName || actorName || 'Member'} left (${payload.reason || 'unknown'})`
                    } else if (event.event_type === 'proposal_received') {
                      summary = `${actorName || 'Member'} submitted ${payload.proposal_type || 'unknown'} proposal`
                    } else if (event.event_type === 'commit_issued') {
                      summary = `${actorName || 'Token holder'} issued commit for epoch ${event.epoch}`
                    } else if (event.event_type === 'add_commit_observed') {
                      summary = `Add commit observed for ${targetName || payload.target_peer_id || 'invitee'}`
                    } else if (event.event_type === 'invite_request_created') {
                      summary = `${actorName || payload.requester_peer_id || 'Member'} requested invite for ${targetName || payload.target_peer_id || 'target'}`
                    } else if (event.event_type === 'invite_request_approved') {
                      summary = `Invite approved for ${targetName || payload.target_peer_id || 'target'}`
                    } else if (event.event_type === 'invite_request_rejected') {
                      summary = `Invite rejected for ${targetName || payload.target_peer_id || 'target'}`
                    } else if (event.event_type === 'fork_heal_started') {
                      summary = `Fork heal started from winner epoch ${payload.winner_epoch ?? 'unknown'}`
                    } else if (event.event_type === 'fork_heal_completed') {
                      summary = `Fork heal completed at epoch ${payload.new_epoch ?? event.epoch}`
                    } else if (event.event_type === 'fork_heal_failed') {
                      summary = `Fork heal failed at ${payload.failed_step || 'unknown step'}`
                    }

                    return (
                      <div key={event.id} className="overflow-hidden rounded-lg border border-slate-800 bg-slate-950">
                        <button
                          type="button"
                          disabled={!expandable}
                          className={cn(
                            'flex w-full items-start justify-between gap-3 p-2.5 text-left',
                            expandable ? 'transition-colors hover:bg-slate-900' : 'cursor-default',
                          )}
                          onClick={() => {
                            if (!expandable) return
                            setExpandedEventId(isExpanded ? null : event.id)
                          }}
                        >
                          <div className="min-w-0 flex-1 space-y-1">
                            <div className="flex items-center gap-2 flex-wrap">
                              <span className={cn('rounded-[4px] border px-1.5 py-0.5 text-[9px] font-bold uppercase tracking-wide', eventToneClass(event.event_type))}>
                                {formatGroupEventLabel(event.event_type)}
                              </span>
                              {event.epoch > 0 ? (
                                <span className="text-[10px] font-mono text-slate-400">e{event.epoch}</span>
                              ) : null}
                            </div>
                            <p className="text-[11px] font-medium leading-relaxed text-slate-200">{summary}</p>
                            <div className="flex flex-wrap items-center gap-2 text-[10px] text-slate-500">
                              <span>{new Date(event.created_at_ms).toLocaleTimeString()}</span>
                              {payload.request_id ? (
                                <>
                                  <span>•</span>
                                  <span>Req {String(payload.request_id).slice(0, 8)}</span>
                                </>
                              ) : null}
                              {payload.operation_id ? (
                                <>
                                  <span>•</span>
                                  <span>Op {String(payload.operation_id).slice(0, 8)}</span>
                                </>
                              ) : null}
                            </div>
                          </div>
                          {expandable ? (
                            isExpanded ? <ChevronUp className="h-4 w-4 shrink-0 text-slate-400" /> : <ChevronDown className="h-4 w-4 shrink-0 text-slate-400" />
                          ) : null}
                        </button>

                        {isExpanded ? (
                          <div className="space-y-2 border-t border-slate-900 bg-slate-950 p-2.5 text-[11px] leading-relaxed">
                            {isCommit ? (
                              <div className="space-y-2">
                                <div className="grid grid-cols-2 gap-2">
                                  <div>
                                    <span className="block text-[9px] uppercase tracking-tight text-slate-500">Token Holder</span>
                                    <span className="block truncate font-mono text-slate-300">{actorName || event.actor_peer_id || 'unknown'}</span>
                                  </div>
                                  <div>
                                    <span className="block text-[9px] uppercase tracking-tight text-slate-500">Epoch Transition</span>
                                    <span className="block font-mono text-slate-300">
                                      e{payload.previous_epoch ?? '?'} → e{payload.new_epoch ?? event.epoch}
                                    </span>
                                  </div>
                                </div>
                                <div className="space-y-1">
                                  <span className="block text-[9px] uppercase tracking-tight text-slate-500">Commit Proposals</span>
                                  {Array.isArray(payload.proposals) && payload.proposals.length > 0 ? (
                                    <div className="space-y-1">
                                      {payload.proposals.map((proposal: any, idx: number) => (
                                        <div key={`${event.id}-${idx}`} className="rounded border border-slate-800 bg-slate-900/50 px-2 py-1.5">
                                          <div className="flex flex-wrap items-center gap-2 text-[10px] text-slate-300">
                                            <span className="font-semibold uppercase">{proposal.proposal_type || 'unknown'}</span>
                                            {proposal.target_peer_id ? <span>{getDisplayName(proposal.target_peer_id)}</span> : null}
                                          </div>
                                          <div className="mt-1 flex flex-wrap items-center gap-2 text-[10px] text-slate-500">
                                            {proposal.request_id ? <span>Req {String(proposal.request_id).slice(0, 8)}</span> : null}
                                            {proposal.operation_id ? <span>Op {String(proposal.operation_id).slice(0, 8)}</span> : null}
                                            {proposal.group_type ? <span>{proposal.group_type}</span> : null}
                                          </div>
                                        </div>
                                      ))}
                                    </div>
                                  ) : (
                                    <p className="text-[10px] text-slate-500">No proposal summary attached.</p>
                                  )}
                                </div>
                              </div>
                            ) : null}

                            {!isCommit && linkedHeal ? (
                              <div className="space-y-2">
                                <div className="grid grid-cols-2 gap-2">
                                  <div>
                                    <span className="block text-[9px] uppercase tracking-tight text-slate-500">Trace</span>
                                    <span className="block font-mono text-slate-300">{traceId}</span>
                                  </div>
                                  <div>
                                    <span className="block text-[9px] uppercase tracking-tight text-slate-500">Outcome</span>
                                    <span className="block text-slate-300">{linkedHeal.outcome}</span>
                                  </div>
                                </div>
                                <div className="space-y-1">
                                  <span className="block text-[9px] uppercase tracking-tight text-slate-500">Audit Steps</span>
                                  <div className="space-y-1">
                                    {linkedHeal.audit?.map((step: any, idx: number) => (
                                      <div key={`${traceId}-${idx}`} className="flex items-start gap-2 rounded border border-slate-800 bg-slate-900/50 px-2 py-1.5">
                                        <span className={cn('mt-1 h-1.5 w-1.5 shrink-0 rounded-full', step.status === 'success' ? 'bg-emerald-500' : 'bg-rose-500')} />
                                        <div className="min-w-0 flex-1">
                                          <div className="flex items-center justify-between gap-2 text-[10px]">
                                            <span className="font-mono uppercase text-slate-300">{step.step}</span>
                                            <span className="text-slate-500">{step.duration_ms}ms</span>
                                          </div>
                                          {step.error ? <p className="mt-1 text-[10px] text-rose-400">{step.error}</p> : null}
                                        </div>
                                      </div>
                                    ))}
                                  </div>
                                </div>
                              </div>
                            ) : null}
                          </div>
                        ) : null}
                      </div>
                    )
                  })}
                </div>
              )}
            </div>

            {/* Fork Healing History */}
            <div className="rounded-xl border border-slate-800 bg-slate-900/40 p-3.5 space-y-3">
              <div className="flex items-center gap-2 border-b border-slate-800/80 pb-2 text-[11px] font-semibold uppercase tracking-wider text-purple-400">
                <ShieldAlert className="h-3.5 w-3.5" />
                <span>Autonomous Fork Healing ({healHistory.length})</span>
              </div>

              {healHistory.length === 0 ? (
                <div className="flex flex-col items-center justify-center py-4 text-center rounded bg-slate-950 border border-slate-800/50">
                  <CheckCircle className="h-6 w-6 text-emerald-500 mb-1" />
                  <span className="text-xs font-semibold text-slate-300">Consistency Intact</span>
                  <span className="text-[10px] text-slate-500">No network forks or stale epochs detected.</span>
                </div>
              ) : (
                <div className="space-y-2">
                  {healHistory.map((item: any) => {
                    const isSuccess = item.outcome === 'success';
                    const isExpanded = expandedHealTrace === item.trace_id;
                    const scheduledDate = item.scheduled_at_ms ? new Date(item.scheduled_at_ms).toLocaleTimeString() : 'Unknown';
                    
                    return (
                      <div key={item.trace_id} className="rounded-lg bg-slate-950 border border-slate-800 overflow-hidden text-xs">
                        <button
                          type="button"
                          className="w-full flex items-center justify-between p-2.5 hover:bg-slate-900 transition-colors text-left"
                          onClick={() => setExpandedHealTrace(isExpanded ? null : item.trace_id)}
                        >
                          <div className="min-w-0 flex-1 space-y-1 pr-2">
                            <div className="flex items-center gap-1.5 flex-wrap">
                              <span className={cn(
                                "px-1.5 py-0.5 rounded-[4px] text-[9px] font-bold uppercase shrink-0 tracking-wide",
                                isSuccess ? "bg-emerald-500/10 text-emerald-400 border border-emerald-500/20" : "bg-red-500/10 text-red-400 border border-red-500/20"
                              )}>
                                {item.outcome?.toUpperCase() ?? "FAILED"}
                              </span>
                              <span className="font-semibold text-[10px] text-slate-200">
                                Trace: {item.trace_id?.substring(0, 8)}...
                              </span>
                            </div>
                            <div className="flex items-center gap-2 text-[10px] text-slate-500">
                              <span>At {scheduledDate}</span>
                              <span>•</span>
                              <span>{item.duration_ms}ms</span>
                              {item.replayed_message_count > 0 && (
                                <>
                                  <span>•</span>
                                  <span className="text-emerald-400">{item.replayed_message_count} replayed</span>
                                </>
                              )}
                            </div>
                          </div>
                          {isExpanded ? <ChevronUp className="h-4 w-4 text-slate-400 shrink-0" /> : <ChevronDown className="h-4 w-4 text-slate-400 shrink-0" />}
                        </button>

                        {isExpanded && (
                          <div className="border-t border-slate-900 bg-slate-950 p-2.5 space-y-2 text-[11px] leading-relaxed">
                            <div className="grid grid-cols-2 gap-x-2 gap-y-1 pb-2 border-b border-slate-900">
                              <div>
                                <span className="block text-[9px] text-slate-500 uppercase tracking-tight">Winning Branch</span>
                                <span className="font-mono text-slate-300 truncate block">
                                  e{item.winner_epoch} ({item.winner_peer_id ? shortPeerId(item.winner_peer_id) : 'unknown'})
                                </span>
                              </div>
                              <div>
                                <span className="block text-[9px] text-slate-500 uppercase tracking-tight">Resolved Branch</span>
                                <span className="font-mono text-slate-300 truncate block">
                                  e{item.new_epoch} ({item.new_tree_hash_hex ? item.new_tree_hash_hex.substring(0, 8) : 'unknown'})
                                </span>
                              </div>
                            </div>

                            {/* 8 Audit steps */}
                            <div className="pt-1.5 space-y-2">
                              <span className="block text-[9px] text-slate-500 uppercase tracking-tight mb-1">Step Audit Log</span>
                              <div className="space-y-1.5 pl-1.5 border-l border-slate-800">
                                {item.audit?.map((step: any, idx: number) => {
                                  const stepOk = step.status === 'success';
                                  return (
                                    <div key={idx} className="relative flex items-start gap-2">
                                      <span className={cn(
                                        "mt-1.5 h-1.5 w-1.5 rounded-full shrink-0",
                                        stepOk ? "bg-emerald-500" : "bg-red-500"
                                      )} />
                                      <div className="min-w-0 flex-1">
                                        <div className="flex items-center justify-between text-[10px]">
                                          <span className="font-mono font-medium text-slate-300 uppercase tracking-tight">
                                            {step.step}
                                          </span>
                                          <span className="text-slate-500 text-[9px]">
                                            {step.duration_ms}ms
                                          </span>
                                        </div>
                                        {step.error && (
                                          <p className="text-[10px] text-red-400 font-medium break-all pl-1.5 mt-0.5 border-l border-red-500/20 bg-red-950/10 rounded">
                                            Error: {step.error}
                                          </p>
                                        )}
                                      </div>
                                    </div>
                                  );
                                })}
                              </div>
                            </div>
                          </div>
                        )}
                      </div>
                    );
                  })}
                </div>
              )}
            </div>
          </div>
        ) : null}
      </div>

      <div className="mt-auto space-y-2 border-t border-slate-800 p-4">
        <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
          <Shield className="h-3.5 w-3.5" />
          <span>{activeKind === 'dm' ? 'DM actions' : 'Group actions'}</span>
        </div>
        {canManageGroupSettings ? (
          <Button
            className="w-full gap-2 border border-slate-700 bg-slate-900 text-slate-100 hover:bg-slate-800"
            variant="secondary"
            disabled={!activeGroupId}
            onClick={() => setIsGroupSettingsOpen(true)}
          >
            <Settings className="h-4 w-4" />
            Group Settings
          </Button>
        ) : null}
        {canManageGroupSettings ? (
          <Button
            className="w-full bg-slate-800 hover:bg-slate-700 text-slate-100 border border-slate-700 gap-2"
            variant="secondary"
            disabled={!activeGroupId}
            onClick={() => setIsAddModalOpen(true)}
          >
            <UserPlus className="h-4 w-4" />
            Add Member
          </Button>
        ) : null}
        <Button
          className="w-full text-slate-400 hover:text-red-400 hover:bg-red-500/10 gap-2"
          variant="ghost"
          disabled={!activeGroupId || isLeaving || !canLeaveGroup}
          onClick={() => void handleLeaveGroup()}
        >
          <LogOut className="h-4 w-4" />
          {isLeaving ? 'Leaving...' : 'Leave Group'}
        </Button>
        {activeKind !== 'dm' && localIsCreator ? (
          <p className="text-[11px] leading-relaxed text-slate-500">
            Creator cannot leave the group in this version. Transfer is intentionally unsupported.
          </p>
        ) : null}
      </div>

      <AddMemberModal
        isOpen={isAddModalOpen}
        onClose={() => setIsAddModalOpen(false)}
        groupId={activeGroupId || ''}
        onSuccess={() => {
          if (refreshGroups) void refreshGroups()
          void loadPendingInviteRequests()
        }}
      />

      <Dialog
        open={isGroupSettingsOpen}
        onOpenChange={(open) => {
          setIsGroupSettingsOpen(open)
          if (!open) {
            if (invitePolicy !== null) {
              setSettingsDraftPolicy(invitePolicy)
            }
            setPendingGroupCompressedAvatar(null)
            setRemoveGroupAvatarOnSave(false)
            revokeGroupAvatarPreview()
          }
        }}
      >
        <DialogContent
          showCloseButton
          className={cn(
            'flex max-h-[90vh] max-w-[calc(100%-2rem)] flex-col gap-0 overflow-hidden border border-slate-700/90 bg-slate-950 p-0 text-slate-100 shadow-2xl shadow-black/50 sm:max-w-lg',
          )}
        >
          <div className="border-b border-slate-800/90 bg-slate-900/40 px-6 py-5">
            <DialogHeader className="gap-2">
              <div className="flex items-start gap-3">
                <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border border-slate-700/80 bg-slate-900">
                  <Settings className="h-5 w-5 text-emerald-400/90" aria-hidden />
                </span>
                <div className="min-w-0 flex-1 space-y-1.5">
                  <DialogTitle className="text-lg font-semibold tracking-tight text-slate-50">
                    Group Settings
                  </DialogTitle>
                  <DialogDescription className="text-sm leading-relaxed text-slate-400">
                    Edit and click <span className="text-slate-300">Save</span> to apply.
                  </DialogDescription>
                </div>
              </div>
            </DialogHeader>
          </div>

          <div className="flex-1 overflow-y-auto px-6 py-6 space-y-5">
            {activeKind === 'group' && canManageGroupSettings ? (
              <section className="rounded-2xl border border-slate-800 bg-slate-900/50 p-5 ring-1 ring-white/[0.03]">
                <input
                  ref={groupAvatarFileRef}
                  type="file"
                  accept="image/png,image/jpeg,image/webp,.png,.jpg,.jpeg,.webp"
                  className="hidden"
                  onChange={handleGroupAvatarFileChange}
                />
                <div className="flex gap-3 border-b border-slate-800/80 pb-4">
                  <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-slate-800/80 text-emerald-400/90">
                    <ImageIcon className="h-4 w-4" aria-hidden />
                  </span>
                  <div>
                    <h3 className="text-sm font-semibold text-slate-100">Group Avatar</h3>
                    <p className="mt-1 text-xs leading-relaxed text-slate-500">
                      PNG, JPEG or WebP — select up to {AVATAR_INPUT_MAX_BYTES / (1024 * 1024)} MiB; app will
                      resize and compress to {AVATAR_OUTPUT_MAX_BYTES / 1024} KiB. Stored locally on your device.
                    </p>
                  </div>
                </div>
                <div className="mt-5 flex flex-wrap items-center gap-3">
                  {displayGroupAvatarSrc ? (
                    <img
                      src={displayGroupAvatarSrc}
                      alt=""
                      className="h-16 w-16 shrink-0 rounded-2xl border border-slate-700 object-cover shadow-inner shadow-black/30"
                    />
                  ) : (
                    <div className="flex h-16 w-16 shrink-0 items-center justify-center rounded-2xl border border-dashed border-slate-700 bg-slate-950/60 text-[11px] text-slate-500">
                      No image
                    </div>
                  )}
                  <div className="flex min-w-0 flex-1 flex-col gap-2">
                    <div className="flex flex-wrap gap-2">
                      <Button
                        type="button"
                        size="sm"
                        variant="secondary"
                        className="text-xs"
                        disabled={savingPolicy || groupAvatarProcessing}
                        onClick={handlePickGroupAvatarClick}
                      >
                        {groupAvatarProcessing ? 'Processing image...' : 'Pick image...'}
                      </Button>
                      {pendingGroupCompressedAvatar ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          className="text-xs"
                          disabled={savingPolicy || groupAvatarProcessing}
                          onClick={handleDiscardGroupAvatarDraft}
                        >
                          Discard
                        </Button>
                      ) : null}
                      {hasStoredAvatar && !pendingGroupCompressedAvatar ? (
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          className="text-xs text-amber-200/90 hover:text-amber-100"
                          disabled={savingPolicy || groupAvatarProcessing}
                          onClick={handleMarkRemoveGroupAvatar}
                        >
                          Remove image on save
                        </Button>
                      ) : null}
                    </div>
                    {pendingGroupCompressedAvatar ? (
                      <p className="truncate text-[11px] text-slate-400">
                        Ready to save: {formatBytesShort(pendingGroupCompressedAvatar.outputBytes)} (
                        {pendingGroupCompressedAvatar.width}×{pendingGroupCompressedAvatar.height})
                      </p>
                    ) : null}
                  </div>
                </div>
              </section>
            ) : null}

            {canManageAdmins ? (
              <section className="rounded-2xl border border-slate-800 bg-slate-900/50 p-5 ring-1 ring-white/[0.03]">
                <div className="flex gap-3 border-b border-slate-800/80 pb-4">
                  <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-slate-800/80 text-sky-300">
                    <Shield className="h-4 w-4" aria-hidden />
                  </span>
                  <div>
                    <h3 className="text-sm font-semibold text-slate-100">Admins</h3>
                    <p className="mt-1 text-xs leading-relaxed text-slate-500">
                      Creator is always an admin. Revoke admin before removing that member from the group.
                    </p>
                  </div>
                </div>
                <div className="mt-5 space-y-2">
                  {peers
                    .filter((peer) => !peer.is_creator && peer.peer_id !== localPeerId)
                    .map((peer) => {
                      const busy = changingAdminPeerId === peer.peer_id
                      return (
                        <div
                          key={`admin-${peer.peer_id}`}
                          className="flex items-center justify-between gap-3 rounded-xl border border-slate-800 bg-slate-950/40 px-3 py-2"
                        >
                          <div className="min-w-0">
                            <p className="truncate text-xs font-medium text-slate-200">{getDisplayName(peer.peer_id)}</p>
                            <p className="text-[11px] text-slate-500">{shortPeerId(peer.peer_id)}</p>
                          </div>
                          <Button
                            type="button"
                            size="sm"
                            variant={peer.is_admin ? 'ghost' : 'secondary'}
                            className={cn('h-7 shrink-0 text-[11px]', peer.is_admin && 'text-amber-200 hover:text-amber-100')}
                            disabled={busy || savingPolicy}
                            onClick={() => void handleSetMemberAdmin(peer.peer_id, !peer.is_admin)}
                          >
                            {busy ? 'Updating...' : peer.is_admin ? 'Revoke admin' : 'Make admin'}
                          </Button>
                        </div>
                      )
                    })}
                  {peers.filter((peer) => !peer.is_creator && peer.peer_id !== localPeerId).length === 0 ? (
                    <p className="text-xs text-slate-500">No eligible member to promote yet.</p>
                  ) : null}
                </div>
              </section>
            ) : null}

            <section className="rounded-2xl border border-slate-800 bg-slate-900/50 p-5 ring-1 ring-white/[0.03]">
              <div className="flex gap-3 border-b border-slate-800/80 pb-4">
                <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-slate-800/80 text-emerald-400/90">
                  <UserPlus className="h-4 w-4" aria-hidden />
                </span>
                <div>
                  <h3 className="text-sm font-semibold text-slate-100">Invite Policy</h3>
                  <p className="mt-1 text-xs leading-relaxed text-slate-500">
                    Who is allowed to invite new members and if creator approval is required.
                  </p>
                </div>
              </div>
              <div className="mt-5 space-y-3" role="radiogroup" aria-label="Invite policy">
                {loadingPolicy || invitePolicy === null ? (
                  <p className="text-sm text-slate-500">Loading...</p>
                ) : (
                  <>
                    <label
                      className={cn(
                        'flex cursor-pointer gap-3 rounded-xl border p-4 transition-colors',
                        settingsDraftPolicy === 'creator_approval'
                          ? 'border-emerald-600/45 bg-emerald-950/35 ring-1 ring-emerald-500/15'
                          : 'border-slate-800 bg-slate-950/40 hover:border-slate-700 hover:bg-slate-900/60',
                        savingPolicy && 'pointer-events-none opacity-60',
                      )}
                    >
                      <input
                        type="radio"
                        name="invite-policy-modal"
                        className="mt-0.5 accent-emerald-500"
                        checked={settingsDraftPolicy === 'creator_approval'}
                        disabled={savingPolicy}
                        onChange={() => setSettingsDraftPolicy('creator_approval')}
                      />
                      <span className="min-w-0 text-sm leading-snug">
                        <span className="font-medium text-slate-100">Admin approval required</span>
                        <span className="mt-1 block text-xs leading-relaxed text-slate-500">
                          Members send invite requests and an active admin or creator reviews them. Best for private groups.
                        </span>
                      </span>
                    </label>
                    <label
                      className={cn(
                        'flex cursor-pointer gap-3 rounded-xl border p-4 transition-colors',
                        settingsDraftPolicy === 'any_member'
                          ? 'border-emerald-600/45 bg-emerald-950/35 ring-1 ring-emerald-500/15'
                          : 'border-slate-800 bg-slate-950/40 hover:border-slate-700 hover:bg-slate-900/60',
                        savingPolicy && 'pointer-events-none opacity-60',
                      )}
                    >
                      <input
                        type="radio"
                        name="invite-policy-modal"
                        className="mt-0.5 accent-emerald-500"
                        checked={settingsDraftPolicy === 'any_member'}
                        disabled={savingPolicy}
                        onChange={() => setSettingsDraftPolicy('any_member')}
                      />
                      <span className="min-w-0 text-sm leading-snug">
                        <span className="font-medium text-slate-100">Anyone can invite</span>
                        <span className="mt-1 block text-xs leading-relaxed text-slate-500">
                          Invites are processed automatically by the network. No manual approval needed.
                        </span>
                      </span>
                    </label>
                  </>
                )}
              </div>
            </section>
          </div>

          <DialogFooter className="gap-2 border-t border-slate-800/90 bg-slate-900/50 px-6 py-4 sm:justify-end">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={savingPolicy}
              className="border-slate-600 bg-slate-950/50 text-slate-200 hover:bg-slate-800 hover:text-white"
              onClick={() => handleCloseGroupSettings()}
            >
              Cancel
            </Button>
            <Button
              type="button"
              size="sm"
              disabled={savingPolicy || groupAvatarProcessing || !settingsDirty || (loadingPolicy && policyDirty)}
              className="border border-emerald-600/50 bg-emerald-600 text-white shadow-sm shadow-emerald-950/40 hover:bg-emerald-500 disabled:border-slate-700 disabled:bg-slate-800 disabled:text-slate-500 disabled:shadow-none"
              onClick={() => void handleSaveGroupSettings()}
            >
              {savingPolicy ? 'Saving...' : 'Save'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </aside>
  )
}
