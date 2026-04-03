import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useSearchParams } from 'react-router-dom'
import { getTeam, getTeamRoster, getLeagueScoreboard, listLeagueTeams, getLeagueRankings } from '../../../api/client'
import type { RankedPlayer } from '../../../api/client'
import { keys } from '../../../api/queryKeys'

export const PERIODS = [
  { label: 'Today',     value: 'today' },
  { label: 'Last Week', value: 'lastweek' },
  { label: 'Season',    value: 'season' },
] as const

export type StatPeriod = (typeof PERIODS)[number]['value']

/**
 * Fetches all data needed for the TeamDetail page:
 * team info, current-week scoreboard matchup, and roster with stats.
 * Also owns the stat period selector state.
 */
export function useTeamDetail(teamId: number) {
  const [searchParams, setSearchParams] = useSearchParams()
  const statPeriod = (searchParams.get('period') ?? 'season') as StatPeriod
  const setStatPeriod = (p: StatPeriod) =>
    setSearchParams((prev) => { prev.set('period', p); return prev }, { replace: true })

  const { data: team, error } = useQuery({
    queryKey: keys.team(teamId),
    queryFn: () => getTeam(teamId),
  })

  const { data: scoreboard } = useQuery({
    queryKey: keys.scoreboard(team?.league_id ?? 0),
    queryFn: () => getLeagueScoreboard(team!.league_id),
    enabled: !!team?.league_id,
  })

  const { data: leagueTeams = [] } = useQuery({
    queryKey: keys.leagueTeams(team?.league_id ?? 0),
    queryFn: () => listLeagueTeams(team!.league_id),
    enabled: !!team?.league_id,
  })

  const { data: roster, error: rosterError } = useQuery({
    queryKey: keys.teamRoster(teamId, statPeriod),
    queryFn: () => getTeamRoster(teamId, statPeriod),
    enabled: !!team,
  })

  const { data: rankings } = useQuery({
    queryKey: keys.leagueRankings(team?.league_id ?? 0, statPeriod),
    queryFn: () => getLeagueRankings(team!.league_id, statPeriod),
    enabled: !!team?.league_id,
  })

  const rankByPlayer = useMemo((): Map<string, RankedPlayer> => {
    if (!rankings) return new Map()
    return new Map(rankings.players.map((p) => [p.player_key, p]))
  }, [rankings])

  const yahooKeyToId = useMemo(() => {
    const m: Record<string, number> = {}
    for (const t of leagueTeams) if (t.yahoo_key) m[t.yahoo_key] = t.id
    return m
  }, [leagueTeams])

  // Derive stat column labels from whatever Yahoo returns for the selected period
  const statLabels = useMemo(() => {
    if (!roster) return []
    const seen = new Set<string>()
    const labels: string[] = []
    for (const p of roster) {
      for (const s of p.stats ?? []) {
        if (!seen.has(s.label)) { seen.add(s.label); labels.push(s.label) }
      }
    }
    return labels
  }, [roster])

  const matchup = scoreboard?.matchups.find((m) =>
    m.teams.some((t) => t.team_key === team?.yahoo_key)
  )
  const thisTeam = matchup?.teams.find((t) => t.team_key === team?.yahoo_key)
  const opponent = matchup?.teams.find((t) => t.team_key !== team?.yahoo_key)

  const opponentId = opponent ? yahooKeyToId[opponent.team_key] : undefined
  const matchupHref = scoreboard?.week != null && opponentId
    ? `/leagues/${team?.league_id}/matchup/${scoreboard.week}/${teamId}/${opponentId}`
    : null

  return {
    team, error,
    statPeriod, setStatPeriod,
    roster, rosterError, statLabels,
    scoreboard, matchup, thisTeam, opponent, matchupHref,
    rankByPlayer,
  }
}
