import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { PlayerAvatar } from '@/components/ui/table-helpers'
import { getLeagueFreeAgents, type Team, type CreatePostRequest, type FreeAgent } from '@/api/client'
import { keys } from '@/api/queryKeys'

type Depth = 'collapsed' | 'focused' | 'poll'

interface Props {
  leagueId: number
  teams: Team[]
  /** Threads have a title + optional poll; replies don't (spec scope: no
   *  nested polls). */
  allowTitleAndPoll: boolean
  placeholder: string
  submitLabel: string
  /** Returns a promise so the composer knows when to clear itself — it
   *  only resets on success, so a failed post doesn't lose the draft. */
  onSubmit: (data: CreatePostRequest) => Promise<unknown>
  submitting: boolean
  error?: string
}

/**
 * The board's one composer, shared by "start a thread" and "reply" — three
 * depths: a single collapsed line, a focused view with a body + toolbar,
 * and (threads only) a poll-builder that replaces the toolbar's Poll button.
 *
 * Player attach searches free agents only, by name — the only
 * name-searchable, gsis_id-keyed endpoint that already exists; a full
 * roster-plus-FA player search would need a new backend endpoint, out of
 * scope for this pass.
 */
export function PostComposer({ leagueId, teams, allowTitleAndPoll, placeholder, submitLabel, onSubmit, submitting, error }: Props) {
  const [depth, setDepth] = useState<Depth>('collapsed')
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [imageUrl, setImageUrl] = useState<string | null>(null)
  const [attachedPlayer, setAttachedPlayer] = useState<FreeAgent | null>(null)
  const [playerSearchOpen, setPlayerSearchOpen] = useState(false)
  const [playerSearch, setPlayerSearch] = useState('')
  const [mentionOpen, setMentionOpen] = useState(false)
  const [pollOptions, setPollOptions] = useState(['', ''])

  const { data: candidates = [] } = useQuery({
    queryKey: keys.freeAgents(leagueId, '', playerSearch),
    queryFn: () => getLeagueFreeAgents(leagueId, '', 8, 0, playerSearch),
    enabled: playerSearchOpen && playerSearch.trim().length > 0,
  })

  function reset() {
    setDepth('collapsed')
    setTitle('')
    setBody('')
    setImageUrl(null)
    setAttachedPlayer(null)
    setPlayerSearchOpen(false)
    setPlayerSearch('')
    setMentionOpen(false)
    setPollOptions(['', ''])
  }

  function insertMention(name: string) {
    setBody((b) => `${b}@${name} `)
    setMentionOpen(false)
  }

  function submitPost() {
    if (depth === 'poll') {
      const opts = pollOptions.map((o) => o.trim()).filter(Boolean)
      if (opts.length < 2 || !body.trim()) return
      onSubmit({ title: title.trim() || undefined, body: body.trim(), poll_options: opts.map((label) => ({ label })) })
        .then(reset)
        .catch(() => {})
      return
    }
    if (!body.trim()) return
    onSubmit({
      title: allowTitleAndPoll ? (title.trim() || undefined) : undefined,
      body: body.trim(),
      image_url: imageUrl || undefined,
      attached_gsis_id: attachedPlayer?.gsis_id,
    })
      .then(reset)
      .catch(() => {})
  }

  if (depth === 'collapsed') {
    return (
      <button
        onClick={() => setDepth('focused')}
        className="w-full rounded-lg border border-border px-4 py-3 text-left text-sm text-muted-foreground hover:bg-muted"
      >
        {placeholder}
      </button>
    )
  }

  return (
    <div className="rounded-lg border border-border p-4">
      {allowTitleAndPoll && (
        <input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Title (optional)"
          className="mb-2 w-full border-b border-border bg-transparent pb-2 font-display text-sm font-semibold text-foreground focus-visible:outline-none"
        />
      )}

      {depth === 'poll' ? (
        <div className="flex flex-col gap-2">
          <textarea
            autoFocus
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="Ask your question…"
            rows={2}
            className="w-full rounded-md border border-input bg-background p-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          {pollOptions.map((opt, i) => (
            <div key={i} className="flex items-center gap-2">
              <input
                value={opt}
                onChange={(e) => setPollOptions((opts) => opts.map((o, j) => (j === i ? e.target.value : o)))}
                placeholder={`Option ${i + 1}`}
                className="flex-1 rounded-md border border-input bg-background px-2 py-1.5 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
              />
              {pollOptions.length > 2 && (
                <button
                  onClick={() => setPollOptions((opts) => opts.filter((_, j) => j !== i))}
                  className="font-mono text-xs text-muted-foreground hover:text-destructive"
                >
                  remove
                </button>
              )}
            </div>
          ))}
          {pollOptions.length < 6 && (
            <button
              onClick={() => setPollOptions((opts) => [...opts, ''])}
              className="self-start font-display text-xs font-semibold text-primary hover:text-primary-hover"
            >
              + Add option
            </button>
          )}
        </div>
      ) : (
        <div className="relative">
          <textarea
            autoFocus
            value={body}
            onChange={(e) => setBody(e.target.value)}
            placeholder="What's on your mind?"
            rows={3}
            className="w-full rounded-md border border-input bg-background p-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          {mentionOpen && (
            <div className="absolute left-0 top-full z-10 mt-1 min-w-[160px] rounded-md border border-border bg-card py-1">
              <button onClick={() => insertMention('all')} className="block w-full px-3 py-1.5 text-left text-xs text-foreground hover:bg-muted">
                @all
              </button>
              {teams.map((t) => (
                <button
                  key={t.id}
                  onClick={() => insertMention(t.name)}
                  className="block w-full px-3 py-1.5 text-left text-xs text-foreground hover:bg-muted"
                >
                  @{t.name}
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      {imageUrl != null && depth === 'focused' && (
        <div className="mt-2 flex items-center gap-2">
          <input
            autoFocus
            value={imageUrl}
            onChange={(e) => setImageUrl(e.target.value)}
            placeholder="https://…"
            className="flex-1 rounded-md border border-input bg-background px-2 py-1.5 text-xs text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
          />
          <button onClick={() => setImageUrl(null)} className="font-mono text-xs text-muted-foreground hover:text-destructive">✕</button>
        </div>
      )}

      {attachedPlayer && depth === 'focused' && (
        <div className="mt-2 flex items-center gap-2 rounded-md border border-border px-2.5 py-1.5">
          <PlayerAvatar src={attachedPlayer.headshot_url} alt={attachedPlayer.name} size={28} />
          <span className="font-display text-xs font-semibold text-foreground">{attachedPlayer.name}</span>
          <span className="text-[11px] text-muted-foreground">{attachedPlayer.position} · {attachedPlayer.team || 'FA'}</span>
          <button onClick={() => setAttachedPlayer(null)} className="ml-auto font-mono text-xs text-muted-foreground hover:text-destructive">✕</button>
        </div>
      )}

      {playerSearchOpen && depth === 'focused' && !attachedPlayer && (
        <div className="mt-2 rounded-md border border-border">
          <input
            autoFocus
            value={playerSearch}
            onChange={(e) => setPlayerSearch(e.target.value)}
            placeholder="Search free agents by name…"
            className="w-full border-b border-border bg-transparent px-2.5 py-1.5 text-xs text-foreground focus-visible:outline-none"
          />
          {candidates.length > 0 && (
            <div className="max-h-40 overflow-y-auto">
              {candidates.map((c) => (
                <button
                  key={c.gsis_id}
                  onClick={() => { setAttachedPlayer(c); setPlayerSearchOpen(false); setPlayerSearch('') }}
                  className="flex w-full items-center gap-2 px-2.5 py-1.5 text-left hover:bg-muted"
                >
                  <PlayerAvatar src={c.headshot_url} alt={c.name} size={28} />
                  <span className="font-display text-xs font-semibold text-foreground">{c.name}</span>
                  <span className="text-[11px] text-muted-foreground">{c.position} · {c.team || 'FA'}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      )}

      <div className="mt-3 flex items-center justify-between gap-3">
        <div className="flex items-center gap-1">
          <button
            onClick={() => setMentionOpen((v) => !v)}
            disabled={depth === 'poll'}
            className="rounded px-2 py-1 font-display text-[11px] font-semibold text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
          >
            Mention
          </button>
          <button
            onClick={() => setPlayerSearchOpen((v) => !v)}
            disabled={depth === 'poll'}
            className="rounded px-2 py-1 font-display text-[11px] font-semibold text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
          >
            Player
          </button>
          <button
            onClick={() => setImageUrl((v) => (v == null ? '' : null))}
            disabled={depth === 'poll'}
            className="rounded px-2 py-1 font-display text-[11px] font-semibold text-muted-foreground hover:bg-muted hover:text-foreground disabled:opacity-40"
          >
            Image
          </button>
          {allowTitleAndPoll && (
            <button
              onClick={() => setDepth(depth === 'poll' ? 'focused' : 'poll')}
              className={`rounded px-2 py-1 font-display text-[11px] font-semibold hover:bg-muted ${depth === 'poll' ? 'text-primary' : 'text-muted-foreground hover:text-foreground'}`}
            >
              Poll
            </button>
          )}
        </div>
        <div className="flex items-center gap-2">
          <button onClick={reset} className="font-display text-xs font-semibold text-muted-foreground hover:text-foreground">
            Cancel
          </button>
          <Button size="sm" onClick={submitPost} disabled={submitting}>
            {submitting ? 'Posting…' : submitLabel}
          </Button>
        </div>
      </div>

      {error && <p className="mt-2 text-xs text-destructive">{error}</p>}
    </div>
  )
}
