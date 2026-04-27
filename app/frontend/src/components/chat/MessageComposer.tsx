import { KeyboardEvent } from 'react'
import { Button } from '../ui/button'
import { Paperclip, SendHorizontal } from 'lucide-react'

interface MessageComposerProps {
  value: string
  disabled: boolean
  onChange: (value: string) => void
  onSend: () => void
}

export default function MessageComposer({ value, disabled, onChange, onSend }: MessageComposerProps) {
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
        <textarea
          className="min-h-[44px] w-full resize-y rounded-md border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 outline-none placeholder:text-slate-500 focus:ring-2 focus:ring-emerald-500/30"
          placeholder="Type a secure message..."
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={handleKeyDown}
          disabled={disabled}
        />
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
