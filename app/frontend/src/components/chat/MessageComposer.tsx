import { KeyboardEvent, useRef } from 'react'
import { Button } from '../ui/button'
import { Paperclip, SendHorizontal } from 'lucide-react'
import MentionTextarea, { MentionTextareaHandle } from './posts/MentionTextarea'
import { MentionCandidate } from '../../lib/chatModel'
import { countUnicodeRunes } from '../../lib/textLimits'
import { cn } from '@/lib/utils'

interface MessageComposerProps {
  value: string
  disabled: boolean
  inputDisabled?: boolean
  attachingFile?: boolean
  mentionCandidates: MentionCandidate[]
  onChange: (value: string) => void
  onSend: () => void
  onAttachFile?: () => void
  /** When set, shows a rune counter and blocks send while over limit (preflight also runs in useChatActions). */
  maxRunes?: number
}

export default function MessageComposer({
  value,
  disabled,
  inputDisabled = false,
  attachingFile = false,
  mentionCandidates,
  onChange,
  onSend,
  onAttachFile,
  maxRunes,
}: MessageComposerProps) {
  const inputRef = useRef<MentionTextareaHandle>(null)
  const usedRunes = maxRunes != null ? countUnicodeRunes(value.trim()) : 0
  const overLimit = maxRunes != null && usedRunes > maxRunes
  const sendBlocked = disabled || attachingFile || !value.trim() || overLimit

  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      if (!sendBlocked) {
        onSend()
        requestAnimationFrame(() => inputRef.current?.focus())
      }
    }
  }

  return (
    <div className="rounded-lg border border-slate-700 bg-slate-800 p-2.5">
      <div className="flex items-end gap-2">
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="mb-1 text-slate-400 hover:text-slate-100"
          disabled={disabled || attachingFile || !onAttachFile}
          onClick={onAttachFile}
        >
          <Paperclip className="h-4 w-4" />
        </Button>
        <div className="w-full">
          <MentionTextarea
            ref={inputRef}
            value={value}
            onChange={onChange}
            placeholder="Nhập tin nhắn đã mã hóa…"
            candidates={mentionCandidates}
            disabled={inputDisabled}
            rows={2}
            onKeyDown={handleKeyDown}
            aria-invalid={overLimit}
          />
        </div>
        <Button
          onClick={onSend}
          disabled={sendBlocked}
          className="mb-1 bg-emerald-500 px-3 text-slate-900 hover:bg-emerald-400"
        >
          <SendHorizontal className="h-4 w-4" />
        </Button>
      </div>
      <div className="mt-2 flex flex-wrap items-center justify-between gap-2 text-xs">
        <p className="text-slate-400">Enter để gửi · Shift+Enter xuống dòng</p>
        {attachingFile ? <p className="text-emerald-300">Đang mã hóa file...</p> : null}
        {maxRunes != null ? (
          <p
            className={cn(
              'tabular-nums font-medium',
              overLimit ? 'text-rose-400' : 'text-slate-500',
            )}
            aria-live="polite"
          >
            {usedRunes.toLocaleString()} / {maxRunes.toLocaleString()} ký tự
            {overLimit ? ' · vượt giới hạn' : ''}
          </p>
        ) : null}
      </div>
      {maxRunes != null && usedRunes > maxRunes * 0.85 ? (
        <p className="mt-1.5 rounded-md border border-amber-500/25 bg-amber-500/10 px-2 py-1.5 text-[11px] leading-snug text-amber-100/90">
          Nội dung rất dài sẽ được gửi dưới dạng file đã mã hóa trong bản cập nhật sau (đang phát triển).
        </p>
      ) : null}
    </div>
  )
}
