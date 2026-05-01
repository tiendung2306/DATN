import { ReactNode, useState, useRef, useEffect } from 'react'
import { ChatMessage, useChatStore } from '../../stores/useChatStore'
import { useContactStore } from '../../stores/useContactStore'
import {
  parseMessageContent,
  serializePostPayload,
  MentionCandidate,
  serializeCommentPayload,
  extractMentionsFromBody,
  MentionEntity,
} from '../../lib/chatModel'
import { runtimeClient } from '../../services/runtime/runtimeClient'
import PostComposerCard from './posts/PostComposerCard'
import PostCard, { PostCardHandle } from './posts/PostCard'
import { Button } from '../ui/button'
import { ChevronUp, Loader2 } from 'lucide-react'

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

  const scrollRef = useRef<HTMLDivElement>(null)

  const sortedPosts = [...messages].sort((a, b) => b.timestamp - a.timestamp)

  const handleCreatePost = async () => {
    if (!postContent.trim()) return

    setSubmittingPost(true)
    try {
      const mentions = extractMentionsFromBody(postContent.trim(), mentionCandidates)
      const payload = serializePostPayload({
        title: postTitle,
        body: postContent.trim(),
        mentions,
      })
      await runtimeClient.sendGroupMessage(activeGroupId!, payload)
      setPostTitle('')
      setPostContent('')
    } catch (err) {
      console.error('Failed to create post:', err)
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
      console.error('Failed to send comment:', err)
    } finally {
      setSendingReplyForPostId(null)
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
          onTitleChange={setPostTitle}
          onBodyChange={setPostContent}
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
                />
              )
            })}
            
            {loadingMore && (
              <div className="flex justify-center py-6">
                <div className="flex items-center gap-2 text-xs text-slate-500">
                  <Loader2 className="h-4 w-4 animate-spin" />
                  <span>Đang tải thêm bài viết...</span>
                </div>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}
