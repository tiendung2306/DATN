import { KeyboardEvent } from 'react'
import { Button } from '../ui/button'
import { Paperclip, SendHorizontal } from 'lucide-react'
import MentionTextarea from './posts/MentionTextarea'
import { MentionCandidate } from '../../lib/chatModel'

interface MessageComposerProps {
  value: string
  disabled: boolean
  mentionCandidates: MentionCandidate[]
  onChange: (value: string) => void
  onSend: () => void
}

export default function MessageComposer({
  value,
  disabled,
  mentionCandidates,
  onChange,
  onSend,
}: MessageComposerProps) {
  const handleKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === 'Enter' && !event.shiftKey) {
      event.preventDefault()
      onSend()
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
          disabled={disabled}
        >
          <Paperclip className="h-4 w-4" />
        </Button>
        <div className="w-full">
          <MentionTextarea
            value={value}
            onChange={onChange}
            placeholder="Type a secure message..."
            candidates={mentionCandidates}
            disabled={disabled}
            rows={2}
            onKeyDown={handleKeyDown}
          />
        </div>
        <Button
          onClick={onSend}
          disabled={disabled || !value.trim()}
          className="mb-1 bg-emerald-500 px-3 text-slate-900 hover:bg-emerald-400"
        >
          <SendHorizontal className="h-4 w-4" />
        </Button>
      </div>
      <p className="mt-2 text-xs text-slate-400">
        Press Enter to send, Shift+Enter for newline.
      </p>
    </div>
  )
}
