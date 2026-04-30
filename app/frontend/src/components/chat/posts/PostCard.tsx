import { ReactNode } from 'react'
import {
  formatMessageTime,
  MentionEntity,
  parseMessageContent,
} from '../../../lib/chatModel'
import { ChatMessage } from '../../../stores/useChatStore'
import { MentionCandidate } from '../../../lib/chatModel'
import CommentList from './CommentList'
import CommentComposer from './CommentComposer'

interface PostCardProps {
  post: ChatMessage
  comments: ChatMessage[]
  expanded: boolean
  commentDraft: string
  mentionCandidates: MentionCandidate[]
  renderMentionedBody: (body: string, mentions?: MentionEntity[]) => ReactNode
  getDisplayName: (peerId: string) => string
  sending: boolean
  onToggleComments: () => void
  onCommentDraftChange: (value: string) => void
  onSendComment: () => Promise<void>
  onReplyComment: (comment: ChatMessage) => void
}

export default function PostCard({
  post,
  comments,
  expanded,
  commentDraft,
  mentionCandidates,
  renderMentionedBody,
  getDisplayName,
  sending,
  onToggleComments,
  onCommentDraftChange,
  onSendComment,
  onReplyComment,
}: PostCardProps) {
  const parsedPost = parseMessageContent(post.content)

  return (
    <article className="rounded-xl border border-slate-800 bg-slate-900/30 p-4 shadow-sm">
      <div className="flex items-center gap-2 text-xs text-slate-400">
        <div className="flex h-7 w-7 items-center justify-center rounded-full border border-slate-700 bg-slate-800 text-[10px] font-bold text-emerald-400">
          {getDisplayName(post.sender).slice(0, 1).toUpperCase()}
        </div>
        <span className="font-medium text-slate-200">{getDisplayName(post.sender)}</span>
        <span>•</span>
        <span>{formatMessageTime(post.timestamp)}</span>
      </div>

      {parsedPost.title && (
        <h3 className="mt-3 text-sm font-semibold text-slate-100">{parsedPost.title}</h3>
      )}
      <p className="mt-2 whitespace-pre-wrap text-sm leading-relaxed text-slate-300">
        {renderMentionedBody(parsedPost.body, parsedPost.mentions)}
      </p>

      <div className="mt-3 border-t border-slate-800/70 pt-2">
        <button
          type="button"
          onClick={onToggleComments}
          className="flex items-center gap-1.5 text-xs font-medium text-slate-400 transition hover:text-emerald-400"
        >
          <span>{comments.length > 0 ? `${comments.length} bình luận` : 'Bình luận'}</span>
        </button>
      </div>

      {expanded && (
        <div className="mt-3 rounded-lg bg-slate-950/50 p-3">
          <CommentList
            comments={comments}
            getDisplayName={getDisplayName}
            onReplyComment={onReplyComment}
            renderMentionedBody={renderMentionedBody}
          />
          <CommentComposer
            value={commentDraft}
            sending={sending}
            mentionCandidates={mentionCandidates}
            placeholder={`Viết bình luận cho ${getDisplayName(post.sender)}...`}
            onChange={onCommentDraftChange}
            onSubmit={onSendComment}
          />
        </div>
      )}
    </article>
  )
}
