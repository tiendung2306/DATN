import { User } from 'lucide-react'
import defaultGroupAvatar from '../../assets/default-group-avatar.png'
import { cn } from '@/lib/utils'

interface ChatListAvatarProps {
  variant: 'channel' | 'dm'
  displayName: string
  size?: 'sm' | 'md'
  className?: string
}

function initialsFromDisplayName(name: string): string {
  const t = name.trim()
  if (!t) return '?'
  const parts = t.split(/\s+/).filter(Boolean)
  if (parts.length >= 2) {
    return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
  }
  return t.slice(0, 2).toUpperCase()
}

export default function ChatListAvatar({
  variant,
  displayName,
  size = 'sm',
  className,
}: ChatListAvatarProps) {
  const dim = size === 'md' ? 'h-10 w-10 text-sm' : 'h-8 w-8 text-xs'

  if (variant === 'channel') {
    return (
      <img
        src={defaultGroupAvatar}
        alt=""
        className={cn('shrink-0 rounded-full object-cover ring-1 ring-slate-700/80', dim, className)}
      />
    )
  }

  const letter = initialsFromDisplayName(displayName)

  return (
    <div
      className={cn(
        'shrink-0 rounded-full bg-slate-700 flex items-center justify-center font-semibold text-slate-200 ring-1 ring-slate-600/80',
        dim,
        className,
      )}
      aria-hidden
    >
      {letter.length <= 2 && letter !== '?' ? (
        letter
      ) : (
        <User className={size === 'md' ? 'h-5 w-5 text-slate-300' : 'h-4 w-4 text-slate-300'} />
      )}
    </div>
  )
}
