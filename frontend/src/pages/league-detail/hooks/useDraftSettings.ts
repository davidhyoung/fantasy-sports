import { useState } from 'react'
import type { DraftValuesParams, DraftValuesResponse } from '@/api/client'

export type ScoringFormat = 'league' | 'ppr' | 'half' | 'standard'

export const SLOT_POSITIONS = ['QB', 'RB', 'WR', 'TE', 'FLEX', 'SFLEX', 'K', 'DEF', 'BN'] as const
export type SlotPosition = (typeof SLOT_POSITIONS)[number]

export const SLOT_LABELS: Record<SlotPosition, string> = {
  QB: 'QB', RB: 'RB', WR: 'WR', TE: 'TE',
  FLEX: 'FLEX', SFLEX: 'SUPERFLEX', K: 'K', DEF: 'DEF',
  // Bench starts nobody and can't move a price; it's here because the team
  // builder needs to know how many roster spots a budget has to cover.
  BN: 'BENCH',
}

// The "pointing system" — exactly the stat categories the backend has
// per-game rate columns for (scoring.ProjectionRates on the Go side). FG
// scoring collapses Yahoo's distance buckets into one flat value here.
export const SCORING_STATS = [
  'pass_yds', 'pass_td', 'rush_yds', 'rush_td', 'rec', 'rec_yds', 'rec_td', 'fg_made', 'pat_made',
] as const
export type ScoringStat = (typeof SCORING_STATS)[number]

export const SCORING_LABELS: Record<ScoringStat, string> = {
  pass_yds: 'Pass Yds', pass_td: 'Pass TD',
  rush_yds: 'Rush Yds', rush_td: 'Rush TD',
  rec: 'Reception', rec_yds: 'Rec Yds', rec_td: 'Rec TD',
  fg_made: 'FG Made', pat_made: 'PAT Made',
}

export interface DraftSettings {
  numTeams: number
  budget: number
  scoringFormat: ScoringFormat
  slots: Record<SlotPosition, number>
  /** Points per stat category — pre-filled from the league's real scoring. */
  scoring: Record<ScoringStat, number>
  /**
   * True once a point value has been hand-edited. Only then does `scoring` get
   * sent as an override; otherwise `scoringFormat` (or the league's own
   * scoring) still governs, so merely touching Teams/Budget/Slots doesn't
   * silently freeze scoring against a league that later changes it.
   */
  scoringCustomized: boolean
}

/** Used until the league's own settings arrive — a standard 12-team lineup. */
export const FALLBACK_SETTINGS: DraftSettings = {
  numTeams: 12,
  budget: 200,
  scoringFormat: 'league',
  slots: { QB: 1, RB: 2, WR: 3, TE: 1, FLEX: 1, SFLEX: 0, K: 1, DEF: 1, BN: 6 },
  scoring: {
    pass_yds: 0.04, pass_td: 4,
    rush_yds: 0.1, rush_td: 6,
    rec: 1, rec_yds: 0.1, rec_td: 6,
    fg_made: 3, pat_made: 1,
  },
  scoringCustomized: false,
}

const settingsKey = (leagueId: number) => `fs.draft.settings.v1.${leagueId}`
const positionKey = (leagueId: number) => `fs.draft.position.v1.${leagueId}`

function readJSON<T>(key: string): T | null {
  try {
    const raw = localStorage.getItem(key)
    return raw ? (JSON.parse(raw) as T) : null
  } catch {
    // Private-mode / quota / hand-edited garbage — fall back to league defaults.
    return null
  }
}

function writeJSON(key: string, value: unknown) {
  try {
    localStorage.setItem(key, JSON.stringify(value))
  } catch {
    /* storage unavailable — settings just won't survive the refresh */
  }
}

/** Reads the server's slot map into the fixed set the panel edits. */
function toSlots(raw: Record<string, number> | undefined): Record<SlotPosition, number> {
  const slots = { ...FALLBACK_SETTINGS.slots }
  if (!raw) return slots
  for (const pos of SLOT_POSITIONS) slots[pos] = raw[pos] ?? 0
  return slots
}

/** Reads the server's scoring map into the fixed set the panel edits. */
function toScoring(raw: Record<string, number> | undefined): Record<ScoringStat, number> {
  const scoring = { ...FALLBACK_SETTINGS.scoring }
  if (!raw) return scoring
  for (const stat of SCORING_STATS) scoring[stat] = raw[stat] ?? scoring[stat]
  return scoring
}

/** Serialises settings for the API's `slots=` override. */
export function serializeSlots(slots: Record<SlotPosition, number>): string {
  return SLOT_POSITIONS.map((pos) => `${pos}:${slots[pos]}`).join(',')
}

/** Serialises settings for the API's `scoring=` override. */
export function serializeScoring(scoring: Record<ScoringStat, number>): string {
  return SCORING_STATS.map((stat) => `${stat}:${scoring[stat]}`).join(',')
}

/**
 * Request params + cache key for a board scored with these settings. Pass null
 * for the league's own settings — that request carries no overrides, which is
 * what lets the response seed the panel without the two chasing each other.
 */
export function draftQuery(
  season: number,
  settings: DraftSettings | null,
): { params: DraftValuesParams; key: string } {
  if (!settings) return { params: { season }, key: 'league' }
  return {
    params: {
      season,
      format: settings.scoringFormat,
      budget: settings.budget,
      teams: settings.numTeams,
      slots: serializeSlots(settings.slots),
      // Only sent once a point value has actually been edited — otherwise the
      // format above (or the league's own scoring) still drives the board.
      ...(settings.scoringCustomized ? { scoring: serializeScoring(settings.scoring) } : {}),
    },
    key: JSON.stringify(settings),
  }
}

/**
 * Reads stored settings, backfilling keys added since they were saved (SFLEX was
 * added after the first release), so an older record can't leave a field
 * undefined and turn its input into an uncontrolled one.
 */
export function readSettings(leagueId: number): DraftSettings | null {
  const stored = readJSON<Partial<DraftSettings>>(settingsKey(leagueId))
  if (!stored) return null
  return {
    ...FALLBACK_SETTINGS,
    ...stored,
    slots: { ...FALLBACK_SETTINGS.slots, ...(stored.slots ?? {}) },
    scoring: { ...FALLBACK_SETTINGS.scoring, ...(stored.scoring ?? {}) },
    scoringCustomized: stored.scoringCustomized ?? false,
  }
}

/**
 * The settings the board was computed with, straight from the response — the
 * server reports them in the same vocabulary the panel edits, so nothing has to
 * be reverse-engineered from the flex-distributed starter slots.
 */
export function serverSettings(data: DraftValuesResponse | undefined): DraftSettings | null {
  if (!data?.settings) return null
  return {
    numTeams: data.settings.num_teams,
    budget: data.settings.budget,
    scoringFormat: (data.settings.format as ScoringFormat) || 'league',
    slots: toSlots(data.settings.slots),
    scoring: toScoring(data.settings.scoring),
    // This describes what the board was actually computed with, not local edit
    // intent — a customized fetch still reports false here, same as scoringFormat
    // reflects the format used without implying the toggle was touched.
    scoringCustomized: false,
  }
}

/**
 * Draft-view state for one league, persisted to localStorage so a refresh lands
 * on the same setup. Edits are held as an unsaved draft until `save()`, which is
 * what applies them to the rankings — so a half-typed roster never rescores the
 * board. Applied settings are stored only once saved; until then the league's
 * real settings show through, so a league that changes its roster rules isn't
 * stuck behind a stale snapshot.
 */
export function useDraftSettings(leagueId: number, defaults: DraftSettings | null) {
  const [applied, setApplied] = useState<DraftSettings | null>(() => readSettings(leagueId))
  const [draft, setDraft] = useState<DraftSettings | null>(null)
  const [position, setPositionState] = useState<string>(() => readJSON<string>(positionKey(leagueId)) ?? '')

  // Navigating between leagues reuses this component, so re-read on league change.
  const [loadedFor, setLoadedFor] = useState(leagueId)
  if (loadedFor !== leagueId) {
    setLoadedFor(leagueId)
    setApplied(readSettings(leagueId))
    setDraft(null)
    setPositionState(readJSON<string>(positionKey(leagueId)) ?? '')
  }

  /** What the rankings are computed from. */
  const settings = applied ?? defaults ?? FALLBACK_SETTINGS
  /** What the panel shows — the draft once the user starts editing. */
  const editing = draft ?? settings

  const update = (patch: Partial<DraftSettings>) => setDraft({ ...editing, ...patch })

  const setSlot = (pos: SlotPosition, count: number) =>
    update({ slots: { ...editing.slots, [pos]: Math.max(0, Math.min(20, count)) } })

  // Touching any point value opts into the pointing system: it now overrides
  // the format toggle, same way an explicit slots/teams edit overrides the
  // league's own roster.
  const setScoring = (stat: ScoringStat, value: number) =>
    update({ scoring: { ...editing.scoring, [stat]: value }, scoringCustomized: true })

  const save = () => {
    if (!draft) return
    writeJSON(settingsKey(leagueId), draft)
    setApplied(draft)
    setDraft(null)
  }

  const discard = () => setDraft(null)

  const reset = () => {
    localStorage.removeItem(settingsKey(leagueId))
    setApplied(null)
    setDraft(null)
  }

  const setPosition = (next: string) => {
    writeJSON(positionKey(leagueId), next)
    setPositionState(next)
  }

  return {
    settings,
    editing,
    update,
    setSlot,
    setScoring,
    save,
    discard,
    reset,
    isDirty: draft !== null && JSON.stringify(draft) !== JSON.stringify(settings),
    isCustomized: applied !== null,
    position,
    setPosition,
  }
}
