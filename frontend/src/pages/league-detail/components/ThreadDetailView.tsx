import { Link as RouterLink } from 'react-router-dom'
import { Loader2 } from 'lucide-react'
import { PostCard } from './PostCard'
import { PostComposer } from './PostComposer'
import {
  useLeagueThread, useCreateReply, useToggleReaction, useVotePoll,
  useEditPost, useDeletePost, usePinThread,
} from '../hooks/useMessageBoard'
import type { Team } from '@/api/client'

interface Props {
  leagueId: number
  threadId: number
  teams: Team[]
  canModerate: boolean
  /** Opens an attached player in-place rather than navigating to `/players/:gsisId`. */
  onPlayerClick: (gsisId: string) => void
}

export function ThreadDetailView({ leagueId, threadId, teams, canModerate, onPlayerClick }: Props) {
  const { data, isLoading, error } = useLeagueThread(leagueId, threadId)
  const reply = useCreateReply(leagueId, threadId)
  const react = useToggleReaction(leagueId)
  const vote = useVotePoll(leagueId)
  const edit = useEditPost(leagueId)
  const del = useDeletePost(leagueId)
  const pin = usePinThread(leagueId)

  if (isLoading) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
  if (error) return <p className="text-destructive">{(error as Error).message}</p>
  if (!data) return null

  return (
    <div className="flex flex-col gap-4">
      <RouterLink to="?tab=messages" className="text-sm text-primary hover:underline">
        ← All threads
      </RouterLink>

      <PostCard
        leagueId={leagueId}
        post={data.thread}
        canModerate={canModerate}
        onReact={() => react.mutate(data.thread.id)}
        onVote={(optionId) => vote.mutate({ postId: data.thread.id, optionId })}
        onEdit={(body) => edit.mutate({ postId: data.thread.id, body })}
        onDelete={() => del.mutate(data.thread.id)}
        onPin={() => pin.mutate({ threadId: data.thread.id, pinned: !data.thread.is_pinned })}
        voting={vote.isPending}
        onPlayerClick={onPlayerClick}
      />

      {data.replies.length > 0 && (
        <div className="flex flex-col gap-2 border-l-2 border-border pl-4">
          {data.replies.map((r) => (
            <PostCard
              key={r.id}
              leagueId={leagueId}
              post={r}
              canModerate={canModerate}
              onReact={() => react.mutate(r.id)}
              onVote={() => {}}
              onEdit={(body) => edit.mutate({ postId: r.id, body })}
              onDelete={() => del.mutate(r.id)}
              onPlayerClick={onPlayerClick}
            />
          ))}
        </div>
      )}

      <PostComposer
        leagueId={leagueId}
        teams={teams}
        allowTitleAndPoll={false}
        placeholder="Write a reply…"
        submitLabel="Reply"
        submitting={reply.isPending}
        error={reply.error ? (reply.error as Error).message : undefined}
        onSubmit={(data) => reply.mutateAsync(data)}
      />
    </div>
  )
}
