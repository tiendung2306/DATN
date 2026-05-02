import { KeyboardEvent, useRef, useImperativeHandle, forwardRef } from 'react'
import { Button } from '../../ui/button'
import MentionTextarea, { MentionTextareaHandle } from './MentionTextarea'
import { MentionCandidate } from '../../../lib/chatModel'
import { countUnicodeRunes } from '../../../lib/textLimits'
import { cn } from '@/lib/utils'
import { Send } from 'lucide-react'

interface CommentComposerProps {
  postId: string
  value: string
  sending: boolean
  mentionCandidates: MentionCandidate[]
  maxBodyRunes: number
  placeholder: string
  onChange: (value: string) => void
  onSubmit: () => Promise<void>
}

export interface CommentComposerHandle {
  focus: () => void
}

const CommentComposer = forwardRef<CommentComposerHandle, CommentComposerProps>(
  ({ postId, value, sending, mentionCandidates, maxBodyRunes, placeholder, onChange, onSubmit }, ref) => {
    const textareaRef = useRef<MentionTextareaHandle>(null)

    useImperativeHandle(ref, () => ({
      focus: () => {
        textareaRef.current?.focus()
      },
    }))

    const used = countUnicodeRunes(value.trim())
    const over = used > maxBodyRunes
    const canSubmit = value.trim() && !over && !sending

    const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        if (canSubmit) {
          void onSubmit()
        }
      }
    }

    return (
    <form
      id={`composer-${postId}`}
      className="mt-3 flex flex-col gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        if (canSubmit) void onSubmit()
      }}
    >
      <div className="flex items-end gap-2">
        <div className="flex-1">
          <MentionTextarea
            ref={textareaRef}
            value={value}
            onChange={onChange}
            placeholder={placeholder}
            candidates={mentionCandidates}
            disabled={sending}
            rows={2}
            onKeyDown={handleKeyDown}
            aria-invalid={over}
          />
        </div>
        <Button
          type="submit"
          size="icon"
          className="h-9 w-9 shrink-0 rounded-full bg-emerald-500 text-slate-950 hover:bg-emerald-400"
          disabled={!canSubmit}
        >
        <Send className="h-3.5 w-3.5" />
        </Button>
      </div>
      <div className="flex justify-end">
        <span className={cn('text-[10px] tabular-nums font-medium', over ? 'text-rose-400' : 'text-slate-500')}>
          {used} / {maxBodyRunes}
        </span>
      </div>
      {used > maxBodyRunes * 0.85 ? (
        <p className="mt-1.5 rounded-md border border-amber-500/20 bg-amber-500/10 px-2 py-1 text-[10px] leading-snug text-amber-100/90">
          Bình luận quá dài — sau này sẽ gửi file đã mã hóa.
        </p>
      ) : null}
    </form>
    )
  })

export default CommentComposer
