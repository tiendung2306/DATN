import { useEffect } from 'react'
import { EventsOn } from '../../wailsjs/runtime/runtime'

type WailsEventHandler<T> = (payload: T) => void

export function useWailsEvent<T = unknown>(eventName: string, handler: WailsEventHandler<T>) {
  useEffect(() => {
    const unsubscribe = EventsOn(eventName, (...data: unknown[]) => {
      handler((data[0] as T) ?? (undefined as T))
    })

    return () => {
      if (typeof unsubscribe === 'function') {
        unsubscribe()
      }
    }
  }, [eventName, handler])
}
