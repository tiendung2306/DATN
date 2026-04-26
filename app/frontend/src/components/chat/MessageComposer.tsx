import { KeyboardEvent } from 'react'
import { Button } from '../ui/button'

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
    <div className="rounded-xl border border-border/80 bg-[#0a0f16] p-3">
      <div className="flex gap-2">
        <textarea
          className="min-h-[46px] w-full resize-y rounded-md border border-border/80 bg-black/35 px-3 py-2 text-sm outline-none focus:ring-2 focus:ring-emerald-500/30"
          placeholder="Type a secure message..."
          value={value}
          onChange={(event) => onChange(event.target.value)}
          onKeyDown={handleKeyDown}
          disabled={disabled}
        />
        <Button onClick={onSend} disabled={disabled || !value.trim()} className="px-4">
          {'>'}
        </Button>
      </div>
      <p className="mt-2 text-xs text-muted-foreground">
        Press Enter to send, Shift+Enter for newline.
      </p>
    </div>
  )
}
