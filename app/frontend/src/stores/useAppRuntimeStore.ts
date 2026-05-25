import { create } from 'zustand'
import { service } from '../../wailsjs/go/models'

const readDevModePreference = (): boolean => {
  if (typeof window === 'undefined') return false
  try {
    return window.localStorage.getItem('isDevMode') === 'true'
  } catch {
    return false
  }
}

const writeDevModePreference = (val: boolean) => {
  if (typeof window === 'undefined') return
  try {
    window.localStorage.setItem('isDevMode', val ? 'true' : 'false')
  } catch {
    // Ignore storage access failures and keep the in-memory state only.
  }
}

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
  isDevMode: boolean
  setAppState: (next: AppRouteState) => void
  setStartupStage: (stage: string) => void
  setFatalError: (message: string | null) => void
  setRuntimeHealth: (health: service.RuntimeHealth | null) => void
  setIsDevMode: (val: boolean) => void
  resetStartup: () => void
}

export const useAppRuntimeStore = create<AppRuntimeState>((set) => ({
  appState: 'LOADING',
  startupStage: 'starting',
  fatalError: null,
  runtimeHealth: null,
  isDevMode: readDevModePreference(),
  setAppState: (next) => set({ appState: next }),
  setStartupStage: (stage) => set({ startupStage: stage }),
  setFatalError: (message) => set({ fatalError: message }),
  setRuntimeHealth: (health) => set({ runtimeHealth: health }),
  setIsDevMode: (val) => {
    writeDevModePreference(val)
    set({ isDevMode: val })
  },
  resetStartup: () =>
    set({
      appState: 'LOADING',
      startupStage: 'starting',
      fatalError: null,
      runtimeHealth: null,
    }),
}))
