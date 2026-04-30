import { FormEvent } from 'react'
import { Button } from '../../ui/button'
import MentionTextarea from './MentionTextarea'
import { MentionCandidate } from '../../../lib/chatModel'

interface PostComposerCardProps {
  title: string
  body: string
  submitting: boolean
  mentionCandidates: MentionCandidate[]
  onTitleChange: (value: string) => void
  onBodyChange: (value: string) => void
  onSubmit: () => Promise<void>
}

export default function PostComposerCard({
  title,
  body,
  submitting,
  mentionCandidates,
  onTitleChange,
  onBodyChange,
  onSubmit,
}: PostComposerCardProps) {
  const handleSubmit = async (e: FormEvent) => {
    e.preventDefault()
    await onSubmit()
  }

  return (
    <form
      onSubmit={handleSubmit}
      className="rounded-xl border border-slate-800 bg-slate-900/50 p-4 shadow-sm"
    >
      <p className="mb-3 text-xs font-semibold uppercase tracking-wide text-slate-500">Tạo bài viết</p>
      <input
        type="text"
        value={title}
        onChange={(e) => onTitleChange(e.target.value)}
        placeholder="Tiêu đề (không bắt buộc)"
        className="w-full border-b border-slate-800 bg-transparent pb-2 text-sm font-semibold text-slate-100 outline-none transition placeholder:text-slate-500 focus:border-emerald-500/50"
      />
      <div className="mt-3">
        <MentionTextarea
          value={body}
          onChange={onBodyChange}
          placeholder="Nội dung thảo luận của bạn..."
          candidates={mentionCandidates}
          disabled={submitting}
          rows={3}
        />
      </div>
      <div className="mt-3 flex justify-end">
        <Button
          type="submit"
          disabled={submitting || !body.trim()}
          className="h-8 bg-emerald-500 px-4 text-xs font-semibold text-slate-950 hover:bg-emerald-400"
        >
          {submitting ? 'Đang đăng...' : 'Đăng bài'}
        </Button>
      </div>
    </form>
  )
}
