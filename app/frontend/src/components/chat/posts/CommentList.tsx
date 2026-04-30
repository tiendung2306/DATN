import { ChatMessage } from '../../../stores/useChatStore'
import { formatMessageTime, MentionEntity, parseMessageContent } from '../../../lib/chatModel'
import { ReactNode } from 'react'

interface CommentListProps {
  comments: ChatMessage[]
  getDisplayName: (peerId: string) => string
  onReplyComment: (comment: ChatMessage) => void
  renderMentionedBody: (body: string, mentions?: MentionEntity[]) => ReactNode
}

export default function CommentList({
  comments,
  getDisplayName,
  onReplyComment,
  renderMentionedBody,
}: CommentListProps) {
  if (comments.length === 0) {
    return <p className="text-xs italic text-slate-500">Chưa có bình luận nào.</p>
  }

  return (
    <div className="space-y-2">
      {comments.map((comment) => {
        const parsedComment = parseMessageContent(comment.content)
        return (
          <div key={comment.id} className="rounded-md border border-slate-800/80 bg-slate-900/70 px-3 py-2">
            <div className="mb-1 flex items-center gap-2 text-[11px] text-slate-400">
              <span className="font-medium text-slate-200">{getDisplayName(comment.sender)}</span>
              <span>{formatMessageTime(comment.timestamp)}</span>
            </div>
            <p className="whitespace-pre-wrap text-xs leading-relaxed text-slate-300">
              {renderMentionedBody(parsedComment.body, parsedComment.mentions)}
            </p>
            <button
              type="button"
              className="mt-1 text-[11px] text-slate-400 hover:text-emerald-300"
              onClick={() => onReplyComment(comment)}
            >
              Trả lời
            </button>
          </div>
        )
      })}
    </div>
  )
}
