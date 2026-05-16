import { useEffect, useState } from 'react'
import AppShell from '../../../components/layout/AppShell'
import MainSidebar from '../../../components/layout/MainSidebar'
import WorkspaceRail, { WorkspaceModule } from '../../../components/layout/WorkspaceRail'
import ChatView from '../../../components/chat/ChatView'
import RoomPanel from '../../../components/chat/RoomPanel'
import { useChatRuntime } from '../hooks/useChatRuntime'
import { useChatEvents } from '../hooks/useChatEvents'
import { useChatActions } from '../hooks/useChatActions'
import { useChannelCategories } from '../hooks/useChannelCategories'
import SettingsScreen from '../../settings/screens/SettingsScreen'
import AdminPanelScreen from '../../admin/screens/AdminPanelScreen'
import { useRuntimeEventStream } from '../../../hooks/useRuntimeEventStream'
import { getConversationKind } from '../../../lib/chatModel'

interface MainChatModuleScreenProps {
  isAdmin: boolean
}

export default function MainChatModuleScreen({ isAdmin }: MainChatModuleScreenProps) {
  const [activeModule, setActiveModule] = useState<WorkspaceModule>('chat')
  const [detailsOpen, setDetailsOpen] = useState(false)
  const {
    categories: channelCategories,
    refresh: refreshChannelCategories,
    create: createChannelCategory,
    remove: deleteChannelCategory,
  } = useChannelCategories()

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
    attachingFile,
    fileTransferStateByMessage,
    fileLocalPathByMessage,
    handleSelectGroup,
    handleCreateGroupWithDetails,
    handleSendMessage,
    handleSendFile,
    handleDownloadFile,
    handleOpenDownloadedFile,
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
      if (hasGap || event.topic === 'group:joined' || event.topic === 'group:left') {
        await refreshGroups()
      }
      if (
        event.topic === 'group:members_changed' &&
        !hasGap &&
        ['profile_push', 'group_avatar', 'presence'].includes(String(payload.reason ?? ''))
      ) {
        await refreshGroups()
      }
      if (event.topic === 'channel_categories:changed' || hasGap) {
        await refreshChannelCategories()
      }
      if (event.topic === 'group:left' && groupId && groupId === activeGroupId) {
        setActiveGroupId(null)
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
          channelCategories={channelCategories}
          onCreateCategory={createChannelCategory}
          onDeleteCategory={deleteChannelCategory}
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
            attachingFile={attachingFile}
            onComposingChange={setComposingMessage}
            onSend={handleSendMessage}
            onAttachFile={handleSendFile}
            onRetry={handleRetryMessage}
            onRemoveFailed={handleRemoveFailed}
            onDownloadFile={handleDownloadFile}
            onOpenDownloadedFile={handleOpenDownloadedFile}
            fileTransferStateByMessage={fileTransferStateByMessage}
            fileLocalPathByMessage={fileLocalPathByMessage}
            detailsOpen={detailsOpen}
            onToggleDetails={() => setDetailsOpen((v) => !v)}
            activeGroupMembers={activeGroupMembers}
            activeKind={activeKind}
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
          <section
            className="min-h-0 min-w-0 flex-1 bg-slate-900"
            aria-label="Hoạt động"
          />
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
            groupAvatarDataUrl={String((activeGroup as { group_avatar_data_url?: string })?.group_avatar_data_url ?? '')}
          />
        ) : null}
      </div>
    </AppShell>
  )
}
