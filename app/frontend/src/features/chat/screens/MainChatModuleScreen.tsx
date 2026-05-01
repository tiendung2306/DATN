import { useState } from 'react'
import AppShell from '../../../components/layout/AppShell'
import MainSidebar from '../../../components/layout/MainSidebar'
import ChatView from '../../../components/chat/ChatView'
import RoomPanel from '../../../components/chat/RoomPanel'
import { useChatRuntime } from '../hooks/useChatRuntime'
import { useChatEvents } from '../hooks/useChatEvents'
import { useChatActions } from '../hooks/useChatActions'
import InvitesScreen from '../../invites/screens/InvitesScreen'
import SettingsScreen from '../../settings/screens/SettingsScreen'
import AdminPanelScreen from '../../admin/screens/AdminPanelScreen'
import { useRuntimeEventStream } from '../../../hooks/useRuntimeEventStream'

interface MainChatModuleScreenProps {
  isAdmin: boolean
}

export default function MainChatModuleScreen({ isAdmin }: MainChatModuleScreenProps) {
  const [activeModule, setActiveModule] = useState<'chat' | 'invites' | 'settings' | 'admin'>('chat')
  const [detailsOpen, setDetailsOpen] = useState(true)
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
  const isDM = activeGroup?.group_type === 'dm'

  useRuntimeEventStream({
    onEvent: async (event, payload, hasGap) => {
      const groupId = typeof payload.group_id === 'string' ? payload.group_id : ''
      if (hasGap || event.topic === 'group:joined' || event.topic === 'group:left') {
        await refreshGroups()
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
          activeModule={activeModule}
          onSelectModule={setActiveModule}
          isAdmin={isAdmin}
        />
        {activeModule === 'chat' ? (
          <ChatView
            activeGroupId={activeGroupId}
            localPeerId={localPeerId}
            groups={groups}
            messages={isDM ? activeMessages : activePosts}
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
            onLoadMore={async () => {
              if (activeGroupId) {
                if (isDM) {
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
        {activeModule === 'invites' ? (
          <section className="min-w-0 flex-1 overflow-y-auto bg-slate-900">
            <InvitesScreen activeGroupId={activeGroupId} />
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
        {activeModule === 'chat' ? (
          <RoomPanel
            activeGroupId={activeGroupId}
            isAdmin={isAdmin}
            peers={activeGroupMembers}
            collapsed={!detailsOpen}
            onToggleCollapsed={() => setDetailsOpen((v) => !v)}
            setActiveGroupId={setActiveGroupId}
            refreshGroups={refreshGroups}
          />
        ) : null}
      </div>
    </AppShell>
  )
}
