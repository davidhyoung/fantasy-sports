import { useMemo } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  getDraftPrep, setDraftPrepPlayer, reorderDraftPrep,
  type DraftPrepEntry, type DraftPrepResponse, type DraftPrepTag,
} from '@/api/client'
import { keys } from '@/api/queryKeys'

const EMPTY: DraftPrepEntry = { gsis_id: '', tag: '', custom_rank: null, note: '' }

/**
 * Your personal board for one league and season: tags (target/sleeper/avoid),
 * a custom ranking, and notes. Stored server-side, so the board you build at
 * your desk is the one you have on draft night.
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

  const entry = (gsisId: string) => byPlayer.get(gsisId) ?? { ...EMPTY, gsis_id: gsisId }

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
    mutationFn: (v: { gsisId: string; tag: DraftPrepTag; customRank: number | null; note: string }) =>
      setDraftPrepPlayer(leagueId!, season, v.gsisId, {
        tag: v.tag, custom_rank: v.customRank, note: v.note,
      }),
    onMutate: async (v) => {
      await qc.cancelQueries({ queryKey })
      const previous = patchCache((players) => {
        const rest = players.filter((p) => p.gsis_id !== v.gsisId)
        // A player with no tag, rank or note carries nothing — drop the row, which
        // is what the server does too.
        if (!v.tag && v.customRank === null && !v.note) return rest
        return [...rest, { gsis_id: v.gsisId, tag: v.tag, custom_rank: v.customRank, note: v.note }]
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
            merged.push({ gsis_id: id, tag: '', custom_rank: rank.get(id)!, note: '' })
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

  /** Cycles a tag off when re-applied, so one control both sets and clears it. */
  const toggleTag = (gsisId: string, tag: Exclude<DraftPrepTag, ''>) => {
    const current = entry(gsisId)
    setPlayer.mutate({
      gsisId,
      tag: current.tag === tag ? '' : tag,
      customRank: current.custom_rank,
      note: current.note,
    })
  }

  const setNote = (gsisId: string, note: string) => {
    const current = entry(gsisId)
    setPlayer.mutate({ gsisId, tag: current.tag, customRank: current.custom_rank, note })
  }

  const counts = useMemo(() => {
    const c = { target: 0, sleeper: 0, avoid: 0, ranked: 0 }
    for (const e of data?.players ?? []) {
      if (e.tag === 'target') c.target++
      else if (e.tag === 'sleeper') c.sleeper++
      else if (e.tag === 'avoid') c.avoid++
      if (e.custom_rank != null) c.ranked++
    }
    return c
  }, [data])

  return { entry, byPlayer, toggleTag, setNote, reorder, counts, isLoaded: !!data }
}
