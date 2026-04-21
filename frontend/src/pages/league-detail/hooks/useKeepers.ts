import { useState, useMemo, useEffect } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getKeeperRules,
  updateKeeperRules,
  getLeagueDraftResults,
  getLeagueKeepers,
  listTeamKeeperWishlist,
  addKeeperWishlist,
  removeKeeperWishlist,
  getLeagueRankings,
  getDraftValues,
} from '../../../api/client'
import type { Team, KeeperRules, KeeperWishlistEntry, RankedPlayer } from '../../../api/client'
import { keys } from '../../../api/queryKeys'

export const MAX_KEEPERS = 3

export interface WishlistPayload {
  player_name: string
  position: string
  draft_cost: number | null
  years_kept: number
}

/**
 * Manages all state, queries, and mutations for the Keepers tab.
 * Owns team filter, rules form, wishlist interactions, and derived keeper data.
 */
export function useKeepers(leagueId: number, myTeam: Team | undefined, active: boolean, season: string) {
  const [keeperTeamFilter, setKeeperTeamFilter] = useState('')
  const [rulesForm, setRulesForm] = useState<KeeperRules>({ cost_increase: 5, undrafted_base: 10, max_years: null })
  const qc = useQueryClient()

  const effectiveTeamFilter = keeperTeamFilter || myTeam?.yahoo_key || ''

  const { data: keeperRulesData } = useQuery({
    queryKey: keys.keeperRules(leagueId),
    queryFn: () => getKeeperRules(leagueId),
    enabled: active,
  })

  const { data: draftResults } = useQuery({
    queryKey: keys.leagueDraft(leagueId),
    queryFn: () => getLeagueDraftResults(leagueId),
    enabled: active,
  })

  const { data: yahooKeepers } = useQuery({
    queryKey: keys.leagueKeepers(leagueId),
    queryFn: () => getLeagueKeepers(leagueId),
    enabled: active,
  })

  const { data: wishlist } = useQuery({
    queryKey: keys.teamKeeperWishlist(myTeam?.id ?? 0),
    queryFn: () => listTeamKeeperWishlist(myTeam!.id),
    enabled: active && !!myTeam,
  })

  const { data: rankings } = useQuery({
    queryKey: keys.leagueRankings(leagueId, 'season'),
    queryFn: () => getLeagueRankings(leagueId, 'season'),
    enabled: active,
  })

  // Draft values for auction $ column
  const draftSeason = (parseInt(season, 10) || 2025) + 1
  const { data: draftValuesData } = useQuery({
    queryKey: keys.draftValues(leagueId, draftSeason, 'league', 200),
    queryFn: () => getDraftValues(leagueId, { season: draftSeason, budget: 200 }),
    enabled: active,
    staleTime: 5 * 60 * 1000,
  })

  // Sync server rules into local form when first loaded
  useEffect(() => {
    if (keeperRulesData) setRulesForm(keeperRulesData)
  }, [keeperRulesData])

  const rulesMutation = useMutation({
    mutationFn: (rules: KeeperRules) => updateKeeperRules(leagueId, rules),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.keeperRules(leagueId) })
      qc.invalidateQueries({ queryKey: keys.leagueDraft(leagueId) })
    },
  })

  const addMutation = useMutation({
    mutationFn: ({ playerKey, payload }: { playerKey: string; payload: WishlistPayload }) =>
      addKeeperWishlist(myTeam!.id, playerKey, payload),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.teamKeeperWishlist(myTeam!.id) })
      qc.invalidateQueries({ queryKey: keys.leagueDraft(leagueId) })
    },
  })

  const removeMutation = useMutation({
    mutationFn: (playerKey: string) => removeKeeperWishlist(myTeam!.id, playerKey),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: keys.teamKeeperWishlist(myTeam!.id) })
      qc.invalidateQueries({ queryKey: keys.leagueDraft(leagueId) })
    },
  })

  const wishlistMap = useMemo(
    () => new Map<string, KeeperWishlistEntry>(wishlist?.map((e) => [e.player_key, e]) ?? []),
    [wishlist],
  )

  const designatedSet = useMemo(
    () => new Set<string>(yahooKeepers?.map((k) => k.player_key) ?? []),
    [yahooKeepers],
  )

  const visiblePicks = useMemo(() => {
    if (!draftResults) return []
    if (!effectiveTeamFilter) return draftResults
    return draftResults.filter((p) => p.team_key === effectiveTeamFilter)
  }, [draftResults, effectiveTeamFilter])

  const rankByPlayer = useMemo(() => {
    if (!rankings) return new Map<string, RankedPlayer>()
    return new Map(rankings.players.map((p) => [p.player_key, p]))
  }, [rankings])

  // Map player name → auction value from draft values (name-based since draft picks use Yahoo keys, not gsis_id)
  const auctionByName = useMemo(() => {
    if (!draftValuesData?.players) return new Map<string, number>()
    return new Map(draftValuesData.players.map((p) => [p.name, p.auction_value]))
  }, [draftValuesData])

  return {
    keeperTeamFilter, setKeeperTeamFilter,
    effectiveTeamFilter,
    rulesForm, setRulesForm,
    rulesMutation, addMutation, removeMutation,
    draftResults, wishlist, wishlistMap, designatedSet, visiblePicks,
    rankings, rankByPlayer, auctionByName,
  }
}
