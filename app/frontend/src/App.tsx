import { useEffect, useState } from 'react'
import { GetAppState } from '../wailsjs/go/main/App'
import SetupScreen from './screens/SetupScreen'
import AwaitingBundleScreen from './screens/AwaitingBundleScreen'
import DashboardScreen from './screens/DashboardScreen'

type AppState = 'LOADING' | 'UNINITIALIZED' | 'AWAITING_BUNDLE' | 'AUTHORIZED' | 'ADMIN_READY' | 'ERROR'

export default function App() {
  const [appState, setAppState] = useState<AppState>('LOADING')

  // Poll the Go backend for app state. Runs every 2s while in LOADING,
  // then slows to 5s once a stable state is reached.
  useEffect(() => {
    let cancelled = false

    const poll = async () => {
      try {
        const state = await GetAppState()
        if (!cancelled) setAppState(state as AppState)
      } catch {
        if (!cancelled) setAppState('ERROR')
      }
    }

    poll()
    const interval = setInterval(poll, appState === 'LOADING' ? 1000 : 5000)
    return () => {
      cancelled = true
      clearInterval(interval)
    }
  }, [appState])

  if (appState === 'LOADING') {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="flex flex-col items-center gap-3">
          <div className="h-8 w-8 animate-spin rounded-full border-2 border-gray-700 border-t-blue-500" />
          <p className="text-sm text-gray-500">Starting...</p>
        </div>
      </div>
    )
  }

  if (appState === 'ERROR') {
    return (
      <div className="flex h-screen items-center justify-center">
        <div className="card max-w-sm text-center">
          <p className="text-red-400 font-medium">Failed to connect to backend</p>
          <p className="text-sm text-gray-500 mt-1">Check the application logs.</p>
          <button className="btn-secondary mt-4" onClick={() => setAppState('LOADING')}>
            Retry
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen">
      {appState === 'UNINITIALIZED' && (
        <SetupScreen onDone={() => setAppState('LOADING')} />
      )}
      {appState === 'AWAITING_BUNDLE' && (
        <AwaitingBundleScreen onImported={() => setAppState('LOADING')} />
      )}
      {(appState === 'AUTHORIZED' || appState === 'ADMIN_READY') && (
        <DashboardScreen isAdmin={appState === 'ADMIN_READY'} />
      )}
    </div>
  )
}
