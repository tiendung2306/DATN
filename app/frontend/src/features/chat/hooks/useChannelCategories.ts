import { useCallback, useEffect, useState } from 'react'
import { runtimeClient } from '../../../services/runtime/runtimeClient'
import { useWailsEvent } from '../../../hooks/useWailsEvent'

export interface ChannelCategory {
  category_id: string
  name: string
  sort_order: number
  created_by: string
  created_at: number
  updated_at: number
}

export function useChannelCategories() {
  const [categories, setCategories] = useState<ChannelCategory[]>([])
  const [loading, setLoading] = useState(false)

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const list = await runtimeClient.listChannelCategories()
      setCategories((list ?? []).slice().sort((a, b) => {
        if ((a.sort_order ?? 0) !== (b.sort_order ?? 0)) return (a.sort_order ?? 0) - (b.sort_order ?? 0)
        return String(a.name || '').localeCompare(String(b.name || ''))
      }))
    } finally {
      setLoading(false)
    }
  }, [])

  const create = useCallback(async (name: string) => {
    await runtimeClient.createChannelCategory(name)
    await refresh()
  }, [refresh])

  const remove = useCallback(async (categoryID: string) => {
    await runtimeClient.deleteChannelCategory(categoryID)
    await refresh()
  }, [refresh])

  useEffect(() => {
    void refresh()
  }, [refresh])

  useWailsEvent('channel_categories:changed', () => {
    void refresh()
  })

  return {
    categories,
    loading,
    refresh,
    create,
    remove,
  }
}
