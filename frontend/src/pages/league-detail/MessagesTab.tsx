import { useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { FilterChip } from '@/components/ui/filter-chip'
import { PostCard } from './components/PostCard'
import { PostComposer } from './components/PostComposer'
import { ThreadDetailView } from './components/ThreadDetailView'
import { useLeagueThreads, useCreateThread, useToggleReaction, useVotePoll, useEditPost, useDeletePost, usePinThread } from './hooks/useMessageBoard'
import type { Team, ThreadFilter } from '@/api/client'

const FILTERS: { label: string; value: ThreadFilter }[] = [
  { label: 'All', value: 'all' },
  { label: 'Unread', value: 'unread' },
  { label: 'Mentions', value: 'mentions' },
  { label: 'Mine', value: 'mine' },
]

interface Props {
  leagueId: number
  active: boolean
  teams: Team[]
  canModerate: boolean
}

/**
 * The league's Messages tab — threads list with filters, a composer to
 * start a new one, and (via ?thread=) the detail view for one thread.
 * Native leagues only; see message_board.go for the backend + scope notes.
 */
export function MessagesTab({ leagueId, active, teams, canModerate }: Props) {
  const [searchParams] = useSearchParams()
  const threadId = searchParams.get('thread')
  const [filter, setFilter] = useState<ThreadFilter>('all')

  const { data, isLoading, error } = useLeagueThreads(leagueId, filter, active && !threadId)
  const create = useCreateThread(leagueId)
  const react = useToggleReaction(leagueId)
  const vote = useVotePoll(leagueId)
  const edit = useEditPost(leagueId)
  const del = useDeletePost(leagueId)
  const pin = usePinThread(leagueId)

  if (!active) return null

  if (threadId) {
    return <ThreadDetailView leagueId={leagueId} threadId={Number(threadId)} teams={teams} canModerate={canModerate} />
  }

  return (
    <div className="flex flex-col gap-4">
      <PostComposer
        leagueId={leagueId}
        teams={teams}
        allowTitleAndPoll
        placeholder="Share an update, ask a question, or start a poll…"
        submitLabel="Post"
        submitting={create.isPending}
        error={create.error ? (create.error as Error).message : undefined}
        onSubmit={(data) => create.mutateAsync(data)}
      />

      <div className="flex items-center gap-1.5">
        {FILTERS.map((f) => (
          <FilterChip key={f.value} active={filter === f.value} onClick={() => setFilter(f.value)}>
            {f.label}
          </FilterChip>
        ))}
      </div>

      {isLoading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : error ? (
        <p className="text-destructive">{(error as Error).message}</p>
      ) : !data || data.threads.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {filter === 'all' ? 'No threads yet — start the conversation above.' : 'Nothing matches this filter.'}
        </p>
      ) : (
        <div className="flex flex-col gap-2.5">
          {data.threads.map((t) => (
            <PostCard
              key={t.id}
              leagueId={leagueId}
              post={t}
              isThread
              canModerate={canModerate}
              onReact={() => react.mutate(t.id)}
              onVote={(optionId) => vote.mutate({ postId: t.id, optionId })}
              onEdit={(body) => edit.mutate({ postId: t.id, body })}
              onDelete={() => del.mutate(t.id)}
              onPin={() => pin.mutate({ threadId: t.id, pinned: !t.is_pinned })}
              voting={vote.isPending}
            />
          ))}
        </div>
      )}
    </div>
  )
}
