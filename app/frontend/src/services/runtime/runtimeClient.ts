import {
  CreateGroupChat,
  GenerateKeys,
  GetAppState,
  GetGroupMessages,
  GetGroups,
  GetNodeStatus,
  GetOnboardingInfo,
  GetRuntimeHealth,
  GetSessionStatus,
  ImportIdentityFromFile,
  OpenAndImportBundle,
  SendGroupMessage,
} from '../../../wailsjs/go/service/Runtime'

export const runtimeClient = {
  getAppState: GetAppState,
  getRuntimeHealth: GetRuntimeHealth,
  getSessionStatus: GetSessionStatus,
  generateKeys: GenerateKeys,
  getOnboardingInfo: GetOnboardingInfo,
  openAndImportBundle: OpenAndImportBundle,
  importIdentityFromFile: ImportIdentityFromFile,
  getNodeStatus: GetNodeStatus,
  getGroups: GetGroups,
  getGroupMessages: GetGroupMessages,
  createGroupChat: CreateGroupChat,
  sendGroupMessage: SendGroupMessage,
}
