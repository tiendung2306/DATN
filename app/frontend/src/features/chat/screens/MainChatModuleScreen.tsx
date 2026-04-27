import AppShell from '../../../components/layout/AppShell'
import MainSidebar from '../../../components/layout/MainSidebar'
import ChatView from '../../../components/chat/ChatView'
import PrimaryRail from '../../../components/layout/PrimaryRail'
import { useChatRuntime } from '../hooks/useChatRuntime'
import { useChatEvents } from '../hooks/useChatEvents'
import { useChatActions } from '../hooks/useChatActions'

interface MainChatModuleScreenProps {
  isAdmin: boolean
}

export default function MainChatModuleScreen({ isAdmin }: MainChatModuleScreenProps) {
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
    refreshGroups,
    setActiveGroupId,
  } = useChatRuntime()

  useChatEvents({ activeGroupId, refreshGroups, setActiveGroupId })

  const {
    createGroupValue,
    setCreateGroupValue,
    creatingGroup,
    composingMessage,
    setComposingMessage,
    sending,
    handleSelectGroup,
    handleCreateGroup,
    handleSendMessage,
    handleRetryMessage,
    handleRemoveFailed,
  } = useChatActions({ activeGroupId, localPeerId, refreshGroups, setActiveGroupId })

  return (
    <AppShell
      title="Secure P2P"
      subtitle={isAdmin ? 'Admin capability enabled' : 'Authorized device'}
    >
      <div className="flex h-full w-full">
        <PrimaryRail isConnected={networkStatus === 'connected'} />
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
      </div>
    </AppShell>
  )
}
