import { useState } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { ListRow } from '@/components/ui/list-row'
import { FilterChip } from '@/components/ui/filter-chip'
import { Badge } from '@/components/ui/badge'
import { PlayerAvatar } from '@/components/ui/table-helpers'
import { useDivergences } from './divergences/hooks/useDivergences'
import { listLeagues, sync, type User, type DivergenceItem } from '../api/client'
import { keys } from '../api/queryKeys'

const SPORT_LABEL: Record<string, string> = { nfl: 'NFL', nba: 'NBA' }

// The design system uses no emoji or imagery — a league with no logo gets a flat
// colored square with its initial, cycling through the three accent hues.
const INITIAL_BG = ['bg-primary', 'bg-secondary', 'bg-highlight']

function LeagueMark({ name, logoUrl, index }: { name: string; logoUrl?: string | null; index: number }) {
  if (logoUrl) {
    return <img src={logoUrl} alt="" className="h-10 w-10 rounded-lg object-contain" />
  }
  return (
    <span
      className={`flex h-10 w-10 items-center justify-center rounded-lg font-display text-sm font-bold text-primary-foreground ${INITIAL_BG[index % INITIAL_BG.length]}`}
    >
      {name.charAt(0).toUpperCase()}
    </span>
  )
}

// ── Player outlooks ─────────────────────────────────────────────────────────

const SIGNAL_FILTERS = [
  { label: 'All moves', value: 'all' },
  { label: "We're higher", value: 'higher' },
  { label: "We're lower", value: 'lower' },
  { label: 'Has news', value: 'news' },
] as const

type SignalFilter = typeof SIGNAL_FILTERS[number]['value']

function matchesFilter(d: DivergenceItem, f: SignalFilter): boolean {
  switch (f) {
    case 'higher': return d.rank_delta < 0
    case 'lower': return d.rank_delta > 0
    case 'news': return d.notes.length > 0
    default: return true
  }
}

/** A negative delta means we rank the player ahead of the field. */
function deltaGlyph(delta: number): string {
  const mag = Math.abs(delta)
  const arrow = delta < 0 ? '▲' : '▼'
  return mag >= 10 ? arrow + arrow : arrow
}

function SignalCard({ d }: { d: DivergenceItem }) {
  const higher = d.rank_delta < 0
  const note = d.notes[0]
  const blurb = note
    ? (note.scope === 'team' ? `Team context — ${note.summary}` : note.summary)
    : 'No situational news — the gap here is model-driven.'

  return (
    <RouterLink
      to={`/players/${d.gsis_id}`}
      className="flex items-start gap-4 rounded-xl border border-border px-[18px] py-4 hover:bg-card"
    >
      <PlayerAvatar src={d.headshot_url} alt={d.name} size={40} />
      <div className="min-w-0 flex-1">
        <div className="font-display text-[15px] font-bold text-foreground">{d.name}</div>
        <div className="mt-0.5 text-xs text-muted-foreground">
          {d.position_group} · {d.team || 'FA'}
        </div>
        <p className="mt-2 text-[13px] leading-relaxed text-muted-foreground">
          {blurb}
        </p>
      </div>
      <div className="flex-none text-right">
        <div className="font-mono text-[11px] text-muted-foreground whitespace-nowrap">
          us {d.position_group}{d.our_rank} · field {d.position_group}
          {Math.round(d.consensus_rank_median)}
        </div>
        <div
          className={`mt-1 font-mono text-lg font-bold ${higher ? 'text-positive' : 'text-negative'}`}
        >
          {deltaGlyph(d.rank_delta)} {d.rank_delta > 0 ? '-' : '+'}
          {Math.abs(Math.round(d.rank_delta))}
        </div>
      </div>
    </RouterLink>
  )
}

// ── Page ────────────────────────────────────────────────────────────────────

export default function Home({ user }: { user: User | null }) {
  const qc = useQueryClient()
  const [signalFilter, setSignalFilter] = useState<SignalFilter>('all')

  const { data: leagues = [], error: leaguesError } = useQuery({
    queryKey: keys.leagues,
    queryFn: listLeagues,
    enabled: !!user,
  })

  const syncMutation = useMutation({
    mutationFn: sync,
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.leagues }),
  })

  const { data: divergenceData } = useDivergences({ format: 'ppr', position: '' })
  const signals = (divergenceData?.players ?? [])
    .filter(d => matchesFilter(d, signalFilter))
    .slice(0, 6)

  return (
    <div className="space-y-10">
      {/* Hero */}
      <section>
        <h1 className="max-w-[460px] font-display text-[30px] font-bold leading-tight tracking-tight text-foreground">
          Draft prep that goes past the box score.
        </h1>
        <p className="mt-2.5 max-w-[420px] text-sm leading-relaxed text-muted-foreground">
          Live Yahoo league data plus original stat comps, real-life player grades, and how our
          numbers stack up against the field.
        </p>
        <div className="mt-4 flex items-center gap-2.5">
          {user ? (
            <Button onClick={() => syncMutation.mutate()} disabled={syncMutation.isPending}>
              {syncMutation.isPending ? 'Syncing…' : 'Sync your league'}
            </Button>
          ) : (
            <Button asChild>
              <a href="/auth/login">Login with Yahoo</a>
            </Button>
          )}
          <Button asChild variant="text" size="bare">
            <RouterLink to="/statistics">See players →</RouterLink>
          </Button>
        </div>
      </section>

      {/* Your leagues */}
      {user && (
        <section>
          <div className="flex items-center justify-between gap-3">
            <h2 className="font-display text-xl font-bold tracking-tight text-foreground">
              Your leagues
            </h2>
            <Button asChild variant="text" size="bare">
              <RouterLink to="/leagues/new">+ New league</RouterLink>
            </Button>
          </div>
          {(leaguesError || syncMutation.error) && (
            <p className="mt-3 text-sm text-destructive">
              {(leaguesError as Error)?.message ?? (syncMutation.error as Error)?.message}
            </p>
          )}
          {leagues.length === 0 ? (
            <p className="mt-3 text-sm text-muted-foreground">
              No leagues yet — click <strong>Sync your league</strong> to import yours.
            </p>
          ) : (
            <div className="mt-3 flex flex-col gap-2.5">
              {leagues.map((l, i) => (
                <ListRow
                  key={l.id}
                  href={`/leagues/${l.id}`}
                  leading={<LeagueMark name={l.name} logoUrl={l.logo_url} index={i} />}
                  title={l.name}
                  subtitle={
                    <span>
                      {SPORT_LABEL[l.sport] ?? l.sport} · {l.season}
                      {l.source !== 'native' && ` · ${l.format.charAt(0).toUpperCase()}${l.format.slice(1)}`}
                    </span>
                  }
                  trailing={
                    <div className="flex items-center gap-1.5">
                      {l.source === 'native' && <Badge variant="format">{l.format}</Badge>}
                      {l.source === 'native' ? (
                        <Badge variant="neutral">Native</Badge>
                      ) : (
                        <Badge variant="neutral">⟳ Yahoo</Badge>
                      )}
                    </div>
                  }
                />
              ))}
            </div>
          )}
        </section>
      )}

      {/* Player outlooks — where our projections disagree with the field */}
      <section>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h2 className="font-display text-xl font-bold tracking-tight text-foreground">
            Player Outlooks
          </h2>
          <RouterLink
            to="/divergences"
            className="font-display text-xs font-semibold text-primary hover:text-primary-hover"
          >
            See all divergences →
          </RouterLink>
        </div>

        <div className="mt-3 flex items-center gap-1.5 overflow-x-auto whitespace-nowrap">
          {SIGNAL_FILTERS.map(f => (
            <FilterChip
              key={f.value}
              active={signalFilter === f.value}
              onClick={() => setSignalFilter(f.value)}
            >
              {f.label}
            </FilterChip>
          ))}
          <span className="ml-1 text-xs text-muted-foreground">Sorted by gap size</span>
        </div>

        {signals.length === 0 ? (
          <p className="mt-4 text-sm text-muted-foreground">No players match this filter.</p>
        ) : (
          <div className="mt-3 flex flex-col gap-2.5">
            {signals.map(d => (
              <SignalCard key={d.gsis_id} d={d} />
            ))}
          </div>
        )}
      </section>
    </div>
  )
}
