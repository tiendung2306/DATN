import { useState } from 'react'
import { Button } from './ui/button'

interface CopyFieldProps {
  label: string
  value: string
  mono?: boolean
}

export default function CopyField({ label, value, mono = true }: CopyFieldProps) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // clipboard not available in some contexts
    }
  }

  return (
    <div className="space-y-1.5">
      <label className="text-xs font-medium text-muted-foreground">{label}</label>
      <div className="flex items-center gap-2">
        <div
          className={`flex-1 overflow-x-auto whitespace-nowrap rounded-lg border border-border bg-card px-3 py-2 text-sm
            text-card-foreground ${mono ? 'font-mono' : ''}`}
          title={value}
        >
          {value || <span className="italic text-muted-foreground">—</span>}
        </div>
        <Button
          onClick={handleCopy}
          disabled={!value}
          variant="secondary"
          size="sm"
          className="shrink-0"
          title="Copy to clipboard"
        >
          {copied ? '✓ Copied' : 'Copy'}
        </Button>
      </div>
    </div>
  )
}
