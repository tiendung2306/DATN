import { ReactNode, useState } from 'react'
import { ChatMessage } from '../../stores/useChatStore'
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
import PostCard from './posts/PostCard'

interface PostViewProps {
  activeGroupId: string | null
  messages: ChatMessage[]
  loadingMessages: boolean
  mentionCandidates: MentionCandidate[]
  renderMentionedBody: (body: string, mentions?: MentionEntity[]) => ReactNode
}

export default function PostView({
  activeGroupId,
  messages,
  loadingMessages,
  mentionCandidates,
  renderMentionedBody,
}: PostViewProps) {
  const getDisplayName = useContactStore((s) => s.getDisplayName)

  const [expandedPostId, setExpandedPostId] = useState<string | null>(null)
  const [commentDrafts, setCommentDrafts] = useState<Record<string, string>>({})
  const [replyContextByPost, setReplyContextByPost] = useState<Record<string, string | null>>({})
  const [sendingReplyForPostId, setSendingReplyForPostId] = useState<string | null>(null)
  const [postTitle, setPostTitle] = useState('')
  const [postContent, setPostContent] = useState('')
  const [submittingPost, setSubmittingPost] = useState(false)

  const posts: ChatMessage[] = []
  const commentsByPost: Record<string, ChatMessage[]> = {}

  messages.forEach((msg) => {
    const parsed = parseMessageContent(msg.content)
    if (parsed.type === 'comment' && parsed.postId) {
      if (!commentsByPost[parsed.postId]) {
        commentsByPost[parsed.postId] = []
      }
      commentsByPost[parsed.postId].push(msg)
    } else {
      posts.push(msg)
    }
  })

  const sortedPosts = [...posts].sort((a, b) => b.timestamp - a.timestamp)

  if (!activeGroupId) {
    return (
      <div className="flex flex-1 items-center justify-center text-slate-500">
        Select a channel to view posts.
      </div>
    )
  }

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
      await runtimeClient.sendGroupMessage(activeGroupId, payload)
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
      await runtimeClient.sendGroupMessage(activeGroupId, payload)
      setCommentDrafts((prev) => ({ ...prev, [postId]: '' }))
      setReplyContextByPost((prev) => ({ ...prev, [postId]: null }))
    } catch (err) {
      console.error('Failed to send comment:', err)
    } finally {
      setSendingReplyForPostId(null)
    }
  }

  return (
    <div className="h-full min-w-0 flex-1 overflow-y-auto px-5 py-4">
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
          sortedPosts.map((post) => {
            const comments = [...(commentsByPost[post.id] ?? [])].sort((a, b) => a.timestamp - b.timestamp)
            return (
              <PostCard
                key={post.id}
                post={post}
                comments={comments}
                expanded={expandedPostId === post.id}
                commentDraft={commentDrafts[post.id] ?? ''}
                mentionCandidates={mentionCandidates}
                renderMentionedBody={renderMentionedBody}
                getDisplayName={getDisplayName}
                sending={sendingReplyForPostId === post.id}
                onToggleComments={() => setExpandedPostId((prev) => (prev === post.id ? null : post.id))}
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
                }}
              />
            )
          })
        )}
      </div>
    </div>
  )
}
