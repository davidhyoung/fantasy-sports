package yahoo

import "encoding/xml"

// FantasyContent is the root element wrapping every Yahoo Fantasy API response.
// Yahoo always returns XML with this outer envelope.
type FantasyContent struct {
	XMLName xml.Name      `xml:"fantasy_content"`
	Users   *Users        `xml:"users"`
	League  *League       `xml:"league"`
	Team    *Team         `xml:"team"`
	Players *LeaguePlayers `xml:"players"` // populated by /players;player_keys=... batch lookups
}

// Users wraps the list of users (typically just the authenticated user).
type Users struct {
	User []User `xml:"user"`
}

// User represents the authenticated Yahoo user and their fantasy games.
type User struct {
	GUID  string `xml:"guid"`
	Games Games  `xml:"games"`
}

// Games wraps the list of fantasy games (NFL, NBA, etc.) the user has leagues in.
type Games struct {
	Game []Game `xml:"game"`
}

// Game represents a Yahoo fantasy game (one sport, one season).
// Game.Code is the sport shorthand: "nfl", "nba", etc.
type Game struct {
	GameKey string  `xml:"game_key"`
	Name    string  `xml:"name"`
	Code    string  `xml:"code"`
	Season  string  `xml:"season"`
	Leagues Leagues `xml:"leagues"`
}

// Leagues wraps the list of leagues within a game.
type Leagues struct {
	League []League `xml:"league"`
}

// League holds metadata about a single fantasy league.
type League struct {
	LeagueKey   string      `xml:"league_key"`
	LeagueID    string      `xml:"league_id"`
	Name        string      `xml:"name"`
	URL         string      `xml:"url"`
	LogoURL     string      `xml:"logo_url"`
	NumTeams    int         `xml:"num_teams"`
	ScoringType string      `xml:"scoring_type"`
	LeagueType  string      `xml:"league_type"`
	Season      string      `xml:"season"`
	CurrentWeek int         `xml:"current_week"`
	GameCode    string      `xml:"game_code"`
	Teams       *Teams          `xml:"teams"`
	Scoreboard  *Scoreboard     `xml:"scoreboard"`
	Players     *LeaguePlayers  `xml:"players"`
	Standings   *Standings      `xml:"standings"`
	Settings     *LeagueSettings `xml:"settings"`
	DraftResults *DraftResults   `xml:"draft_results"`
}

// --- Scoreboard types ---

// Scoreboard holds the weekly matchup data for a league.
type Scoreboard struct {
	Week     int     `xml:"week"`
	Matchups Matchups `xml:"matchups"`
}

// Matchups wraps the list of head-to-head matchups for a given week.
type Matchups struct {
	Matchup []Matchup `xml:"matchup"`
}

// Matchup is a single head-to-head game between two teams.
type Matchup struct {
	Week          int          `xml:"week"`
	WeekStart     string       `xml:"week_start"`
	WeekEnd       string       `xml:"week_end"`
	Status        string       `xml:"status"`
	IsPlayoffs    string       `xml:"is_playoffs"`
	IsConsolation string       `xml:"is_consolation"`
	MatchupTeams  MatchupTeams `xml:"teams"`
}

// MatchupTeams wraps the two teams in a matchup.
type MatchupTeams struct {
	Team []MatchupTeam `xml:"team"`
}

// MatchupTeam is a team's entry in a scoreboard matchup, including their score.
type MatchupTeam struct {
	TeamKey             string     `xml:"team_key"`
	TeamID              string     `xml:"team_id"`
	Name                string     `xml:"name"`
	TeamLogos           TeamLogos  `xml:"team_logos"`
	TeamPoints          TeamPoints `xml:"team_points"`
	TeamProjectedPoints TeamPoints `xml:"team_projected_points"`
}

// LogoURL returns the URL of the first team logo, or "" if none.
func (t *MatchupTeam) LogoURL() string {
	if len(t.TeamLogos.TeamLogo) > 0 {
		return t.TeamLogos.TeamLogo[0].URL
	}
	return ""
}

// TeamPoints holds the actual or projected score for a team in a given week.
type TeamPoints struct {
	CoverageType string `xml:"coverage_type"`
	Week         int    `xml:"week"`
	Total        string `xml:"total"`
}

// --- Standings types ---

// Standings holds the ranked list of teams in a league.
type Standings struct {
	Teams StandingsTeams `xml:"teams"`
}

// StandingsTeams wraps the list of teams with their standing data.
type StandingsTeams struct {
	Team []StandingTeam `xml:"team"`
}

// StandingTeam is a team entry in the standings, including win/loss record.
type StandingTeam struct {
	TeamKey       string        `xml:"team_key"`
	TeamID        string        `xml:"team_id"`
	Name          string        `xml:"name"`
	TeamLogos     TeamLogos     `xml:"team_logos"`
	TeamStandings TeamStandings `xml:"team_standings"`
}

// LogoURL returns the URL of the first team logo, or "" if none.
func (t *StandingTeam) LogoURL() string {
	if len(t.TeamLogos.TeamLogo) > 0 {
		return t.TeamLogos.TeamLogo[0].URL
	}
	return ""
}

// TeamStandings holds the record and points totals for a team.
type TeamStandings struct {
	Rank          int           `xml:"rank"`
	PlayoffSeed   int           `xml:"playoff_seed"`
	OutcomeTotals OutcomeTotals `xml:"outcome_totals"`
	Streak        Streak        `xml:"streak"`
	PointsFor     string        `xml:"points_for"`
	PointsAgainst string        `xml:"points_against"`
}

// OutcomeTotals holds the wins, losses, and ties for a team.
type OutcomeTotals struct {
	Wins       int    `xml:"wins"`
	Losses     int    `xml:"losses"`
	Ties       int    `xml:"ties"`
	Percentage string `xml:"percentage"`
}

// Streak holds the current win or loss streak for a team.
type Streak struct {
	Type  string `xml:"type"`
	Value int    `xml:"value"`
}

// Teams wraps the list of teams in a league.
type Teams struct {
	Team []Team `xml:"team"`
}

// TeamLogos wraps the list of logo sizes for a team.
type TeamLogos struct {
	TeamLogo []TeamLogo `xml:"team_logo"`
}

// TeamLogo is one size variant of a team's logo image.
type TeamLogo struct {
	Size string `xml:"size"`
	URL  string `xml:"url"`
}

// Team represents a single fantasy team inside a league.
type Team struct {
	TeamKey              string    `xml:"team_key"`
	TeamID               string   `xml:"team_id"`
	Name                 string   `xml:"name"`
	IsOwnedByCurrentUser string   `xml:"is_owned_by_current_login"`
	TeamLogos            TeamLogos `xml:"team_logos"`
	Managers             Managers `xml:"managers"`
	Roster               *Roster  `xml:"roster"`
}

// LogoURL returns the URL of the first team logo, or "" if none.
func (t *Team) LogoURL() string {
	if len(t.TeamLogos.TeamLogo) > 0 {
		return t.TeamLogos.TeamLogo[0].URL
	}
	return ""
}

// Managers wraps the list of managers for a team (usually just one).
type Managers struct {
	Manager []Manager `xml:"manager"`
}

// Manager represents one manager (owner) of a team.
type Manager struct {
	GUID           string `xml:"guid"`
	Nickname       string `xml:"nickname"`
	IsCommissioner string `xml:"is_commissioner"`
}

// Roster holds the current roster for a team.
type Roster struct {
	Players RosterPlayers `xml:"players"`
}

// RosterPlayers wraps the list of players on a roster.
type RosterPlayers struct {
	Player []RosterPlayer `xml:"player"`
}

// PlayerStat is a single raw stat entry returned by Yahoo when ;out=stats is used.
type PlayerStat struct {
	StatID string `xml:"stat_id"`
	Value  string `xml:"value"`
}

// PlayerStats is the stats container within a player element when ;out=stats is requested.
type PlayerStats struct {
	CoverageType string       `xml:"coverage_type"`
	Week         int          `xml:"week"`
	Stats        []PlayerStat `xml:"stats>stat"`
}

// Headshot holds the URL and size of a player's headshot image.
type Headshot struct {
	URL  string `xml:"url"`
	Size string `xml:"size"`
}

// RosterPlayer is a single player on a team's roster.
type RosterPlayer struct {
	PlayerKey         string           `xml:"player_key"          json:"player_key"`
	PlayerID          string           `xml:"player_id"           json:"player_id"`
	Name              PlayerName       `xml:"name"                json:"name"`
	EditorialTeamAbbr string           `xml:"editorial_team_abbr" json:"team_abbr"`
	DisplayPosition   string           `xml:"display_position"    json:"display_position"`
	SelectedPosition  SelectedPosition `xml:"selected_position"   json:"selected_position"`
	Headshot          Headshot         `xml:"headshot"            json:"-"`
	ImageURL          string           `xml:"image_url"           json:"-"`
	// PlayerStats is populated only when fetched with ;out=stats. Never sent raw
	// to the frontend — the handler maps stat IDs to labels before responding.
	PlayerStats *PlayerStats `xml:"player_stats" json:"-"`
}

// HeadshotURL returns the player's headshot URL, preferring the headshot element
// over the simpler image_url field.
func (p *RosterPlayer) HeadshotURL() string {
	if p.Headshot.URL != "" {
		return p.Headshot.URL
	}
	return p.ImageURL
}

// PlayerName holds the full and split name for a player.
type PlayerName struct {
	Full  string `xml:"full"  json:"full"`
	First string `xml:"first" json:"first"`
	Last  string `xml:"last"  json:"last"`
}

// SelectedPosition is the slot a player is currently placed in.
type SelectedPosition struct {
	Position string `xml:"position" json:"position"`
}

// --- League settings types ---

// LeagueStat describes one stat category in a league's scoring settings.
// IsOnlyDisplayStat="1" means the stat is shown but not used to determine wins.
// Modifier is the point value per unit of this stat (points leagues only; 0 for category leagues).
type LeagueStat struct {
	StatID            string  `xml:"stat_id"`
	Name              string  `xml:"name"`
	DisplayName       string  `xml:"display_name"`
	SortOrder         string  `xml:"sort_order"` // "1" = higher is better, "0" = lower is better (e.g. TO)
	IsOnlyDisplayStat string  `xml:"is_only_display_stat"`
	Modifier          float64 // populated from stat_modifiers; 0 if not a points league
}

// StatCategories holds the full list of stat categories for a league.
type StatCategories struct {
	Stats []LeagueStat `xml:"stats>stat"`
}

// StatModifier holds the point value assigned to a single stat in a points league.
type StatModifier struct {
	StatID string  `xml:"stat_id"`
	Value  float64 `xml:"value"`
}

// StatModifiers holds the list of stat point modifiers from /league/{key}/settings.
type StatModifiers struct {
	Stats []StatModifier `xml:"stats>stat"`
}

// RosterPosition describes one roster slot type from /league/{key}/settings.
// FLEX positions use "/" syntax: "W/R/T" = WR/RB/TE eligible, "Q/W/R/T" = superflex.
type RosterPosition struct {
	Position     string `xml:"position"`
	PositionType string `xml:"position_type"` // "O" offense, "K" kicker, "DT" defense, "BN" bench, "IR" injured
	Count        int    `xml:"count"`
}

// RosterPositions wraps the list of roster positions from league settings.
type RosterPositions struct {
	RosterPosition []RosterPosition `xml:"roster_position"`
}

// LeagueSettings holds the settings block returned by /league/{key}/settings.
type LeagueSettings struct {
	StatCategories  StatCategories  `xml:"stat_categories"`
	StatModifiers   StatModifiers   `xml:"stat_modifiers"`
	RosterPositions RosterPositions `xml:"roster_positions"`
}

// --- Draft results types ---

// DraftResults wraps the list of draft picks for a league.
type DraftResults struct {
	DraftPick []DraftPick `xml:"draft_result"`
}

// DraftPick is a single pick from the league draft.
// For auction drafts, Cost holds the dollar amount paid.
type DraftPick struct {
	PlayerKey string `xml:"player_key"`
	PlayerID  string `xml:"player_id"`
	Round     string `xml:"round"`
	Pick      string `xml:"pick"`
	Cost      string `xml:"cost"`
	TeamKey   string `xml:"team_key"`
}

// --- League player browse / search types ---

// LeaguePlayers wraps the paginated list of players returned in a league context.
type LeaguePlayers struct {
	Player []LeaguePlayer `xml:"player"`
}

// LeaguePlayer is a player as returned from league-level search or browse endpoints.
// It includes ownership data (percent owned/started in this league).
// PlayerStats is populated only when fetched with a /stats;type=... subresource.
type LeaguePlayer struct {
	PlayerKey         string          `xml:"player_key"`
	PlayerID          string          `xml:"player_id"`
	Name              PlayerName      `xml:"name"`
	EditorialTeamAbbr string          `xml:"editorial_team_abbr"`
	DisplayPosition   string          `xml:"display_position"`
	Status            string          `xml:"status"`
	Headshot          Headshot        `xml:"headshot"`
	Ownership         PlayerOwnership `xml:"ownership"`
	PlayerStats       *PlayerStats    `xml:"player_stats"`
}

// HeadshotURL returns the player's headshot URL, or "" if none.
func (p *LeaguePlayer) HeadshotURL() string {
	return p.Headshot.URL
}

// PlayerOwnership holds the ownership data for a player within a specific league.
type PlayerOwnership struct {
	OwnershipType  string `xml:"ownership_type"`
	OwnedPercent   string `xml:"owned_percent"`
	StartedPercent string `xml:"started_percent"`
}
