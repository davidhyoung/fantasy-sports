import { NBA_CATEGORY_LABELS, type NBACategory, type NBAFreeAgent, type NBARosterEntry } from '@/api/client'

export const NBA_RANK_CATEGORIES = Object.keys(NBA_CATEGORY_LABELS) as NBACategory[]

// A free agent has no team_id; normalizing it onto both shapes lets a
// rostered+free-agent pool concatenate into one typed, uniformly-handled
// row list (see NativePlayersTabNBA's "All" view and the Draft Prep board).
export type NBAPlayerRow = (NBARosterEntry | (NBAFreeAgent & { team_id?: undefined })) & { team_id?: number }

export function nbaStatValue(row: NBAPlayerRow, cat: NBACategory) {
  return row.stats?.find((s) => s.category === cat)?.value ?? 0
}

/** Rough, deliberately simple value index (counting categories, turnovers
 *  subtracted, no z-scoring) used only to pick out the "rostered player"
 *  pool below — not the final ranking, which is the z-score sum computed
 *  from the baseline that pool produces. */
export function nbaRoughValue(row: NBAPlayerRow) {
  return nbaStatValue(row, 'points') + nbaStatValue(row, 'rebounds') + nbaStatValue(row, 'assists')
    + nbaStatValue(row, 'steals') + nbaStatValue(row, 'blocks') + nbaStatValue(row, 'three_pm')
    - nbaStatValue(row, 'turnovers')
}

/**
 * The top `rosterPoolSize` players by roughValue — "average" should mean
 * the average *rostered* player, not an entire pool padded with 200+ free
 * agents nobody would actually roster, which would drag the baseline down
 * and make an ordinary rostered player look like a false outlier.
 * `rosterPoolSize` is the league's real roster math (num_teams × every
 * roster slot, BN/IR included — anyone a team would actually carry, not
 * just starters) — falls back to the full pool when settings haven't
 * loaded yet or the pool is already smaller than that count.
 */
export function nbaRosteredPlayerPool(rows: NBAPlayerRow[], rosterPoolSize: number): NBAPlayerRow[] {
  if (rosterPoolSize <= 0 || rows.length <= rosterPoolSize) return rows
  return [...rows].sort((a, b) => nbaRoughValue(b) - nbaRoughValue(a)).slice(0, rosterPoolSize)
}

export interface NBACategoryStat { cat: NBACategory; mean: number; stdev: number }

/** Population mean/stdev per category over the given pool. */
export function nbaComputeCategoryStats(rows: NBAPlayerRow[]): NBACategoryStat[] {
  return NBA_RANK_CATEGORIES.map((cat) => {
    const values = rows.map((r) => nbaStatValue(r, cat))
    const mean = values.reduce((a, b) => a + b, 0) / (values.length || 1)
    const variance = values.reduce((a, v) => a + (v - mean) ** 2, 0) / (values.length || 1)
    return { cat, mean, stdev: Math.sqrt(variance) }
  })
}

/** A category's z-score for one player, direction-corrected so "positive"
 *  always means "good for fantasy" — turnovers is the one category where a
 *  lower raw value is better, so its z-score is negated here (both for cell
 *  color and the rank sum this feeds). */
export function nbaFantasyZScore(row: NBAPlayerRow, cat: NBACategory, catStats: NBACategoryStat[]): number {
  const s = catStats.find((c) => c.cat === cat)
  if (!s || s.stdev === 0) return 0
  const z = (nbaStatValue(row, cat) - s.mean) / s.stdev
  return cat === 'turnovers' ? -z : z
}

/**
 * Equal-weight z-score sum per player, same "overall_score = weighted
 * z-sum" shape the NFL z-score ranker (analysis.go) established, simplified
 * here (no CV×scarcity weighting) since this is a client-side convenience
 * rank over whichever pool/period is currently on screen, not a persisted
 * league-wide ranking.
 */
export function nbaComputeRanks(rows: NBAPlayerRow[], catStats: NBACategoryStat[]): Map<string, number> {
  const scored = rows.map((r) => ({
    espnId: r.espn_id,
    total: NBA_RANK_CATEGORIES.reduce((sum, cat) => sum + nbaFantasyZScore(r, cat, catStats), 0),
  }))
  scored.sort((a, b) => b.total - a.total)

  const ranks = new Map<string, number>()
  scored.forEach((s, i) => ranks.set(s.espnId, i + 1))
  return ranks
}
