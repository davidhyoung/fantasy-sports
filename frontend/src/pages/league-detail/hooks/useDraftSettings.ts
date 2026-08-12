import { useState } from 'react'
import type { DraftValuesParams, DraftValuesResponse } from '@/api/client'

export type ScoringFormat = 'league' | 'ppr' | 'half' | 'standard'

export const SLOT_POSITIONS = ['QB', 'RB', 'WR', 'TE', 'FLEX', 'SFLEX', 'K', 'DEF'] as const
export type SlotPosition = (typeof SLOT_POSITIONS)[number]

export const SLOT_LABELS: Record<SlotPosition, string> = {
  QB: 'QB', RB: 'RB', WR: 'WR', TE: 'TE',
  FLEX: 'FLEX', SFLEX: 'SUPERFLEX', K: 'K', DEF: 'DEF',
}

export interface DraftSettings {
  numTeams: number
  budget: number
  scoringFormat: ScoringFormat
  slots: Record<SlotPosition, number>
}

/** Used until the league's own settings arrive — a standard 12-team lineup. */
export const FALLBACK_SETTINGS: DraftSettings = {
  numTeams: 12,
  budget: 200,
  scoringFormat: 'league',
  slots: { QB: 1, RB: 2, WR: 3, TE: 1, FLEX: 1, SFLEX: 0, K: 1, DEF: 1 },
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

/** Serialises settings for the API's `slots=` override. */
export function serializeSlots(slots: Record<SlotPosition, number>): string {
  return SLOT_POSITIONS.map((pos) => `${pos}:${slots[pos]}`).join(',')
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
    save,
    discard,
    reset,
    isDirty: draft !== null && JSON.stringify(draft) !== JSON.stringify(settings),
    isCustomized: applied !== null,
    position,
    setPosition,
  }
}
