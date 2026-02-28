const STATE_CONFIG: Record<string, { label: string; classes: string }> = {
  UNINITIALIZED:   { label: 'Uninitialized',    classes: 'bg-gray-700 text-gray-300' },
  AWAITING_BUNDLE: { label: 'Awaiting Bundle',  classes: 'bg-yellow-900 text-yellow-300' },
  AUTHORIZED:      { label: 'Authorized',        classes: 'bg-green-900 text-green-300' },
  ADMIN_READY:     { label: 'Admin Ready',       classes: 'bg-blue-900 text-blue-300' },
  ERROR:           { label: 'Error',             classes: 'bg-red-900 text-red-300' },
}

interface StatusBadgeProps {
  state: string
}

export default function StatusBadge({ state }: StatusBadgeProps) {
  const cfg = STATE_CONFIG[state] ?? { label: state, classes: 'bg-gray-700 text-gray-300' }
  return (
    <span className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium ${cfg.classes}`}>
      <span className="h-1.5 w-1.5 rounded-full bg-current opacity-80" />
      {cfg.label}
    </span>
  )
}
