import { KeyboardEvent, useRef, useImperativeHandle, forwardRef } from 'react'
import { Button } from '../../ui/button'
import MentionTextarea, { MentionTextareaHandle } from './MentionTextarea'
import { MentionCandidate } from '../../../lib/chatModel'
import { Send, Reply } from 'lucide-react'

interface CommentComposerProps {
  postId: string
  value: string
  sending: boolean
  mentionCandidates: MentionCandidate[]
  placeholder: string
  onChange: (value: string) => void
  onSubmit: () => Promise<void>
}

export interface CommentComposerHandle {
  focus: () => void
}

const CommentComposer = forwardRef<CommentComposerHandle, CommentComposerProps>(
  ({ postId, value, sending, mentionCandidates, placeholder, onChange, onSubmit }, ref) => {
    const textareaRef = useRef<MentionTextareaHandle>(null)

    useImperativeHandle(ref, () => ({
      focus: () => {
        textareaRef.current?.focus()
      },
    }))

    const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      if (value.trim()) {
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
        void onSubmit()
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
          />
        </div>
        <Button
          type="submit"
          size="icon"
          className="h-9 w-9 shrink-0 rounded-full bg-emerald-500 text-slate-950 hover:bg-emerald-400"
          disabled={sending || !value.trim()}
        >
        <Send className="h-3.5 w-3.5" />
      </Button>
      </div>
    </form>
  )
})

export default CommentComposer
