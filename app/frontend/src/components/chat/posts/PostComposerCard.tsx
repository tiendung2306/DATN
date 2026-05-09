import { FormEvent, useState, KeyboardEvent, useRef, useEffect } from 'react'
import { Button } from '../../ui/button'
import MentionTextarea from './MentionTextarea'
import { FileAttachment, MentionCandidate, formatFileSize } from '../../../lib/chatModel'
import { countUnicodeRunes } from '../../../lib/textLimits'
import { cn } from '@/lib/utils'
import { Loader2, Paperclip, X } from 'lucide-react'

interface PostComposerCardProps {
  title: string
  body: string
  submitting: boolean
  mentionCandidates: MentionCandidate[]
  maxTitleRunes: number
  maxBodyRunes: number
  pendingAttachments: FileAttachment[]
  attachingFile: boolean
  maxAttachments: number
  onTitleChange: (value: string) => void
  onBodyChange: (value: string) => void
  onAttachFile: () => Promise<void>
  onRemoveAttachment: (fileId: string) => void
  onSubmit: () => Promise<void>
}

export default function PostComposerCard({
  title,
  body,
  submitting,
  mentionCandidates,
  maxTitleRunes,
  maxBodyRunes,
  pendingAttachments,
  attachingFile,
  maxAttachments,
  onTitleChange,
  onBodyChange,
  onAttachFile,
  onRemoveAttachment,
  onSubmit,
}: PostComposerCardProps) {
  const [isExpanded, setIsExpanded] = useState(false)
  const formRef = useRef<HTMLFormElement>(null)

  const titleUsed = countUnicodeRunes(title.trim())
  const bodyUsed = countUnicodeRunes(body.trim())
  const titleOver = titleUsed > maxTitleRunes
  const bodyOver = bodyUsed > maxBodyRunes
  const cannotSubmit = submitting || !body.trim() || titleOver || bodyOver
  const attachmentLimitReached = pendingAttachments.length >= maxAttachments

  const handleSubmit = async (e?: FormEvent) => {
    e?.preventDefault()
    if (!body.trim() || cannotSubmit) return
    await onSubmit()
    setIsExpanded(false)
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (body.trim() && !cannotSubmit) {
        void handleSubmit()
      }
    }
  }

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
      <div className="space-y-1">
        <input
          type="text"
          value={title}
          onChange={(e) => onTitleChange(e.target.value)}
          placeholder="Tiêu đề (không bắt buộc)"
          autoFocus
          aria-invalid={titleOver}
          className="w-full border-b border-slate-700 bg-transparent pb-2 text-base font-semibold text-slate-100 outline-none transition placeholder:text-slate-500 focus:border-emerald-500/50"
        />
        <div className="flex justify-end">
          <span
            className={cn(
              'text-[10px] tabular-nums font-medium',
              titleOver ? 'text-rose-400' : 'text-slate-500',
            )}
          >
            {titleUsed} / {maxTitleRunes}
          </span>
        </div>
      </div>
      <div className="mt-3 space-y-1">
        <MentionTextarea
          value={body}
          onChange={onBodyChange}
          placeholder="Nội dung thảo luận của bạn..."
          candidates={mentionCandidates}
          disabled={submitting}
          rows={3}
          onKeyDown={handleKeyDown}
          aria-invalid={bodyOver}
        />
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-[10px] text-slate-500">Enter để đăng · Shift+Enter xuống dòng</p>
          <span
            className={cn(
              'text-[10px] tabular-nums font-medium',
              bodyOver ? 'text-rose-400' : 'text-slate-500',
            )}
          >
            {bodyUsed} / {maxBodyRunes} ký tự
          </span>
        </div>
      </div>
      <div className="mt-3 space-y-2 rounded-lg border border-slate-800 bg-slate-950/30 p-2.5">
        <div className="flex items-center justify-between">
          <p className="text-[11px] text-slate-400">Tệp đính kèm ({pendingAttachments.length}/{maxAttachments})</p>
          <Button
            type="button"
            size="xs"
            variant="secondary"
            className="gap-1.5"
            onClick={() => void onAttachFile()}
            disabled={submitting || attachingFile || attachmentLimitReached}
          >
            {attachingFile ? <Loader2 className="h-3 w-3 animate-spin" /> : <Paperclip className="h-3 w-3" />}
            {attachingFile ? 'Đang chuẩn bị...' : 'Đính kèm file'}
          </Button>
        </div>
        {attachmentLimitReached ? (
          <p className="text-[11px] text-amber-300">Đã đạt giới hạn {maxAttachments} file cho một bài viết.</p>
        ) : null}
        {pendingAttachments.length > 0 ? (
          <div className="space-y-1.5">
            {pendingAttachments.map((file) => (
              <div key={file.file_id} className="flex items-center justify-between gap-2 rounded-md border border-slate-800 bg-slate-900/70 px-2 py-1.5">
                <div className="min-w-0">
                  <p className="truncate text-xs font-medium text-slate-200">{file.name}</p>
                  <p className="text-[10px] text-slate-400">{formatFileSize(file.size)}</p>
                </div>
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  className="h-6 w-6 text-slate-400 hover:text-rose-300"
                  onClick={() => onRemoveAttachment(file.file_id)}
                  disabled={submitting || attachingFile}
                >
                  <X className="h-3.5 w-3.5" />
                </Button>
              </div>
            ))}
          </div>
        ) : (
          <p className="text-[11px] text-slate-500">Bạn có thể thêm nhiều tệp vào cùng một bài viết.</p>
        )}
      </div>
      {bodyUsed > maxBodyRunes * 0.85 ? (
        <p className="mt-2 rounded-md border border-amber-500/25 bg-amber-500/10 px-2 py-1.5 text-[11px] leading-snug text-amber-100/90">
          Nội dung rất dài sẽ được gửi dưới dạng file đã mã hóa trong bản cập nhật sau.
        </p>
      ) : null}
      <div className="mt-3 flex items-center justify-end gap-2">
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
          disabled={cannotSubmit}
          className="h-8 bg-emerald-500 px-4 text-xs font-bold text-slate-950 hover:bg-emerald-400"
        >
          {submitting ? 'Đang đăng...' : 'Đăng bài'}
        </Button>
      </div>
    </form>
  )
}
