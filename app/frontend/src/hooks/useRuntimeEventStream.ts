import { useCallback, useEffect, useRef } from 'react'
import { runtimeClient } from '../services/runtime/runtimeClient'
import { useWailsEvent } from './useWailsEvent'

export interface RuntimeEventEnvelope {
  seq: number
  topic: string
  aggregate: string
  aggregate_id: string
  revision: number
  payload_json: string
  created_at: number
}

interface UseRuntimeEventStreamOptions {
  maxBatchSize?: number
  cursorStorageKey?: string
  onEvent: (event: RuntimeEventEnvelope, payload: Record<string, unknown>, hasGap: boolean) => Promise<void> | void
}

export function useRuntimeEventStream({ maxBatchSize = 200, cursorStorageKey = 'runtime:lastSeq', onEvent }: UseRuntimeEventStreamOptions) {
  const lastSeqRef = useRef(0)
  const inFlightRef = useRef(false)
  const revisionsRef = useRef<Record<string, number>>({})

  const drain = useCallback(async () => {
    if (inFlightRef.current) return
    inFlightRef.current = true
    try {
      let loops = 0
      while (loops < 8) {
        loops += 1
        const events = (await runtimeClient.getRuntimeEventsSince(lastSeqRef.current, maxBatchSize)) as RuntimeEventEnvelope[]
        if (!events || events.length === 0) {
          break
        }
        for (const event of events) {
          lastSeqRef.current = Math.max(lastSeqRef.current, Number(event.seq) || 0)
          if (typeof window !== 'undefined') {
            window.localStorage.setItem(cursorStorageKey, String(lastSeqRef.current))
          }
          let payload: Record<string, unknown> = {}
          try {
            payload = event.payload_json ? JSON.parse(event.payload_json) : {}
          } catch {
            payload = {}
          }
          const aggregate = event.aggregate || event.topic
          const prev = revisionsRef.current[aggregate] ?? 0
          const incoming = Number(event.revision) || 0
          const hasGap = incoming > 0 && prev > 0 && incoming > prev + 1
          if (incoming > prev) {
            revisionsRef.current[aggregate] = incoming
          }
          await onEvent(event, payload, hasGap)
        }
        if (events.length < maxBatchSize) {
          break
        }
      }
    } finally {
      inFlightRef.current = false
    }
  }, [maxBatchSize, onEvent])

  useEffect(() => {
    void (async () => {
      const persisted = typeof window !== 'undefined' ? Number(window.localStorage.getItem(cursorStorageKey) || '0') : 0
      const [cursor, revisions] = await Promise.all([
        runtimeClient.getRuntimeEventCursor(),
        runtimeClient.getAggregateRevisions(),
      ])
      lastSeqRef.current = Math.max(Number(cursor) || 0, persisted || 0)
      revisionsRef.current = (revisions as Record<string, number>) ?? {}
    })()
  }, [cursorStorageKey])

  useWailsEvent('runtime:event_available', () => {
    void drain()
  })

  return {
    drainRuntimeEvents: drain,
  }
}

