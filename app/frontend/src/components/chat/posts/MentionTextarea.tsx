import { KeyboardEvent, useMemo, useState, useRef, useImperativeHandle, forwardRef } from 'react'
import { MentionCandidate } from '../../../lib/chatModel'

interface MentionTextareaProps {
  value: string
  onChange: (value: string) => void
  placeholder: string
  candidates: MentionCandidate[]
  disabled?: boolean
  rows?: number
  onKeyDown?: (event: KeyboardEvent<HTMLTextAreaElement>) => void
}

function isMentionQuery(query: string): boolean {
  return query.length > 0 && !/\s/.test(query)
}

export interface MentionTextareaHandle {
  focus: () => void
}

const MentionTextarea = forwardRef<MentionTextareaHandle, MentionTextareaProps>(
  ({ value, onChange, placeholder, candidates, disabled = false, rows = 1, onKeyDown }, ref) => {
    const [activeIndex, setActiveIndex] = useState(0)
    const [queryRange, setQueryRange] = useState<{ start: number; end: number } | null>(null)
    const textareaRef = useRef<HTMLTextAreaElement>(null)

    useImperativeHandle(ref, () => ({
      focus: () => {
        textareaRef.current?.focus()
      },
    }))

  const filtered = useMemo(() => {
    if (!queryRange) return []
    const query = value.slice(queryRange.start + 1, queryRange.end).toLowerCase()
    if (!isMentionQuery(query)) return []
    return candidates
      .filter((candidate) => candidate.displayName.toLowerCase().includes(query))
      .slice(0, 6)
  }, [candidates, queryRange, value])

  const updateQueryRange = (text: string, cursorPos: number) => {
    const trigger = text.lastIndexOf('@', cursorPos - 1)
    if (trigger < 0) {
      setQueryRange(null)
      return
    }
    const fragment = text.slice(trigger + 1, cursorPos)
    if (fragment.includes('\n') || fragment.includes('\t') || fragment.includes(' ')) {
      setQueryRange(null)
      return
    }
    setQueryRange({ start: trigger, end: cursorPos })
    setActiveIndex(0)
  }

  const applyMention = (candidate: MentionCandidate) => {
    if (!queryRange) return
    const before = value.slice(0, queryRange.start)
    const after = value.slice(queryRange.end)
    const mentionText = `@${candidate.displayName} `
    onChange(`${before}${mentionText}${after}`)
    setQueryRange(null)
    setActiveIndex(0)
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (filtered.length === 0) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex((prev) => (prev + 1) % filtered.length)
      return
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex((prev) => (prev - 1 + filtered.length) % filtered.length)
      return
    }
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault()
      applyMention(filtered[activeIndex])
      return
    }
    if (e.key === 'Escape') {
      e.preventDefault()
      setQueryRange(null)
      setActiveIndex(0)
    }
  }

  return (
    <div className="relative">
      <textarea
        ref={textareaRef}
        value={value}
        rows={rows}
        disabled={disabled}
        placeholder={placeholder}
        onKeyDown={(e) => {
          handleKeyDown(e)
          if (!e.defaultPrevented) {
            onKeyDown?.(e)
          }
        }}
        onChange={(e) => {
          const text = e.target.value
          onChange(text)
          updateQueryRange(text, e.target.selectionStart ?? text.length)
        }}
        onClick={(e) => {
          const target = e.target as HTMLTextAreaElement
          updateQueryRange(target.value, target.selectionStart ?? target.value.length)
        }}
        className="w-full resize-none rounded-2xl border border-slate-700 bg-slate-900 px-3 py-2 text-sm text-slate-200 placeholder:text-slate-500 outline-none focus:border-emerald-500/50 disabled:opacity-50"
      />
      {filtered.length > 0 && (
        <div className="absolute left-0 right-0 top-full z-10 mt-1 rounded-lg border border-slate-700 bg-slate-950 shadow-lg">
          {filtered.map((candidate, idx) => (
            <button
              key={candidate.userId}
              type="button"
              className={`block w-full px-3 py-2 text-left text-xs transition ${
                idx === activeIndex ? 'bg-slate-800 text-emerald-300' : 'text-slate-300 hover:bg-slate-800/70'
              }`}
              onMouseDown={(e) => {
                e.preventDefault()
                applyMention(candidate)
              }}
            >
              @{candidate.displayName}
            </button>
          ))}
        </div>
      )}
    </div>
  )
})

export default MentionTextarea
