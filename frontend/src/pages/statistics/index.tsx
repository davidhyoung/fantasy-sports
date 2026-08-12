import { useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useProjections } from '../projections/hooks/useProjections'
import { useRankings } from '../rankings/hooks/useRankings'
import { useDivergences } from '../divergences/hooks/useDivergences'
import ProjectionTable from '../projections/components/ProjectionTable'
import GradesTable from './components/GradesTable'
import { FilterChip, SelectControl } from '@/components/ui/filter-chip'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { CURRENT_SEASON, PROJECTION_SEASON } from '@/lib/constants'
import type { RankingsFormat } from '@/api/client'

const POSITIONS = ['All', 'QB', 'RB', 'WR', 'TE', 'K']
const FORMATS: { label: string; value: RankingsFormat }[] = [
  { label: 'PPR', value: 'ppr' },
  { label: 'Half PPR', value: 'half' },
  { label: 'Standard', value: 'standard' },
]

/** Completed seasons available for grades, newest first. */
const GRADE_SEASONS = Array.from({ length: 6 }, (_, i) => CURRENT_SEASON - i)

type View = 'projections' | 'grades'

function isView(v: string | null): v is View {
  return v === 'projections' || v === 'grades'
}

/**
 * Statistics — the single player-data surface, merging comp-based projections
 * with real-life player grades. Which season is meaningful depends on the view:
 * projections only exist for the upcoming season, grades only for completed
 * ones, so the year control is scoped to the active view rather than offering
 * combinations with no data behind them.
 */
export default function Statistics() {
  const [searchParams, setSearchParams] = useSearchParams()
  const viewParam = searchParams.get('view')
  const view: View = isView(viewParam) ? viewParam : 'projections'

  const [position, setPosition] = useState('')
  const [format, setFormat] = useState<RankingsFormat>('ppr')
  const [gradeSeason, setGradeSeason] = useState(CURRENT_SEASON)

  const setView = (v: View) => {
    searchParams.set('view', v)
    setSearchParams(searchParams, { replace: true })
  }

  const projections = useProjections({
    season: PROJECTION_SEASON,
    position,
    sort: 'proj_fpts_ppr',
  })
  const grades = useRankings({ season: gradeSeason, position })

  // Consensus context, shown alongside each projection where we have it.
  // Coverage is partial (only players present in the imported consensus data),
  // so the table renders a placeholder for the rest.
  const { data: divergenceData } = useDivergences({ format, position })
  const divergenceByGsisID = useMemo(() => {
    const map = new Map<string, { consensusRank: number; delta: number }>()
    for (const d of divergenceData?.players ?? []) {
      map.set(d.gsis_id, { consensusRank: d.consensus_rank_median, delta: d.rank_delta })
    }
    return map
  }, [divergenceData])

  const active = view === 'projections' ? projections : grades

  return (
    <div className="max-w-6xl mx-auto space-y-4">
      {/* Header */}
      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="font-display text-[28px] font-bold text-foreground">Statistics</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Comp-based projections and real-life player grades, with how our numbers stack up
            against the field.
          </p>
        </div>
        {view === 'projections' && (
          <SelectControl
            label="Scoring"
            value={format}
            onChange={e => setFormat(e.target.value as RankingsFormat)}
          >
            {FORMATS.map(f => (
              <option key={f.value} value={f.value}>{f.label}</option>
            ))}
          </SelectControl>
        )}
      </div>

      {/* View toggle */}
      <Tabs value={view} onValueChange={v => setView(v as View)}>
        <TabsList className="static mx-0 px-0 pt-0 pb-0">
          <TabsTrigger value="projections">Projections</TabsTrigger>
          <TabsTrigger value="grades">Grades</TabsTrigger>
        </TabsList>
      </Tabs>

      {/* Position filter + year */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex flex-wrap gap-1.5">
          {POSITIONS.map(pos => (
            <FilterChip
              key={pos}
              active={(pos === 'All' && position === '') || pos === position}
              onClick={() => setPosition(pos === 'All' ? '' : pos)}
            >
              {pos}
            </FilterChip>
          ))}
        </div>

        {view === 'grades' ? (
          <SelectControl
            label="Year"
            value={gradeSeason}
            onChange={e => setGradeSeason(Number(e.target.value))}
          >
            {GRADE_SEASONS.map(y => (
              <option key={y} value={y}>{y}</option>
            ))}
          </SelectControl>
        ) : (
          <span className="font-display text-xs font-semibold text-muted-foreground">
            Year <span className="font-mono text-highlight">{PROJECTION_SEASON}*</span>
          </span>
        )}
      </div>

      {/* Table */}
      {active.isLoading ? (
        <p className="text-sm text-muted-foreground">Loading…</p>
      ) : active.isError ? (
        <p className="text-sm text-destructive">
          Failed to load {view === 'projections' ? 'projections' : 'grades'}.
        </p>
      ) : view === 'projections' && projections.data ? (
        <>
          <p className="text-xs text-muted-foreground">
            {projections.data.total} player{projections.data.total !== 1 ? 's' : ''} projected for{' '}
            {projections.data.season}
            {position ? ` · ${position}s only` : ''}
          </p>
          <ProjectionTable
            players={projections.data.players}
            scoringFormat={format}
            divergences={divergenceByGsisID}
          />
        </>
      ) : view === 'grades' && grades.data ? (
        <>
          <p className="text-xs text-muted-foreground">
            {grades.data.total} player{grades.data.total !== 1 ? 's' : ''} graded for {grades.data.season}
            {position ? ` · ${position}s only` : ''}
          </p>
          <GradesTable players={grades.data.players} />
        </>
      ) : null}
    </div>
  )
}
