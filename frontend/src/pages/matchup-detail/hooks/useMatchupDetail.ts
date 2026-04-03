import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { getLeagueScoreboard, getTeamRoster, listLeagueTeams } from '../../../api/client'
import type { RosterPlayer } from '../../../api/client'
import { keys } from '../../../api/queryKeys'

export type StatAccum = Record<string, { sum: number; count: number }>

/**
 * Fetches all data for a head-to-head matchup:
 * scoreboard (for team names/scores), both rosters with week stats,
 * and all derived category totals and comparison logic.
 */
export function useMatchupDetail(leagueId: number, week: number, t1Id: number, t2Id: number) {
  const weekStatType = `week:${week}`

  const { data: scoreboard, error: scoreboardError } = useQuery({
    queryKey: keys.scoreboard(leagueId, week),
    queryFn: () => getLeagueScoreboard(leagueId, week),
  })

  const { data: teams = [] } = useQuery({
    queryKey: keys.leagueTeams(leagueId),
    queryFn: () => listLeagueTeams(leagueId),
  })

  const { data: roster1, error: r1Error } = useQuery({
    queryKey: keys.teamRoster(t1Id, weekStatType),
    queryFn: () => getTeamRoster(t1Id, weekStatType),
  })

  const { data: roster2, error: r2Error } = useQuery({
    queryKey: keys.teamRoster(t2Id, weekStatType),
    queryFn: () => getTeamRoster(t2Id, weekStatType),
  })

  const yahooKeyById = useMemo(() => {
    const m: Record<number, string> = {}
    for (const t of teams) m[t.id] = t.yahoo_key ?? ''
    return m
  }, [teams])

  const t1YahooKey = yahooKeyById[t1Id]
  const t2YahooKey = yahooKeyById[t2Id]

  const matchup = useMemo(() => {
    if (!scoreboard || !t1YahooKey || !t2YahooKey) return null
    return scoreboard.matchups.find(
      (m) =>
        m.teams.some((t) => t.team_key === t1YahooKey) &&
        m.teams.some((t) => t.team_key === t2YahooKey),
    ) ?? null
  }, [scoreboard, t1YahooKey, t2YahooKey])

  const t1Score = matchup?.teams.find((t) => t.team_key === t1YahooKey)
  const t2Score = matchup?.teams.find((t) => t.team_key === t2YahooKey)

  // Union of stat labels from both rosters, preserving encounter order
  const statLabels = useMemo(() => {
    const seen = new Set<string>()
    const labels: string[] = []
    for (const p of [...(roster1 ?? []), ...(roster2 ?? [])]) {
      for (const s of p.stats ?? []) {
        if (!seen.has(s.label)) { seen.add(s.label); labels.push(s.label) }
      }
    }
    return labels
  }, [roster1, roster2])

  // sort_order per stat label ("0" = lower is better, e.g. turnovers)
  const sortOrderByLabel = useMemo(() => {
    const m: Record<string, string> = {}
    for (const p of [...(roster1 ?? []), ...(roster2 ?? [])]) {
      for (const s of p.stats ?? []) m[s.label] = s.sort_order
    }
    return m
  }, [roster1, roster2])

  function accumStats(roster: RosterPlayer[] | undefined): StatAccum {
    const acc: StatAccum = {}
    for (const p of roster ?? []) {
      for (const s of p.stats ?? []) {
        const prev = acc[s.label] ?? { sum: 0, count: 0 }
        acc[s.label] = { sum: prev.sum + parseFloat(s.value || '0'), count: prev.count + 1 }
      }
    }
    return acc
  }

  // Percentage stats (FG%, FT%) are averaged across players; counting stats are summed
  function teamValue(acc: StatAccum, label: string): number {
    const s = acc[label]
    if (!s) return 0
    return label.includes('%') ? (s.count > 0 ? s.sum / s.count : 0) : s.sum
  }

  const accum1 = useMemo(() => accumStats(roster1), [roster1])
  const accum2 = useMemo(() => accumStats(roster2), [roster2])

  function wins(label: string): 1 | 2 | 0 {
    const v1 = teamValue(accum1, label)
    const v2 = teamValue(accum2, label)
    const higherIsBetter = sortOrderByLabel[label] !== '0'
    if (Math.abs(v1 - v2) < 0.0001) return 0
    return (higherIsBetter ? v1 > v2 : v1 < v2) ? 1 : 2
  }

  function fmt(n: number, label: string): string {
    if (label.includes('%')) return n.toFixed(3).replace(/^0\./, '.')
    return Number.isInteger(n) ? String(n) : n.toFixed(1)
  }

  const t1Name = t1Score?.name ?? teams.find((t) => t.id === t1Id)?.name ?? `Team ${t1Id}`
  const t2Name = t2Score?.name ?? teams.find((t) => t.id === t2Id)?.name ?? `Team ${t2Id}`

  const loading = !scoreboard || !roster1 || !roster2 || teams.length === 0
  const error = scoreboardError ?? r1Error ?? r2Error

  return {
    loading, error,
    matchup, t1Score, t2Score, t1Name, t2Name,
    roster1, roster2,
    statLabels,
    accum1, accum2,
    teamValue, wins, fmt,
  }
}
