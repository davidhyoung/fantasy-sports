import { useCallback, useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getDraftPrep, setDraftPrepPlayer, reorderDraftPrep,
  type DraftPrepEntry, type DraftPrepResponse, type InterestLevel,
} from '@/api/client'
import { keys } from '@/api/queryKeys'

const EMPTY: DraftPrepEntry = { gsis_id: '', interest: null, custom_rank: null, custom_tier: null, note: '', planned_cost: null, my_value: null, my_value_source: null }

/**
 * Your personal board for one league and season: targets and avoids, a custom
 * ranking, notes, and planned costs.
 * Stored server-side, so the board you build at your desk is the one you have on
 * draft night.
 *
 * Writes update the cache optimistically — tagging a player during prep should
 * feel instant, and a failed write rolls back rather than leaving the UI lying.
 */
export function useDraftPrep(leagueId: number | null, season: number) {
  const qc = useQueryClient()
  const queryKey = keys.draftPrep(leagueId ?? 0, season)

  const { data } = useQuery({
    queryKey,
    queryFn: () => getDraftPrep(leagueId!, season),
    enabled: !!leagueId,
    staleTime: 60 * 1000,
  })

  const byPlayer = useMemo(() => {
    const map = new Map<string, DraftPrepEntry>()
    for (const e of data?.players ?? []) map.set(e.gsis_id, e)
    return map
  }, [data])

  // Stable across renders where the underlying data hasn't changed — every
  // consumer downstream (TiersView's grouping, row-level React.memo) relies
  // on that to avoid recomputing/re-rendering on renders unrelated to this
  // data.
  const entry = useCallback(
    (gsisId: string) => byPlayer.get(gsisId) ?? { ...EMPTY, gsis_id: gsisId },
    [byPlayer],
  )

  /** Applies a change to the cached board and returns the previous copy for rollback. */
  const patchCache = (next: (prev: DraftPrepEntry[]) => DraftPrepEntry[]) => {
    const previous = qc.getQueryData<DraftPrepResponse>(queryKey)
    qc.setQueryData<DraftPrepResponse>(queryKey, (old) => ({
      season,
      players: next(old?.players ?? []),
    }))
    return previous
  }

  const setPlayer = useMutation({
    mutationFn: (v: { gsisId: string; interest: InterestLevel | null; customRank: number | null; customTier: number | null; note: string; plannedCost: number | null; myValue: number | null; myValueSource: 'user' | 'derived' | null }) =>
      setDraftPrepPlayer(leagueId!, season, v.gsisId, {
        interest: v.interest, custom_rank: v.customRank, custom_tier: v.customTier, note: v.note, planned_cost: v.plannedCost, my_value: v.myValue, my_value_source: v.myValueSource,
      }),
    onMutate: async (v) => {
      await qc.cancelQueries({ queryKey })
      const previous = patchCache((players) => {
        const rest = players.filter((p) => p.gsis_id !== v.gsisId)
        // A player with no interest, rank, tier override, note, planned cost
        // or personal value carries nothing — drop the row, same as the server.
        if (v.interest === null && v.customRank === null && v.customTier === null && !v.note && v.plannedCost === null && v.myValue === null) return rest
        return [...rest, {
          gsis_id: v.gsisId, interest: v.interest, custom_rank: v.customRank, custom_tier: v.customTier,
          note: v.note, planned_cost: v.plannedCost, my_value: v.myValue, my_value_source: v.myValueSource,
        }]
      })
      return { previous }
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.previous) qc.setQueryData(queryKey, ctx.previous)
    },
    onSettled: () => qc.invalidateQueries({ queryKey }),
  })

  const reorder = useMutation({
    mutationFn: (gsisIds: string[]) => reorderDraftPrep(leagueId!, season, gsisIds),
    onMutate: async (gsisIds) => {
      await qc.cancelQueries({ queryKey })
      const rank = new Map(gsisIds.map((id, i) => [id, i + 1]))
      const previous = patchCache((players) => {
        const merged = players.map((p) => ({ ...p, custom_rank: rank.get(p.gsis_id) ?? null }))
        // Players ranked for the first time have no row yet.
        for (const id of gsisIds) {
          if (!merged.some((p) => p.gsis_id === id)) {
            merged.push({ gsis_id: id, interest: null, custom_rank: rank.get(id)!, custom_tier: null, note: '', planned_cost: null, my_value: null, my_value_source: null })
          }
        }
        return merged
      })
      return { previous }
    },
    onError: (_e, _v, ctx) => {
      if (ctx?.previous) qc.setQueryData(queryKey, ctx.previous)
    },
    onSettled: () => qc.invalidateQueries({ queryKey }),
  })

  /** Writes one field, carrying the rest of the row through unchanged. */
  const patch = useCallback(
    (gsisId: string, changes: Partial<{ interest: InterestLevel | null; customRank: number | null; customTier: number | null; note: string; plannedCost: number | null; myValue: number | null; myValueSource: 'user' | 'derived' | null }>) => {
      const current = entry(gsisId)
      setPlayer.mutate({
        gsisId,
        interest: changes.interest !== undefined ? changes.interest : current.interest,
        customRank: changes.customRank !== undefined ? changes.customRank : current.custom_rank,
        customTier: changes.customTier !== undefined ? changes.customTier : current.custom_tier,
        note: changes.note ?? current.note,
        plannedCost: changes.plannedCost !== undefined ? changes.plannedCost : current.planned_cost,
        myValue: changes.myValue !== undefined ? changes.myValue : current.my_value,
        myValueSource: changes.myValueSource !== undefined ? changes.myValueSource : current.my_value_source,
      })
    },
    [entry, setPlayer.mutate],
  )

  /** Re-picking the current level clears it, so one control both sets and clears. */
  const setInterest = useCallback(
    (gsisId: string, level: InterestLevel) =>
      patch(gsisId, { interest: entry(gsisId).interest === level ? null : level }),
    [patch, entry],
  )

  const setNote = useCallback((gsisId: string, note: string) => patch(gsisId, { note }), [patch])

  /** null removes the player from the team plan; a number sets the price. */
  const setPlannedCost = useCallback((gsisId: string, cost: number | null) => patch(gsisId, { plannedCost: cost }), [patch])

  /** null reverts to the algorithm's own tier for this player. */
  const setCustomTier = useCallback((gsisId: string, tier: number | null) => patch(gsisId, { customTier: tier }), [patch])

  /** Your own valuation, distinct from the algorithm's auction_value — always
   *  a hand-typed edit, so it's tagged 'user' (or cleared to null alongside a
   *  cleared value). */
  const setMyValue = useCallback(
    (gsisId: string, value: number | null) => patch(gsisId, { myValue: value, myValueSource: value == null ? null : 'user' }),
    [patch],
  )

  /**
   * Sets several fields in one write. Needed whenever a single action changes
   * two fields at once (a move that reassigns rank/tier *and* auto-fills my
   * value) — two independent `patch` calls back to back would each build
   * their payload from the same pre-mutation snapshot (React hasn't
   * re-rendered between them), so the second call's stale copy of the first
   * field would silently overwrite it once both requests land.
   */
  const setFields = useCallback(
    (
      gsisId: string,
      changes: Partial<{ customRank: number | null; customTier: number | null; myValue: number | null; myValueSource: 'user' | 'derived' | null }>,
    ) => patch(gsisId, changes),
    [patch],
  )

  const counts = useMemo(() => {
    const c = { targets: 0, avoids: 0, ranked: 0, planned: 0 }
    for (const e of data?.players ?? []) {
      if (e.interest === 1) c.targets++
      else if (e.interest === -1) c.avoids++
      if (e.custom_rank != null) c.ranked++
      if (e.planned_cost != null) c.planned++
    }
    return c
  }, [data])

  return { entry, byPlayer, setInterest, setNote, setPlannedCost, setCustomTier, setMyValue, setFields, reorder, counts, isLoaded: !!data }
}
