package yahoo

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"

	"github.com/jackc/pgx/v5/pgxpool"
)

const baseURL = "https://fantasysports.yahooapis.com/fantasy/v2"

// Client is an authenticated Yahoo Fantasy Sports API client for a single user.
// It handles token refresh automatically and saves new tokens to the database.
type Client struct {
	httpClient *http.Client
}

// dbTokenSource is a custom oauth2.TokenSource that persists refreshed tokens
// back to the database so they survive server restarts.
//
// In Go, an interface is satisfied implicitly — any type with the right method
// signatures "is" that interface. oauth2.TokenSource requires one method: Token().
type dbTokenSource struct {
	mu     sync.Mutex         // prevents concurrent token refreshes
	inner  oauth2.TokenSource // the real source that handles expiry + refresh
	db     *pgxpool.Pool
	userID int64
	cached *oauth2.Token
}

// Token returns a valid token, refreshing it if necessary and saving it to the DB.
func (ts *dbTokenSource) Token() (*oauth2.Token, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	tok, err := ts.inner.Token()
	if err != nil {
		return nil, err
	}

	// If the access token changed, the library refreshed it — save to DB.
	if ts.cached == nil || tok.AccessToken != ts.cached.AccessToken {
		ts.cached = tok
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := ts.db.Exec(ctx,
			`UPDATE users
			 SET access_token  = $1,
			     refresh_token = $2,
			     token_expiry  = $3
			 WHERE id = $4`,
			tok.AccessToken, tok.RefreshToken, tok.Expiry, ts.userID,
		)
		if err != nil {
			// Log but don't fail the request — the token is still valid in memory.
			log.Printf("[yahoo/client] failed to persist refreshed token for user %d: %v", ts.userID, err)
		}
	}
	return tok, nil
}

// NewClient creates a Client for a specific user using their stored tokens.
// If the access token is expired, the oauth2 library will automatically use
// the refresh token to get a new one when the first request is made.
func NewClient(
	ctx context.Context,
	db *pgxpool.Pool,
	cfg *oauth2.Config,
	userID int64,
	accessToken, refreshToken string,
	expiry time.Time,
) *Client {
	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       expiry,
	}

	ts := &dbTokenSource{
		inner:  cfg.TokenSource(ctx, token),
		db:     db,
		userID: userID,
	}

	return &Client{
		// oauth2.NewClient wraps our token source so every request automatically
		// gets an Authorization: Bearer <token> header.
		httpClient: oauth2.NewClient(ctx, ts),
	}
}

// get performs an authenticated GET to the Yahoo Fantasy API and returns the body.
func (c *Client) get(url string) ([]byte, error) {
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("yahoo API returned %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}

// decode parses XML from body into dst, logging the raw body on failure so we
// can inspect exactly what Yahoo sent.
func decode(body []byte, dst any) error {
	if err := xml.Unmarshal(body, dst); err != nil {
		log.Printf("[yahoo/client] XML parse error: %v\nRaw body:\n%s", err, string(body))
		return err
	}
	return nil
}

// GetUserLeagues returns all leagues the logged-in user belongs to for the
// given sport codes (e.g. "nfl", "nba").
func (c *Client) GetUserLeagues(ctx context.Context, gameCodes ...string) ([]Game, error) {
	codes := strings.Join(gameCodes, ",")
	url := fmt.Sprintf(
		"%s/users;use_login=1/games;game_keys=%s/leagues",
		baseURL, codes,
	)

	body, err := c.get(url)
	if err != nil {
		return nil, err
	}

	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}

	if fc.Users == nil || len(fc.Users.User) == 0 {
		return nil, nil
	}

	return fc.Users.User[0].Games.Game, nil
}

// GetRoster returns the current roster for the given team key (e.g. "449.l.12345.t.1").
func (c *Client) GetRoster(ctx context.Context, teamKey string) ([]RosterPlayer, error) {
	url := fmt.Sprintf("%s/team/%s/roster/players", baseURL, teamKey)

	body, err := c.get(url)
	if err != nil {
		return nil, err
	}

	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}

	if fc.Team == nil || fc.Team.Roster == nil {
		return nil, nil
	}

	return fc.Team.Roster.Players.Player, nil
}

// GetRosterWithStats returns the current roster for teamKey enriched with stats for
// the given statType. statType is passed directly as Yahoo's semicolon-path type value,
// e.g. "week", "week;week=20", "lastweek", "season", "date;date=2026-03-14".
// Do not URL-encode the value — Yahoo expects raw semicolon path syntax.
func (c *Client) GetRosterWithStats(ctx context.Context, teamKey, statType string) ([]RosterPlayer, error) {
	if statType == "" {
		statType = "week"
	}
	// Yahoo requires different URL forms depending on the stat period:
	// - season / lastweek / specific week (week;week=N): use the /stats subresource,
	//   which correctly returns accumulated totals for the requested period.
	// - week (current week) / date (today): use the ;out=stats inline form,
	//   which returns live in-progress data for the current period.
	var apiURL string
	if statType == "week" || strings.HasPrefix(statType, "date;") {
		apiURL = fmt.Sprintf("%s/team/%s/roster/players;out=stats;type=%s", baseURL, teamKey, statType)
	} else {
		apiURL = fmt.Sprintf("%s/team/%s/roster/players/stats;type=%s", baseURL, teamKey, statType)
	}

	body, err := c.get(apiURL)
	if err != nil {
		return nil, err
	}

	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}

	if fc.Team == nil || fc.Team.Roster == nil {
		return nil, nil
	}
	return fc.Team.Roster.Players.Player, nil
}

// GetScoreboard returns the weekly matchups for a league.
// Pass week=0 to get the current week (Yahoo's default).
func (c *Client) GetScoreboard(ctx context.Context, leagueKey string, week int) (*Scoreboard, error) {
	url := fmt.Sprintf("%s/league/%s/scoreboard", baseURL, leagueKey)
	if week > 0 {
		url = fmt.Sprintf("%s/league/%s/scoreboard;week=%d", baseURL, leagueKey, week)
	}

	body, err := c.get(url)
	if err != nil {
		return nil, err
	}

	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}

	if fc.League == nil {
		return nil, nil
	}
	return fc.League.Scoreboard, nil
}

// GetStandings returns the ranked team standings for a league.
func (c *Client) GetStandings(ctx context.Context, leagueKey string) ([]StandingTeam, error) {
	url := fmt.Sprintf("%s/league/%s/standings", baseURL, leagueKey)

	body, err := c.get(url)
	if err != nil {
		return nil, err
	}

	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}

	if fc.League == nil || fc.League.Standings == nil {
		return nil, nil
	}
	return fc.League.Standings.Teams.Team, nil
}

// GetLeagueTeams returns all teams in the given league (e.g. "449.l.12345").
func (c *Client) GetLeagueTeams(ctx context.Context, leagueKey string) ([]Team, error) {
	url := fmt.Sprintf("%s/league/%s/teams", baseURL, leagueKey)

	body, err := c.get(url)
	if err != nil {
		return nil, err
	}

	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}

	if fc.League == nil || fc.League.Teams == nil {
		return nil, nil
	}

	return fc.League.Teams.Team, nil
}

// GetLeagueScoringStats fetches the stat categories for a league and returns
// a stat_id → display_name map containing only scoring stats (i.e. stats where
// is_only_display_stat != "1"). This is used to filter and label roster stats
// so we show exactly the categories that affect the score — for any sport.
// GetLeagueScoringStats fetches the stat categories for a league and returns
// a stat_id → LeagueStat map containing only scoring stats (is_only_display_stat != "1").
// The full LeagueStat is returned so callers can access DisplayName, SortOrder, etc.
func (c *Client) GetLeagueScoringStats(ctx context.Context, leagueKey string) (map[string]LeagueStat, error) {
	apiURL := fmt.Sprintf("%s/league/%s/settings", baseURL, leagueKey)
	body, err := c.get(apiURL)
	if err != nil {
		return nil, err
	}

	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}

	if fc.League == nil || fc.League.Settings == nil {
		return nil, nil
	}

	// Build modifier lookup (present in points leagues; empty for category leagues).
	modifiers := make(map[string]float64)
	for _, m := range fc.League.Settings.StatModifiers.Stats {
		modifiers[m.StatID] = m.Value
	}

	result := make(map[string]LeagueStat)
	for _, s := range fc.League.Settings.StatCategories.Stats {
		if s.IsOnlyDisplayStat == "1" {
			continue // shown in Yahoo UI but not used to determine match wins
		}
		s.Modifier = modifiers[s.StatID]
		result[s.StatID] = s
	}
	return result, nil
}

// GetLeagueRosterPositions fetches the roster slot configuration for a league
// (e.g. 1 QB, 2 RB, 2 WR, 1 TE, 1 W/R/T FLEX, 1 K, 1 DEF, 6 BN).
func (c *Client) GetLeagueRosterPositions(ctx context.Context, leagueKey string) ([]RosterPosition, error) {
	apiURL := fmt.Sprintf("%s/league/%s/settings", baseURL, leagueKey)
	body, err := c.get(apiURL)
	if err != nil {
		return nil, err
	}

	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}

	if fc.League == nil || fc.League.Settings == nil {
		return nil, nil
	}

	return fc.League.Settings.RosterPositions.RosterPosition, nil
}

// GetLeagueDraftResults returns all draft picks for a league.
// For auction leagues, each pick includes a Cost field with the dollar amount paid.
func (c *Client) GetLeagueDraftResults(ctx context.Context, leagueKey string) ([]DraftPick, error) {
	apiURL := fmt.Sprintf("%s/league/%s/draftresults", baseURL, leagueKey)

	body, err := c.get(apiURL)
	if err != nil {
		return nil, err
	}

	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}

	if fc.League == nil || fc.League.DraftResults == nil {
		return nil, nil
	}
	return fc.League.DraftResults.DraftPick, nil
}

// GetLeagueKeepers returns players in the league with keeper status (status=K).
func (c *Client) GetLeagueKeepers(ctx context.Context, leagueKey string) ([]LeaguePlayer, error) {
	apiURL := fmt.Sprintf("%s/league/%s/players;status=K", baseURL, leagueKey)
	return c.fetchLeaguePlayers(apiURL)
}

// GetLeagueRosters fetches all teams' current rosters for a league in a single
// API call. Returns the raw Yahoo Team slice with each team's Roster populated.
// Player names and positions come from the roster data, so no separate batch
// player-key lookup is needed when using this as the data source.
func (c *Client) GetLeagueRosters(ctx context.Context, leagueKey string) ([]Team, error) {
	apiURL := fmt.Sprintf("%s/league/%s/teams/roster", baseURL, leagueKey)
	body, err := c.get(apiURL)
	if err != nil {
		return nil, err
	}
	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}
	if fc.League == nil || fc.League.Teams == nil {
		return nil, nil
	}
	return fc.League.Teams.Team, nil
}

// GetLeagueRostersWithStats fetches all teams' current rosters with per-player stats.
// statType is passed as Yahoo's semicolon-path type value (e.g. "season", "lastweek").
// Falls back to "season" if empty.
//
// Implementation: fetches the team list first, then concurrently calls GetRosterWithStats
// per team. This ensures stats are populated correctly (Yahoo's league-level roster
// endpoint does not reliably return inline player_stats elements).
func (c *Client) GetLeagueRostersWithStats(ctx context.Context, leagueKey, statType string) ([]Team, error) {
	if statType == "" {
		statType = "season"
	}

	// Step 1: get all teams (keys, names; no rosters yet).
	teams, err := c.GetLeagueTeams(ctx, leagueKey)
	if err != nil {
		return nil, err
	}
	if len(teams) == 0 {
		return nil, nil
	}

	// Step 2: concurrently fetch each team's roster+stats.
	type result struct {
		idx     int
		players []RosterPlayer
		err     error
	}
	ch := make(chan result, len(teams))
	for i, t := range teams {
		go func(idx int, teamKey string) {
			players, err := c.GetRosterWithStats(ctx, teamKey, statType)
			ch <- result{idx, players, err}
		}(i, t.TeamKey)
	}

	for range teams {
		r := <-ch
		if r.err != nil {
			return nil, fmt.Errorf("roster fetch for team index %d: %w", r.idx, r.err)
		}
		teams[r.idx].Roster = &Roster{
			Players: RosterPlayers{Player: r.players},
		}
	}

	return teams, nil
}

// SearchPlayers searches for players in a league by name and returns them with
// ownership data (percent owned/started). Returns up to 25 results.
func (c *Client) SearchPlayers(ctx context.Context, leagueKey, query string) ([]LeaguePlayer, error) {
	apiURL := fmt.Sprintf(
		"%s/league/%s/players;search=%s/ownership",
		baseURL, leagueKey, url.PathEscape(query),
	)
	return c.fetchLeaguePlayers(apiURL)
}

// GetAvailablePlayers returns players in a league filtered by status and position.
//
// statusFilter controls ownership:
//   - "A"  → available (free agents + waivers) — default
//   - "FA" → free agents only
//   - "W"  → waivers only
//   - ""   → all players (rostered + available)
//
// position filters by position (e.g. "QB", "RB") — pass "" for all positions.
// start is the pagination offset; results come in pages of 25.
func (c *Client) GetAvailablePlayers(ctx context.Context, leagueKey, statusFilter, position string, start int) ([]LeaguePlayer, error) {
	filter := fmt.Sprintf("count=25;start=%d", start)
	if statusFilter != "" {
		filter = fmt.Sprintf("status=%s;%s", statusFilter, filter)
	}
	if position != "" {
		filter += ";position=" + position
	}
	apiURL := fmt.Sprintf("%s/league/%s/players;%s/ownership", baseURL, leagueKey, filter)
	return c.fetchLeaguePlayers(apiURL)
}

// GetPlayersByKeys batch-fetches player info (name, position) for the given player keys.
// Yahoo's draftresults API omits player names, so this is used to enrich draft picks.
// Requests are fanned out concurrently in chunks of 25 to stay within API limits.
func (c *Client) GetPlayersByKeys(ctx context.Context, playerKeys []string) ([]LeaguePlayer, error) {
	if len(playerKeys) == 0 {
		return nil, nil
	}

	const chunkSize = 25

	// Split into chunks.
	var chunks [][]string
	for i := 0; i < len(playerKeys); i += chunkSize {
		end := i + chunkSize
		if end > len(playerKeys) {
			end = len(playerKeys)
		}
		chunks = append(chunks, playerKeys[i:end])
	}

	// Fetch all chunks concurrently.
	type chunkResult struct {
		players []LeaguePlayer
		err     error
	}
	results := make([]chunkResult, len(chunks))

	var wg sync.WaitGroup
	for i, chunk := range chunks {
		wg.Add(1)
		go func(idx int, keys []string) {
			defer wg.Done()
			apiURL := fmt.Sprintf("%s/players;player_keys=%s", baseURL, strings.Join(keys, ","))
			body, err := c.get(apiURL)
			if err != nil {
				results[idx].err = err
				return
			}
			var fc FantasyContent
			if err := decode(body, &fc); err != nil {
				results[idx].err = err
				return
			}
			if fc.Players != nil {
				results[idx].players = fc.Players.Player
			}
		}(i, chunk)
	}
	wg.Wait()

	var all []LeaguePlayer
	for _, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		all = append(all, r.players...)
	}
	return all, nil
}

// GetAvailablePlayersWithStats returns available (free agent + waiver) players with
// their season stats for a league. statType is passed as Yahoo's semicolon-path value
// (e.g. "season", "lastweek"). count controls how many players to fetch (max 25 per page).
// PlayerStats is populated on each returned LeaguePlayer.
func (c *Client) GetAvailablePlayersWithStats(ctx context.Context, leagueKey, statType string, count int) ([]LeaguePlayer, error) {
	if statType == "" {
		statType = "season"
	}
	if count <= 0 {
		count = 25
	}
	apiURL := fmt.Sprintf(
		"%s/league/%s/players;status=A;count=%d/stats;type=%s",
		baseURL, leagueKey, count, statType,
	)
	return c.fetchLeaguePlayers(apiURL)
}

// GetAvailablePlayersLean returns available (FA + waiver) players without stats.
// Use this when stats will be sourced locally (nfl_player_stats) — avoids the
// expensive Yahoo stats subresource.
func (c *Client) GetAvailablePlayersLean(ctx context.Context, leagueKey string, count int) ([]LeaguePlayer, error) {
	if count <= 0 {
		count = 25
	}
	apiURL := fmt.Sprintf(
		"%s/league/%s/players;status=A;count=%d/ownership",
		baseURL, leagueKey, count,
	)
	return c.fetchLeaguePlayers(apiURL)
}

// fetchLeaguePlayers is a shared helper that calls an already-built player list URL.
func (c *Client) fetchLeaguePlayers(apiURL string) ([]LeaguePlayer, error) {
	body, err := c.get(apiURL)
	if err != nil {
		return nil, err
	}

	var fc FantasyContent
	if err := decode(body, &fc); err != nil {
		return nil, err
	}

	if fc.League == nil || fc.League.Players == nil {
		return nil, nil
	}
	return fc.League.Players.Player, nil
}
