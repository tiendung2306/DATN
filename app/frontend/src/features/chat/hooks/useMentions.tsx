import { ReactNode, useCallback, useMemo } from 'react'
import { service } from '../../../../wailsjs/go/models'
import { extractMentionsFromBody, MentionCandidate, MentionEntity } from '../../../lib/chatModel'

interface UseMentionsOptions {
  groupMembers: service.MemberInfo[]
  localPeerId: string
}

export function useMentions({ groupMembers, localPeerId }: UseMentionsOptions) {
  const mentionCandidates = useMemo<MentionCandidate[]>(
    () =>
      groupMembers.map((member) => ({
        userId: member.peer_id,
        displayName: member.display_name || member.peer_id,
      })),
    [groupMembers],
  )

  const renderMentionedBody = useCallback(
    (body: string, mentions?: MentionEntity[]): ReactNode => {
      const resolvedMentions = mentions && mentions.length > 0 ? mentions : extractMentionsFromBody(body, mentionCandidates)
      if (!resolvedMentions || resolvedMentions.length === 0) return body

      const ordered = [...resolvedMentions].sort((a, b) => a.start - b.start)
      const out: ReactNode[] = []
      let cursor = 0

      ordered.forEach((mention, index) => {
        if (mention.start < cursor || mention.end <= mention.start || mention.end > body.length) {
          return
        }
        if (mention.start > cursor) {
          out.push(<span key={`txt-${index}-${cursor}`}>{body.slice(cursor, mention.start)}</span>)
        }
        const isSelf = mention.user_id === localPeerId
        out.push(
          <span
            key={`m-${index}-${mention.start}`}
            className={
              isSelf
                ? 'rounded px-1 font-semibold text-orange-200 bg-orange-500/20'
                : 'font-semibold text-emerald-300'
            }
          >
            {body.slice(mention.start, mention.end)}
          </span>,
        )
        cursor = mention.end
      })

      if (cursor < body.length) {
        out.push(<span key={`tail-${cursor}`}>{body.slice(cursor)}</span>)
      }
      return out
    },
    [localPeerId, mentionCandidates],
  )

  return {
    mentionCandidates,
    renderMentionedBody,
  }
}
