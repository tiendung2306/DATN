import { useCallback, useEffect, useState } from 'react'
import AppShell from '../../../components/layout/AppShell'
import { useAppRuntimeStore, type AppRouteState } from '../../../stores/useAppRuntimeStore'
import { useWailsEvent } from '../../../hooks/useWailsEvent'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import MainChatModuleScreen from '../../chat/screens/MainChatModuleScreen'
import AwaitingBundleScreen from '../../onboarding/screens/AwaitingBundleScreen'
import WelcomeScreen from '../../onboarding/screens/WelcomeScreen'
import ImportBackupScreen from '../../onboarding/screens/ImportBackupScreen'

interface StartupProgressPayload {
  stage?: string
}

interface StartupErrorPayload {
  message?: string
}

function normalizeAppState(state: string): AppRouteState {
  switch (state) {
    case 'UNINITIALIZED':
    case 'AWAITING_BUNDLE':
    case 'AUTHORIZED':
    case 'ADMIN_READY':
      return state
    default:
      return 'ERROR'
  }
}

export default function RootRouterScreen() {
  const [route, setRoute] = useState<'main' | 'import-backup'>('main')
  const appState = useAppRuntimeStore((s) => s.appState)
  const startupStage = useAppRuntimeStore((s) => s.startupStage)
  const fatalError = useAppRuntimeStore((s) => s.fatalError)
  const setAppState = useAppRuntimeStore((s) => s.setAppState)
  const setStartupStage = useAppRuntimeStore((s) => s.setStartupStage)
  const setFatalError = useAppRuntimeStore((s) => s.setFatalError)
  const setRuntimeHealth = useAppRuntimeStore((s) => s.setRuntimeHealth)
  const resetStartup = useAppRuntimeStore((s) => s.resetStartup)

  const refreshState = useCallback(async () => {
    try {
      const [state, health, session] = await Promise.all([
        runtimeClient.getAppState(),
        runtimeClient.getRuntimeHealth(),
        runtimeClient.getSessionStatus(),
      ])
      if (session?.state === 'replaced') {
        setAppState('ERROR')
        setFatalError('Session replaced by a newer active device.')
        return
      }
      setAppState(normalizeAppState(state))
      setRuntimeHealth(health)
      setStartupStage(health.startup_stage || 'ready')
      setFatalError(null)
    } catch (error) {
      setAppState('ERROR')
      setFatalError(String(error))
    }
  }, [setAppState, setFatalError, setRuntimeHealth, setStartupStage])

  useWailsEvent<StartupProgressPayload>('startup:progress', (payload) => {
    if (payload?.stage) {
      setStartupStage(payload.stage)
    }
  })

  useWailsEvent<StartupErrorPayload>('startup:error', (payload) => {
    setAppState('ERROR')
    setFatalError(payload?.message || 'Startup failed.')
  })

  useEffect(() => {
    void refreshState()
    const interval = setInterval(() => {
      void refreshState()
    }, appState === 'LOADING' ? 1000 : 5000)
    return () => clearInterval(interval)
  }, [appState, refreshState])

  if (appState === 'LOADING') {
    return (
      <AppShell title="Secure P2P Node" subtitle="Runtime startup">
        <div className="flex min-h-[55vh] items-center justify-center">
          <div className="flex flex-col items-center gap-3">
            <div className="h-8 w-8 animate-spin rounded-full border-2 border-muted border-t-primary" />
            <p className="text-sm text-muted-foreground">Starting... ({startupStage})</p>
          </div>
        </div>
      </AppShell>
    )
  }

  if (appState === 'ERROR') {
    return (
      <AppShell title="Secure P2P Node" subtitle="Runtime startup">
        <div className="flex min-h-[55vh] items-center justify-center">
          <div className="card max-w-sm text-center">
            <p className="font-medium text-red-400">Failed to connect to backend</p>
            <p className="mt-1 text-sm text-muted-foreground">{fatalError ?? 'Check application logs.'}</p>
            <button
              className="btn-secondary mt-4"
              onClick={() => {
                resetStartup()
              }}
            >
              Retry
            </button>
          </div>
        </div>
      </AppShell>
    )
  }

  if (appState === 'UNINITIALIZED') {
    if (route === 'import-backup') {
      return <ImportBackupScreen onBack={() => setRoute('main')} onImported={refreshState} />
    }
    return (
      <WelcomeScreen onIdentityCreated={refreshState} onOpenImportBackup={() => setRoute('import-backup')} />
    )
  }

  if (appState === 'AWAITING_BUNDLE') {
    return <AwaitingBundleScreen onImported={refreshState} />
  }

  return <MainChatModuleScreen isAdmin={appState === 'ADMIN_READY'} />
}
