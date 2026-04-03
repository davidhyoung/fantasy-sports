import { useState } from 'react'
import { useProjections } from './hooks/useProjections'
import ProjectionTable from './components/ProjectionTable'

const POSITIONS = ['All', 'QB', 'RB', 'WR', 'TE', 'K']
const FORMATS = [
  { label: 'PPR',       value: 'ppr'      as const },
  { label: 'Half PPR',  value: 'half'     as const },
  { label: 'Standard',  value: 'standard' as const },
]

export default function Projections() {
  const [position, setPosition] = useState('')
  const [format, setFormat] = useState<'ppr' | 'half' | 'standard'>('ppr')

  const sortField = 'proj_fpts_ppr' // always sort by PPR proj; display can vary

  const { data, isLoading, isError } = useProjections({
    season: 2026,
    position,
    sort: sortField,
  })

  return (
    <div className="max-w-6xl mx-auto space-y-4">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div>
          <h1 className="text-2xl font-bold text-foreground">2026 NFL Draft Rankings</h1>
          <p className="text-sm text-muted-foreground mt-1">
            Comp-based projections — historical player comparisons drive each forecast
          </p>
        </div>

        {/* Scoring format toggle */}
        <div className="flex gap-1 bg-muted rounded-lg p-1 self-start sm:self-center">
          {FORMATS.map(f => (
            <button
              key={f.value}
              onClick={() => setFormat(f.value)}
              className={`px-3 py-1 rounded text-sm font-medium transition-colors ${
                format === f.value
                  ? 'bg-primary text-primary-foreground'
                  : 'text-muted-foreground hover:text-foreground'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>

      {/* Position filter */}
      <div className="flex gap-1 flex-wrap">
        {POSITIONS.map(pos => (
          <button
            key={pos}
            onClick={() => setPosition(pos === 'All' ? '' : pos)}
            className={`px-3 py-1 rounded text-sm font-medium transition-colors border ${
              (pos === 'All' && position === '') || pos === position
                ? 'bg-primary/20 text-primary border-primary/50'
                : 'text-muted-foreground border-border hover:text-foreground hover:border-muted-foreground'
            }`}
          >
            {pos}
          </button>
        ))}
      </div>

      {/* Table */}
      {isLoading ? (
        <p className="text-muted-foreground text-sm">Loading projections…</p>
      ) : isError ? (
        <p className="text-red-600 dark:text-red-400 text-sm">Failed to load projections.</p>
      ) : data ? (
        <>
          <p className="text-xs text-muted-foreground">
            {data.total} player{data.total !== 1 ? 's' : ''} projected for {data.season}
            {position ? ` · ${position}s only` : ''}
          </p>
          <ProjectionTable players={data.players} scoringFormat={format} />
        </>
      ) : null}
    </div>
  )
}
