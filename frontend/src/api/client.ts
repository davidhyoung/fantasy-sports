// --- Types ---

export interface User {
  id: number
  yahoo_guid: string
  display_name: string
  email?: string
  created_at: string
}

export interface League {
  id: number
  name: string
  sport: string
  season: string
  yahoo_key?: string
  logo_url?: string
  // "yahoo" (synced from a live Yahoo league) or "native" (no external league
  // at all — created and owned entirely in this app).
  source: 'yahoo' | 'native'
  format: 'redraft' | 'keeper' | 'dynasty'
  created_at: string
}

export interface LeagueSettings {
  league_id: number
  num_teams: number
  budget: number
  slots: Record<string, number>
  scoring: Record<string, number>
  taxi_slots: number
  ir_slots: number
  draft_rounds: number
}

export interface CreateLeagueRequest {
  name: string
  sport: string
  season: string
  format: 'redraft' | 'keeper' | 'dynasty'
  settings: {
    budget: number
    slots: Record<string, number>
    scoring: Record<string, number>
  }
  teams: { name: string }[]
}

export interface CreateLeagueResponse extends League {
  settings: LeagueSettings
  teams: Team[]
}

export interface Team {
  id: number
  league_id: number
  name: string
  yahoo_key?: string
  user_id?: number
  logo_url?: string
  is_commissioner?: boolean
}

export interface RosterStat {
  label: string
  value: string
  sort_order: string // "1" = higher is better, "0" = lower is better (e.g. TO)
}

export interface RosterPlayer {
  player_key: string
  player_id: string
  gsis_id?: string  // present when player can be resolved to an NFL player
  name: { full: string; first: string; last: string }
  team_abbr: string
  display_position: string
  selected_position: { position: string }
  image_url?: string
  stats?: RosterStat[]
}

export interface Player {
  id: number
  name: string
  sport: string
  position: string
  external_id?: string
}

// --- Generic fetch helper ---

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`/api${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  })
  if (!res.ok) {
    // Every handler error is {"error": "message"} — surface it when present,
    // since messages like "settings.num_teams must match the number of teams"
    // are the whole point of a 400 here, not just a status code.
    const body = await res.json().catch(() => null)
    throw new Error(body?.error || `${res.status} ${res.statusText}`)
  }
  return res.json()
}

// --- Auth ---

// getMe calls GET /api/auth/me and returns the logged-in user.
// Throws a "401 Unauthorized" error when no valid session exists —
// callers should catch this and treat it as "not logged in".
export const getMe = () => request<User>('/auth/me')

// --- Sync ---

// sync calls POST /api/sync, which fetches the user's Yahoo leagues and upserts
// them into our database. Returns the list of synced leagues.
export const sync = () => request<League[]>('/sync', { method: 'POST' })

// --- Leagues ---

export const listLeagues = () => request<League[]>('/leagues')
export const getLeague = (id: number) => request<League>(`/leagues/${id}`)
// Creates a native league — no Yahoo league behind it. There is no way to
// create a Yahoo-backed league through this API; those arrive via POST /api/sync.
export const createLeague = (data: CreateLeagueRequest) =>
  request<CreateLeagueResponse>('/leagues', { method: 'POST', body: JSON.stringify(data) })

// --- Native league settings ---

export const getLeagueSettings = (leagueId: number) =>
  request<LeagueSettings>(`/leagues/${leagueId}/settings`)

export const updateLeagueSettings = (
  leagueId: number,
  settings: { num_teams: number; budget: number; slots: Record<string, number>; scoring: Record<string, number>; taxi_slots: number; ir_slots: number; draft_rounds: number }
) =>
  request<LeagueSettings>(`/leagues/${leagueId}/settings`, { method: 'PUT', body: JSON.stringify(settings) })

// --- Teams ---

export const listLeagueTeams = (leagueId: number) =>
  request<Team[]>(`/leagues/${leagueId}/teams`)
export const getTeam = (id: number) => request<Team>(`/teams/${id}`)
// Renames and/or claims/releases a native-league team as the caller's own —
// both fields optional and independent.
export const updateLeagueTeam = (leagueId: number, teamId: number, patch: { name?: string; claim?: boolean }) =>
  request<{ status: string }>(`/leagues/${leagueId}/teams/${teamId}`, { method: 'PUT', body: JSON.stringify(patch) })
// statType: 'week' (default/current), 'lastweek', 'season', 'today', or 'week:N' for a specific week number.
// 'week:N' is translated to ?week=N; all other values are passed as ?stat_type=<value>.
export const getTeamRoster = (id: number, statType?: string) => {
  let qs = ''
  if (statType) {
    if (statType.startsWith('week:')) {
      qs = `?week=${statType.slice(5)}`
    } else {
      qs = `?stat_type=${encodeURIComponent(statType)}`
    }
  }
  return request<RosterPlayer[]>(`/teams/${id}/roster${qs}`)
}

// --- Scoreboard & Standings ---

export interface MatchupTeam {
  team_key: string
  // Populated only for native leagues — lets native UI key off a real id
  // instead of reverse-engineering a fake team_key.
  team_id?: number
  name: string
  logo_url?: string
  points: string
  projected_points: string
}

export interface Matchup {
  week: number
  week_start: string
  week_end: string
  status: string
  is_playoffs: string
  teams: MatchupTeam[]
}

export interface Scoreboard {
  week: number
  matchups: Matchup[]
}

export interface Standing {
  team_key: string
  team_id?: number // native leagues only — see MatchupTeam.team_id
  name: string
  logo_url?: string
  rank: number
  playoff_seed: number
  wins: number
  losses: number
  ties: number
  percentage: string
  points_for: string
  points_against: string
  streak_type: string
  streak_value: number
  // Native leagues only — chronological "W"/"L"/"T" for the last up to 5
  // scored matchups, and whether this rank currently makes the postseason
  // (a size-based heuristic — see backend playoffCutoff, no real
  // playoff-teams setting exists yet).
  last5?: string[]
  makes_playoffs?: boolean
}

export const getLeagueScoreboard = (id: number, week?: number) =>
  request<Scoreboard>(`/leagues/${id}/scoreboard${week ? `?week=${week}` : ''}`)
export const getLeagueStandings = (id: number) =>
  request<Standing[]>(`/leagues/${id}/standings`)

// --- Native league schedule & scoring ---

export const generateLeagueSchedule = (leagueId: number, season?: number, weeks?: number) =>
  request<{ status: string; season: number; weeks: number; matchups: number }>(`/leagues/${leagueId}/schedule/generate`, {
    method: 'POST',
    body: JSON.stringify({ season, weeks }),
  })

export const scoreLeagueWeek = (leagueId: number, week: number, season?: number) => {
  const params = new URLSearchParams({ week: String(week) })
  if (season) params.set('season', String(season))
  return request<{ status: string; week: number; season: number; matchups: number }>(
    `/leagues/${leagueId}/scoreboard/score?${params.toString()}`,
    { method: 'POST' }
  )
}

// --- League Players (Yahoo search / available) ---

export interface LeaguePlayer {
  player_key: string
  name: string
  team_abbr: string
  position: string
  status: string
  ownership_type: string
  owned_percent: string
  image_url?: string
}

export const searchLeaguePlayers = (leagueId: number, query: string) =>
  request<LeaguePlayer[]>(`/leagues/${leagueId}/players?search=${encodeURIComponent(query)}`)

// statusFilter: 'A' = available (FA+W), 'FA' = free agents, 'W' = waivers, 'all' = everyone
export const getAvailablePlayers = (leagueId: number, position = '', start = 0, statusFilter = 'A') => {
  const params = new URLSearchParams()
  if (position) params.set('position', position)
  if (start > 0) params.set('start', String(start))
  if (statusFilter !== 'A') params.set('status', statusFilter)
  const qs = params.toString()
  return request<LeaguePlayer[]>(`/leagues/${leagueId}/players/available${qs ? `?${qs}` : ''}`)
}

// --- Native League Rosters ---
//
// Native leagues have no Yahoo league behind them, so `/players` and
// `/players/available` above (and `/rankings` below) don't work for them —
// those hit Yahoo. These are the native-league equivalents: roster state and
// the free-agent pool both live in this app's own DB.

// One projected season-total stat category, restricted server-side to
// whatever the league's own scoring actually weights nonzero — "relevant"
// meaning it impacts this league's scoring. `stat` is a canonical key
// (pass_yds, pass_td, ...) matching SCORING_STATS in useDraftSettings.ts.
export interface PlayerStat {
  stat: string
  value: number
}

export interface RosterEntry {
  gsis_id: string
  name: string
  position: string
  team: string
  headshot_url?: string
  team_id: number
  slot: string
  acquired_via: string
  acquired_at: string
  salary: number
  signed_season: number
  years_total: number | null
  years_used: number
  stats?: PlayerStat[]
  // Chronological (oldest first) real fantasy points for the trailing up to
  // 4 weeks with imported stats — powers the Players tab's "L4 wks" sparkline.
  trend?: number[]
}

export interface FreeAgent {
  gsis_id: string
  name: string
  position: string
  team: string
  headshot_url?: string
  proj_fpts_ppr: number | null
  stats?: PlayerStat[]
  trend?: number[]
}

export const getLeagueRosters = (leagueId: number) =>
  request<RosterEntry[]>(`/leagues/${leagueId}/rosters`)

export const assignLeagueRoster = (
  leagueId: number,
  data: { gsis_id: string; team_id: number; slot?: string; acquired_via: string; salary: number; years_total?: number | null }
) =>
  request<{ status: string }>(`/leagues/${leagueId}/rosters`, { method: 'POST', body: JSON.stringify(data) })

export const dropLeagueRoster = (leagueId: number, gsisId: string) =>
  request<{ status: string }>(`/leagues/${leagueId}/rosters/${encodeURIComponent(gsisId)}`, { method: 'DELETE' })

// Moves a rostered player to another team (a trade), a different slot,
// and/or edits contract terms. Only fields present in the patch are changed.
export const updateLeagueRoster = (
  leagueId: number,
  gsisId: string,
  patch: { team_id?: number; slot?: string; salary?: number; years_total?: number | null; clear_years_total?: boolean }
) =>
  request<{ status: string }>(`/leagues/${leagueId}/rosters/${encodeURIComponent(gsisId)}`, {
    method: 'PUT',
    body: JSON.stringify(patch),
  })

export const getLeagueFreeAgents = (leagueId: number, position = '', limit = 100, offset = 0, search = '') => {
  const params = new URLSearchParams()
  if (position) params.set('position', position)
  params.set('limit', String(limit))
  if (offset > 0) params.set('offset', String(offset))
  if (search) params.set('search', search)
  return request<FreeAgent[]>(`/leagues/${leagueId}/free-agents?${params.toString()}`)
}

// --- Draft picks & trades (native leagues only) ---

// Named LeagueDraftPick, not DraftPick — that name is already taken below by
// the Yahoo keeper-draft-results type.
export interface LeagueDraftPick {
  id: number
  season: number
  round: number
  original_team_id: number
  original_team_name: string
  current_team_id: number
  current_team_name: string
  overall_pick: number | null
  used_on_gsis_id: string | null
  used_on_name: string | null
}

export const getLeagueDraftPicks = (leagueId: number, season?: number, teamId?: number) => {
  const params = new URLSearchParams()
  if (season) params.set('season', String(season))
  if (teamId) params.set('team_id', String(teamId))
  const qs = params.toString()
  return request<LeagueDraftPick[]>(`/leagues/${leagueId}/picks${qs ? `?${qs}` : ''}`)
}

export const generateLeagueDraftPicks = (leagueId: number, season?: number, rounds?: number) =>
  request<{ status: string; season: number; rounds: number; picks: number }>(`/leagues/${leagueId}/picks/generate`, {
    method: 'POST',
    body: JSON.stringify({ season, rounds }),
  })

export const useLeagueDraftPick = (
  leagueId: number,
  pickId: number,
  data: { gsis_id: string; slot?: string; salary: number; signed_season?: number; years_total?: number | null }
) =>
  request<{ status: string }>(`/leagues/${leagueId}/picks/${pickId}/use`, { method: 'POST', body: JSON.stringify(data) })

export interface TradeAsset {
  kind: 'player' | 'pick'
  gsis_id?: string
  pick_id?: number
  to_team_id: number
}

export const createLeagueTrade = (leagueId: number, assets: TradeAsset[]) =>
  request<{ status: string; moves: unknown[] }>(`/leagues/${leagueId}/trades`, {
    method: 'POST',
    body: JSON.stringify({ assets }),
  })

export interface LeagueTransaction {
  kind: 'draft' | 'auction' | 'trade' | 'add' | 'drop' | 'keeper' | 'rollover'
  payload: Record<string, unknown>
  created_at: string
}

export const getLeagueTransactions = (leagueId: number, season?: number) =>
  request<LeagueTransaction[]>(`/leagues/${leagueId}/transactions${season ? `?season=${season}` : ''}`)

export const rolloverLeague = (leagueId: number, toSeason?: number, rounds?: number) =>
  request<{ status: string; from_season: number; to_season: number }>(`/leagues/${leagueId}/rollover`, {
    method: 'POST',
    body: JSON.stringify({ to_season: toSeason, rounds }),
  })

// --- Keepers ---

export interface KeeperRules {
  cost_increase: number    // $ increase per year kept
  undrafted_base: number   // base $ for undrafted / FA players
  max_years: number | null // null = unlimited
}

export interface DraftPick {
  player_key: string
  player_name: string
  position: string
  image_url?: string
  team_key: string
  owner_team_id: number
  owner_team_name: string
  draft_cost: number       // original auction price (0 = snake / undrafted)
  keeper_cost: number      // projected cost if kept this year
  years_kept: number       // 0 = not on user's wishlist
  not_keepable: boolean    // true if max_years exceeded
  undrafted: boolean       // true if picked up off waivers
  stats?: RosterStat[]     // season stats mapped to scoring categories
}

export interface KeeperPlayer {
  player_key: string
  name: string
  position: string
}

export interface KeeperWishlistEntry {
  id: number
  team_id: number
  player_key: string
  player_name: string
  position: string
  draft_cost: number | null // null if undrafted
  years_kept: number
}

export const getKeeperRules = (leagueId: number) =>
  request<KeeperRules>(`/leagues/${leagueId}/keeper-rules`)

export const updateKeeperRules = (leagueId: number, rules: KeeperRules) =>
  request<KeeperRules>(`/leagues/${leagueId}/keeper-rules`, {
    method: 'PUT',
    body: JSON.stringify(rules),
  })

export const getLeagueDraftResults = (leagueId: number) =>
  request<DraftPick[]>(`/leagues/${leagueId}/draftresults`)

export const getLeagueKeepers = (leagueId: number) =>
  request<KeeperPlayer[]>(`/leagues/${leagueId}/keepers`)

export const listTeamKeeperWishlist = (teamId: number) =>
  request<KeeperWishlistEntry[]>(`/teams/${teamId}/keepers`)

export const addKeeperWishlist = (
  teamId: number,
  playerKey: string,
  data: { player_name: string; position?: string; draft_cost?: number | null; years_kept?: number }
) =>
  request<KeeperWishlistEntry>(
    `/teams/${teamId}/keepers/${encodeURIComponent(playerKey)}`,
    { method: 'POST', body: JSON.stringify(data) }
  )

export const removeKeeperWishlist = (teamId: number, playerKey: string) =>
  request<void>(
    `/teams/${teamId}/keepers/${encodeURIComponent(playerKey)}`,
    { method: 'DELETE' }
  )

export const submitKeepers = (teamId: number) =>
  request<void>(`/teams/${teamId}/keepers/submit`, { method: 'POST' })

export const unsubmitKeepers = (teamId: number) =>
  request<void>(`/teams/${teamId}/keepers/submit`, { method: 'DELETE' })

export interface KeeperSummaryEntry {
  team_id: number
  team_name: string
  logo_url?: string
  submitted: boolean
  submitted_at?: string
  keepers: KeeperWishlistEntry[]
}

export const getKeeperSummary = (leagueId: number) =>
  request<KeeperSummaryEntry[]>(`/leagues/${leagueId}/keeper-summary`)

// --- Rankings / Analysis ---

export interface CategoryStats {
  label: string
  sort_order: string
  mean: number
  stdev: number
  weight: number  // mean-normalised; 1.0 = average category weight
}

export interface PlayerCategoryScore {
  label: string
  value: number
  z_score: number
  percentile: number
}

export interface RankingTrajectoryPoint {
  season: number
  fpts_ppr_pg: number
}

export interface RankedPlayer {
  player_key: string
  gsis_id?: string  // present when player can be resolved to an NFL player
  name: string
  headshot_url?: string
  position: string
  team_abbr: string
  owner_team_key: string
  overall_score: number
  overall_rank: number
  position_score: number  // z-score relative to same-position peers
  position_rank: number
  total_points?: number   // raw fantasy points (points leagues only)
  category_scores: PlayerCategoryScore[]
  trajectory?: RankingTrajectoryPoint[]  // year-over-year fpts_ppr_pg from season profiles
  player_grade?: number | null
  yoy_trend?: number | null
}

export interface ReplacementLevel {
  position: string
  threshold: number  // number of starters league-wide
  points: number     // fantasy points of the replacement-level player
}

export interface LeagueRankings {
  stat_type: string
  scoring_mode: string  // "points" (NFL) or "categories" (NBA)
  categories: CategoryStats[]
  players: RankedPlayer[]
  replacement_levels?: ReplacementLevel[]  // points leagues only
}

export const getLeagueRankings = (leagueId: number, statType = 'season') =>
  request<LeagueRankings>(`/leagues/${leagueId}/rankings?stat_type=${encodeURIComponent(statType)}`)

// --- Players (local DB) ---

export const listPlayers = () => request<Player[]>('/players')
export const getPlayer = (id: number) => request<Player>(`/players/${id}`)
export const createPlayer = (data: Omit<Player, 'id'>) =>
  request<Player>('/players', { method: 'POST', body: JSON.stringify(data) })

// --- Projections ---

export interface ProjPlayerListItem {
  gsis_id: string
  name: string
  position: string
  position_group: string
  team: string
  headshot_url: string
  age: number
  target_season: number
  proj_fpts: number
  proj_fpts_ppr: number
  proj_fpts_half: number
  proj_fpts_ppr_pg: number
  confidence: number
  comp_count: number
  uniqueness: 'common' | 'moderate' | 'rare' | 'unique'
  overall_rank: number
  position_rank: number
  player_grade: number | null
  grade_rank: number | null
}

export interface ProjListResponse {
  season: number
  players: ProjPlayerListItem[]
  total: number
}

export interface ProjTrajPoint {
  season: number
  age: number
  fpts_ppr_pg: number
  fpts_pg: number
  growth: number
}

export interface ProjComp {
  gsis_id: string
  name: string
  match_season: number
  match_age: number
  similarity: number
  weight: number
  headshot_url: string
  match_profile: Record<string, number>
  pre_match: ProjTrajPoint[]
  trajectory: ProjTrajPoint[]
  matching_dims: string[]
  divergent_dims: string[]
}

export interface ProjConfidence {
  overall: number
  similarity: number
  comp_count: number
  agreement: number
  sample_depth: number
  data_quality: number
}

export interface ProjStats {
  fpts_pg: number
  fpts_ppr_pg: number
  pass_yds_pg: number
  pass_td_pg: number
  rush_yds_pg: number
  rush_td_pg: number
  rec_pg: number
  rec_yds_pg: number
  rec_td_pg: number
  fg_made_pg: number
  pat_made_pg: number
  games: number
  fpts: number
  fpts_ppr: number
  fpts_half: number
  // Outcome distribution from the comp set (season-total PPR points).
  // p10 ≈ floor, p90 ≈ ceiling; p10 === p90 means too few comps to estimate.
  fpts_ppr_stdev_pg: number
  fpts_ppr_p10: number
  fpts_ppr_p50: number
  fpts_ppr_p90: number
}

export interface HistoricalSeason {
  season: number
  age: number
  fpts_ppr_pg: number
  fpts_pg: number
  games: number
}

export interface ProjDetailResponse {
  gsis_id: string
  name: string
  position: string
  position_group: string
  team: string
  headshot_url: string
  age: number
  base_season: number
  target_season: number
  projection: ProjStats
  confidence: ProjConfidence
  comp_count: number
  uniqueness: 'common' | 'moderate' | 'rare' | 'unique'
  comps: ProjComp[]
  historical: HistoricalSeason[]
  player_grade: number | null
}

export const getProjections = (params: {
  season?: number
  position?: string
  sort?: string
  limit?: number
  offset?: number
}) => {
  const qs = new URLSearchParams()
  if (params.season) qs.set('season', String(params.season))
  if (params.position) qs.set('position', params.position)
  if (params.sort) qs.set('sort', params.sort)
  if (params.limit) qs.set('limit', String(params.limit))
  if (params.offset) qs.set('offset', String(params.offset))
  const q = qs.toString()
  return request<ProjListResponse>(`/projections${q ? `?${q}` : ''}`)
}

export const getProjectionDetail = (gsisId: string, season?: number) => {
  const qs = season ? `?season=${season}` : ''
  return request<ProjDetailResponse>(`/projections/${encodeURIComponent(gsisId)}${qs}`)
}

// --- Public Rankings (projection + format-based, no league/Yahoo required) ---

export type RankingsFormat = 'ppr' | 'half' | 'standard'

export interface PublicRankedPlayer {
  gsis_id: string
  name: string
  position: string
  position_group: string
  team: string
  headshot_url: string
  age: number
  games: number
  fpts: number
  fpts_pg: number
  confidence: number
  comp_count: number
  uniqueness: 'common' | 'moderate' | 'rare' | 'unique'
  overall_rank: number
  position_rank: number
  player_grade: number | null
}

export interface PublicRankingsResponse {
  season: number
  format: RankingsFormat
  players: PublicRankedPlayer[]
  total: number
}

export const getPublicRankings = (params: {
  season?: number
  format?: RankingsFormat
  position?: string
  limit?: number
  offset?: number
}) => {
  const qs = new URLSearchParams()
  if (params.season) qs.set('season', String(params.season))
  if (params.format) qs.set('format', params.format)
  if (params.position) qs.set('position', params.position)
  if (params.limit) qs.set('limit', String(params.limit))
  if (params.offset) qs.set('offset', String(params.offset))
  const q = qs.toString()
  return request<PublicRankingsResponse>(`/rankings${q ? `?${q}` : ''}`)
}

// --- Draft Values (league-specific auction values) ---

export interface DraftReplacementLevel {
  position: string
  starter_slots: number
  threshold: number
  points: number
}

export interface DraftPlayer {
  gsis_id: string
  name: string
  position: string
  position_group: string
  team: string
  headshot_url: string
  age: number
  proj_fpts: number
  proj_fpts_ppr: number
  proj_fpts_half: number
  proj_fpts_ppr_pg: number
  proj_league_fpts: number
  /** Per-game points in this league's actual scoring (not fixed full-PPR). */
  proj_league_ppg: number
  confidence: number
  comp_count: number
  uniqueness: 'common' | 'moderate' | 'rare' | 'unique'
  vor: number
  auction_value: number
  overall_rank: number
  position_rank: number
  /** Players at the same position whose value is close enough to be interchangeable. 1 = best. */
  tier: number
  /** Market price on this league's dollar scale. null where no source covers him. */
  consensus_auction_value: number | null
  consensus_position_rank: number | null
  consensus_sources: number
  /** true = inferred from consensus rank; false = an imported market price. */
  consensus_derived: boolean
  trajectory?: RankingTrajectoryPoint[]
  player_grade: number | null
}

/** The settings a board was computed with, in the vocabulary overrides use. */
export interface DraftSettingsResponse {
  num_teams: number
  budget: number
  format: string
  /** Editable roster slots: QB, RB, WR, TE, FLEX, SFLEX, K, DEF. */
  slots: Record<string, number>
  /** Points per stat category, keyed by canonical stat name (e.g. "pass_yds"). */
  scoring: Record<string, number>
  overridden: boolean
}

export interface DraftValuesResponse {
  season: number
  budget_per_team: number
  num_teams: number
  scoring_format: string
  settings: DraftSettingsResponse
  replacement_levels: DraftReplacementLevel[]
  players: DraftPlayer[]
}

export interface DraftValuesParams {
  season?: number
  /** 'league' keeps the league's own reception scoring; the rest force a format. */
  format?: string
  budget?: number
  teams?: number
  /** Roster override, e.g. `QB:1,RB:2,WR:3,TE:1,FLEX:1,SFLEX:1,K:1,DEF:1`. */
  slots?: string
  /** Per-stat point-value override, e.g. `pass_yds:0.04,pass_td:4,rec:1`. Wins over `format`. */
  scoring?: string
}

export const getDraftValues = (leagueId: number, params: DraftValuesParams) => {
  const qs = new URLSearchParams()
  if (params.season) qs.set('season', String(params.season))
  if (params.format && params.format !== 'league') qs.set('format', params.format)
  if (params.budget) qs.set('budget', String(params.budget))
  if (params.teams) qs.set('teams', String(params.teams))
  if (params.slots) qs.set('slots', params.slots)
  if (params.scoring) qs.set('scoring', params.scoring)
  const q = qs.toString()
  return request<DraftValuesResponse>(`/leagues/${leagueId}/draft-values${q ? `?${q}` : ''}`)
}

// --- Draft Prep (personal board: tags + custom ranking) ---

/** +1 target, -1 avoid. null = no opinion (0 is not a value). */
export type InterestLevel = -1 | 1

export interface DraftPrepEntry {
  gsis_id: string
  interest: InterestLevel | null
  custom_rank: number | null
  /** Overrides the algorithm's tier for this player. null = use the computed tier. */
  custom_tier: number | null
  note: string
  /** What you plan to pay. null = not in the team plan ($0 is a real bid). */
  planned_cost: number | null
}

export interface DraftPrepResponse {
  season: number
  players: DraftPrepEntry[]
}

export const getDraftPrep = (leagueId: number, season: number) =>
  request<DraftPrepResponse>(`/leagues/${leagueId}/draft-prep?season=${season}`)

export const setDraftPrepPlayer = (
  leagueId: number,
  season: number,
  gsisId: string,
  body: { interest: InterestLevel | null; custom_rank: number | null; custom_tier: number | null; note: string; planned_cost: number | null },
) =>
  request<DraftPrepEntry>(`/leagues/${leagueId}/draft-prep/${gsisId}?season=${season}`, {
    method: 'PUT',
    body: JSON.stringify(body),
  })

/** Replaces the whole board order — position in the array becomes the rank. */
export const reorderDraftPrep = (leagueId: number, season: number, gsisIds: string[]) =>
  request<{ ranked: number }>(`/leagues/${leagueId}/draft-prep/order?season=${season}`, {
    method: 'PUT',
    body: JSON.stringify({ gsis_ids: gsisIds }),
  })

// --- NFL Player Detail ---

export interface NFLPlayerMeta {
  gsis_id: string
  name: string
  position: string
  position_group: string
  team: string
  headshot_url: string
  birth_date: string
  height: number
  weight: number
  college: string
  years_exp: number
  entry_year: number
  rookie_year: number
  draft_club: string
  draft_number: number
  jersey_number: number
  yahoo_id: string
}

export interface NFLSeasonStats {
  season: number
  age: number
  team: string
  games: number
  completions: number
  pass_attempts: number
  pass_yards: number
  pass_tds: number
  interceptions: number
  sacks: number
  carries: number
  rush_yards: number
  rush_tds: number
  fumbles: number
  receptions: number
  targets: number
  rec_yards: number
  rec_tds: number
  fg_made: number
  fg_att: number
  fg_long: number
  pat_made: number
  fpts_ppr: number
  fpts: number
  fpts_ppr_pg: number
  fpts_pg: number
  tags: string[]
}

export interface NFLPlayerGradeSeason {
  season: number
  overall: number
  production: number
  efficiency: number
  usage: number
  durability: number
  career_phase: string
  yoy_trend: number | null
}

export interface NFLPlayerDetailResponse {
  player: NFLPlayerMeta
  seasons: NFLSeasonStats[]
  projection: ProjDetailResponse | null
  grades: NFLPlayerGradeSeason[]
  /** External context the stats-based projection can't see. See SituationalNote. */
  notes: SituationalNote[]
}

export const getNFLPlayer = (gsisId: string) =>
  request<NFLPlayerDetailResponse>(`/nfl/players/${encodeURIComponent(gsisId)}`)

// --- Player Grades (real-life value) ---

export interface GradePlayerItem {
  gsis_id: string
  name: string
  position: string
  position_group: string
  team: string
  headshot_url: string
  age: number
  overall: number
  production: number
  efficiency: number
  usage: number
  durability: number
  career_phase: string
  yoy_trend: number | null
  overall_rank: number
  position_rank: number
}

export interface GradeListResponse {
  season: number
  players: GradePlayerItem[]
  total: number
}

export interface GradePlayerDetailResponse {
  gsis_id: string
  name: string
  position: string
  position_group: string
  team: string
  headshot_url: string
  seasons: NFLPlayerGradeSeason[]
}

export const getGrades = (params: {
  season?: number
  position?: string
  limit?: number
  offset?: number
}) => {
  const qs = new URLSearchParams()
  if (params.season) qs.set('season', String(params.season))
  if (params.position) qs.set('position', params.position)
  if (params.limit) qs.set('limit', String(params.limit))
  if (params.offset) qs.set('offset', String(params.offset))
  const q = qs.toString()
  return request<GradeListResponse>(`/grades${q ? `?${q}` : ''}`)
}

export const getPlayerGrades = (gsisId: string) =>
  request<GradePlayerDetailResponse>(`/grades/${encodeURIComponent(gsisId)}`)

// --- Consensus divergences ---
// Compares our comp-based projection rank against the median rank/ADP from
// external expert sources, within position group. A divergence is a prompt to
// look, not a verdict — nothing here feeds back into the projections.

export interface SituationalNote {
  category: string
  summary: string
  impact_direction: 'positive' | 'negative' | 'neutral'
  impact_magnitude: 'major' | 'moderate' | 'minor'
  confidence: string
  source: string
  reported_date: string
  /** 'player' = about this player; 'team' = team-wide context affecting them. */
  scope: 'player' | 'team'
}

export interface DivergenceItem {
  gsis_id: string
  name: string
  position: string
  position_group: string
  team: string
  headshot_url: string
  our_rank: number
  consensus_rank_median: number
  source_count: number
  /** our_rank - consensus_rank_median. Positive = we rank them lower than consensus. */
  rank_delta: number
  proj_fpts_ppr: number
  notes: SituationalNote[]
}

export interface DivergenceListResponse {
  season: number
  format: string
  players: DivergenceItem[]
  total: number
}

/**
 * The divergence API uses the DB's format vocabulary ('half_ppr'), while the
 * projections/rankings UI uses 'half'. Map at this boundary so neither side
 * has to change.
 */
export const toDivergenceFormat = (f: RankingsFormat): string =>
  f === 'half' ? 'half_ppr' : f

export const getDivergences = (params: {
  season?: number
  format?: RankingsFormat
  position?: string
  limit?: number
  offset?: number
}) => {
  const qs = new URLSearchParams()
  if (params.season) qs.set('season', String(params.season))
  if (params.format) qs.set('format', toDivergenceFormat(params.format))
  if (params.position) qs.set('position', params.position)
  if (params.limit) qs.set('limit', String(params.limit))
  if (params.offset) qs.set('offset', String(params.offset))
  const q = qs.toString()
  return request<DivergenceListResponse>(`/divergences${q ? `?${q}` : ''}`)
}
