import { ReactNode, useState, useRef } from 'react'
import { ChatMessage, useChatStore } from '../../stores/useChatStore'
import { useContactStore } from '../../stores/useContactStore'
import {
  serializePostPayload,
  MentionCandidate,
  serializeCommentPayload,
  extractMentionsFromBody,
  MentionEntity,
  FileAttachment,
} from '../../lib/chatModel'
import { runtimeClient } from '../../services/runtime/runtimeClient'
import { formatOutboundSendError } from '../../lib/formatSendError'
import { countUnicodeRunes } from '../../lib/textLimits'
import { useMessageLimitsStore } from '../../stores/useMessageLimitsStore'
import { useToastStore } from '../../stores/useToastStore'
import PostComposerCard from './posts/PostComposerCard'
import PostCard, { PostCardHandle } from './posts/PostCard'
import { Loader2 } from 'lucide-react'

const MAX_ATTACHMENTS_PER_POST = 10

interface PostViewProps {
  activeGroupId: string | null
  messages: ChatMessage[]
  loadingMessages: boolean
  mentionCandidates: MentionCandidate[]
  renderMentionedBody: (body: string, mentions?: MentionEntity[]) => ReactNode
  onLoadMore?: () => Promise<void>
  onLoadComments?: (postId: string) => Promise<void>
  onLoadMoreComments?: (postId: string) => Promise<void>
}

export default function PostView({
  activeGroupId,
  messages,
  loadingMessages,
  mentionCandidates,
  renderMentionedBody,
  onLoadMore,
  onLoadComments,
  onLoadMoreComments,
}: PostViewProps) {
  const getDisplayName = useContactStore((s) => s.getDisplayName)
  const channelTitleMaxRunes = useMessageLimitsStore((s) => s.channelTitleMaxRunes)
  const channelBodyMaxRunes = useMessageLimitsStore((s) => s.channelBodyMaxRunes)
  const channelCommentMaxRunes = useMessageLimitsStore((s) => s.channelCommentMaxRunes)
  const commentsByPost = useChatStore((s) => s.commentsByPost)
  const postRefs = useRef<Record<string, PostCardHandle | null>>({})
  const [loadingMore, setLoadingMore] = useState(false)

  const [expandedPostId, setExpandedPostId] = useState<string | null>(null)
  const [commentDrafts, setCommentDrafts] = useState<Record<string, string>>({})
  const [replyContextByPost, setReplyContextByPost] = useState<Record<string, string | null>>({})
  const [sendingReplyForPostId, setSendingReplyForPostId] = useState<string | null>(null)
  const [postTitle, setPostTitle] = useState('')
  const [postContent, setPostContent] = useState('')
  const [submittingPost, setSubmittingPost] = useState(false)
  const [attachingFile, setAttachingFile] = useState(false)
  const [pendingAttachments, setPendingAttachments] = useState<FileAttachment[]>([])
  const [fileStateByKey, setFileStateByKey] = useState<Record<string, 'idle' | 'downloading' | 'completed' | 'failed'>>({})
  const [filePathByKey, setFilePathByKey] = useState<Record<string, string>>({})

  const scrollRef = useRef<HTMLDivElement>(null)

  const sortedPosts = [...messages].sort((a, b) => b.timestamp - a.timestamp)

  const attachmentStateKey = (messageId: string, fileId: string) => `${messageId}:${fileId}`

  const fromPreparedDTO = (dto: unknown): FileAttachment | null => {
    const rec = dto as Record<string, unknown>
    const fileID = String(rec.file_id ?? rec.FileID ?? '').trim()
    const name = String(rec.file_name ?? rec.FileName ?? '').trim()
    const ext = String(rec.file_ext ?? rec.FileExt ?? '').trim()
    const mimeType = String(rec.mime_type ?? rec.MimeType ?? '').trim()
    const size = Number(rec.plaintext_size ?? rec.PlaintextSize ?? 0)
    const sha256 = String(rec.plaintext_sha256_hex ?? rec.PlaintextSHA256Hex ?? '').trim()
    const chunkCount = Number(rec.chunk_count ?? rec.ChunkCount ?? 0)
    const chunkSize = Number(rec.chunk_size ?? rec.ChunkSize ?? 0)
    const exportEpoch = Number(rec.export_epoch ?? rec.ExportEpoch ?? 0)
    const senderPeerID = String(rec.sender_peer_id ?? rec.SenderPeerID ?? '').trim()
    if (!fileID || !name || !mimeType || !sha256 || !senderPeerID) return null
    return {
      type: 'file',
      file_id: fileID,
      name,
      ext: ext || undefined,
      mime_type: mimeType,
      size: Number.isFinite(size) ? size : 0,
      sha256,
      chunk_count: Number.isFinite(chunkCount) ? chunkCount : 0,
      chunk_size: Number.isFinite(chunkSize) ? chunkSize : 0,
      export_epoch: Number.isFinite(exportEpoch) ? exportEpoch : 0,
      sender_peer_id: senderPeerID,
    }
  }

  const handleCreatePost = async () => {
    if (!postContent.trim()) return

    const titleTrim = postTitle.trim()
    const bodyTrim = postContent.trim()
    if (countUnicodeRunes(titleTrim) > channelTitleMaxRunes) {
      const mapped = formatOutboundSendError(new Error('ERR_CHANNEL_PAYLOAD_INVALID: title exceeds'))
      useToastStore.getState().pushToast({ title: mapped.title, description: mapped.description, variant: mapped.variant })
      return
    }
    if (countUnicodeRunes(bodyTrim) > channelBodyMaxRunes) {
      const mapped = formatOutboundSendError(new Error('ERR_CHANNEL_PAYLOAD_INVALID: body exceeds'))
      useToastStore.getState().pushToast({ title: mapped.title, description: mapped.description, variant: mapped.variant })
      return
    }

    setSubmittingPost(true)
    try {
      const mentions = extractMentionsFromBody(bodyTrim, mentionCandidates)
      const payload = serializePostPayload({
        title: postTitle,
        body: bodyTrim,
        mentions,
        attachments: pendingAttachments,
      })
      await runtimeClient.sendGroupMessage(activeGroupId!, payload)
      setPostTitle('')
      setPostContent('')
      setPendingAttachments([])
    } catch (err) {
      const mapped = formatOutboundSendError(err)
      useToastStore.getState().pushToast({ title: mapped.title, description: mapped.description, variant: mapped.variant })
    } finally {
      setSubmittingPost(false)
    }
  }

  const handleSendComment = async (postId: string, payload: string) => {
    setSendingReplyForPostId(postId)
    try {
      await runtimeClient.sendGroupMessage(activeGroupId!, payload)
      setCommentDrafts((prev) => ({ ...prev, [postId]: '' }))
      setReplyContextByPost((prev) => ({ ...prev, [postId]: null }))
    } catch (err) {
      const mapped = formatOutboundSendError(err)
      useToastStore.getState().pushToast({ title: mapped.title, description: mapped.description, variant: mapped.variant })
    } finally {
      setSendingReplyForPostId(null)
    }
  }

  const isUserCancelled = (err: unknown): boolean => {
    const raw = err instanceof Error ? err.message : String(err)
    return raw.includes('ERR_USER_CANCELLED')
  }

  const handleAttachFile = async () => {
    if (!activeGroupId || attachingFile || submittingPost) return
    if (pendingAttachments.length >= MAX_ATTACHMENTS_PER_POST) {
      const mapped = formatOutboundSendError(new Error('ERR_TOO_MANY_ATTACHMENTS'))
      useToastStore.getState().pushToast({ title: mapped.title, description: mapped.description, variant: mapped.variant })
      return
    }
    setAttachingFile(true)
    try {
      const prepared = await runtimeClient.prepareGroupFile(activeGroupId)
      const attachment = fromPreparedDTO(prepared)
      if (!attachment) {
        throw new Error('ERR_INVALID_FILE_PREPARE_DTO')
      }
      setPendingAttachments((prev) => {
        if (prev.some((entry) => entry.file_id === attachment.file_id)) return prev
        return [...prev, attachment].slice(0, MAX_ATTACHMENTS_PER_POST)
      })
    } catch (err) {
      if (!isUserCancelled(err)) {
        const mapped = formatOutboundSendError(err)
        useToastStore.getState().pushToast({ title: mapped.title, description: mapped.description, variant: mapped.variant })
      }
    } finally {
      setAttachingFile(false)
    }
  }

  const handleDownloadFile = async (messageId: string, file: FileAttachment) => {
    if (!activeGroupId) return
    const key = attachmentStateKey(messageId, file.file_id)
    setFileStateByKey((prev) => ({ ...prev, [key]: 'downloading' }))
    try {
      const path = await runtimeClient.downloadGroupFile(activeGroupId, file.file_id, file.sender_peer_id, file.name)
      if (path) {
        setFileStateByKey((prev) => ({ ...prev, [key]: 'completed' }))
        setFilePathByKey((prev) => ({ ...prev, [key]: path }))
      } else {
        setFileStateByKey((prev) => ({ ...prev, [key]: 'idle' }))
      }
    } catch (err) {
      if (isUserCancelled(err)) {
        setFileStateByKey((prev) => ({ ...prev, [key]: 'idle' }))
        return
      }
      setFileStateByKey((prev) => ({ ...prev, [key]: 'failed' }))
      const mapped = formatOutboundSendError(err)
      useToastStore.getState().pushToast({ title: mapped.title, description: mapped.description, variant: mapped.variant })
    }
  }

  const handleOpenFile = async (messageId: string, file: FileAttachment) => {
    if (!activeGroupId) return
    const key = attachmentStateKey(messageId, file.file_id)
    const fallbackPath = filePathByKey[key] ?? ''
    try {
      const openedPath = await runtimeClient.openFileTransfer(activeGroupId, file.file_id, fallbackPath)
      if (openedPath) {
        setFileStateByKey((prev) => ({ ...prev, [key]: 'completed' }))
        setFilePathByKey((prev) => ({ ...prev, [key]: openedPath }))
      }
    } catch (err) {
      const mapped = formatOutboundSendError(err)
      useToastStore.getState().pushToast({ title: mapped.title, description: mapped.description, variant: mapped.variant })
    }
  }

  const handleOpenFileLocation = async (messageId: string, file: FileAttachment) => {
    if (!activeGroupId) return
    const key = attachmentStateKey(messageId, file.file_id)
    const fallbackPath = filePathByKey[key] ?? ''
    try {
      await runtimeClient.openFileTransferLocation(activeGroupId, file.file_id, fallbackPath)
    } catch (err) {
      const mapped = formatOutboundSendError(err)
      useToastStore.getState().pushToast({ title: mapped.title, description: mapped.description, variant: mapped.variant })
    }
  }

  // Handle scroll for infinite loading (scrolling DOWN for older posts)
  const handleScroll = async () => {
    const el = scrollRef.current
    if (!el || !onLoadMore || loadingMore) return

    // If we are close to the bottom, load more
    if (el.scrollHeight - el.scrollTop <= el.clientHeight + 200) {
      setLoadingMore(true)
      try {
        await onLoadMore()
      } finally {
        setLoadingMore(false)
      }
    }
  }

  if (!activeGroupId) {
    return (
      <div className="flex flex-1 items-center justify-center text-slate-500">
        Select a channel to view posts.
      </div>
    )
  }

  return (
    <div 
      ref={scrollRef}
      onScroll={handleScroll}
      className="h-full min-w-0 flex-1 overflow-y-auto px-5 py-4"
    >
      <div className="mx-auto w-full max-w-3xl space-y-5">
        <PostComposerCard
          title={postTitle}
          body={postContent}
          submitting={submittingPost}
          mentionCandidates={mentionCandidates}
          maxTitleRunes={channelTitleMaxRunes}
          maxBodyRunes={channelBodyMaxRunes}
          pendingAttachments={pendingAttachments}
          attachingFile={attachingFile}
          maxAttachments={MAX_ATTACHMENTS_PER_POST}
          onTitleChange={setPostTitle}
          onBodyChange={setPostContent}
          onAttachFile={handleAttachFile}
          onRemoveAttachment={(fileId) =>
            setPendingAttachments((prev) => prev.filter((entry) => entry.file_id !== fileId))
          }
          onSubmit={handleCreatePost}
        />

        {loadingMessages ? (
          <p className="text-xs text-slate-500">Đang tải...</p>
        ) : sortedPosts.length === 0 ? (
          <p className="text-xs text-slate-500 text-center py-10">
            Chưa có bài viết nào trong kênh này. Hãy bắt đầu thảo luận!
          </p>
        ) : (
          <>
            {sortedPosts.map((post) => {
              const comments = [...(commentsByPost[post.id] ?? [])].sort((a, b) => a.timestamp - b.timestamp)
              return (
                <PostCard
                  key={post.id}
                  ref={(el) => (postRefs.current[post.id] = el)}
                  post={post}
                  comments={comments}
                  expanded={expandedPostId === post.id}
                  commentDraft={commentDrafts[post.id] ?? ''}
                  commentMaxRunes={channelCommentMaxRunes}
                  mentionCandidates={mentionCandidates}
                  renderMentionedBody={renderMentionedBody}
                  getDisplayName={getDisplayName}
                  sending={sendingReplyForPostId === post.id}
                  onToggleComments={() => {
                    setExpandedPostId((prev) => {
                      const expanding = prev !== post.id
                      if (expanding && onLoadComments) {
                        onLoadComments(post.id).catch(console.error)
                      }
                      return expanding ? post.id : null
                    })
                  }}
                  onLoadMoreComments={onLoadMoreComments}
                  onCommentDraftChange={(value) => setCommentDrafts((prev) => ({ ...prev, [post.id]: value }))}
                  onSendComment={async () => {
                    const draftBody = commentDrafts[post.id]?.trim() ?? ''
                    if (!draftBody) return
                    if (countUnicodeRunes(draftBody) > channelCommentMaxRunes) {
                      const mapped = formatOutboundSendError(new Error('ERR_CHANNEL_PAYLOAD_INVALID: body exceeds'))
                      useToastStore.getState().pushToast({
                        title: mapped.title,
                        description: mapped.description,
                        variant: mapped.variant,
                      })
                      return
                    }
                    const mentions = extractMentionsFromBody(draftBody, mentionCandidates)
                    const replyToCommentId = replyContextByPost[post.id]
                    await handleSendComment(
                      post.id,
                      serializeCommentPayload({
                        postId: post.id,
                        body: draftBody,
                        mentions,
                        replyToCommentId: replyToCommentId || undefined,
                      }),
                    )
                  }}
                  onReplyComment={(comment) => {
                    const mentionDisplayName = getDisplayName(comment.sender)
                    const current = commentDrafts[post.id] ?? ''
                    const mentionPrefix = `@${mentionDisplayName} `
                    setExpandedPostId(post.id)
                    setReplyContextByPost((prev) => ({ ...prev, [post.id]: comment.id }))
                    setCommentDrafts((prev) => ({
                      ...prev,
                      [post.id]: current.includes(mentionPrefix) ? current : `${mentionPrefix}${current}`.trimStart(),
                    }))

                    setTimeout(() => {
                      postRefs.current[post.id]?.focusComposer()
                      const el = document.getElementById(`composer-${post.id}`)
                      el?.scrollIntoView({ behavior: 'smooth', block: 'center' })
                    }, 100)
                  }}
                  onDownloadFile={handleDownloadFile}
                  onOpenFile={handleOpenFile}
                  onOpenFileLocation={handleOpenFileLocation}
                  fileStateByKey={fileStateByKey}
                  fileLocalPathByKey={filePathByKey}
                />
              )
            })}
            
            {loadingMore && (
              <div className="flex justify-center py-6">
                <div className="flex items-center gap-2 text-xs text-slate-500">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  <span>Loading more posts...</span>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
