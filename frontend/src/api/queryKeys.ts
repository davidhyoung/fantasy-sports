export const keys = {
  me: ['me'] as const,
  leagues: ['leagues'] as const,
  league: (id: number) => ['league', id] as const,
  leagueTeams: (id: number) => ['league', id, 'teams'] as const,
  scoreboard: (id: number, week?: number) =>
    week != null ? ['league', id, 'scoreboard', week] as const : ['league', id, 'scoreboard'] as const,
  standings: (id: number) => ['league', id, 'standings'] as const,
  availablePlayers: (id: number, pos: string, start: number, status: string) =>
    ['league', id, 'players', 'available', pos, start, status] as const,
  searchPlayers: (id: number, q: string) =>
    ['league', id, 'players', 'search', q] as const,
  team: (id: number) => ['team', id] as const,
  teamRoster: (id: number, statType?: string) =>
    statType ? ['team', id, 'roster', statType] as const : ['team', id, 'roster'] as const,
  leagueKeepers: (id: number) => ['league', id, 'keepers'] as const,
  leagueDraft: (id: number) => ['league', id, 'draft'] as const,
  keeperRules: (id: number) => ['league', id, 'keeper-rules'] as const,
  teamKeeperWishlist: (id: number) => ['team', id, 'keepers'] as const,
  leagueRankings: (id: number, statType: string) =>
    ['league', id, 'rankings', statType] as const,
  keeperSummary: (id: number) => ['league', id, 'keeper-summary'] as const,
  teamKeeperSubmission: (id: number) => ['team', id, 'keepers', 'submit'] as const,
  projections: (season: number, position: string, sort: string) =>
    ['projections', season, position, sort] as const,
  publicRankings: (season: number, format: string, position: string) =>
    ['rankings', season, format, position] as const,
  projectionDetail: (gsisId: string, season: number) =>
    ['projection', gsisId, season] as const,
  draftValues: (leagueId: number, season: number, format: string, budget: number) =>
    ['league', leagueId, 'draft-values', season, format, budget] as const,
  nflPlayer: (gsisId: string) => ['nfl-player', gsisId] as const,
  grades: (season: number, position: string) =>
    ['grades', season, position] as const,
  playerGrades: (gsisId: string) => ['player-grades', gsisId] as const,
}
