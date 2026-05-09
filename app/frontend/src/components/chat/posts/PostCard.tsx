import { ReactNode, useRef, useImperativeHandle, forwardRef } from 'react'
import {
  formatMessageTime,
  FileAttachment,
  MentionEntity,
  parseMessageContent,
  isFilePayload,
  MentionCandidate,
} from '../../../lib/chatModel'
import { ChatMessage } from '../../../stores/useChatStore'
import CommentList from './CommentList'
import CommentComposer, { CommentComposerHandle } from './CommentComposer'
import FileAttachmentCard from '../FileAttachmentCard'

interface PostCardProps {
  post: ChatMessage
  comments: ChatMessage[]
  expanded: boolean
  commentDraft: string
  commentMaxRunes: number
  mentionCandidates: MentionCandidate[]
  renderMentionedBody: (body: string, mentions?: MentionEntity[]) => ReactNode
  getDisplayName: (peerId: string) => string
  sending: boolean
  onToggleComments: () => void
  onCommentDraftChange: (value: string) => void
  onSendComment: () => Promise<void>
  onReplyComment: (comment: ChatMessage) => void
  onLoadMoreComments?: (postId: string) => Promise<void>
  onDownloadFile?: (messageId: string, file: FileAttachment) => void
  onOpenFile?: (messageId: string, file: FileAttachment) => void
  fileStateByKey?: Record<string, 'idle' | 'downloading' | 'completed' | 'failed'>
  fileLocalPathByKey?: Record<string, string>
}

export interface PostCardHandle {
  focusComposer: () => void
}

const PostCard = forwardRef<PostCardHandle, PostCardProps>(
  (
    {
      post,
      comments,
      expanded,
      commentDraft,
      commentMaxRunes,
      mentionCandidates,
      renderMentionedBody,
      getDisplayName,
      sending,
      onToggleComments,
      onCommentDraftChange,
      onSendComment,
      onReplyComment,
      onLoadMoreComments,
      onDownloadFile,
      onOpenFile,
      fileStateByKey,
      fileLocalPathByKey,
    },
    ref,
  ) => {
    const parsedPost = parseMessageContent(post.content)
    const attachments = parsedPost.attachments ?? (parsedPost.file ? [parsedPost.file] : [])
    const isFilePost = isFilePayload(post)
    const composerRef = useRef<CommentComposerHandle>(null)

    useImperativeHandle(ref, () => ({
      focusComposer: () => {
        composerRef.current?.focus()
      },
    }))

  return (
    <article className="rounded-xl border border-slate-800/80 bg-[#0F172A] p-5 shadow-sm transition hover:border-slate-700/80">
      <div className="flex items-center gap-2 text-xs text-slate-400">
        <div className="flex h-8 w-8 items-center justify-center rounded-full border border-slate-700 bg-slate-800 text-xs font-bold text-emerald-400">
          {getDisplayName(post.sender).slice(0, 1).toUpperCase()}
        </div>
        <span className="font-semibold text-slate-200">{getDisplayName(post.sender)}</span>
        <span className="text-slate-600">•</span>
        <span>{formatMessageTime(post.timestamp)}</span>
      </div>

      {parsedPost.title && (
        <h3 className="mt-3 text-lg font-bold text-slate-50">{parsedPost.title}</h3>
      )}
      <p className="mt-2 whitespace-pre-wrap [overflow-wrap:anywhere] text-[15px] leading-relaxed text-slate-300">
        {renderMentionedBody(parsedPost.body || (attachments[0] ? `Da chia se tep: ${attachments[0].name}` : ''), parsedPost.mentions)}
      </p>
      {attachments.map((file) => {
        const key = `${post.id}:${file.file_id}`
        return (
          <FileAttachmentCard
            key={key}
            file={file}
            isMine={post.isMine}
            state={fileStateByKey?.[key] ?? 'idle'}
            localPath={fileLocalPathByKey?.[key]}
            onDownload={onDownloadFile ? () => onDownloadFile(post.id, file) : undefined}
            onOpenFile={onOpenFile ? () => onOpenFile(post.id, file) : undefined}
          />
        )
      })}

      <div className="mt-4 border-t border-slate-800/50 pt-3">
        <button
          type="button"
          onClick={onToggleComments}
          className="flex items-center gap-1.5 text-xs font-semibold text-slate-400 transition hover:text-emerald-400"
        >
          <span>{post.commentCount && post.commentCount > 0 ? `${post.commentCount} binh luan` : isFilePost ? 'Thao luan ve tep' : 'Thao luan'}</span>
        </button>
      </div>

      {expanded && (
        <div className="mt-3 rounded-xl bg-slate-950/40 border border-slate-800/40 p-4 shadow-inner">
          <CommentList
            comments={comments}
            totalComments={post.commentCount}
            getDisplayName={getDisplayName}
            onReplyComment={onReplyComment}
            renderMentionedBody={renderMentionedBody}
            onLoadMore={() => {
              if (onLoadMoreComments) return onLoadMoreComments(post.id)
              return Promise.resolve()
            }}
          />
          <CommentComposer
            ref={composerRef}
            postId={post.id}
            value={commentDraft}
            sending={sending}
            mentionCandidates={mentionCandidates}
            maxBodyRunes={commentMaxRunes}
            placeholder={`Viết bình luận cho ${getDisplayName(post.sender)}...`}
            onChange={onCommentDraftChange}
            onSubmit={onSendComment}
          />
        </div>
      )}
    </article>
  )
})

export default PostCard
