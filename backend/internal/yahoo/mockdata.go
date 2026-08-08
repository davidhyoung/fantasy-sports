package yahoo

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
)

// mockdata.go builds a deterministic fake fantasy league used by mock.go when
// YAHOO_MOCK=1. Everything here is generated once at package init from a fixed
// seed, so the league is byte-identical across restarts — a roster or standings
// row that moved between two runs would mean a real bug, not fixture churn.
//
// The player pool is a committed static list rather than a read from
// nfl_players. That keeps package yahoo free of a database dependency it
// doesn't otherwise have, and means mock mode works against an empty database —
// which is much of the point of having a mock at all.

const (
	MockLeagueKey  = "mock.l.100001"
	MockGameKey    = "461"
	MockSeason     = "2026"
	MockUserGUID   = "MOCKUSER00000000000000000"
	mockNumTeams   = 12
	mockCurrentWk  = 1
	mockRosterSize = 15
)

// mockPlayer is one entry in the static pool.
type mockPlayer struct {
	name string
	pos  string // QB, RB, WR, TE, K, DEF
	team string // NFL team abbreviation
}

// mockTeam is one fantasy team in the generated league.
type mockTeam struct {
	key           string
	id            string
	name          string
	manager       string
	isMine        bool
	roster        []mockPlayer
	wins          int
	losses        int
	ties          int
	pointsFor     float64
	pointsAgainst float64
}

// Generated once at init; read-only thereafter.
var (
	mockTeams      []mockTeam
	mockFreeAgents []mockPlayer
	mockDraftPicks []DraftPick
)

// playerKey derives a stable Yahoo-style player key from the pool index so the
// same player always has the same key across requests.
func playerKey(idx int) string {
	return fmt.Sprintf("%s.p.%d", MockGameKey, 30000+idx)
}

// poolIndex maps a player back to their pool position, for key generation.
var poolIndex = map[string]int{}

func init() {
	for i, p := range mockPlayerPool {
		poolIndex[p.name] = i
	}
	buildMockLeague()
}

// buildMockLeague snake-drafts the player pool across 12 teams, then derives
// standings and draft results that are consistent with those rosters.
func buildMockLeague() {
	rng := rand.New(rand.NewSource(20260807)) // fixed seed → deterministic league

	teamNames := []string{
		"Gridiron Gurus", "Blitz Brigade", "End Zone Elite", "Pocket Passers",
		"Hail Mary Heroes", "Red Zone Raiders", "Fourth Down Fury", "Play Action Pros",
		"Sack Masters", "Two Minute Drill", "Gronk Spike", "Audible Alliance",
	}
	managers := []string{
		"Dave", "Sam", "Alex", "Jordan", "Casey", "Riley",
		"Morgan", "Taylor", "Jamie", "Quinn", "Avery", "Rowan",
	}

	mockTeams = make([]mockTeam, mockNumTeams)
	for i := 0; i < mockNumTeams; i++ {
		mockTeams[i] = mockTeam{
			key:     fmt.Sprintf("%s.t.%d", MockLeagueKey, i+1),
			id:      strconv.Itoa(i + 1),
			name:    teamNames[i],
			manager: managers[i],
			isMine:  i == 0, // team 1 belongs to the mock user
		}
	}

	// Snake draft: order reverses each round, so early picks don't compound.
	drafted := 0
	for round := 0; round < mockRosterSize; round++ {
		for slot := 0; slot < mockNumTeams; slot++ {
			teamIdx := slot
			if round%2 == 1 {
				teamIdx = mockNumTeams - 1 - slot
			}
			if drafted >= len(mockPlayerPool) {
				break
			}
			p := mockPlayerPool[drafted]
			mockTeams[teamIdx].roster = append(mockTeams[teamIdx].roster, p)

			mockDraftPicks = append(mockDraftPicks, DraftPick{
				PlayerKey: playerKey(poolIndex[p.name]),
				PlayerID:  strconv.Itoa(30000 + poolIndex[p.name]),
				Round:     strconv.Itoa(round + 1),
				Pick:      strconv.Itoa(drafted + 1),
				Cost:      "", // snake draft, not auction
				TeamKey:   mockTeams[teamIdx].key,
			})
			drafted++
		}
	}
	mockFreeAgents = append(mockFreeAgents, mockPlayerPool[drafted:]...)

	// Records: 12 teams, one week played, so exactly 6 winners and 6 losers.
	// Assigning them by index keeps this consistent with the scoreboard below,
	// which pairs team i against team i+6.
	for i := range mockTeams {
		t := &mockTeams[i]
		t.pointsFor = 90 + rng.Float64()*50
		t.pointsAgainst = 90 + rng.Float64()*50
		if t.pointsFor >= t.pointsAgainst {
			t.wins, t.losses = 1, 0
		} else {
			t.wins, t.losses = 0, 1
		}
	}
}

// standingsOrder returns team indexes sorted the way Yahoo ranks them:
// wins descending, then points-for descending.
func standingsOrder() []int {
	idx := make([]int, len(mockTeams))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		ta, tb := mockTeams[idx[a]], mockTeams[idx[b]]
		if ta.wins != tb.wins {
			return ta.wins > tb.wins
		}
		return ta.pointsFor > tb.pointsFor
	})
	return idx
}

// ── XML builders ─────────────────────────────────────────────────────────────
//
// Each returns a FantasyContent that mock.go marshals. Building Go structs and
// marshalling them (rather than templating XML strings) means the fixtures
// cannot drift from the types the parser expects.

func mockLeagueMeta() League {
	return League{
		LeagueKey:   MockLeagueKey,
		LeagueID:    "100001",
		Name:        "Mock Dev League",
		URL:         "https://example.invalid/mock-league",
		NumTeams:    mockNumTeams,
		ScoringType: "head",
		LeagueType:  "private",
		Season:      MockSeason,
		CurrentWeek: mockCurrentWk,
		GameCode:    "nfl",
	}
}

func mockUsersContent() FantasyContent {
	return FantasyContent{
		Users: &Users{User: []User{{GUID: MockUserGUID}}},
	}
}

func mockUserLeaguesContent() FantasyContent {
	lg := mockLeagueMeta()
	return FantasyContent{
		Users: &Users{User: []User{{
			GUID: MockUserGUID,
			Games: Games{Game: []Game{{
				GameKey: MockGameKey,
				Name:    "Football",
				Code:    "nfl",
				Season:  MockSeason,
				Leagues: Leagues{League: []League{lg}},
			}}},
		}}},
	}
}

func toTeam(t mockTeam, withRoster bool) Team {
	owned := "0"
	if t.isMine {
		owned = "1"
	}
	commish := "0"
	if t.id == "1" {
		commish = "1"
	}
	team := Team{
		TeamKey:              t.key,
		TeamID:               t.id,
		Name:                 t.name,
		IsOwnedByCurrentUser: owned,
		Managers: Managers{Manager: []Manager{{
			GUID:           "MOCKMGR" + t.id,
			Nickname:       t.manager,
			IsCommissioner: commish,
		}}},
	}
	if withRoster {
		team.Roster = &Roster{Players: rosterPlayers(t)}
	}
	return team
}

// rosterSlots assigns lineup positions in a standard PPR layout, with anything
// past the starters going to the bench.
func rosterSlots(roster []mockPlayer) []string {
	starters := []string{"QB", "RB", "RB", "WR", "WR", "TE", "W/R/T", "K", "DEF"}
	slots := make([]string, len(roster))
	used := make([]bool, len(starters))
	for i, p := range roster {
		slots[i] = "BN"
		for s, want := range starters {
			if used[s] {
				continue
			}
			ok := want == p.pos ||
				(want == "W/R/T" && (p.pos == "WR" || p.pos == "RB" || p.pos == "TE"))
			if ok {
				slots[i] = want
				used[s] = true
				break
			}
		}
	}
	return slots
}

func rosterPlayers(t mockTeam) RosterPlayers {
	slots := rosterSlots(t.roster)
	players := make([]RosterPlayer, 0, len(t.roster))
	for i, p := range t.roster {
		idx := poolIndex[p.name]
		players = append(players, RosterPlayer{
			PlayerKey:         playerKey(idx),
			PlayerID:          strconv.Itoa(30000 + idx),
			Name:              splitName(p.name),
			EditorialTeamAbbr: p.team,
			DisplayPosition:   p.pos,
			SelectedPosition:  SelectedPosition{Position: slots[i]},
			PlayerStats:       mockPlayerStats(idx),
		})
	}
	return RosterPlayers{Player: players}
}

// mockPlayerStats produces stable per-player stat lines keyed off the pool
// index, using the same stat IDs as the league settings below.
func mockPlayerStats(idx int) *PlayerStats {
	r := rand.New(rand.NewSource(int64(idx) * 7919))
	return &PlayerStats{
		CoverageType: "season",
		Stats: []PlayerStat{
			{StatID: "4", Value: strconv.Itoa(r.Intn(4200))},  // passing yards
			{StatID: "5", Value: strconv.Itoa(r.Intn(35))},    // passing TDs
			{StatID: "6", Value: strconv.Itoa(r.Intn(15))},    // interceptions
			{StatID: "9", Value: strconv.Itoa(r.Intn(1400))},  // rushing yards
			{StatID: "10", Value: strconv.Itoa(r.Intn(14))},   // rushing TDs
			{StatID: "11", Value: strconv.Itoa(r.Intn(110))},  // receptions
			{StatID: "12", Value: strconv.Itoa(r.Intn(1500))}, // receiving yards
			{StatID: "13", Value: strconv.Itoa(r.Intn(13))},   // receiving TDs
		},
	}
}

func splitName(full string) PlayerName {
	first, last := full, ""
	for i := 0; i < len(full); i++ {
		if full[i] == ' ' {
			first, last = full[:i], full[i+1:]
			break
		}
	}
	return PlayerName{Full: full, First: first, Last: last}
}

func mockTeamsContent(withRosters bool) FantasyContent {
	lg := mockLeagueMeta()
	teams := make([]Team, 0, len(mockTeams))
	for _, t := range mockTeams {
		teams = append(teams, toTeam(t, withRosters))
	}
	lg.Teams = &Teams{Team: teams}
	return FantasyContent{League: &lg}
}

// mockTeamRosterContent serves a single team's roster. An unknown key falls
// back to the first team rather than erroring — the mock's job is to keep the
// UI moving, and a wrong-but-present roster is easier to debug than a 500.
func mockTeamRosterContent(teamKey string) FantasyContent {
	for _, t := range mockTeams {
		if t.key == teamKey {
			team := toTeam(t, true)
			return FantasyContent{Team: &team}
		}
	}
	team := toTeam(mockTeams[0], true)
	return FantasyContent{Team: &team}
}

func mockStandingsContent() FantasyContent {
	lg := mockLeagueMeta()
	order := standingsOrder()
	teams := make([]StandingTeam, 0, len(order))
	for rank, ti := range order {
		t := mockTeams[ti]
		pct := "0.000"
		if t.wins > 0 {
			pct = "1.000"
		}
		streakType := "win"
		if t.losses > 0 {
			streakType = "loss"
		}
		teams = append(teams, StandingTeam{
			TeamKey: t.key,
			TeamID:  t.id,
			Name:    t.name,
			TeamStandings: TeamStandings{
				Rank:        rank + 1,
				PlayoffSeed: rank + 1,
				OutcomeTotals: OutcomeTotals{
					Wins: t.wins, Losses: t.losses, Ties: t.ties, Percentage: pct,
				},
				Streak:        Streak{Type: streakType, Value: 1},
				PointsFor:     fmt.Sprintf("%.2f", t.pointsFor),
				PointsAgainst: fmt.Sprintf("%.2f", t.pointsAgainst),
			},
		})
	}
	lg.Standings = &Standings{Teams: StandingsTeams{Team: teams}}
	return FantasyContent{League: &lg}
}

// mockScoreboardContent pairs team i against team i+6, matching how the win/loss
// records above were assigned so the two views agree.
func mockScoreboardContent(week int) FantasyContent {
	if week <= 0 {
		week = mockCurrentWk
	}
	lg := mockLeagueMeta()
	half := mockNumTeams / 2
	matchups := make([]Matchup, 0, half)
	for i := 0; i < half; i++ {
		a, b := mockTeams[i], mockTeams[i+half]
		matchups = append(matchups, Matchup{
			Week:       week,
			WeekStart:  "2026-09-10",
			WeekEnd:    "2026-09-15",
			Status:     "postevent",
			IsPlayoffs: "0",
			MatchupTeams: MatchupTeams{Team: []MatchupTeam{
				matchupTeam(a, week), matchupTeam(b, week),
			}},
		})
	}
	lg.Scoreboard = &Scoreboard{Week: week, Matchups: Matchups{Matchup: matchups}}
	return FantasyContent{League: &lg}
}

func matchupTeam(t mockTeam, week int) MatchupTeam {
	return MatchupTeam{
		TeamKey: t.key,
		TeamID:  t.id,
		Name:    t.name,
		TeamPoints: TeamPoints{
			CoverageType: "week", Week: week, Total: fmt.Sprintf("%.2f", t.pointsFor),
		},
		TeamProjectedPoints: TeamPoints{
			CoverageType: "week", Week: week, Total: fmt.Sprintf("%.2f", t.pointsFor*0.97),
		},
	}
}

// mockSettingsContent serves both GetLeagueScoringStats and
// GetLeagueRosterPositions, which hit the same /settings URL.
func mockSettingsContent() FantasyContent {
	lg := mockLeagueMeta()
	cats := []struct {
		id, name, display string
		sortOrder         string
		modifier          float64
	}{
		{"4", "Passing Yards", "Pass Yds", "1", 0.04},
		{"5", "Passing Touchdowns", "Pass TD", "1", 4},
		{"6", "Interceptions", "Int", "0", -1},
		{"9", "Rushing Yards", "Rush Yds", "1", 0.1},
		{"10", "Rushing Touchdowns", "Rush TD", "1", 6},
		{"11", "Receptions", "Rec", "1", 1}, // full PPR
		{"12", "Reception Yards", "Rec Yds", "1", 0.1},
		{"13", "Reception Touchdowns", "Rec TD", "1", 6},
	}
	stats := make([]LeagueStat, 0, len(cats))
	mods := make([]StatModifier, 0, len(cats))
	for _, c := range cats {
		stats = append(stats, LeagueStat{
			StatID: c.id, Name: c.name, DisplayName: c.display,
			SortOrder: c.sortOrder, IsOnlyDisplayStat: "0",
		})
		mods = append(mods, StatModifier{StatID: c.id, Value: c.modifier})
	}

	lg.Settings = &LeagueSettings{
		StatCategories: StatCategories{Stats: stats},
		StatModifiers:  StatModifiers{Stats: mods},
		RosterPositions: RosterPositions{RosterPosition: []RosterPosition{
			{Position: "QB", PositionType: "O", Count: 1},
			{Position: "RB", PositionType: "O", Count: 2},
			{Position: "WR", PositionType: "O", Count: 2},
			{Position: "TE", PositionType: "O", Count: 1},
			{Position: "W/R/T", PositionType: "O", Count: 1},
			{Position: "K", PositionType: "K", Count: 1},
			{Position: "DEF", PositionType: "DT", Count: 1},
			{Position: "BN", PositionType: "BN", Count: 6},
		}},
	}
	return FantasyContent{League: &lg}
}

func mockDraftResultsContent() FantasyContent {
	lg := mockLeagueMeta()
	lg.DraftResults = &DraftResults{DraftPick: mockDraftPicks}
	return FantasyContent{League: &lg}
}

// mockPlayersContent serves the league player-browse endpoints. Filters are
// applied loosely — enough for the UI to behave sensibly without reimplementing
// Yahoo's query semantics.
func mockPlayersContent(position, search string, limit int, freeAgentsOnly bool) FantasyContent {
	lg := mockLeagueMeta()

	pool := mockPlayerPool
	if freeAgentsOnly {
		pool = mockFreeAgents
	}

	out := make([]LeaguePlayer, 0, limit)
	for _, p := range pool {
		if position != "" && p.pos != position {
			continue
		}
		if search != "" && !containsFold(p.name, search) {
			continue
		}
		idx := poolIndex[p.name]
		owned := "0.0"
		ownType := "freeagents"
		if !freeAgentsOnly && idx < len(mockPlayerPool)-len(mockFreeAgents) {
			owned = "100.0"
			ownType = "team"
		}
		out = append(out, LeaguePlayer{
			PlayerKey:         playerKey(idx),
			PlayerID:          strconv.Itoa(30000 + idx),
			Name:              splitName(p.name),
			EditorialTeamAbbr: p.team,
			DisplayPosition:   p.pos,
			Ownership: PlayerOwnership{
				OwnershipType: ownType, OwnedPercent: owned, StartedPercent: "0.0",
			},
			PlayerStats: mockPlayerStats(idx),
		})
		if limit > 0 && len(out) >= limit {
			break
		}
	}

	lg.Players = &LeaguePlayers{Player: out}
	return FantasyContent{League: &lg}
}

// containsFold reports whether s contains sub, case-insensitively (ASCII).
func containsFold(s, sub string) bool {
	ls, lsub := lower(s), lower(sub)
	return len(lsub) == 0 || indexOf(ls, lsub) >= 0
}

func lower(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
