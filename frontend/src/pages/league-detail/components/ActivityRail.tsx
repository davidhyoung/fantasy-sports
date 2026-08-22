import { Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { TeamAvatar } from '@/components/ui/table-helpers'
import { Badge } from '@/components/ui/badge'
import { useLeagueFeed } from '../hooks/useMessageBoard'

function timeAgo(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime()
  const min = Math.round(diffMs / 60000)
  if (min < 1) return 'just now'
  if (min < 60) return `${min}m`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr}h`
  return `${Math.round(hr / 24)}d`
}

interface Props {
  leagueId: number
  active: boolean
}

/**
 * The Standings tab's activity rail — pinned thread + recent posts/trades/
 * scored weeks, newest first, capped at a handful of items. Trade/scored-
 * week events are derived from existing tables (league_transactions,
 * league_matchups), not stored — see message_board.go.
 */
export function ActivityRail({ leagueId, active }: Props) {
  const { data, isLoading } = useLeagueFeed(leagueId, active)

  if (!active) return null
  if (isLoading) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />

  const items = data?.items ?? []

  return (
    <div className="rounded-lg border border-border p-4">
      <div className="flex items-center justify-between">
        <h3 className="font-display text-sm font-bold text-foreground">League activity</h3>
        <RouterLink to="?tab=messages" className="font-display text-xs font-semibold text-primary hover:text-primary-hover">
          Messages{data && data.unread_count > 0 ? ` (${data.unread_count})` : ''} →
        </RouterLink>
      </div>

      {items.length === 0 ? (
        <p className="mt-3 text-xs text-muted-foreground">
          Nothing yet — post the first update on the Messages tab.
        </p>
      ) : (
        <div className="mt-3 flex flex-col gap-2.5">
          {items.map((item, i) =>
            item.kind === 'post' ? (
              <RouterLink
                key={`post-${item.post.id}`}
                to={`?tab=messages&thread=${item.post.parent_id ?? item.post.id}`}
                className="flex items-start gap-2 rounded-md hover:bg-muted"
              >
                <TeamAvatar name={item.post.author.name} size={28} />
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5">
                    <span className="truncate font-display text-xs font-semibold text-foreground">{item.post.author.name}</span>
                    {item.post.is_pinned && <Badge variant="commissioner">Pinned</Badge>}
                    <span className="font-mono text-[10px] text-muted-foreground">{timeAgo(item.post.created_at)}</span>
                  </div>
                  <p className="truncate text-xs text-muted-foreground">{item.post.title || item.post.body}</p>
                </div>
              </RouterLink>
            ) : (
              <RouterLink
                key={`event-${i}`}
                to={item.event_link === 'scoreboard' ? '?tab=scoreboard' : '?tab=roster'}
                className="block rounded-md text-xs text-muted-foreground hover:bg-muted hover:text-foreground"
              >
                {item.event_text}
              </RouterLink>
            )
          )}
        </div>
      )}
    </div>
  )
}
