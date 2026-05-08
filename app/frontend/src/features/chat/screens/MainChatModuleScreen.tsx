import { useEffect, useState } from 'react'
import AppShell from '../../../components/layout/AppShell'
import MainSidebar from '../../../components/layout/MainSidebar'
import WorkspaceRail, { WorkspaceModule } from '../../../components/layout/WorkspaceRail'
import ChatView from '../../../components/chat/ChatView'
import RoomPanel from '../../../components/chat/RoomPanel'
import { useChatRuntime } from '../hooks/useChatRuntime'
import { useChatEvents } from '../hooks/useChatEvents'
import { useChatActions } from '../hooks/useChatActions'
import InvitesScreen from '../../invites/screens/InvitesScreen'
import SettingsScreen from '../../settings/screens/SettingsScreen'
import AdminPanelScreen from '../../admin/screens/AdminPanelScreen'
import { useRuntimeEventStream } from '../../../hooks/useRuntimeEventStream'
import { usePendingInvites } from '../../invites/hooks/usePendingInvites'
import { getConversationKind } from '../../../lib/chatModel'

interface MainChatModuleScreenProps {
  isAdmin: boolean
}

export default function MainChatModuleScreen({ isAdmin }: MainChatModuleScreenProps) {
  const [activeModule, setActiveModule] = useState<WorkspaceModule>('chat')
  const [detailsOpen, setDetailsOpen] = useState(false)
  const { pending, refresh: refreshPendingInvites, accept, reject, busyId } = usePendingInvites()

  const handleAcceptInvite = async (id: string) => {
    await accept(id)
    await refreshGroups()
  }

  const handleRejectInvite = async (id: string) => {
    await reject(id)
    await refreshGroups()
  }

  const {
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
    loadGroupMembers,
    setActiveGroupId,
    loadMoreMessages,
    loadMorePosts,
    loadComments,
    loadMoreComments,
  } = useChatRuntime()

  useChatEvents({
    activeGroupId,
    localPeerId,
    refreshGroups,
    refreshNodeStatus,
    refreshGroupMembers: loadGroupMembers,
    setActiveGroupId,
  })

  const {
    creatingGroup,
    composingMessage,
    setComposingMessage,
    sending,
    handleSelectGroup,
    handleCreateGroupWithDetails,
    handleSendMessage,
    handleRetryMessage,
    handleRemoveFailed,
  } = useChatActions({ activeGroupId, localPeerId, refreshGroups, setActiveGroupId })

  const activeGroup = groups.find((g) => g.group_id === activeGroupId)
  const activeKind = getConversationKind(activeGroup)
  const usesMessageStream = activeKind === 'dm' || activeKind === 'group'

  useEffect(() => {
    if (activeModule !== 'chat' && detailsOpen) {
      setDetailsOpen(false)
    }
  }, [activeModule, detailsOpen])

  useRuntimeEventStream({
    onEvent: async (event, payload, hasGap) => {
      const groupId = typeof payload.group_id === 'string' ? payload.group_id : ''
      const isInviteEvent =
        event.topic === 'invite:received' || event.topic === 'invite:accepted' || event.topic === 'invite:rejected'
      if (hasGap || event.topic === 'group:joined' || event.topic === 'group:left') {
        await refreshGroups()
        await refreshPendingInvites()
      }
      if (event.topic === 'group:left' && groupId && groupId === activeGroupId) {
        setActiveGroupId(null)
      }
      if (isInviteEvent) {
        await refreshPendingInvites()
      }
      if (event.topic === 'node:status' || event.topic === 'p2p:status' || hasGap) {
        await refreshNodeStatus()
      }
      if (groupId && groupId === activeGroupId && (event.topic === 'group:members_changed' || hasGap)) {
        await loadGroupMembers(groupId)
      }
    },
  })

  return (
    <AppShell
      title="Secure P2P"
      subtitle={isAdmin ? 'Admin capability enabled' : 'Authorized device'}
    >
      <div className="flex h-full w-full">
        <WorkspaceRail
          activeModule={activeModule}
          onSelectModule={setActiveModule}
          isAdmin={isAdmin}
          pendingInviteCount={pending.length}
        />
        <MainSidebar
          displayName={displayName}
          localPeerId={localPeerId}
          networkStatus={networkStatus}
          groups={groups}
          activeGroupId={activeGroupId}
          unreadByGroup={unreadByGroup}
          peerCount={connectedPeers.length}
          creatingGroup={creatingGroup}
          onCreateGroupWithDetails={handleCreateGroupWithDetails}
          onSelectGroup={handleSelectGroup}
          showWorkspaceLists={activeModule === 'chat'}
        />
        {activeModule === 'chat' ? (
          <ChatView
            activeGroupId={activeGroupId}
            localPeerId={localPeerId}
            groups={groups}
            messages={usesMessageStream ? activeMessages : activePosts}
            loadingMessages={loadingMessages}
            composingMessage={composingMessage}
            sending={sending}
            onComposingChange={setComposingMessage}
            onSend={handleSendMessage}
            onRetry={handleRetryMessage}
            onRemoveFailed={handleRemoveFailed}
            detailsOpen={detailsOpen}
            onToggleDetails={() => setDetailsOpen((v) => !v)}
            activeGroupMembers={activeGroupMembers}
            pendingInviteCount={pending.length}
            pendingInvites={pending}
            inviteBusyId={busyId}
            activeKind={activeKind}
            onAcceptInvite={handleAcceptInvite}
            onRejectInvite={handleRejectInvite}
            onRefreshPendingInvites={refreshPendingInvites}
            onLoadMore={async () => {
              if (activeGroupId) {
                if (usesMessageStream) {
                  await loadMoreMessages(activeGroupId)
                } else {
                  await loadMorePosts(activeGroupId)
                }
              }
            }}
            onLoadComments={async (postId) => {
              if (activeGroupId) await loadComments(activeGroupId, postId)
            }}
            onLoadMoreComments={async (postId) => {
              if (activeGroupId) await loadMoreComments(activeGroupId, postId)
            }}
          />
        ) : null}
        {activeModule === 'activity' ? (
          <section className="min-w-0 flex-1 overflow-y-auto bg-slate-900">
            <InvitesScreen
              activeGroupId={activeGroupId}
              pendingInvites={pending}
              busyInviteId={busyId}
              onAcceptInvite={handleAcceptInvite}
              onRejectInvite={handleRejectInvite}
              onRefreshPendingInvites={refreshPendingInvites}
            />
          </section>
        ) : null}
        {activeModule === 'settings' ? (
          <section className="min-w-0 flex-1 overflow-y-auto bg-slate-900">
            <SettingsScreen />
          </section>
        ) : null}
        {activeModule === 'admin' ? (
          <section className="min-w-0 flex-1 overflow-y-auto bg-slate-900">
            {isAdmin ? <AdminPanelScreen /> : <p className="p-4 text-sm text-slate-400">Admin mode required.</p>}
          </section>
        ) : null}
        {activeModule === 'chat' && detailsOpen ? (
          <RoomPanel
            activeGroupId={activeGroupId}
            activeKind={activeKind}
            myRole={activeGroup?.my_role}
            localPeerId={localPeerId}
            peers={activeGroupMembers}
            onClose={() => setDetailsOpen(false)}
            setActiveGroupId={setActiveGroupId}
            refreshGroups={refreshGroups}
          />
        ) : null}
      </div>
    </AppShell>
  )
}
