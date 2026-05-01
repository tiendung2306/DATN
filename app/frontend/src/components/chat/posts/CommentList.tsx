import { useState } from 'react'
import { ChatMessage } from '../../../stores/useChatStore'
import { formatMessageTime, MentionEntity, parseMessageContent } from '../../../lib/chatModel'
import { ReactNode } from 'react'
import { Reply, ChevronUp } from 'lucide-react'
import { Button } from '../../ui/button'

interface CommentListProps {
  comments: ChatMessage[]
  getDisplayName: (peerId: string) => string
  onReplyComment: (comment: ChatMessage) => void
  renderMentionedBody: (body: string, mentions?: MentionEntity[]) => ReactNode
  onLoadMore?: () => Promise<void>
  totalComments?: number
}

export default function CommentList({
  comments,
  getDisplayName,
  onReplyComment,
  renderMentionedBody,
  onLoadMore,
  totalComments,
}: CommentListProps) {
  const [loadingMore, setLoadingMore] = useState(false)

  if (comments.length === 0) {
    return <p className="text-xs italic text-slate-500">Chưa có bình luận nào.</p>
  }

  const handleLoadMore = async () => {
    if (!onLoadMore || loadingMore) return
    setLoadingMore(true)
    try {
      await onLoadMore()
    } finally {
      setLoadingMore(false)
    }
  }

  return (
    <div className="space-y-2">
      {onLoadMore && comments.length < (totalComments ?? 0) && (
        <div className="flex justify-center pb-2">
          <button
            type="button"
            className="flex items-center gap-1 text-xs font-medium text-slate-400 hover:text-slate-200 transition"
            onClick={handleLoadMore}
            disabled={loadingMore}
          >
            <ChevronUp className="h-3 w-3" />
            {loadingMore ? 'Đang tải...' : 'Xem các bình luận cũ hơn'}
          </button>
        </div>
      )}
      {comments.map((comment) => {
        const parsedComment = parseMessageContent(comment.content)
        return (
          <div key={comment.id} className="group rounded-lg border border-slate-800/40 bg-slate-900/50 px-3.5 py-3 transition hover:border-slate-700/60">
            <div className="mb-2 flex items-center justify-between gap-2 text-[11px] text-slate-400">
              <div className="flex items-center gap-2">
                <div className="flex h-5 w-5 items-center justify-center rounded-full bg-slate-800 text-[9px] font-bold text-emerald-500/80">
                  {getDisplayName(comment.sender).slice(0, 1).toUpperCase()}
                </div>
                <span className="font-semibold text-slate-200">{getDisplayName(comment.sender)}</span>
                <span className="text-slate-600">•</span>
                <span>{formatMessageTime(comment.timestamp)}</span>
              </div>
              <button
                type="button"
                className="flex items-center gap-1 rounded px-1.5 py-0.5 font-medium text-emerald-500/80 transition hover:bg-emerald-500/10 hover:text-emerald-400"
                onClick={() => onReplyComment(comment)}
              >
                <Reply className="h-3 w-3" />
                <span>Trả lời</span>
              </button>
            </div>
            <p className="whitespace-pre-wrap text-xs leading-relaxed text-slate-300">
              {renderMentionedBody(parsedComment.body, parsedComment.mentions)}
            </p>
          </div>
        )
      })}
    </div>
  )
}
