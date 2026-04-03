import { useState, useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { searchLeaguePlayers, getLeagueRankings } from '../../../api/client'
import type { RankedPlayer, LeaguePlayer } from '../../../api/client'
import { keys } from '../../../api/queryKeys'

export const STATUS_FILTERS = [
  { label: 'Available', value: 'available' as const },
  { label: 'Rostered',  value: 'rostered'  as const },
  { label: 'All',       value: 'all'        as const },
]
export type StatusFilter = 'available' | 'rostered' | 'all'

/**
 * Unified row type for the players table.
 * In browse mode, built directly from RankedPlayer (already has stats).
 * In search mode, built from LeaguePlayer + rankByPlayer lookup.
 */
export interface PlayerRow {
  playerKey: string
  gsisId?: string
  name: string
  teamAbbr: string
  position: string
  imageUrl?: string
  /** Empty string = unowned (available). Non-empty = rostered by that team. */
  ownerTeamKey: string
  /** Rankings entry — always present in browse, present in search if player is in top-N */
  rp?: RankedPlayer
}

/**
 * Manages all state and data for the Players tab.
 *
 * Browse mode: data comes from rankings (all rostered + top-100 FAs), filtered client-side.
 * This means sort always applies to the full relevant player universe — no pagination.
 *
 * Search mode: hits the Yahoo search API, cross-referenced with rankings for stats.
 */
export function usePlayers(leagueId: number, active: boolean) {
  const [searchInput, setSearchInput]   = useState('')
  const [activeSearch, setActiveSearch] = useState('')
  const [position, setPosition]         = useState('')
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('available')

  const { data: rankings, isFetching: loadingRankings } = useQuery({
    queryKey: keys.leagueRankings(leagueId, 'season'),
    queryFn: () => getLeagueRankings(leagueId, 'season'),
    enabled: active,
  })

  const { data: searchResults, isFetching: loadingSearch } = useQuery({
    queryKey: keys.searchPlayers(leagueId, activeSearch),
    queryFn: () => searchLeaguePlayers(leagueId, activeSearch),
    enabled: active && !!activeSearch,
  })

  const rankByPlayer = useMemo(() => {
    if (!rankings) return new Map<string, RankedPlayer>()
    return new Map(rankings.players.map((p) => [p.player_key, p]))
  }, [rankings])

  /** Full filtered+unified row list, ready for client-side sort. */
  const playerRows = useMemo((): PlayerRow[] => {
    if (activeSearch) {
      // Search mode: Yahoo results, enriched with rankings data where available.
      return (searchResults ?? []).map((p: LeaguePlayer): PlayerRow => {
        const rp = rankByPlayer.get(p.player_key)
        return {
          playerKey:    p.player_key,
          gsisId:       rp?.gsis_id,
          name:         p.name,
          teamAbbr:     p.team_abbr ?? '',
          position:     p.position ?? '',
          imageUrl:     p.image_url,
          ownerTeamKey: rp?.owner_team_key ?? '',
          rp,
        }
      })
    }

    // Browse mode: filter rankings client-side — no round-trips, full-universe sort.
    const all = rankings?.players ?? []
    return all
      .filter((p) => {
        if (statusFilter === 'available' && p.owner_team_key !== '') return false
        if (statusFilter === 'rostered'  && p.owner_team_key === '') return false
        if (position && p.position.split(',')[0] !== position)       return false
        return true
      })
      .map((p: RankedPlayer): PlayerRow => ({
        playerKey:    p.player_key,
        gsisId:       p.gsis_id,
        name:         p.name,
        teamAbbr:     p.team_abbr ?? '',
        position:     p.position ?? '',
        imageUrl:     p.headshot_url,
        ownerTeamKey: p.owner_team_key,
        rp:           p,
      }))
  }, [rankings, searchResults, activeSearch, statusFilter, position, rankByPlayer])

  function handleSearch(e: React.FormEvent) {
    e.preventDefault()
    const q = searchInput.trim()
    if (!q) { clearSearch(); return }
    setActiveSearch(q)
  }

  function clearSearch() {
    setSearchInput('')
    setActiveSearch('')
  }

  const isSearchMode = !!activeSearch
  const loading = loadingRankings || loadingSearch
  const ready   = isSearchMode ? searchResults !== undefined : rankings !== undefined

  return {
    searchInput, setSearchInput,
    activeSearch,
    position,      setPosition:     (pos: string)     => setPosition(pos),
    statusFilter,  setStatusFilter: (s: StatusFilter) => setStatusFilter(s),
    isSearchMode,
    playerRows,
    loading, ready,
    rankings, rankByPlayer,
    handleSearch, clearSearch,
  }
}
