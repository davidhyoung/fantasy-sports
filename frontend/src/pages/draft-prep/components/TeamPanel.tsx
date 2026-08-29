import { useMemo, useState } from 'react'
import { Link as RouterLink } from 'react-router-dom'
import type { DraftPlayer, DraftReplacementLevel } from '@/api/client'
import { MobileSheet } from '@/components/ui/mobile-sheet'
import {
  SLOT_LABELS, type DraftSettings, type SlotPosition, type ScoringStat,
} from '@/pages/league-detail/hooks/useDraftSettings'
import { DraftSettingsPanel } from '@/pages/league-detail/components/DraftSettingsPanel'
import { buildRoster, type PlannedPlayer } from '../lib/roster'
import { TargetList } from './TargetList'
import type { useDraftPrep } from '../hooks/useDraftPrep'

/** Two ways to read the same board: what it adds up to, and what's left to get. */
const MODES = [
  { value: 'team', label: 'Team' },
  { value: 'targets', label: 'Targets' },
] as const
type PanelMode = (typeof MODES)[number]['value']

const MODE_KEY = 'fs.draft-prep.panel-mode'

/** The panel's two top-level compartments — which league you're modelling vs.
 *  what you're doing with that model. */
const BUCKETS = [
  { value: 'settings', label: 'Settings' },
  { value: 'plan', label: 'Plan' },
] as const
type Bucket = (typeof BUCKETS)[number]['value']

const BUCKET_KEY = 'fs.draft-prep.panel-bucket'

interface SettingsControls {
  isCustomized: boolean
  isDirty: boolean
  onChange: (patch: Partial<DraftSettings>) => void
  onSlotChange: (pos: SlotPosition, count: number) => void
  onScoringChange: (stat: ScoringStat, value: number) => void
  onSave: () => void
  onDiscard: () => void
  onReset: () => void
}

interface Props {
  open: boolean
  onToggle: () => void
  plannedCount: number
  players: DraftPlayer[]
  replacementLevels: DraftReplacementLevel[]
  /** Applied settings — what the roster/budget math below is built from. */
  settings: DraftSettings
  /** Unsaved draft — what the Settings bucket's form displays and edits. */
  editingSettings: DraftSettings
  settingsControls: SettingsControls
  prep: ReturnType<typeof useDraftPrep>
}

/** Editable dollar figure that commits on blur, not on every keystroke. */
function CostField({ value, onCommit }: { value: number; onCommit: (v: number) => void }) {
  const [draft, setDraft] = useState(String(value))
  const [seen, setSeen] = useState(value)
  if (seen !== value) {
    setSeen(value)
    setDraft(String(value))
  }
  return (
    <span className="inline-flex items-baseline">
      <span className="font-mono text-[11px] text-muted-foreground">$</span>
      <input
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onBlur={() => {
          const next = parseInt(draft, 10)
          if (Number.isNaN(next) || next < 0) {
            setDraft(String(value))
            return
          }
          if (next !== value) onCommit(Math.min(next, 10000))
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') e.currentTarget.blur()
          if (e.key === 'Escape') setDraft(String(value))
        }}
        inputMode="numeric"
        aria-label="Planned cost"
        className="w-8 bg-transparent text-right font-mono text-xs tabular-nums text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
      />
    </span>
  )
}

function Stat({ label, value, tone = '' }: { label: string; value: string; tone?: string }) {
  return (
    <div>
      <div className="font-display text-[10px] font-semibold uppercase tracking-wide text-muted-foreground">
        {label}
      </div>
      <div className={`font-mono text-sm tabular-nums ${tone || 'text-foreground'}`}>{value}</div>
    </div>
  )
}

/**
 * The docked side panel, in two buckets:
 *
 *   Settings — the league settings that drive draft values (teams, budget,
 *              scoring, roster slots). Modelling the league, not the board.
 *   Plan     — your draft plan, in two modes:
 *                Team    — the roster your planned prices add up to, with the budget maths.
 *                Targets — just the players you're chasing, grouped by position and tier.
 *
 * Settings and Plan answer different questions ("what league am I drafting
 * into?" vs. "what am I doing about it?") so they're separate buckets rather
 * than one long scroll; within Plan, Team and Targets read the same board
 * from different ends. Both the bucket and the Plan mode persist, since which
 * one you care about depends on whether you're modelling or drafting.
 *
 * It sits next to the board rather than replacing it because whichever bucket
 * is open is a running readout of decisions made over there — edit a setting
 * or add a player on the left and watch this side react. On wide screens it's
 * pinned to the window edge and scrolls independently, so neither bucket's
 * content ever drags the board with it; collapsed it shrinks to a tab. Below
 * `lg` there's nowhere to dock, so it stacks under the board as an ordinary
 * block.
 */
export function TeamPanel({
  open, onToggle, plannedCount, players, replacementLevels, settings, editingSettings, settingsControls, prep,
}: Props) {
  const [bucket, setBucketState] = useState<Bucket>(
    () => (localStorage.getItem(BUCKET_KEY) === 'settings' ? 'settings' : 'plan'),
  )
  const setBucket = (next: Bucket) => {
    localStorage.setItem(BUCKET_KEY, next)
    setBucketState(next)
  }
  const [mode, setModeState] = useState<PanelMode>(
    () => (localStorage.getItem(MODE_KEY) === 'targets' ? 'targets' : 'team'),
  )
  const setMode = (next: PanelMode) => {
    localStorage.setItem(MODE_KEY, next)
    setModeState(next)
  }
  const byId = useMemo(() => new Map(players.map((p) => [p.gsis_id, p])), [players])

  const planned: PlannedPlayer[] = useMemo(() => {
    const out: PlannedPlayer[] = []
    for (const [gsisId, entry] of prep.byPlayer) {
      if (entry.planned_cost == null) continue
      const player = byId.get(gsisId)
      if (player) out.push({ player, cost: entry.planned_cost })
    }
    return out
  }, [prep.byPlayer, byId])

  const replacementByPosition = useMemo(
    () => new Map(replacementLevels.map((r) => [r.position, r.points])),
    [replacementLevels],
  )

  const roster = useMemo(
    () => buildRoster(planned, settings, replacementByPosition),
    [planned, settings, replacementByPosition],
  )

  const overBudget = roster.spent > roster.budget

  const bucketToggle = (
    <div className="flex rounded-lg bg-muted overflow-hidden">
      {BUCKETS.map((b) => (
        <button
          key={b.value}
          onClick={() => setBucket(b.value)}
          aria-pressed={bucket === b.value}
          className={`px-2.5 py-1 font-display text-[11px] font-semibold ${
            bucket === b.value
              ? 'bg-foreground text-background'
              : 'bg-card text-muted-foreground hover:text-foreground'
          }`}
        >
          {b.label}
          {b.value === 'settings' && (settingsControls.isDirty || settingsControls.isCustomized) && ' •'}
        </button>
      ))}
    </div>
  )

  const modeToggle = (
    <div className="flex rounded-lg bg-muted overflow-hidden">
      {MODES.map((m) => (
        <button
          key={m.value}
          onClick={() => setMode(m.value)}
          aria-pressed={mode === m.value}
          className={`px-2.5 py-1 font-display text-[11px] font-semibold ${
            mode === m.value
              ? 'bg-foreground text-background'
              : 'bg-card text-muted-foreground hover:text-foreground'
          }`}
        >
          {m.label}
          {m.value === 'team' && plannedCount > 0 && ` ${plannedCount}`}
          {m.value === 'targets' && prep.counts.targets > 0 && ` ${prep.counts.targets}`}
        </button>
      ))}
    </div>
  )

  const body = (
    <>
      {mode === 'targets' ? (
        <TargetList players={players} prep={prep} />
      ) : (
      <>
        <div className="rounded-lg bg-card lg:mb-3">
          {planned.length === 0 ? (
            <p className="px-3 py-3 text-xs text-muted-foreground">
              Add players with the <span className="font-mono">+</span> in the board's Plan column —
              they start at our auction value and the price is editable here.
            </p>
          ) : (
            <>
              {/* 2 columns below sm keeps mono figures legible at 375px; 3 once there's room. */}
              <div className="grid grid-cols-2 gap-2 px-3 py-2 sm:grid-cols-3">
                <Stat
                  label="Spent"
                  value={`$${roster.spent}`}
                  tone={overBudget ? 'text-secondary' : 'text-foreground'}
                />
                <Stat
                  label="Left"
                  value={`$${roster.remaining}`}
                  tone={roster.remaining < 0 ? 'text-secondary' : 'text-positive'}
                />
                <Stat label="Max bid" value={`$${roster.maxBid}`} />
                <Stat label="Starters" value={roster.starterPoints.toFixed(0)} />
                <Stat label="VOR" value={roster.starterVOR.toFixed(0)} />
                <Stat
                  label="Filled"
                  value={`${planned.length - roster.overflow.length}/${roster.slots.length}`}
                />
              </div>

              <ul className="border-t border-border">
                {roster.slots.map((s, i) => {
                  const p = s.player
                  const bench = s.slot === 'BN'
                  return (
                    <li
                      key={`${s.slot}-${i}`}
                      className="flex items-center gap-2 border-b border-border px-3 py-1 last:border-0"
                    >
                      <span
                        className={`w-11 shrink-0 font-display text-[10px] font-semibold uppercase ${
                          bench ? 'text-muted-foreground/60' : 'text-muted-foreground'
                        }`}
                      >
                        {SLOT_LABELS[s.slot]}
                      </span>
                      {p ? (
                        <>
                          <RouterLink
                            to={`/players/${p.player.gsis_id}`}
                            className="min-w-0 flex-1 truncate font-display text-xs font-semibold text-foreground hover:underline"
                            title={`${p.player.name} · ${p.player.proj_league_fpts.toFixed(0)} proj · our value $${p.player.auction_value}`}
                          >
                            {p.player.name}
                          </RouterLink>
                          <CostField
                            value={p.cost}
                            onCommit={(v) => prep.setPlannedCost(p.player.gsis_id, v)}
                          />
                          <button
                            onClick={() => prep.setPlannedCost(p.player.gsis_id, null)}
                            title={`Remove ${p.player.name}`}
                            className="font-mono text-[11px] text-muted-foreground hover:text-foreground"
                          >
                            ×
                          </button>
                        </>
                      ) : (
                        <span className="flex-1 text-xs text-muted-foreground/40">empty</span>
                      )}
                    </li>
                  )
                })}
              </ul>
            </>
          )}
        </div>

        {roster.warnings.length > 0 && (
          <ul className="space-y-1 rounded-lg bg-negative-light px-3 py-2 lg:mb-3">
            {roster.warnings.map((w) => (
              <li key={w} className="font-display text-[11px] font-semibold text-negative-foreground">
                {w}
              </li>
            ))}
          </ul>
        )}

        {roster.overflow.length > 0 && (
          <div className="rounded-lg bg-card px-3 py-2">
            <h3 className="font-display text-[10px] font-semibold uppercase tracking-wide text-secondary">
              No roster spot
            </h3>
            <ul className="mt-1 space-y-1">
              {roster.overflow.map((p) => (
                <li key={p.player.gsis_id} className="flex items-center gap-2">
                  <RouterLink
                    to={`/players/${p.player.gsis_id}`}
                    className="min-w-0 flex-1 truncate font-display text-xs font-semibold text-foreground hover:underline"
                  >
                    {p.player.name}
                  </RouterLink>
                  <span className="font-mono text-[11px] tabular-nums text-muted-foreground">
                    ${p.cost}
                  </span>
                  <button
                    onClick={() => prep.setPlannedCost(p.player.gsis_id, null)}
                    className="font-mono text-[11px] text-muted-foreground hover:text-foreground"
                  >
                    ×
                  </button>
                </li>
              ))}
            </ul>
          </div>
        )}
      </>
      )}
    </>
  )

  const settingsBody = <DraftSettingsPanel settings={editingSettings} {...settingsControls} />
  const panelBody = bucket === 'settings' ? settingsBody : body

  // The collapsed tab always names whichever bucket is currently selected,
  // independent of open/closed — same state either way.
  const collapsedLabel = bucket === 'settings'
    ? `Settings${settingsControls.isDirty || settingsControls.isCustomized ? ' •' : ''}`
    : `Plan${plannedCount > 0 ? ` ${plannedCount}` : ''}`

  return (
    <>
      {/* Desktop: docked to the window edge, or a collapsed vertical tab. */}
      {open ? (
        <aside
          className="hidden lg:block lg:fixed lg:bottom-0 lg:right-0 lg:top-[var(--nav-height)] lg:z-30 lg:w-[320px] lg:overflow-y-auto lg:border-l lg:border-border lg:bg-background lg:p-3"
          aria-label="Draft prep panel"
        >
          <div className="mb-3 flex items-center justify-between gap-2">
            {bucketToggle}
            <button
              onClick={onToggle}
              aria-expanded
              title="Hide the panel"
              className="font-mono text-xs text-muted-foreground hover:text-foreground"
            >
              ▸
            </button>
          </div>
          {bucket === 'plan' && <div className="mb-3">{modeToggle}</div>}
          {panelBody}
        </aside>
      ) : (
        <button
          onClick={onToggle}
          aria-expanded={false}
          title="Show the draft prep panel"
          className="hidden lg:fixed lg:right-0 lg:top-[calc(var(--nav-height)+1.5rem)] lg:z-30 lg:flex lg:w-auto lg:flex-col lg:items-center lg:gap-2 lg:rounded-l-lg lg:border-y lg:border-l lg:border-border lg:bg-card lg:px-2 lg:py-4 lg:hover:bg-muted"
        >
          <span className="font-mono text-xs text-muted-foreground">◂</span>
          <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground lg:[writing-mode:vertical-rl]">
            {collapsedLabel}
          </span>
        </button>
      )}

      {/* Below lg: collapsed pill fixed bottom-right; expanded is a bottom sheet, not an
          in-flow block, so a long roster never drags the board down with it. */}
      <button
        onClick={onToggle}
        aria-expanded={open}
        title={open ? 'Hide the draft prep panel' : 'Show the draft prep panel'}
        className="fixed bottom-4 right-4 z-30 flex h-11 items-center gap-1.5 rounded-pill border border-border bg-card px-4 shadow-none lg:hidden"
      >
        <span className="font-display text-xs font-semibold text-foreground">{collapsedLabel}</span>
        <span className="font-mono text-xs text-muted-foreground">▸</span>
      </button>
      <div className="lg:hidden">
        <MobileSheet open={open} onClose={onToggle} title="Draft prep panel">
          <div className="mb-3">{bucketToggle}</div>
          {bucket === 'plan' && <div className="mb-3">{modeToggle}</div>}
          {panelBody}
        </MobileSheet>
      </div>
    </>
  )
}
