import { useEffect, useRef, useState, useCallback } from 'react'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  CreateGroupChat,
  SendGroupMessage,
  GetGroupMessages,
  GetGroups,
} from '../../wailsjs/go/main/App'

interface MessageData {
  group_id: string
  sender: string
  content: string
  timestamp: number
  is_mine: boolean
}

interface GroupData {
  group_id: string
  epoch: number
  my_role: string
}

function shortID(id: string): string {
  if (id.length <= 12) return id
  return id.slice(0, 6) + '…' + id.slice(-4)
}

function formatTime(tsMs: number): string {
  if (tsMs === 0) return ''
  const d = new Date(tsMs)
  return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

export default function ChatPanel() {
  const [groups, setGroups] = useState<GroupData[]>([])
  const [activeGroup, setActiveGroup] = useState<string | null>(null)
  const [messages, setMessages] = useState<MessageData[]>([])
  const [newGroupID, setNewGroupID] = useState('')
  const [messageText, setMessageText] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)

  const scrollToBottom = useCallback(() => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [])

  // Load groups on mount
  useEffect(() => {
    const loadGroups = async () => {
      try {
        const g = await GetGroups()
        if (g) setGroups(g)
      } catch {
        // coordination stack may not be ready yet
      }
    }
    loadGroups()
    const interval = setInterval(loadGroups, 5000)
    return () => clearInterval(interval)
  }, [])

  // Load messages when active group changes
  useEffect(() => {
    if (!activeGroup) {
      setMessages([])
      return
    }
    const load = async () => {
      try {
        const msgs = await GetGroupMessages(activeGroup)
        setMessages(msgs ?? [])
      } catch {
        setMessages([])
      }
    }
    load()
  }, [activeGroup])

  // Listen for real-time messages
  useEffect(() => {
    const cancel = EventsOn('group:message', (data: MessageData) => {
      if (data.group_id === activeGroup) {
        setMessages((prev) => [...prev, data])
      }
    })
    return () => {
      if (typeof cancel === 'function') cancel()
      EventsOff('group:message')
    }
  }, [activeGroup])

  // Listen for epoch changes
  useEffect(() => {
    const cancel = EventsOn('group:epoch', (data: { group_id: string; epoch: number }) => {
      setGroups((prev) =>
        prev.map((g) =>
          g.group_id === data.group_id ? { ...g, epoch: data.epoch } : g
        )
      )
    })
    return () => {
      if (typeof cancel === 'function') cancel()
      EventsOff('group:epoch')
    }
  }, [])

  // Auto-scroll on new messages
  useEffect(() => {
    scrollToBottom()
  }, [messages, scrollToBottom])

  const handleCreateGroup = async () => {
    const id = newGroupID.trim()
    if (!id) return
    setLoading(true)
    setError(null)
    try {
      await CreateGroupChat(id)
      setNewGroupID('')
      const g = await GetGroups()
      if (g) setGroups(g)
      setActiveGroup(id)
    } catch (e: any) {
      setError(e?.message || String(e))
    } finally {
      setLoading(false)
    }
  }

  const handleSend = async () => {
    const text = messageText.trim()
    if (!text || !activeGroup) return
    setMessageText('')
    try {
      await SendGroupMessage(activeGroup, text)
    } catch (e: any) {
      setError(e?.message || String(e))
    }
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  return (
    <div className="card mt-4">
      <h2 className="text-xs font-semibold uppercase tracking-wide text-gray-500 mb-4">
        Group Chat
      </h2>

      {/* Create / Join group */}
      <div className="flex gap-2 mb-4">
        <input
          type="text"
          className="input flex-1"
          placeholder="Enter Group ID..."
          value={newGroupID}
          onChange={(e) => setNewGroupID(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') handleCreateGroup()
          }}
          disabled={loading}
        />
        <button
          className="btn btn-primary text-sm"
          onClick={handleCreateGroup}
          disabled={loading || !newGroupID.trim()}
        >
          {loading ? 'Creating...' : 'Create / Join'}
        </button>
      </div>

      {error && (
        <div className="mb-3 rounded-lg bg-red-900/30 border border-red-800 px-3 py-2 text-sm text-red-300">
          {error}
          <button
            className="ml-2 text-red-400 hover:text-red-200"
            onClick={() => setError(null)}
          >
            Dismiss
          </button>
        </div>
      )}

      {/* Group tabs */}
      {groups.length > 0 && (
        <div className="flex gap-1 mb-4 overflow-x-auto pb-1">
          {groups.map((g) => (
            <button
              key={g.group_id}
              onClick={() => setActiveGroup(g.group_id)}
              className={`shrink-0 rounded-lg px-3 py-1.5 text-xs font-medium transition-colors ${
                activeGroup === g.group_id
                  ? 'bg-blue-600 text-white'
                  : 'bg-gray-800 text-gray-400 hover:bg-gray-700 hover:text-gray-200'
              }`}
            >
              {shortID(g.group_id)}
              <span className="ml-1.5 text-[10px] opacity-60">E{g.epoch}</span>
            </button>
          ))}
        </div>
      )}

      {/* Chat area */}
      {activeGroup ? (
        <div className="flex flex-col">
          {/* Messages */}
          <div className="h-72 overflow-y-auto rounded-lg bg-gray-950 border border-gray-800 p-3 mb-3 space-y-2">
            {messages.length === 0 ? (
              <div className="flex items-center justify-center h-full text-sm text-gray-600 italic">
                No messages yet. Send the first one!
              </div>
            ) : (
              messages.map((m, i) => (
                <div
                  key={`${m.timestamp}-${m.sender}-${i}`}
                  className={`flex flex-col ${m.is_mine ? 'items-end' : 'items-start'}`}
                >
                  <div className="flex items-baseline gap-1.5 mb-0.5">
                    <span className="text-[10px] font-mono text-gray-500">
                      {m.is_mine ? 'You' : shortID(m.sender)}
                    </span>
                    <span className="text-[10px] text-gray-600">{formatTime(m.timestamp)}</span>
                  </div>
                  <div
                    className={`max-w-[80%] rounded-lg px-3 py-1.5 text-sm break-words ${
                      m.is_mine
                        ? 'bg-blue-600/80 text-white'
                        : 'bg-gray-800 text-gray-200'
                    }`}
                  >
                    {m.content}
                  </div>
                </div>
              ))
            )}
            <div ref={messagesEndRef} />
          </div>

          {/* Input */}
          <div className="flex gap-2">
            <input
              type="text"
              className="input flex-1"
              placeholder="Type a message..."
              value={messageText}
              onChange={(e) => setMessageText(e.target.value)}
              onKeyDown={handleKeyDown}
            />
            <button
              className="btn btn-primary text-sm"
              onClick={handleSend}
              disabled={!messageText.trim()}
            >
              Send
            </button>
          </div>
        </div>
      ) : groups.length > 0 ? (
        <div className="flex items-center justify-center h-32 text-sm text-gray-600 italic">
          Select a group above to start chatting
        </div>
      ) : (
        <div className="flex items-center justify-center h-32 text-sm text-gray-600 italic">
          Create a group to get started
        </div>
      )}
    </div>
  )
}
