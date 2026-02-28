import { useState } from 'react'

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
    <div>
      <label className="label">{label}</label>
      <div className="flex items-center gap-2">
        <div
          className={`flex-1 bg-gray-800 border border-gray-700 rounded-lg px-3 py-2 text-sm
            text-gray-200 overflow-x-auto whitespace-nowrap ${mono ? 'font-mono' : ''}`}
          title={value}
        >
          {value || <span className="text-gray-600 italic">—</span>}
        </div>
        <button
          onClick={handleCopy}
          disabled={!value}
          className="btn-secondary shrink-0 text-xs px-3"
          title="Copy to clipboard"
        >
          {copied ? '✓ Copied' : 'Copy'}
        </button>
      </div>
    </div>
  )
}
