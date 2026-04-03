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
  created_at: string
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
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
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
export const createLeague = (data: Omit<League, 'id' | 'created_at'>) =>
  request<League>('/leagues', { method: 'POST', body: JSON.stringify(data) })

// --- Teams ---

export const listLeagueTeams = (leagueId: number) =>
  request<Team[]>(`/leagues/${leagueId}/teams`)
export const getTeam = (id: number) => request<Team>(`/teams/${id}`)
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
}

export const getLeagueScoreboard = (id: number, week?: number) =>
  request<Scoreboard>(`/leagues/${id}/scoreboard${week ? `?week=${week}` : ''}`)
export const getLeagueStandings = (id: number) =>
  request<Standing[]>(`/leagues/${id}/standings`)

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
  confidence: number
  comp_count: number
  uniqueness: 'common' | 'moderate' | 'rare' | 'unique'
  vor: number
  auction_value: number
  overall_rank: number
  position_rank: number
  trajectory?: RankingTrajectoryPoint[]
  player_grade: number | null
}

export interface DraftValuesResponse {
  season: number
  budget_per_team: number
  num_teams: number
  scoring_format: string
  replacement_levels: DraftReplacementLevel[]
  players: DraftPlayer[]
}

export const getDraftValues = (leagueId: number, params: {
  season?: number
  format?: string
  budget?: number
}) => {
  const qs = new URLSearchParams()
  if (params.season) qs.set('season', String(params.season))
  if (params.format) qs.set('format', params.format)
  if (params.budget) qs.set('budget', String(params.budget))
  const q = qs.toString()
  return request<DraftValuesResponse>(`/leagues/${leagueId}/draft-values${q ? `?${q}` : ''}`)
}

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
