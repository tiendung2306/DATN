import { useState } from 'react'
import { ChatMessage } from '../../stores/useChatStore'
import { useContactStore } from '../../stores/useContactStore'
import { parseMessageContent, formatMessageTime } from '../../lib/chatModel'
import { runtimeClient } from '../../services/runtime/runtimeClient'
import { Button } from '../ui/button'
import { MessageSquare, Send, X } from 'lucide-react'

interface PostViewProps {
  activeGroupId: string | null
  messages: ChatMessage[]
  loadingMessages: boolean
  localPeerId: string
}

export default function PostView({
  activeGroupId,
  messages,
  loadingMessages,
  localPeerId,
}: PostViewProps) {
  const getDisplayName = useContactStore((s) => s.getDisplayName)

  // Thread State
  const [activeThreadId, setActiveThreadId] = useState<string | null>(null)
  const [replyText, setReplyText] = useState('')
  const [submittingReply, setSubmittingReply] = useState(false)

  // New Post State
  const [postTitle, setPostTitle] = useState('')
  const [postContent, setPostContent] = useState('')
  const [submittingPost, setSubmittingPost] = useState(false)

  if (!activeGroupId) {
    return (
      <div className="flex flex-1 items-center justify-center text-slate-500">
        Select a channel to view posts.
      </div>
    )
  }

  // Group Messages into Posts & Replies
  const posts: ChatMessage[] = []
  const repliesByParent: Record<string, ChatMessage[]> = {}

  messages.forEach((msg) => {
    const parsed = parseMessageContent(msg.content)
    if (parsed.type === 'reply' && parsed.parentId) {
      if (!repliesByParent[parsed.parentId]) {
        repliesByParent[parsed.parentId] = []
      }
      repliesByParent[parsed.parentId].push(msg)
    } else {
      posts.push(msg)
    }
  })

  // Sort posts by timestamp descending (newest at the top)
  const sortedPosts = [...posts].sort((a, b) => b.timestamp - a.timestamp)

  const handleCreatePost = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!postContent.trim()) return

    setSubmittingPost(true)
    try {
      const payload = JSON.stringify({
        type: 'post',
        title: postTitle.trim(),
        content: postContent.trim(),
      })
      await runtimeClient.sendGroupMessage(activeGroupId, payload)
      setPostTitle('')
      setPostContent('')
    } catch (err) {
      console.error('Failed to create post:', err)
    } finally {
      setSubmittingPost(false)
    }
  }

  const handleSendReply = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!activeThreadId || !replyText.trim()) return

    setSubmittingReply(true)
    try {
      const payload = JSON.stringify({
        type: 'reply',
        parent_id: activeThreadId,
        content: replyText.trim(),
      })
      await runtimeClient.sendGroupMessage(activeGroupId, payload)
      setReplyText('')
    } catch (err) {
      console.error('Failed to send reply:', err)
    } finally {
      setSubmittingReply(false)
    }
  }

  const selectedPost = sortedPosts.find((p) => p.id === activeThreadId)
  const activeReplies = activeThreadId ? repliesByParent[activeThreadId] ?? [] : []

  return (
    <div className="flex h-full min-w-0 flex-1">
      {/* POSTS TIMELINE */}
      <div className="flex min-w-0 flex-1 flex-col overflow-y-auto px-5 py-4 space-y-6">
        {/* New Post Box */}
        <form
          onSubmit={handleCreatePost}
          className="rounded-xl border border-slate-800 bg-slate-900/40 p-4 space-y-3 shadow-sm"
        >
          <input
            type="text"
            value={postTitle}
            onChange={(e) => setPostTitle(e.target.value)}
            placeholder="Tiêu đề bài viết (Không bắt buộc)..."
            className="w-full bg-transparent text-slate-100 placeholder:text-slate-500 font-semibold text-sm outline-none border-b border-slate-800 pb-2 focus:border-emerald-500/50 transition"
          />
          <textarea
            value={postContent}
            onChange={(e) => setPostContent(e.target.value)}
            placeholder="Nội dung thảo luận của bạn..."
            rows={3}
            className="w-full bg-transparent text-slate-200 placeholder:text-slate-500 text-sm outline-none resize-none transition"
          />
          <div className="flex justify-end pt-2">
            <Button
              type="submit"
              disabled={submittingPost || !postContent.trim()}
              className="bg-emerald-500 text-slate-950 hover:bg-emerald-400 h-8 text-xs font-semibold px-4"
            >
              {submittingPost ? 'Đang đăng...' : 'Đăng bài'}
            </Button>
          </div>
        </form>

        {/* Posts List */}
        {loadingMessages ? (
          <p className="text-xs text-slate-500">Đang tải...</p>
        ) : sortedPosts.length === 0 ? (
          <p className="text-xs text-slate-500 text-center py-10">
            Chưa có bài viết nào trong kênh này. Hãy bắt đầu thảo luận!
          </p>
        ) : (
          sortedPosts.map((post) => {
            const parsed = parseMessageContent(post.content)
            const replies = repliesByParent[post.id] ?? []
            return (
              <div
                key={post.id}
                className="flex flex-col rounded-xl border border-slate-800 bg-slate-900/20 p-5 space-y-3 hover:border-slate-700/60 transition shadow-sm"
              >
                {/* Author row */}
                <div className="flex items-center gap-2 text-xs text-slate-400">
                  <div className="h-6 w-6 rounded-full bg-slate-800 flex items-center justify-center font-bold text-[10px] text-emerald-400 border border-slate-700">
                    {getDisplayName(post.sender).slice(0, 1).toUpperCase()}
                  </div>
                  <span className="font-medium text-slate-200">{getDisplayName(post.sender)}</span>
                  <span>•</span>
                  <span>{formatMessageTime(post.timestamp)}</span>
                </div>

                {/* Title & Body */}
                {parsed.title && (
                  <h3 className="text-sm font-semibold text-slate-100">{parsed.title}</h3>
                )}
                <p className="text-slate-300 text-sm leading-relaxed whitespace-pre-wrap">
                  {parsed.content}
                </p>

                {/* Bottom Bar */}
                <div className="flex items-center gap-2 pt-2 border-t border-slate-800/60">
                  <button
                    type="button"
                    onClick={() => setActiveThreadId(post.id)}
                    className="flex items-center gap-1.5 text-xs text-slate-400 hover:text-emerald-400 transition"
                  >
                    <MessageSquare className="h-3.5 w-3.5" />
                    <span>
                      {replies.length > 0 ? `${replies.length} trả lời` : 'Trả lời'}
                    </span>
                  </button>
                </div>
              </div>
            )
          })
        )}
      </div>

      {/* SIDE THREAD PANEL */}
      {activeThreadId && selectedPost && (
        <aside className="w-96 border-l border-slate-800 bg-slate-950 flex flex-col h-full animate-in slide-in-from-right duration-200">
          <div className="flex items-center justify-between border-b border-slate-800 px-4 py-3">
            <span className="text-sm font-semibold text-slate-100">Cuộc thảo luận</span>
            <button
              type="button"
              onClick={() => setActiveThreadId(null)}
              className="p-1 rounded-md text-slate-500 hover:bg-slate-800 hover:text-slate-300 transition"
            >
              <X className="h-4 w-4" />
            </button>
          </div>

          <div className="flex-1 overflow-y-auto px-4 py-3 space-y-4">
            {/* Original Post */}
            <div className="pb-3 border-b border-slate-800">
              <div className="flex items-center gap-2 text-xs text-slate-400 mb-2">
                <span className="font-medium text-slate-200">
                  {getDisplayName(selectedPost.sender)}
                </span>
                <span>{formatMessageTime(selectedPost.timestamp)}</span>
              </div>
              {parseMessageContent(selectedPost.content).title && (
                <h4 className="text-xs font-semibold text-slate-100 mb-1">
                  {parseMessageContent(selectedPost.content).title}
                </h4>
              )}
              <p className="text-slate-300 text-xs whitespace-pre-wrap leading-relaxed">
                {parseMessageContent(selectedPost.content).content}
              </p>
            </div>

            {/* Replies List */}
            {activeReplies.map((reply) => {
              const rParsed = parseMessageContent(reply.content)
              return (
                <div key={reply.id} className="text-xs space-y-1">
                  <div className="flex items-center gap-2 text-slate-400">
                    <span className="font-medium text-slate-200">
                      {getDisplayName(reply.sender)}
                    </span>
                    <span>{formatMessageTime(reply.timestamp)}</span>
                  </div>
                  <p className="text-slate-300 leading-relaxed whitespace-pre-wrap">
                    {rParsed.content}
                  </p>
                </div>
              )
            })}
          </div>

          {/* Reply Composer */}
          <form
            onSubmit={handleSendReply}
            className="border-t border-slate-800 p-3 flex gap-2 items-center bg-slate-900/60"
          >
            <input
              type="text"
              value={replyText}
              onChange={(e) => setReplyText(e.target.value)}
              placeholder={`Trả lời ${getDisplayName(selectedPost.sender)}...`}
              className="flex-1 bg-slate-800/80 border border-slate-700/50 rounded-lg px-3 py-2 text-slate-200 text-xs placeholder:text-slate-500 focus:outline-none focus:border-emerald-500/50 transition"
            />
            <Button
              type="submit"
              disabled={submittingReply || !replyText.trim()}
              size="icon"
              className="h-8 w-8 bg-emerald-500 text-slate-950 hover:bg-emerald-400 flex-shrink-0"
            >
              <Send className="h-3.5 w-3.5" />
            </Button>
          </form>
        </aside>
      )}
    </div>
  )
}
