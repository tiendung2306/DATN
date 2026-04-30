import { Button } from '../../ui/button'
import MentionTextarea from './MentionTextarea'
import { MentionCandidate } from '../../../lib/chatModel'
import { Send } from 'lucide-react'

interface CommentComposerProps {
  value: string
  sending: boolean
  mentionCandidates: MentionCandidate[]
  placeholder: string
  onChange: (value: string) => void
  onSubmit: () => Promise<void>
}

export default function CommentComposer({
  value,
  sending,
  mentionCandidates,
  placeholder,
  onChange,
  onSubmit,
}: CommentComposerProps) {
  return (
    <form
      className="mt-3 flex items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault()
        void onSubmit()
      }}
    >
      <div className="flex-1">
        <MentionTextarea
          value={value}
          onChange={onChange}
          placeholder={placeholder}
          candidates={mentionCandidates}
          disabled={sending}
          rows={2}
        />
      </div>
      <Button
        type="submit"
        size="icon"
        className="h-9 w-9 rounded-full bg-emerald-500 text-slate-950 hover:bg-emerald-400"
        disabled={sending || !value.trim()}
      >
        <Send className="h-3.5 w-3.5" />
      </Button>
    </form>
  )
}
