import { create } from 'zustand'
import { service } from '../../wailsjs/go/models'

export type AppRouteState =
  | 'LOADING'
  | 'UNINITIALIZED'
  | 'AWAITING_BUNDLE'
  | 'AUTHORIZED'
  | 'ADMIN_READY'
  | 'ERROR'

interface AppRuntimeState {
  appState: AppRouteState
  startupStage: string
  fatalError: string | null
  runtimeHealth: service.RuntimeHealth | null
  setAppState: (next: AppRouteState) => void
  setStartupStage: (stage: string) => void
  setFatalError: (message: string | null) => void
  setRuntimeHealth: (health: service.RuntimeHealth | null) => void
  resetStartup: () => void
}

export const useAppRuntimeStore = create<AppRuntimeState>((set) => ({
  appState: 'LOADING',
  startupStage: 'starting',
  fatalError: null,
  runtimeHealth: null,
  setAppState: (next) => set({ appState: next }),
  setStartupStage: (stage) => set({ startupStage: stage }),
  setFatalError: (message) => set({ fatalError: message }),
  setRuntimeHealth: (health) => set({ runtimeHealth: health }),
  resetStartup: () =>
    set({
      appState: 'LOADING',
      startupStage: 'starting',
      fatalError: null,
      runtimeHealth: null,
    }),
}))
