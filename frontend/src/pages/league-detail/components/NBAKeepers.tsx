import { useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2 } from 'lucide-react'
import { Table, TableHeader, TableBody, TableHead, TableCell } from '@/components/ui/table'
import { PlayerCell, ClickableRow, HeaderRow, SortableHead, useTableSort } from '@/components/ui/table-helpers'
import { SelectControl, FilterChip } from '@/components/ui/filter-chip'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  getNBAKeeperCandidates, designateNBAKeeper, removeNBAKeeper,
  type Team, type NBAKeeperCandidate,
} from '@/api/client'
import { keys } from '@/api/queryKeys'

const POSITIONS = ['G', 'F', 'C']

interface Props {
  leagueId: number
  season: number
  teams: Team[]
  myTeam: Team | undefined
  onSelectPlayer: (espnId: string) => void
}

/**
 * Standalone native-NBA keeper designation tool — not wired into
 * rollover.go's dormant `keeper` format (deliberate, see .claude/plans).
 * Candidate pool is every team's roster from last season (not just the
 * selected team's), since "the Brian Rule" lets any team reclaim a player
 * someone else dropped. Cost is computed server-side and never trusted from
 * the client; this view is a planning tool — it doesn't touch rosters.
 */
export function NBAKeepers({ leagueId, season, teams, myTeam, onSelectPlayer }: Props) {
  const qc = useQueryClient()
  const [teamId, setTeamId] = useState<number>(myTeam?.id ?? teams[0]?.id ?? 0)
  const [search, setSearch] = useState('')
  const [positions, setPositions] = useState<Set<string>>(new Set())
  const { sortCol, sortDir, handleSort } = useTableSort('cost', 'asc', ['cost', 'player', 'team', 'pos', 'held', 'salary'])

  const { data, isFetching } = useQuery({
    queryKey: keys.nbaKeepers(leagueId, season, teamId),
    queryFn: () => getNBAKeeperCandidates(leagueId, season, teamId),
    enabled: teamId > 0,
  })

  const designate = useMutation({
    mutationFn: (espnId: string) => designateNBAKeeper(leagueId, { season, team_id: teamId, espn_id: espnId }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.nbaKeepers(leagueId, season, teamId) }),
  })
  const remove = useMutation({
    mutationFn: (espnId: string) => removeNBAKeeper(leagueId, espnId, season),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.nbaKeepers(leagueId, season, teamId) }),
  })

  const togglePosition = (p: string) => {
    setPositions((prev) => {
      const next = new Set(prev)
      if (next.has(p)) next.delete(p); else next.add(p)
      return next
    })
  }

  const candidates = data?.candidates ?? []
  const myKeeperCount = candidates.filter((c) => c.kept_by_team_id === teamId).length
  const maxKeepers = data?.max_keepers ?? 3

  const search_ = search.trim().toLowerCase()
  const filtered = candidates.filter((c) => {
    if (positions.size > 0 && !positions.has(c.position)) return false
    if (search_ && !`${c.name} ${c.team} ${c.position}`.toLowerCase().includes(search_)) return false
    return true
  })

  const rows = useMemo(() => {
    const dirMul = sortDir === 'asc' ? 1 : -1
    return [...filtered].sort((a, b) => {
      let cmp: number
      switch (sortCol) {
        case 'cost': cmp = (a.keeper_cost ?? 0) - (b.keeper_cost ?? 0); break
        case 'player': cmp = a.name.localeCompare(b.name); break
        case 'team': cmp = (a.team || '').localeCompare(b.team || ''); break
        case 'pos': cmp = (a.position || '').localeCompare(b.position || ''); break
        case 'held': cmp = a.held_by_team_name.localeCompare(b.held_by_team_name); break
        case 'salary': cmp = a.last_salary - b.last_salary; break
        default: cmp = 0
      }
      return cmp * dirMul
    })
  }, [filtered, sortCol, sortDir])

  return (
    <>
      <div className="flex items-center gap-3 mb-4 flex-wrap">
        <SelectControl value={teamId} onChange={(e) => setTeamId(Number(e.target.value))}>
          {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
        </SelectControl>
        <div className="flex flex-none gap-1.5">
          {POSITIONS.map((p) => (
            <FilterChip key={p} active={positions.has(p)} onClick={() => togglePosition(p)}>{p}</FilterChip>
          ))}
        </div>
        <Input
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="Search players…"
          className="w-48"
        />
        <span className={`text-xs ml-1 font-mono ${myKeeperCount >= maxKeepers ? 'text-secondary' : 'text-muted-foreground'}`}>
          {myKeeperCount}/{maxKeepers} keepers
        </span>
      </div>

      {isFetching ? (
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      ) : rows.length === 0 ? (
        <p className="text-muted-foreground">
          {candidates.length === 0
            ? `No roster data for season ${season - 1} — nothing to keep from.`
            : 'No players match this filter.'}
        </p>
      ) : (
        <div className="rounded-lg bg-card overflow-x-auto max-w-[calc(100vw-3rem)]">
          <Table>
            <TableHeader style={{ top: 0 }}>
              <HeaderRow>
                <SortableHead col="player" current={sortCol} dir={sortDir} onSort={handleSort}>Player</SortableHead>
                <SortableHead col="team" current={sortCol} dir={sortDir} onSort={handleSort}>Team</SortableHead>
                <SortableHead col="pos" current={sortCol} dir={sortDir} onSort={handleSort}>Pos</SortableHead>
                <SortableHead col="held" current={sortCol} dir={sortDir} onSort={handleSort}>Held By (2025)</SortableHead>
                <SortableHead col="salary" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">2025 $</SortableHead>
                <SortableHead col="cost" current={sortCol} dir={sortDir} onSort={handleSort} className="text-right">Keeper $</SortableHead>
                <TableHead />
              </HeaderRow>
            </TableHeader>
            <TableBody>
              {rows.map((p: NBAKeeperCandidate) => {
                const keptByMe = p.kept_by_team_id === teamId
                const keptByOther = p.kept_by_team_id != null && !keptByMe
                return (
                  <ClickableRow key={p.espn_id}>
                    <PlayerCell name={p.name} imageUrl={p.headshot_url} onClick={() => onSelectPlayer(p.espn_id)} />
                    <TableCell className="text-muted-foreground">{p.team || '—'}</TableCell>
                    <TableCell className="text-muted-foreground">{p.position}</TableCell>
                    <TableCell className="text-muted-foreground">{p.held_by_team_name}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums text-muted-foreground">${p.last_salary}</TableCell>
                    <TableCell className="text-right font-mono tabular-nums text-foreground">
                      {p.keeper_cost != null ? `$${p.keeper_cost}` : '—'}
                    </TableCell>
                    <TableCell className="text-right">
                      {keptByMe ? (
                        <Button
                          variant="outline"
                          size="sm"
                          disabled={remove.isPending}
                          onClick={() => remove.mutate(p.espn_id)}
                        >
                          Drop
                        </Button>
                      ) : keptByOther ? (
                        <span className="text-xs text-muted-foreground">Kept by {p.kept_by_team_name}</span>
                      ) : (
                        <Button
                          size="sm"
                          disabled={designate.isPending || myKeeperCount >= maxKeepers}
                          onClick={() => designate.mutate(p.espn_id)}
                          title={myKeeperCount >= maxKeepers ? `Max ${maxKeepers} keepers reached` : undefined}
                        >
                          Keep
                        </Button>
                      )}
                    </TableCell>
                  </ClickableRow>
                )
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  )
}
