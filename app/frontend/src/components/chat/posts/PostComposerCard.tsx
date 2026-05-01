import { FormEvent, useState, KeyboardEvent, useRef, useEffect } from 'react'
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
  const [isExpanded, setIsExpanded] = useState(false)
  const formRef = useRef<HTMLFormElement>(null)

  const handleSubmit = async (e?: FormEvent) => {
    e?.preventDefault()
    if (!body.trim()) return
    await onSubmit()
    setIsExpanded(false)
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (body.trim()) {
        void handleSubmit()
      }
    }
  }

  // Handle click outside to collapse if empty
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (formRef.current && !formRef.current.contains(event.target as Node)) {
        if (!title.trim() && !body.trim()) {
          setIsExpanded(false)
        }
      }
    }
    document.addEventListener('mousedown', handleClickOutside)
    return () => document.removeEventListener('mousedown', handleClickOutside)
  }, [title, body])

  if (!isExpanded) {
    return (
      <div 
        onClick={() => setIsExpanded(true)}
        className="cursor-text rounded-xl border border-slate-800 bg-slate-900/40 p-4 shadow-sm transition hover:bg-slate-800/60"
      >
        <p className="text-sm font-medium text-slate-400">Bạn muốn chia sẻ điều gì với nhóm?</p>
      </div>
    )
  }

  return (
    <form
      ref={formRef}
      onSubmit={handleSubmit}
      className="rounded-xl border border-emerald-500/30 bg-slate-900/80 p-4 shadow-md ring-1 ring-emerald-500/20 transition-all"
    >
      <input
        type="text"
        value={title}
        onChange={(e) => onTitleChange(e.target.value)}
        placeholder="Tiêu đề (không bắt buộc)"
        autoFocus
        className="w-full border-b border-slate-700 bg-transparent pb-2 text-base font-semibold text-slate-100 outline-none transition placeholder:text-slate-500 focus:border-emerald-500/50"
      />
      <div className="mt-3">
        <MentionTextarea
          value={body}
          onChange={onBodyChange}
          placeholder="Nội dung thảo luận của bạn..."
          candidates={mentionCandidates}
          disabled={submitting}
          rows={3}
          onKeyDown={handleKeyDown}
        />
      </div>
      <div className="mt-3 flex items-center justify-between">
        <p className="text-[10px] text-slate-500">Nhấn Enter để gửi, Shift+Enter để xuống dòng</p>
        <div className="flex gap-2">
          <Button
            type="button"
            variant="ghost"
            onClick={() => setIsExpanded(false)}
            className="h-8 text-xs font-medium text-slate-400 hover:text-slate-200"
          >
            Hủy
          </Button>
          <Button
            type="submit"
            disabled={submitting || !body.trim()}
            className="h-8 bg-emerald-500 px-4 text-xs font-bold text-slate-950 hover:bg-emerald-400"
          >
            {submitting ? 'Đang đăng...' : 'Đăng bài'}
          </Button>
        </div>
      </div>
    </form>
  )
}
