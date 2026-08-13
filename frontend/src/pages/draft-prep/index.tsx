import { useEffect, useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { getDraftValues, listLeagues, type DraftPlayer, type DraftReplacementLevel } from '@/api/client'
import { keys } from '@/api/queryKeys'
import { FilterChip, SelectControl } from '@/components/ui/filter-chip'
import { PROJECTION_SEASON } from '@/lib/constants'
import { DraftSettingsPanel } from '@/pages/league-detail/components/DraftSettingsPanel'
import {
  useDraftSettings, serverSettings, draftQuery, type DraftSettings,
} from '@/pages/league-detail/hooks/useDraftSettings'
import { DraftBoardTable, boardOrder } from './components/DraftBoardTable'
import { TeamPanel } from './components/TeamPanel'
import { useDraftPrep } from './hooks/useDraftPrep'

const POSITIONS = ['All', 'QB', 'RB', 'WR', 'TE', 'K']
const VIEWS = [
  { value: 'all', label: 'All players' },
  { value: 'positive', label: 'Targets' },
  { value: 'negative', label: 'Avoids' },
] as const
type View = (typeof VIEWS)[number]['value']

const NO_PLAYERS: DraftPlayer[] = []
const NO_LEVELS: DraftReplacementLevel[] = []
const LEAGUE_KEY = 'fs.draft-prep.league'
const TEAM_PANEL_KEY = 'fs.draft-prep.team-panel'

/**
 * Draft Prep — the board you build before a draft: league settings you can model,
 * your own ranking, targets and avoids, and a team you can assemble at planned
 * prices. The league page's Draft tab shows the same projections
 * read-only; everything you can change lives here.
 */
export default function DraftPrep() {
  const { data: leagues, isLoading: leaguesLoading } = useQuery({
    queryKey: keys.leagues,
    queryFn: listLeagues,
  })

  // Draft values are NFL-only (the projection engine has no NBA equivalent).
  const nflLeagues = useMemo(() => (leagues ?? []).filter((l) => l.sport === 'nfl'), [leagues])

  const [leagueId, setLeagueId] = useState<number | null>(() => {
    const stored = Number(localStorage.getItem(LEAGUE_KEY))
    return stored > 0 ? stored : null
  })
  // Fall back to the first NFL league once they load, and recover if the stored
  // league is gone (unsynced, or belongs to another account).
  useEffect(() => {
    if (!nflLeagues.length) return
    if (!leagueId || !nflLeagues.some((l) => l.id === leagueId)) {
      setLeagueId(nflLeagues[0].id)
    }
  }, [nflLeagues, leagueId])

  const selectLeague = (id: number) => {
    localStorage.setItem(LEAGUE_KEY, String(id))
    setLeagueId(id)
  }

  const league = nflLeagues.find((l) => l.id === leagueId)
  const seasonNum = Math.min((parseInt(league?.season ?? '', 10) || 2025) + 1, PROJECTION_SEASON)

  const [leagueDefaults, setLeagueDefaults] = useState<DraftSettings | null>(null)
  const {
    settings, editing, update, setSlot, save, discard, reset,
    isDirty, isCustomized, position, setPosition,
  } = useDraftSettings(leagueId ?? 0, leagueDefaults)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [view, setView] = useState<View>('all')

  // Whether the team panel is docked open is a preference that should survive a
  // reload, not a place you navigate to — so it lives in localStorage, not the URL.
  const [teamOpen, setTeamOpen] = useState(() => localStorage.getItem(TEAM_PANEL_KEY) !== 'closed')
  const toggleTeam = () => {
    setTeamOpen((open) => {
      localStorage.setItem(TEAM_PANEL_KEY, open ? 'closed' : 'open')
      return !open
    })
  }

  const { params, key } = draftQuery(seasonNum, isCustomized ? settings : null)
  const { data, isLoading, isError, isFetching } = useQuery({
    queryKey: keys.draftValues(leagueId ?? 0, seasonNum, key),
    queryFn: () => getDraftValues(leagueId!, params),
    enabled: !!leagueId,
    staleTime: 5 * 60 * 1000,
    placeholderData: (prev) => prev,
  })

  useEffect(() => {
    const incoming = serverSettings(data)
    if (incoming) setLeagueDefaults(incoming)
  }, [data])

  const prep = useDraftPrep(leagueId, seasonNum)

  const allPlayers = data?.players ?? NO_PLAYERS
  // The team builder measures a lineup against replacement, same as the board's prices do.
  const replacementLevels = data?.replacement_levels ?? NO_LEVELS

  const gradeRankMap = useMemo(() => {
    const map = new Map<string, number>()
    const withGrade = allPlayers
      .filter((p) => p.player_grade != null)
      .sort((a, b) => (b.player_grade ?? 0) - (a.player_grade ?? 0))
    withGrade.forEach((p, i) => map.set(p.gsis_id, i + 1))
    return map
  }, [allPlayers])

  const filtered = useMemo(
    () =>
      allPlayers.filter((p) => {
        if (position && p.position_group !== position && p.position !== position) return false
        const interest = prep.entry(p.gsis_id).interest
        switch (view) {
          case 'positive': return interest === 1
          case 'negative': return interest === -1
          default: return true
        }
      }),
    [allPlayers, position, view, prep],
  )

  /**
   * Reordering rewrites the whole board, not just the rows on screen — a rank is
   * only meaningful relative to every other player, and a filtered view would
   * otherwise renumber the board around whatever happened to be visible.
   */
  const handleMove = (movingId: string, neighbourId: string, place: 'before' | 'after') => {
    const order = boardOrder(allPlayers, prep.entry).map((p) => p.gsis_id)
    const from = order.indexOf(movingId)
    if (from < 0) return
    order.splice(from, 1)
    const at = order.indexOf(neighbourId)
    if (at < 0) return
    order.splice(place === 'before' ? at : at + 1, 0, movingId)
    prep.reorder.mutate(order)
  }

  if (leaguesLoading) return <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />

  if (!nflLeagues.length) {
    return (
      <div className="max-w-2xl">
        <h1 className="text-2xl font-bold text-foreground">Draft Prep</h1>
        <p className="mt-3 text-sm text-muted-foreground">
          Draft prep works from one of your NFL leagues — its roster and scoring settings are what
          turn projections into draft values. Sync a league from the Leagues page to get started.
        </p>
      </div>
    )
  }

  return (
    <div className={`space-y-5 ${teamOpen ? 'lg:mr-[332px]' : 'lg:mr-14'}`}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-foreground">Draft Prep</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {isFetching && !isLoading
              ? 'Rescoring for your settings…'
              : `${seasonNum} board · mark targets and avoids, plan prices, model your league settings`}
          </p>
        </div>
        <SelectControl
          label="League"
          value={leagueId ?? ''}
          onChange={(e) => selectLeague(Number(e.target.value))}
        >
          {nflLeagues.map((l) => (
            <option key={l.id} value={l.id}>
              {l.name} ({l.season})
            </option>
          ))}
        </SelectControl>
      </div>

      <DraftSettingsPanel
        settings={editing}
        isCustomized={isCustomized}
        isDirty={isDirty}
        open={settingsOpen}
        onToggle={() => setSettingsOpen((o) => !o)}
        onChange={update}
        onSlotChange={setSlot}
        onSave={save}
        onDiscard={discard}
        onReset={reset}
      />

      {/* The team panel is fixed to the window edge on wide screens, so it's out
          of flow — the page reserves the width instead of laying it out. */}
      <div className="min-w-0 space-y-4">
        <div className="flex flex-wrap items-center gap-4">
          <div className="flex flex-wrap gap-1.5">
            {POSITIONS.map((pos) => (
              <FilterChip
                key={pos}
                active={(pos === 'All' && position === '') || pos === position}
                onClick={() => setPosition(pos === 'All' ? '' : pos)}
              >
                {pos}
              </FilterChip>
            ))}
        </div>
        <div className="flex flex-wrap gap-1.5">
          {VIEWS.map((v) => (
            <FilterChip key={v.value} active={view === v.value} onClick={() => setView(v.value)}>
              {v.label}
              {v.value === 'positive' && prep.counts.targets > 0 && ` ${prep.counts.targets}`}
              {v.value === 'negative' && prep.counts.avoids > 0 && ` ${prep.counts.avoids}`}
            </FilterChip>
          ))}
        </div>
      </div>

      {isLoading ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : isError ? (
        <p className="text-sm text-destructive">
          Failed to load draft values. Make sure this league is synced.
        </p>
      ) : allPlayers.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No {seasonNum} projections available yet. Run{' '}
          <code className="font-mono text-xs">make project-nfl ARGS="-project -season {seasonNum}"</code>{' '}
          to generate them.
        </p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            {filtered.length} player{filtered.length !== 1 ? 's' : ''}
            {position ? ` (${position})` : ''} ·{' '}
            {settings.scoringFormat === 'league'
              ? 'league scoring'
              : `${settings.scoringFormat.toUpperCase()} scoring`}
            {isCustomized && ' · custom settings'}
            {prep.counts.ranked > 0 && ` · ${prep.counts.ranked} ranked on your board`}
          </p>
          <DraftBoardTable
            players={filtered}
            gradeRankMap={gradeRankMap}
            showConsensus
            prep={{
              entry: prep.entry,
              setInterest: prep.setInterest,
              setPlannedCost: prep.setPlannedCost,
              setNote: prep.setNote,
              onMove: handleMove,
            }}
          />
        </>
      )}
      </div>

      <TeamPanel
        open={teamOpen}
        onToggle={toggleTeam}
        plannedCount={prep.counts.planned}
        players={allPlayers}
        replacementLevels={replacementLevels}
        settings={settings}
        prep={prep}
      />
    </div>
  )
}
