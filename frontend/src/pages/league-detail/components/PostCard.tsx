import { useState } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import { TeamAvatar, PlayerAvatar } from '@/components/ui/table-helpers'
import { Badge } from '@/components/ui/badge'
import { PollBar } from './PollBar'
import type { Post } from '@/api/client'

function timeAgo(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime()
  const min = Math.round(diffMs / 60000)
  if (min < 1) return 'just now'
  if (min < 60) return `${min}m`
  const hr = Math.round(min / 60)
  if (hr < 24) return `${hr}h`
  const day = Math.round(hr / 24)
  return `${day}d`
}

interface Props {
  leagueId: number
  post: Post
  /** Root threads show reply count + link into the thread; replies don't. */
  isThread?: boolean
  canModerate: boolean
  onReact: () => void
  onVote: (optionId: number) => void
  onEdit: (body: string) => void
  onDelete: () => void
  onPin?: () => void
  voting?: boolean
}

/**
 * A single post — a thread's root or one of its replies. Own posts (or the
 * commissioner, for moderation) get an edit/delete menu; everyone gets a
 * "+1" reaction toggle. Deleted posts render as a tombstone with none of
 * that chrome, since the row still has to exist (a reply's parent pointer
 * and a thread's reply count both depend on the row surviving).
 */
export function PostCard({ leagueId, post, isThread, canModerate, onReact, onVote, onEdit, onDelete, onPin, voting }: Props) {
  const [menuOpen, setMenuOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(post.body)
  const canEdit = post.mine || canModerate
  const myReaction = post.reactions?.find((r) => r.mine)
  const reactionCount = post.reactions?.reduce((n, r) => n + r.count, 0) ?? 0

  if (post.is_deleted) {
    return (
      <div className="rounded-lg border border-border px-4 py-3 text-sm italic text-muted-foreground">
        [deleted]
      </div>
    )
  }

  return (
    <div className={`rounded-lg border px-4 py-3 ${post.is_pinned ? 'border-commissioner-border bg-commissioner-light' : 'border-border'}`}>
      <div className="flex items-start gap-2.5">
        <TeamAvatar name={post.author.name} size={32} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-1.5">
            <span className="font-display text-[13px] font-semibold text-foreground">{post.author.name}</span>
            <span className="font-mono text-[11px] text-muted-foreground">{timeAgo(post.created_at)}</span>
            {post.edited_at && <span className="text-[11px] text-muted-foreground">(edited)</span>}
            {post.is_pinned && <Badge variant="commissioner">Pinned</Badge>}
          </div>
          {post.title && (
            <div className="mt-0.5 font-display text-[15px] font-bold text-foreground">
              {isThread ? (
                <RouterLink to={`?tab=messages&thread=${post.id}`} className="hover:text-primary">{post.title}</RouterLink>
              ) : post.title}
            </div>
          )}

          {editing ? (
            <div className="mt-1.5 flex flex-col gap-2">
              <textarea
                autoFocus
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                rows={3}
                className="w-full rounded-md border border-input bg-background p-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
              <div className="flex justify-end gap-2">
                <button onClick={() => { setEditing(false); setDraft(post.body) }} className="font-display text-xs font-semibold text-muted-foreground hover:text-foreground">
                  Cancel
                </button>
                <button
                  onClick={() => { onEdit(draft); setEditing(false) }}
                  className="font-display text-xs font-semibold text-primary hover:text-primary-hover"
                >
                  Save
                </button>
              </div>
            </div>
          ) : (
            <p className="mt-1 whitespace-pre-wrap text-sm text-foreground">{post.body}</p>
          )}

          {post.image_url && (
            <img src={post.image_url} alt="" className="mt-2 max-h-64 rounded-md border border-border object-contain" />
          )}

          {post.attached_player && (
            <RouterLink
              to={`/players/${post.attached_player.gsis_id}?league=${leagueId}`}
              className="mt-2 flex items-center gap-2 rounded-md border border-border px-2.5 py-1.5 hover:bg-muted"
            >
              <PlayerAvatar src={post.attached_player.headshot_url} alt={post.attached_player.name} size={28} />
              <span className="font-display text-[13px] font-semibold text-foreground">{post.attached_player.name}</span>
              <span className="text-xs text-muted-foreground">{post.attached_player.position} · {post.attached_player.team || 'FA'}</span>
            </RouterLink>
          )}

          {post.poll && <PollBar poll={post.poll} onVote={onVote} voting={!!voting} />}

          <div className="mt-2 flex items-center gap-3">
            <button
              onClick={onReact}
              className={`font-mono text-xs tabular-nums ${myReaction ? 'text-primary' : 'text-muted-foreground hover:text-foreground'}`}
            >
              +1{reactionCount > 0 ? ` · ${reactionCount}` : ''}
            </button>
            {isThread && (post.reply_count ?? 0) > 0 && (
              <RouterLink to={`?tab=messages&thread=${post.id}`} className="text-xs text-muted-foreground hover:text-foreground">
                {post.reply_count} repl{post.reply_count === 1 ? 'y' : 'ies'}
              </RouterLink>
            )}
          </div>
        </div>

        {(canEdit || onPin) && !editing && (
          <div className="relative flex-none">
            <button
              onClick={() => setMenuOpen((v) => !v)}
              aria-label="Post actions"
              className="rounded px-1.5 py-0.5 font-mono text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
            >
              ···
            </button>
            {menuOpen && (
              <>
                <div className="fixed inset-0 z-10" onClick={() => setMenuOpen(false)} />
                <div className="absolute right-0 top-6 z-20 min-w-[140px] rounded-md border border-border bg-card py-1">
                  {canEdit && (
                    <button
                      onClick={() => { setEditing(true); setMenuOpen(false) }}
                      className="block w-full px-3 py-1.5 text-left text-xs text-foreground hover:bg-muted"
                    >
                      Edit
                    </button>
                  )}
                  {onPin && (
                    <button
                      onClick={() => { onPin(); setMenuOpen(false) }}
                      className="block w-full px-3 py-1.5 text-left text-xs text-foreground hover:bg-muted"
                    >
                      {post.is_pinned ? 'Unpin' : 'Pin'}
                    </button>
                  )}
                  {canEdit && (
                    <button
                      onClick={() => { onDelete(); setMenuOpen(false) }}
                      className="block w-full px-3 py-1.5 text-left text-xs text-destructive hover:bg-muted"
                    >
                      Delete
                    </button>
                  )}
                </div>
              </>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
