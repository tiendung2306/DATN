import { useEffect, useRef, useState, useCallback } from 'react'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  AddMemberToGroup,
  CreateGroupChat,
  GenerateKeyPackage,
  GetGroupMembers,
  GetGroupMessages,
  GetGroups,
  JoinGroupWithWelcome,
  SendGroupMessage,
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

interface MemberRow {
  peer_id: string
  is_online: boolean
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

  const [membersOpen, setMembersOpen] = useState(false)
  const [kpPublicHex, setKpPublicHex] = useState('')
  const [kpBundleHex, setKpBundleHex] = useState('')
  const [addPeerID, setAddPeerID] = useState('')
  const [addKpHex, setAddKpHex] = useState('')
  const [welcomeOutHex, setWelcomeOutHex] = useState('')
  const [joinWelcomeHex, setJoinWelcomeHex] = useState('')
  const [joinBundleHex, setJoinBundleHex] = useState('')
  const [joinGroupID, setJoinGroupID] = useState('')
  const [members, setMembers] = useState<MemberRow[]>([])

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

  // Members list when panel is open
  useEffect(() => {
    if (!activeGroup || !membersOpen) {
      setMembers([])
      return
    }
    const load = async () => {
      try {
        const m = await GetGroupMembers(activeGroup)
        setMembers((m as MemberRow[]) ?? [])
      } catch {
        setMembers([])
      }
    }
    load()
    const interval = setInterval(load, 4000)
    return () => clearInterval(interval)
  }, [activeGroup, membersOpen])

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

  const copyText = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text)
    } catch {
      setError('Could not copy to clipboard')
    }
  }

  const activeRole = groups.find((g) => g.group_id === activeGroup)?.my_role

  const handleGenKP = async () => {
    setError(null)
    try {
      const r = await GenerateKeyPackage()
      setKpPublicHex(r.public_hex)
      setKpBundleHex(r.bundle_private_hex)
    } catch (e: any) {
      setError(e?.message || String(e))
    }
  }

  const handleAddMember = async () => {
    if (!activeGroup) return
    setLoading(true)
    setError(null)
    try {
      const w = await AddMemberToGroup(activeGroup, addPeerID.trim(), addKpHex.trim())
      setWelcomeOutHex(w)
      setAddKpHex('')
      const g = await GetGroups()
      if (g) setGroups(g)
    } catch (e: any) {
      setError(e?.message || String(e))
    } finally {
      setLoading(false)
    }
  }

  const handleJoinWelcome = async () => {
    const gid = joinGroupID.trim() || activeGroup || ''
    if (!gid) {
      setError('Enter a group ID')
      return
    }
    setLoading(true)
    setError(null)
    try {
      await JoinGroupWithWelcome(gid, joinWelcomeHex.trim(), joinBundleHex.trim())
      setJoinWelcomeHex('')
      setJoinBundleHex('')
      setJoinGroupID('')
      const g = await GetGroups()
      if (g) setGroups(g)
      setActiveGroup(gid)
    } catch (e: any) {
      setError(e?.message || String(e))
    } finally {
      setLoading(false)
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

      {/* Members & MLS add/join (out-of-band) */}
      {activeGroup && (
        <div className="mb-4 rounded-lg border border-gray-800 bg-gray-950/50">
          <button
            type="button"
            className="w-full flex items-center justify-between px-3 py-2 text-left text-xs font-medium text-gray-400 hover:bg-gray-900/80 rounded-lg"
            onClick={() => setMembersOpen((o) => !o)}
          >
            <span>Members &amp; keys</span>
            <span className="text-gray-600">{membersOpen ? '▲' : '▼'}</span>
          </button>
          {membersOpen && (
            <div className="px-3 pb-3 space-y-4 border-t border-gray-800 pt-3">
              <div>
                <h3 className="text-[11px] font-semibold text-gray-500 uppercase mb-2">
                  My Key Package
                </h3>
                <button
                  type="button"
                  className="btn btn-secondary text-xs mb-2"
                  onClick={handleGenKP}
                  disabled={loading}
                >
                  Generate Key Package
                </button>
                {kpPublicHex && (
                  <div className="space-y-1">
                    <p className="text-[10px] text-gray-500">Public (share with creator)</p>
                    <div className="flex gap-1">
                      <textarea
                        className="input text-[10px] font-mono h-16 w-full"
                        readOnly
                        value={kpPublicHex}
                      />
                      <button
                        type="button"
                        className="btn btn-secondary text-xs shrink-0"
                        onClick={() => copyText(kpPublicHex)}
                      >
                        Copy
                      </button>
                    </div>
                    <p className="text-[10px] text-amber-600/90">
                      Keep the private bundle below on this device until you receive a Welcome.
                    </p>
                    <div className="flex gap-1">
                      <textarea
                        className="input text-[10px] font-mono h-16 w-full"
                        readOnly
                        value={kpBundleHex}
                      />
                      <button
                        type="button"
                        className="btn btn-secondary text-xs shrink-0"
                        onClick={() => copyText(kpBundleHex)}
                      >
                        Copy
                      </button>
                    </div>
                  </div>
                )}
              </div>

              {activeRole === 'creator' && (
                <div>
                  <h3 className="text-[11px] font-semibold text-gray-500 uppercase mb-2">
                    Add member
                  </h3>
                  <input
                    type="text"
                    className="input w-full mb-2 text-xs"
                    placeholder="New member Peer ID"
                    value={addPeerID}
                    onChange={(e) => setAddPeerID(e.target.value)}
                  />
                  <textarea
                    className="input w-full text-[10px] font-mono h-20 mb-2"
                    placeholder="Key Package (hex)"
                    value={addKpHex}
                    onChange={(e) => setAddKpHex(e.target.value)}
                  />
                  <button
                    type="button"
                    className="btn btn-primary text-xs"
                    onClick={handleAddMember}
                    disabled={loading || !addPeerID.trim() || !addKpHex.trim()}
                  >
                    Add member
                  </button>
                  {welcomeOutHex && (
                    <div className="mt-2">
                      <p className="text-[10px] text-gray-500 mb-1">Welcome (send OOB to invitee)</p>
                      <div className="flex gap-1">
                        <textarea
                          className="input text-[10px] font-mono h-16 w-full"
                          readOnly
                          value={welcomeOutHex}
                        />
                        <button
                          type="button"
                          className="btn btn-secondary text-xs shrink-0"
                          onClick={() => copyText(welcomeOutHex)}
                        >
                          Copy
                        </button>
                      </div>
                    </div>
                  )}
                </div>
              )}

              <div>
                <h3 className="text-[11px] font-semibold text-gray-500 uppercase mb-2">
                  Join via Welcome
                </h3>
                <input
                  type="text"
                  className="input w-full mb-2 text-xs"
                  placeholder="Group ID (defaults to active tab if empty)"
                  value={joinGroupID}
                  onChange={(e) => setJoinGroupID(e.target.value)}
                />
                <textarea
                  className="input w-full text-[10px] font-mono h-16 mb-2"
                  placeholder="Welcome (hex)"
                  value={joinWelcomeHex}
                  onChange={(e) => setJoinWelcomeHex(e.target.value)}
                />
                <textarea
                  className="input w-full text-[10px] font-mono h-16 mb-2"
                  placeholder="Your Key Package private bundle (hex) from Generate"
                  value={joinBundleHex}
                  onChange={(e) => setJoinBundleHex(e.target.value)}
                />
                <button
                  type="button"
                  className="btn btn-primary text-xs"
                  onClick={handleJoinWelcome}
                  disabled={loading}
                >
                  Join group
                </button>
              </div>

              <div>
                <h3 className="text-[11px] font-semibold text-gray-500 uppercase mb-2">
                  Online members (active view)
                </h3>
                {members.length === 0 ? (
                  <p className="text-[10px] text-gray-600 italic">No members listed yet</p>
                ) : (
                  <ul className="space-y-1">
                    {members.map((m) => (
                      <li
                        key={m.peer_id}
                        className="flex items-center gap-2 text-[11px] font-mono text-gray-300"
                      >
                        <span
                          className={`h-2 w-2 rounded-full ${
                            m.is_online ? 'bg-green-500' : 'bg-gray-600'
                          }`}
                          title={m.is_online ? 'connected' : 'not connected'}
                        />
                        {shortID(m.peer_id)}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            </div>
          )}
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
