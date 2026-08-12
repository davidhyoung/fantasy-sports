import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import {
  SLOT_POSITIONS,
  SLOT_LABELS,
  type DraftSettings,
  type ScoringFormat,
  type SlotPosition,
} from '../hooks/useDraftSettings'

const FORMATS: { value: ScoringFormat; label: string }[] = [
  { value: 'league', label: 'League' },
  { value: 'ppr', label: 'PPR' },
  { value: 'half', label: 'Half' },
  { value: 'standard', label: 'Std' },
]

/** Label + numeric field. Numbers use the mono face, per the design system. */
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

interface Props {
  settings: DraftSettings
  isCustomized: boolean
  isDirty: boolean
  open: boolean
  onToggle: () => void
  onChange: (patch: Partial<DraftSettings>) => void
  onSlotChange: (pos: SlotPosition, count: number) => void
  onSave: () => void
  onDiscard: () => void
  onReset: () => void
}

/**
 * Editable copy of the league settings that drive draft values. Edits stay
 * unsaved until Save, which rescores the board and stores them per league on
 * this device.
 */
export function DraftSettingsPanel({
  settings, isCustomized, isDirty, open, onToggle, onChange, onSlotChange, onSave, onDiscard, onReset,
}: Props) {
  const starters = SLOT_POSITIONS.reduce((sum, pos) => sum + settings.slots[pos], 0)

  return (
    <div className="rounded-lg bg-card">
      <div className="flex items-center justify-between gap-3 px-4 py-3">
        <button
          onClick={onToggle}
          aria-expanded={open}
          className="flex items-center gap-2 font-display text-sm font-semibold text-foreground"
        >
          <span className="font-mono text-xs text-muted-foreground">{open ? '▼' : '▶'}</span>
          League Settings
        </button>
        <div className="flex items-center gap-3">
          <span className="font-mono text-xs text-muted-foreground tabular-nums">
            {settings.numTeams} teams · ${settings.budget} · {starters} starters
            {settings.slots.SFLEX > 0 && ' · superflex'}
          </span>
          {isDirty && <Badge variant="pink">Unsaved</Badge>}
          {!isDirty && isCustomized && <Badge variant="pink">Customized</Badge>}
          {isCustomized && !isDirty && (
            <Button variant="text" size="sm" onClick={onReset}>
              Reset
            </Button>
          )}
        </div>
      </div>

      {open && (
        <div className="flex flex-col gap-4 border-t border-border px-4 py-4">
          <div className="flex flex-wrap items-end gap-4">
            <NumberField
              label="Teams"
              value={settings.numTeams}
              min={2}
              max={32}
              onChange={(v) => onChange({ numTeams: v })}
            />
            <NumberField
              label="Budget $"
              value={settings.budget}
              min={1}
              max={10000}
              onChange={(v) => onChange({ budget: v })}
            />
            <div className="flex flex-col gap-1">
              <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
                Scoring
              </span>
              <div className="flex w-fit rounded-lg bg-muted overflow-hidden">
                {FORMATS.map((f) => (
                  <button
                    key={f.value}
                    onClick={() => onChange({ scoringFormat: f.value })}
                    className={`px-3 py-1.5 font-display text-xs font-semibold ${
                      settings.scoringFormat === f.value
                        ? 'bg-foreground text-background'
                        : 'bg-card text-muted-foreground hover:bg-muted'
                    }`}
                  >
                    {f.label}
                  </button>
                ))}
              </div>
            </div>
          </div>

          <div className="flex flex-col gap-1.5">
            <span className="font-display text-[11px] font-semibold uppercase tracking-wide text-muted-foreground">
              Starting lineup
            </span>
            <div className="flex flex-wrap gap-3">
              {SLOT_POSITIONS.map((pos) => (
                <NumberField
                  key={pos}
                  label={SLOT_LABELS[pos]}
                  value={settings.slots[pos]}
                  max={20}
                  width="w-14"
                  onChange={(v) => onSlotChange(pos, v)}
                />
              ))}
            </div>
          </div>

          <div className="flex items-center gap-3 border-t border-border pt-3">
            <Button size="sm" onClick={onSave} disabled={!isDirty}>
              {isDirty ? 'Save & recalculate' : 'Saved'}
            </Button>
            {isDirty && (
              <Button variant="text" size="sm" onClick={onDiscard}>
                Discard
              </Button>
            )}
            <p className="text-xs text-muted-foreground">
              Saving rescores VOR, auction values and ranks against these settings, and keeps them
              on this device for next time.
            </p>
          </div>
        </div>
      )}
    </div>
  )
}
