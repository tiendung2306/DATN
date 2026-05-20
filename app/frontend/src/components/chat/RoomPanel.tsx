import { useCallback, useEffect, useRef, useState, type ChangeEvent } from 'react'
import { service } from '../../../wailsjs/go/models'
import { shortPeerId } from '../../lib/chatModel'
import { formatLeaveGroupError, formatRemoveMemberError } from '../../lib/formatRemoveMemberError'
import { useContactStore } from '../../stores/useContactStore'
import { useToastStore } from '../../stores/useToastStore'
import { Button } from '../ui/button'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '../ui/dialog'
import { LogOut, Settings, Shield, UserMinus, UserPlus, Users, X, ImageIcon } from 'lucide-react'
import { runtimeClient } from '../../services/runtime/runtimeClient'
import AddMemberModal from '../../features/chat/components/AddMemberModal'
import { ConversationKind } from '../../lib/chatModel'
import { formatOutboundSendError } from '../../lib/formatSendError'
import { cn } from '@/lib/utils'
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
  const [isAddModalOpen, setIsAddModalOpen] = useState(false)
  const [isLeaving, setIsLeaving] = useState(false)
  const [removingPeerId, setRemovingPeerId] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'members' | 'invites'>('members')
  const [inviteItems, setInviteItems] = useState<InviteListItem[]>([])
  const [loadingInvites, setLoadingInvites] = useState(false)
  const [changingInviteId, setChangingInviteId] = useState<string | null>(null)
  const [invitePolicy, setInvitePolicy] = useState<'creator_approval' | 'any_member' | null>(null)
  const [loadingPolicy, setLoadingPolicy] = useState(false)
  const [savingPolicy, setSavingPolicy] = useState(false)
  const [settingsDraftPolicy, setSettingsDraftPolicy] = useState<'creator_approval' | 'any_member'>(
    'creator_approval',
  )
  const [isGroupSettingsOpen, setIsGroupSettingsOpen] = useState(false)
  const canRemoveMembers = activeKind !== 'dm' && myRole === 'creator'

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

  const hasStoredAvatar = Boolean(String(groupAvatarDataUrl || '').trim())
  const avatarDirty =
    activeKind === 'group' &&
    canRemoveMembers &&
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
    if (!activeGroupId || !canRemoveMembers || !peerId || peerId === localPeerId) return
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
          <div className="mb-3 grid grid-cols-2 gap-2 rounded-lg border border-slate-800 p-1">
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
                    <p className="text-xs font-medium text-slate-200">{getDisplayName(peer.peer_id)}</p>
                    <p className="text-[11px] text-slate-500">{shortPeerId(peer.peer_id)}</p>
                  </div>
                </div>
                <div className="flex items-center gap-2">
                  {canRemoveMembers && peer.peer_id !== localPeerId ? (
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
        ) : (
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
                const isLocalCreator = canRemoveMembers
                return (
                  <div key={requestId} className="rounded-md border border-slate-800 bg-slate-900/60 px-2 py-2">
                    <p className="text-xs font-medium text-slate-200">{getDisplayName(targetPeer)}</p>
                    <p className="text-[11px] text-slate-500">{shortPeerId(targetPeer)}</p>
                    {requesterPeer ? (
                      <p className="mt-1 text-[11px] text-slate-400">
                        Requested by: {getDisplayName(requesterPeer)}
                      </p>
                    ) : null}
                    {isLocalCreator ? (
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
                      <p className="mt-2 text-[11px] text-slate-500">Awaiting creator approval.</p>
                    )}
                  </div>
                )
              })}
            </div>
          </>
        )}
      </div>

      <div className="mt-auto space-y-2 border-t border-slate-800 p-4">
        <div className="mb-2 flex items-center gap-2 text-[11px] font-semibold uppercase tracking-wide text-slate-500">
          <Shield className="h-3.5 w-3.5" />
          <span>{activeKind === 'dm' ? 'DM actions' : 'Group actions'}</span>
        </div>
        {activeKind !== 'dm' && canRemoveMembers ? (
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
        {activeKind !== 'dm' ? (
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
          disabled={!activeGroupId || isLeaving}
          onClick={() => void handleLeaveGroup()}
        >
          <LogOut className="h-4 w-4" />
          {isLeaving ? 'Leaving...' : 'Leave Group'}
        </Button>
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
            {activeKind === 'group' && canRemoveMembers ? (
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
                        <span className="font-medium text-slate-100">Creator approval only</span>
                        <span className="mt-1 block text-xs leading-relaxed text-slate-500">
                          Members send invite requests → only creator can approve. Best for private groups.
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
