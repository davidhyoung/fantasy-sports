import { Loader2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import type { KeeperRules } from '../../../api/client'

interface Props {
  rulesForm: KeeperRules
  onChange: (rules: KeeperRules) => void
  onSave: () => void
  isPending: boolean
}

/** Inline form bar for editing and saving keeper cost rules for a league. */
export function KeeperRulesBar({ rulesForm, onChange, onSave, isPending }: Props) {
  return (
    <div className="bg-card rounded-lg p-3 mb-4 flex flex-wrap items-center gap-3">
      <span className="text-sm font-medium text-foreground">Keeper Rules:</span>

      <label className="flex items-center gap-1 text-sm text-muted-foreground">
        Cost increase $
        <Input
          type="number"
          min={0}
          value={rulesForm.cost_increase}
          onChange={(e) => onChange({ ...rulesForm, cost_increase: Number(e.target.value) })}
          className="w-16 h-7 text-sm px-2"
        />
        /yr
      </label>

      <label className="flex items-center gap-1 text-sm text-muted-foreground">
        FA base $
        <Input
          type="number"
          min={1}
          value={rulesForm.undrafted_base}
          onChange={(e) => onChange({ ...rulesForm, undrafted_base: Number(e.target.value) })}
          className="w-16 h-7 text-sm px-2"
        />
      </label>

      <label className="flex items-center gap-1 text-sm text-muted-foreground">
        Max years
        <Input
          type="number"
          min={0}
          placeholder="∞"
          value={rulesForm.max_years ?? ''}
          onChange={(e) =>
            onChange({
              ...rulesForm,
              max_years: e.target.value === '' ? null : Number(e.target.value),
            })
          }
          className="w-16 h-7 text-sm px-2"
        />
      </label>

      <Button size="sm" disabled={isPending} onClick={onSave}>
        {isPending ? <Loader2 className="h-3 w-3 animate-spin" /> : 'Save'}
      </Button>
    </div>
  )
}
