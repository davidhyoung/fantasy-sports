import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/table'
import { HeaderRow } from '@/components/ui/table-helpers'
import type { StatAccum } from '../hooks/useMatchupDetail'

interface Props {
  statLabels: string[]
  accum1: StatAccum
  accum2: StatAccum
  teamValue: (acc: StatAccum, label: string) => number
  wins: (label: string) => 1 | 2 | 0
  fmt: (n: number, label: string) => string
  t1Name: string
  t2Name: string
}

/** Side-by-side category totals with a win indicator (✓) for the leading team. */
export function CategoryTotalsTable({ statLabels, accum1, accum2, teamValue, wins, fmt, t1Name, t2Name }: Props) {
  return (
    <div className="mb-8">
      <h2 className="text-sm font-semibold text-muted-foreground uppercase tracking-wide mb-3">
        Category Totals
      </h2>
      <div className="rounded-lg bg-card">
        <Table>
          <TableHeader>
            <HeaderRow>
              <TableHead className="w-1/3">{t1Name}</TableHead>
              <TableHead className="text-center w-1/3">Category</TableHead>
              <TableHead className="text-right w-1/3">{t2Name}</TableHead>
            </HeaderRow>
          </TableHeader>
          <TableBody>
            {statLabels.map((label) => {
              const winner = wins(label)
              const v1 = teamValue(accum1, label)
              const v2 = teamValue(accum2, label)
              const isPct = label.includes('%')
              return (
                <TableRow key={label}>
                  <TableCell className={`tabular-nums font-medium ${winner === 1 ? 'text-foreground' : 'text-muted-foreground'}`}>
                    {fmt(v1, label)}
                    {winner === 1 && <span className="ml-1.5 text-green-600 dark:text-green-400 text-xs" role="img" aria-label="Winner">✓</span>}
                  </TableCell>
                  <TableCell className="text-center text-sm">
                    <span className="text-muted-foreground">{label}</span>
                    {isPct && <span className="block text-muted-foreground text-xs">avg</span>}
                  </TableCell>
                  <TableCell className={`text-right tabular-nums font-medium ${winner === 2 ? 'text-foreground' : 'text-muted-foreground'}`}>
                    {winner === 2 && <span className="mr-1.5 text-green-600 dark:text-green-400 text-xs" role="img" aria-label="Winner">✓</span>}
                    {fmt(v2, label)}
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </div>
    </div>
  )
}
