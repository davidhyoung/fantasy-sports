import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { FilterChip } from '@/components/ui/filter-chip'
import {
  SLOT_POSITIONS, SLOT_LABELS, SCORING_STATS, SCORING_LABELS, FALLBACK_SETTINGS,
  type SlotPosition, type ScoringStat,
} from './league-detail/hooks/useDraftSettings'
import { createLeague, type CreateLeagueRequest } from '../api/client'
import { keys } from '../api/queryKeys'
import { PROJECTION_SEASON } from '../lib/constants'

const FORMATS: { value: CreateLeagueRequest['format']; label: string; blurb: string }[] = [
  { value: 'dynasty', label: 'Dynasty', blurb: 'Rosters carry forward every season. Rookie picks are tradable assets.' },
  { value: 'keeper', label: 'Keeper', blurb: 'Keep a limited number of players each year; the rest re-drafts.' },
  { value: 'redraft', label: 'Redraft', blurb: 'Every roster resets to empty each season.' },
]

/** Label + integer field, matching the draft-settings panel's field style. */
function NumberField({
  label, value, onChange, min = 0, max = 999, width = 'w-20',
}: {
  label: string
  value: number
  onChange: (v: number) => void
  min?: number
  max?: number
  width?: string
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      <input
        type="number"
        inputMode="numeric"
        min={min}
        max={max}
        value={value}
        onChange={(e) => {
          const next = parseInt(e.target.value, 10)
          onChange(Number.isNaN(next) ? min : Math.max(min, Math.min(max, next)))
        }}
        className={`${width} h-8 rounded-md border border-input bg-background px-2 font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring`}
      />
    </label>
  )
}

/** Like NumberField, but for point values that need fractional precision. */
function DecimalField({
  label, value, onChange, step = 0.5,
}: {
  label: string
  value: number
  onChange: (v: number) => void
  step?: number
}) {
  return (
    <label className="flex flex-col gap-1">
      <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </span>
      <input
        type="number"
        inputMode="decimal"
        min={-99}
        max={99}
        step={step}
        value={value}
        onChange={(e) => {
          const next = parseFloat(e.target.value)
          onChange(Number.isNaN(next) ? 0 : Math.max(-99, Math.min(99, next)))
        }}
        className="w-16 h-8 rounded-md border border-input bg-background px-2 font-mono text-sm tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
      />
    </label>
  )
}

/** Resizes a team-name array to `n`, padding with placeholders and trimming from the end. */
function resizeTeams(names: string[], n: number): string[] {
  if (n <= names.length) return names.slice(0, n)
  const grown = [...names]
  for (let i = names.length; i < n; i++) grown.push(`Team ${i + 1}`)
  return grown
}

export default function CreateLeague() {
  const navigate = useNavigate()
  const qc = useQueryClient()

  const [name, setName] = useState('')
  const [season, setSeason] = useState(String(PROJECTION_SEASON))
  const [format, setFormat] = useState<CreateLeagueRequest['format']>('dynasty')
  const [budget, setBudget] = useState(FALLBACK_SETTINGS.budget)
  const [numTeams, setNumTeams] = useState(12)
  const [teamNames, setTeamNames] = useState<string[]>(() => resizeTeams([], 12))
  // DEF is dropped for native leagues — nflverse has no team-defense gsis_ids,
  // so a DEF roster slot can never actually be filled. Seeded to 0 (not
  // omitted) so the payload always satisfies the backend's validateNativeSlots
  // check trivially; SLOT_POSITIONS itself stays untouched since Yahoo leagues
  // still use DEF elsewhere (the draft-values settings panel).
  const [slots, setSlots] = useState<Record<SlotPosition, number>>({ ...FALLBACK_SETTINGS.slots, DEF: 0 })
  const [scoring, setScoring] = useState<Record<ScoringStat, number>>({ ...FALLBACK_SETTINGS.scoring })

  const setTeamCount = (n: number) => {
    setNumTeams(n)
    setTeamNames((prev) => resizeTeams(prev, n))
  }

  const mutation = useMutation({
    mutationFn: (req: CreateLeagueRequest) => createLeague(req),
    onSuccess: (league) => {
      qc.invalidateQueries({ queryKey: keys.leagues })
      navigate(`/leagues/${league.id}`)
    },
  })

  const teamsValid = teamNames.every((n) => n.trim() !== '')
  const canSubmit = name.trim() !== '' && season.trim() !== '' && teamsValid && !mutation.isPending

  const submit = () => {
    if (!canSubmit) return
    mutation.mutate({
      name: name.trim(),
      sport: 'nfl',
      season: season.trim(),
      format,
      settings: { budget, slots, scoring },
      teams: teamNames.map((n) => ({ name: n.trim() })),
    })
  }

  return (
    <div className="max-w-2xl">
      <h1 className="font-display text-2xl font-bold text-foreground">New league</h1>
      <p className="mt-1.5 text-sm text-muted-foreground">
        A native league — no Yahoo account required. Roster slots and scoring drive the draft
        board and auction values, same as a synced league. NFL only for now; projections don't
        cover other sports yet.
      </p>

      <div className="mt-8 flex flex-col gap-8">
        {/* Name / season */}
        <section className="flex flex-wrap items-end gap-4">
          <label className="flex flex-1 min-w-[220px] flex-col gap-1">
            <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
              League name
            </span>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder="e.g. The Dynasty" />
          </label>
          <label className="flex w-28 flex-col gap-1">
            <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
              Season
            </span>
            <Input value={season} onChange={(e) => setSeason(e.target.value)} className="font-mono" />
          </label>
        </section>

        {/* Format */}
        <section>
          <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            Format
          </span>
          <div className="mt-2 flex flex-wrap gap-1.5">
            {FORMATS.map((f) => (
              <FilterChip key={f.value} active={format === f.value} onClick={() => setFormat(f.value)}>
                {f.label}
              </FilterChip>
            ))}
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            {FORMATS.find((f) => f.value === format)?.blurb}
          </p>
        </section>

        {/* Teams */}
        <section>
          <div className="flex items-end gap-4">
            <NumberField label="Teams" value={numTeams} min={2} max={32} onChange={setTeamCount} />
            <NumberField label="Budget $" value={budget} min={1} max={10000} onChange={setBudget} />
          </div>
          <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-2">
            {teamNames.map((teamName, i) => (
              <Input
                key={i}
                value={teamName}
                onChange={(e) => {
                  const next = [...teamNames]
                  next[i] = e.target.value
                  setTeamNames(next)
                }}
                placeholder={`Team ${i + 1}`}
              />
            ))}
          </div>
        </section>

        {/* Roster slots */}
        <section>
          <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            Starting lineup
          </span>
          <div className="mt-2 flex flex-wrap gap-3">
            {SLOT_POSITIONS.filter((pos) => pos !== 'DEF').map((pos) => (
              <NumberField
                key={pos}
                label={SLOT_LABELS[pos]}
                value={slots[pos]}
                max={20}
                width="w-14"
                onChange={(v) => setSlots({ ...slots, [pos]: v })}
              />
            ))}
          </div>
          <p className="mt-2 text-xs text-muted-foreground">
            No DEF slot — nflverse doesn't track team defenses as individually
            rostered players, so a native league can't roster one yet.
          </p>
        </section>

        {/* Scoring */}
        <section>
          <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
            Pointing system
          </span>
          <div className="mt-2 flex flex-wrap gap-3">
            {SCORING_STATS.map((stat) => (
              <DecimalField
                key={stat}
                label={SCORING_LABELS[stat]}
                value={scoring[stat]}
                step={stat.endsWith('_yds') ? 0.01 : 0.5}
                onChange={(v) => setScoring({ ...scoring, [stat]: v })}
              />
            ))}
          </div>
        </section>

        {mutation.error && (
          <p className="text-sm text-destructive">{(mutation.error as Error).message}</p>
        )}

        <div className="flex items-center gap-3 border-t border-border pt-6">
          <Button onClick={submit} disabled={!canSubmit}>
            {mutation.isPending ? 'Creating…' : 'Create league'}
          </Button>
          {!teamsValid && (
            <p className="text-xs text-muted-foreground">Every team needs a name.</p>
          )}
        </div>
      </div>
    </div>
  )
}
