// cmd/projections builds player season profiles and computes comp-based projections.
//
// Usage:
//
//	go run ./cmd/projections -profiles               # step 1: build season profiles
//	go run ./cmd/projections -project -season 2025   # step 2: compute projections
//	go run ./cmd/projections -all -season 2025       # both steps
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

// ── constants ────────────────────────────────────────────────────────────────

const (
	minGames          = 4    // minimum games to include a player-season in profiles
	similarityThresh  = 0.60 // include comps with similarity ≥ this value
	commonArchetype   = 10   // comp count for "common archetype"
	rareProfileMax    = 3    // comp count at or below = "rare profile"
	maxGrowthCap      = 3.0  // cap growth rate multiplier
	minGrowthFloor    = 0.1  // floor growth rate multiplier
	defaultProjGames  = 17   // modern NFL season length
	draftCapitalYears = 3    // drop draft_number from similarity after this many years_exp
)

// ── position dimension groups ─────────────────────────────────────────────────
//
// Each dimGroup has a name, a position-level weight, and a list of z-score
// fields that contribute to it. computeSimilarity averages the z-scores within
// each group before computing the weighted Euclidean distance across groups.
// This means the group's weight is fixed regardless of how many stats feed it —
// adding more rushing stats to a QB profile doesn't dilute the passing weight.

type dimGroup struct {
	name   string
	weight float64
	fields []string
}

// positionGroups returns the dimension groups used for similarity matching.
// Groups are named (e.g. "passing", "rushing") so the config / autotune can
// tune one weight per dimension rather than one weight per stat.
func positionGroups(posGroup string, yearsExp int) []dimGroup {
	useDraftCapital := yearsExp < draftCapitalYears

	withDraftCapital := func(groups []dimGroup) []dimGroup {
		if useDraftCapital {
			groups = append(groups, dimGroup{"draft_capital", 1.0, []string{"draft_number"}})
		}
		return groups
	}

	// Grade dimension — present for all positions. Prefers comps who were similarly
	// "good at football" overall, not just stat-line twins.
	gradeDim := dimGroup{"grade", 1.25, []string{"overall_grade_z"}}

	switch posGroup {
	case "QB":
		return withDraftCapital([]dimGroup{
			{"passing", 3.0, []string{
				"pass_yds_pg", "pass_td_pg", "pass_ypa", "comp_pct", "int_pg",
				"pass_epa_play", "passing_air_yards_pg", "air_yards_share",
				"passing_yac_pg", "sacks_pg",
			}},
			{"rushing", 2.0, []string{
				"rush_yds_pg", "rush_td_pg", "rush_att_pg", "rush_ypc",
				"rush_epa_play", "rush_yard_share", "rushing_first_downs_pg", "fumbles_pg",
			}},
			{"value", 2.0, []string{"fpts_ppr_pg"}},
			{"physical", 0.75, []string{"age", "height", "weight"}},
			{"context", 0.75, []string{"team_pass_yds_pg", "team_fpts_pg"}},
			gradeDim,
		})
	case "RB":
		return withDraftCapital([]dimGroup{
			{"rushing", 3.0, []string{
				"rush_yds_pg", "rush_td_pg", "rush_ypc", "rush_att_pg",
				"rush_epa_play", "rush_yard_share", "rushing_first_downs_pg", "fumbles_pg",
			}},
			{"receiving", 2.0, []string{
				"rec_pg", "rec_yds_pg", "targets_pg", "target_share",
				"rec_epa_play", "receiving_yac_pg",
			}},
			{"value", 2.0, []string{"fpts_ppr_pg"}},
			{"physical", 1.25, []string{"age", "weight"}},
			{"context", 0.75, []string{"team_rush_yds_pg", "team_fpts_pg"}},
			gradeDim,
		})
	case "WR":
		return withDraftCapital([]dimGroup{
			{"receiving", 3.0, []string{
				"rec_yds_pg", "rec_td_pg", "rec_pg", "targets_pg", "target_share",
				"wopr", "rec_ypr", "rec_epa_play", "receiving_air_yards_pg",
				"receiving_yac_pg", "receiving_first_downs_pg",
			}},
			{"rushing", 0.75, []string{
				"rush_yds_pg", "rush_td_pg", "rush_att_pg", "rush_ypc",
				"rush_epa_play", "rush_yard_share", "rushing_first_downs_pg",
			}},
			{"value", 2.0, []string{"fpts_ppr_pg"}},
			{"physical", 1.0, []string{"age", "height"}},
			{"context", 0.75, []string{"team_pass_yds_pg", "team_fpts_pg"}},
			gradeDim,
		})
	case "TE":
		return withDraftCapital([]dimGroup{
			{"receiving", 3.0, []string{
				"rec_yds_pg", "rec_td_pg", "rec_pg", "targets_pg", "target_share",
				"rec_ypr", "rec_epa_play", "receiving_air_yards_pg",
				"receiving_yac_pg", "receiving_first_downs_pg",
			}},
			{"rushing", 0.5, []string{
				"rush_yds_pg", "rush_td_pg", "rush_att_pg", "rush_ypc",
				"rush_epa_play", "rush_yard_share",
			}},
			{"value", 2.0, []string{"fpts_ppr_pg"}},
			{"physical", 0.75, []string{"age", "height", "weight"}},
			{"context", 0.75, []string{"team_pass_yds_pg"}},
			gradeDim,
		})
	case "K":
		return withDraftCapital([]dimGroup{
			{"kicking", 3.0, []string{"fg_made_pg", "pat_made_pg", "fg_pct"}},
			{"value", 3.0, []string{"fpts_pg"}},
			{"physical", 1.0, []string{"age"}},
			gradeDim,
		})
	default:
		return withDraftCapital([]dimGroup{
			{"value", 3.0, []string{"fpts_ppr_pg", "fpts_pg"}},
			{"physical", 1.5, []string{"age"}},
			gradeDim,
		})
	}
}

// avgGroupZScore returns the mean z-score across the given fields, and whether
// at least one field had data. Fields missing from zscores are skipped.
func avgGroupZScore(zscores map[string]float64, fields []string) (float64, bool) {
	var sum float64
	var count int
	for _, f := range fields {
		if z, ok := zscores[f]; ok {
			sum += z
			count++
		}
	}
	if count == 0 {
		return 0, false
	}
	return sum / float64(count), true
}

// computeCompExplanation identifies the most similar and most divergent
// dimension groups between a target and candidate profile.
// Returns matching (top 3 closest) and divergent (top 2 furthest) group names.
func computeCompExplanation(target, cand *seasonProfile, groups []dimGroup) (matching, divergent []string) {
	type groupDiff struct {
		name string
		diff float64 // |mean_z_target - mean_z_cand|
	}
	var diffs []groupDiff
	for _, g := range groups {
		tz, tok := avgGroupZScore(target.ZScores, g.fields)
		cz, cok := avgGroupZScore(cand.ZScores, g.fields)
		if !tok || !cok {
			continue
		}
		diffs = append(diffs, groupDiff{name: g.name, diff: math.Abs(tz - cz)})
	}

	// Sort ascending by diff for matching groups
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].diff < diffs[j].diff })

	matching = make([]string, 0, 3)
	for i := 0; i < len(diffs) && i < 3; i++ {
		matching = append(matching, diffs[i].name)
	}

	// Sort descending by diff for divergent groups
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].diff > diffs[j].diff })

	divergent = make([]string, 0, 2)
	for i := 0; i < len(diffs) && i < 2; i++ {
		divergent = append(divergent, diffs[i].name)
	}

	return matching, divergent
}

// ── data types ───────────────────────────────────────────────────────────────

// seasonProfile mirrors nfl_player_season_profiles.
type seasonProfile struct {
	ID            int64
	GsisID        string
	Season        int
	Age           int
	YearsExp      int
	DraftNumber   *int
	PositionGroup string
	GamesPlayed   int
	Height        *int
	Weight        *int

	PassAttPG   float64
	PassYdsPG   float64
	PassTdPG    float64
	IntPG       float64
	RushAttPG   float64
	RushYdsPG   float64
	RushTdPG    float64
	TargetsPG   float64
	RecPG       float64
	RecYdsPG    float64
	RecTdPG     float64
	FptsPG      float64
	FptsPPRPG   float64
	FgMadePG    float64
	PatMadePG   float64

	PassYPA      *float64
	CompPct      *float64
	RushYPC      *float64
	RecYPR       *float64
	TargetShare  *float64
	WOPR         *float64
	PassEPAPlay  *float64
	RushEPAPlay  *float64
	RecEPAPlay   *float64
	RushYardShare *float64

	SacksPG             float64
	PassingAirYardsPG   float64
	PassingYACPG        float64
	AirYardsShare       *float64
	RushingFirstDownsPG float64
	ReceivingAirYardsPG float64
	ReceivingYACPG      float64
	ReceivingFirstDownsPG float64
	FumblesPG           float64
	FgPct               *float64

	TeamFptsPG     *float64
	TeamPassYdsPG  *float64
	TeamRushYdsPG  *float64

	ZScores map[string]float64
}

// playerMeta holds metadata from nfl_players.
type playerMeta struct {
	GsisID        string
	Name          string
	Position      *string
	PositionGroup *string
	Team          *string
	HeadshotURL   *string
	BirthYear     *int
	Height        *int
	Weight        *int
	YearsExp      *int
	EntryYear     *int
	RookieYear    *int
	DraftNumber   *int
}

// trajPoint is one season in a comp's future trajectory.
type trajPoint struct {
	Season    int     `json:"season"`
	Age       int     `json:"age"`
	FptsPPRPG float64 `json:"fpts_ppr_pg"`
	FptsPG    float64 `json:"fpts_pg"`
	Growth    float64 `json:"growth"` // fpts_ppr_pg / match fpts_ppr_pg
}

// compResult is one qualifying historical comp for a target player.
type compResult struct {
	GsisID        string             `json:"gsis_id"`
	Name          string             `json:"name"`
	MatchSeason   int                `json:"match_season"`
	MatchAge      int                `json:"match_age"`
	Similarity    float64            `json:"similarity"`
	Weight        float64            `json:"weight"`
	HeadshotURL   string             `json:"headshot_url"`
	MatchProfile  map[string]float64 `json:"match_profile"`
	PreMatch      []trajPoint        `json:"pre_match"`
	Trajectory    []trajPoint        `json:"trajectory"`
	MatchingDims  []string           `json:"matching_dims"`
	DivergentDims []string           `json:"divergent_dims"`
}

// projection holds the final computed projection for one player.
type projection struct {
	GsisID      string
	BaseSeason  int
	TargetSeason int

	ProjFptsPG     float64
	ProjFptsPPRPG  float64
	ProjPassYdsPG  float64
	ProjPassTdPG   float64
	ProjRushYdsPG  float64
	ProjRushTdPG   float64
	ProjRecPG      float64
	ProjRecYdsPG   float64
	ProjRecTdPG    float64
	ProjFgMadePG   float64
	ProjPatMadePG  float64

	ProjGames    int
	ProjFpts     float64
	ProjFptsPPR  float64
	ProjFptsHalf float64

	Confidence     float64
	ConfSimilarity float64
	ConfCompCount  float64
	ConfAgreement  float64
	ConfSampleDepth float64
	ConfDataQuality float64

	CompCount    int
	AvgSimilarity float64
	Uniqueness   string

	Comps []compResult
}

// ── main ─────────────────────────────────────────────────────────────────────

func main() {
	doProfiles := flag.Bool("profiles", false, "build season profiles")
	doProject := flag.Bool("project", false, "compute projections")
	doGrades := flag.Bool("grades", false, "compute player grades (real-life value, 0-100 percentile)")
	doAll := flag.Bool("all", false, "build profiles then compute projections")
	doBacktest := flag.Bool("backtest", false, "backtest projections against historical actuals")
	doAutotune := flag.Bool("autotune", false, "auto-tune dimension weights via coordinate descent")
	targetSeason := flag.Int("season", 2026, "target projection season")
	baseSeason := flag.Int("base", 0, "base season for input (defaults to target-1)")
	simThresh := flag.Float64("threshold", similarityThresh, "similarity threshold (0-1)")
	fromSeason := flag.Int("from", 2015, "first season for backtest/autotune")
	toSeason := flag.Int("to", 2024, "last season for backtest/autotune")
	trainTo := flag.Int("train-to", 2021, "last training season for autotune (rest used for validation)")
	flag.Parse()

	if !*doProfiles && !*doProject && !*doGrades && !*doAll && !*doBacktest && !*doAutotune {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/projections [-profiles] [-project -season N] [-grades] [-all -season N] [-backtest -from N -to N] [-autotune -from N -to N -train-to N]")
		os.Exit(1)
	}

	if *baseSeason == 0 {
		*baseSeason = *targetSeason - 1
	}

	_ = godotenv.Load()
	_ = godotenv.Load("../.env")

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		log.Fatalf("db connect: %v", err)
	}
	defer pool.Close()

	if *doProfiles || *doAll {
		log.Println("=== Building season profiles ===")
		if err := buildProfiles(ctx, pool); err != nil {
			log.Fatalf("build profiles: %v", err)
		}
	}

	// Grades must run before projections so that overall_grade_z is available
	// as a similarity dimension for comp matching.
	if *doGrades || *doAll {
		if err := computeGrades(ctx, pool); err != nil {
			log.Fatalf("compute grades: %v", err)
		}
		log.Println("=== Enriching profiles with grade z-scores ===")
		if err := enrichGradeZScores(ctx, pool); err != nil {
			log.Fatalf("enrich grade z-scores: %v", err)
		}
	}

	if *doProject || *doAll {
		cfg := loadConfig()
		// Allow -threshold flag to override config value
		if *simThresh != similarityThresh {
			cfg.SimilarityThreshold = *simThresh
		}
		log.Printf("=== Computing projections: base=%d → target=%d (threshold=%.2f) ===", *baseSeason, *targetSeason, cfg.SimilarityThreshold)
		if err := computeProjections(ctx, pool, *baseSeason, *targetSeason, cfg); err != nil {
			log.Fatalf("compute projections: %v", err)
		}
	}

	if *doBacktest {
		cfg := loadConfig()
		log.Printf("=== Backtesting seasons %d–%d ===", *fromSeason, *toSeason)
		results, err := runBacktest(ctx, pool, *fromSeason, *toSeason, cfg)
		if err != nil {
			log.Fatalf("backtest: %v", err)
		}
		log.Printf("  backtest complete: %d result sets stored", len(results))
	}

	if *doAutotune {
		log.Printf("=== Auto-tuning: train=%d–%d  validate=%d–%d ===", *fromSeason, *trainTo, *trainTo+1, *toSeason)
		if err := runAutotune(ctx, pool, *fromSeason, *trainTo, *trainTo+1, *toSeason); err != nil {
			log.Fatalf("autotune: %v", err)
		}
	}

	log.Println("done")
}

// ── profile builder ───────────────────────────────────────────────────────────

func buildProfiles(ctx context.Context, pool *pgxpool.Pool) error {
	// 1. Aggregate all player-seasons from nfl_player_stats (REG season only).
	log.Println("  aggregating player-season stats…")
	type rawSeason struct {
		GsisID       string
		Season       int
		Games        int
		PassAtt      float64
		PassYds      float64
		PassTds      float64
		Ints         float64
		RushAtt      float64
		RushYds      float64
		RushTds      float64
		Targets      float64
		Recs         float64
		RecYds       float64
		RecTds       float64
		Fpts         float64
		FptsPPR      float64
		FgMade       float64
		PatMade      float64
		Completions  float64
		PassEPA      *float64
		RushEPA      *float64
		RecEPA       *float64
		TargetShare  *float64
		WOPR         *float64
		Team         string
		Sacks        float64
		PassAirYds   float64
		PassYAC      float64
		RushFirstDowns float64
		RecAirYds    float64
		RecYAC       float64
		RecFirstDowns float64
		Fumbles      float64
		FgAtt        float64
	}

	rows, err := pool.Query(ctx, `
		SELECT
			gsis_id,
			season,
			COUNT(*) FILTER (WHERE (passing_yards + rushing_yards + receiving_yards + targets + fg_made) > 0) AS games,
			SUM(pass_attempts)::float8,
			SUM(passing_yards)::float8,
			SUM(passing_tds)::float8,
			SUM(interceptions)::float8,
			SUM(carries)::float8,
			SUM(rushing_yards)::float8,
			SUM(rushing_tds)::float8,
			SUM(targets)::float8,
			SUM(receptions)::float8,
			SUM(receiving_yards)::float8,
			SUM(receiving_tds)::float8,
			SUM(fantasy_points)::float8,
			SUM(fantasy_points_ppr)::float8,
			SUM(fg_made)::float8,
			SUM(pat_made)::float8,
			SUM(completions)::float8,
			AVG(passing_epa) FILTER (WHERE passing_epa IS NOT NULL),
			AVG(rushing_epa) FILTER (WHERE rushing_epa IS NOT NULL),
			AVG(receiving_epa) FILTER (WHERE receiving_epa IS NOT NULL),
			AVG(target_share) FILTER (WHERE target_share IS NOT NULL),
			AVG(wopr) FILTER (WHERE wopr IS NOT NULL),
			MODE() WITHIN GROUP (ORDER BY team) AS team,
			SUM(sacks)::float8,
			SUM(passing_air_yards)::float8,
			SUM(passing_yac)::float8,
			SUM(rushing_first_downs)::float8,
			SUM(receiving_air_yards)::float8,
			SUM(receiving_yac)::float8,
			SUM(receiving_first_downs)::float8,
			SUM(COALESCE(rushing_fumbles, 0) + COALESCE(receiving_fumbles, 0))::float8,
			SUM(fg_att)::float8
		FROM nfl_player_stats
		WHERE season_type = 'REG'
		GROUP BY gsis_id, season
		HAVING COUNT(*) FILTER (WHERE (passing_yards + rushing_yards + receiving_yards + targets + fg_made) > 0) >= $1
	`, minGames)
	if err != nil {
		return fmt.Errorf("query player-seasons: %w", err)
	}
	defer rows.Close()

	var seasons []rawSeason
	for rows.Next() {
		var s rawSeason
		if err := rows.Scan(
			&s.GsisID, &s.Season, &s.Games,
			&s.PassAtt, &s.PassYds, &s.PassTds, &s.Ints,
			&s.RushAtt, &s.RushYds, &s.RushTds,
			&s.Targets, &s.Recs, &s.RecYds, &s.RecTds,
			&s.Fpts, &s.FptsPPR, &s.FgMade, &s.PatMade, &s.Completions,
			&s.PassEPA, &s.RushEPA, &s.RecEPA,
			&s.TargetShare, &s.WOPR, &s.Team,
			&s.Sacks, &s.PassAirYds, &s.PassYAC,
			&s.RushFirstDowns, &s.RecAirYds, &s.RecYAC,
			&s.RecFirstDowns, &s.Fumbles, &s.FgAtt,
		); err != nil {
			return fmt.Errorf("scan player-season: %w", err)
		}
		seasons = append(seasons, s)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	log.Printf("  found %d qualifying player-seasons", len(seasons))

	// 2. Load player metadata.
	log.Println("  loading player metadata…")
	metaRows, err := pool.Query(ctx, `
		SELECT gsis_id, position_group,
		       EXTRACT(YEAR FROM birth_date)::int,
		       height, weight, years_exp, entry_year, draft_number
		FROM nfl_players
	`)
	if err != nil {
		return fmt.Errorf("query players: %w", err)
	}
	defer metaRows.Close()

	type meta struct {
		PosGroup    *string
		BirthYear   *int
		Height      *int
		Weight      *int
		YearsExp    *int
		EntryYear   *int
		DraftNumber *int
	}
	playerMetas := make(map[string]meta)
	for metaRows.Next() {
		var id string
		var m meta
		if err := metaRows.Scan(&id, &m.PosGroup, &m.BirthYear, &m.Height, &m.Weight, &m.YearsExp, &m.EntryYear, &m.DraftNumber); err != nil {
			return fmt.Errorf("scan meta: %w", err)
		}
		playerMetas[id] = m
	}
	if err := metaRows.Err(); err != nil {
		return err
	}

	// 3. Build team-season offensive stats (proxy for team context).
	log.Println("  computing team-season context…")
	teamRows, err := pool.Query(ctx, `
		SELECT team, season,
		       SUM(fantasy_points)::float8 / NULLIF(COUNT(DISTINCT week), 0) AS fpts_pg,
		       SUM(passing_yards)::float8 / NULLIF(COUNT(DISTINCT week), 0) AS pass_yds_pg,
		       SUM(rushing_yards)::float8 / NULLIF(COUNT(DISTINCT week), 0) AS rush_yds_pg
		FROM nfl_player_stats
		WHERE season_type = 'REG' AND team IS NOT NULL
		GROUP BY team, season
	`)
	if err != nil {
		return fmt.Errorf("query team stats: %w", err)
	}
	defer teamRows.Close()

	type teamKey struct{ team string; season int }
	type teamStats struct{ fptsPG, passYdsPG, rushYdsPG float64 }
	teamCtx := make(map[teamKey]teamStats)
	for teamRows.Next() {
		var tk teamKey
		var ts teamStats
		if err := teamRows.Scan(&tk.team, &tk.season, &ts.fptsPG, &ts.passYdsPG, &ts.rushYdsPG); err != nil {
			return fmt.Errorf("scan team stats: %w", err)
		}
		teamCtx[tk] = ts
	}
	if err := teamRows.Err(); err != nil {
		return err
	}

	// 4. Build profiles and insert them.
	log.Println("  upserting profiles…")
	upserted := 0
	for _, s := range seasons {
		m, ok := playerMetas[s.GsisID]
		if !ok {
			continue
		}

		if m.PosGroup == nil || *m.PosGroup == "" {
			continue
		}
		posGroup := *m.PosGroup
		// Normalise position groups to the ones we handle
		switch posGroup {
		case "QB", "RB", "WR", "TE", "K":
			// ok
		default:
			continue // skip DEF, OL, etc.
		}

		g := float64(s.Games)

		// Per-game rates
		passAttPG := s.PassAtt / g
		passYdsPG := s.PassYds / g
		passTdPG := s.PassTds / g
		intPG := s.Ints / g
		rushAttPG := s.RushAtt / g
		rushYdsPG := s.RushYds / g
		rushTdPG := s.RushTds / g
		targetsPG := s.Targets / g
		recPG := s.Recs / g
		recYdsPG := s.RecYds / g
		recTdPG := s.RecTds / g
		fptsPG := s.Fpts / g
		fptsPPRPG := s.FptsPPR / g
		fgMadePG := s.FgMade / g
		patMadePG := s.PatMade / g

		// Efficiency
		var passYPA, compPct, rushYPC, recYPR *float64
		if s.PassAtt > 0 {
			v := s.PassYds / s.PassAtt
			passYPA = &v
			c := (s.Completions / s.PassAtt) * 100
			compPct = &c
		}
		if s.RushAtt > 0 {
			v := s.RushYds / s.RushAtt
			rushYPC = &v
		}
		if s.Recs > 0 {
			v := s.RecYds / s.Recs
			recYPR = &v
		}

		// New per-game rates
		sacksPG := s.Sacks / g
		passingAirYardsPG := s.PassAirYds / g
		passingYACPG := s.PassYAC / g
		rushingFirstDownsPG := s.RushFirstDowns / g
		receivingAirYardsPG := s.RecAirYds / g
		receivingYACPG := s.RecYAC / g
		receivingFirstDownsPG := s.RecFirstDowns / g
		fumblesPG := s.Fumbles / g

		// Air yards share: what fraction of passing yards came from air (vs YAC)
		var airYardsShare *float64
		if s.PassYds > 0 {
			v := s.PassAirYds / s.PassYds
			airYardsShare = &v
		}

		// FG percentage
		var fgPct *float64
		if s.FgAtt > 0 {
			v := s.FgMade / s.FgAtt * 100
			fgPct = &v
		}

		// Rush yard share (0 = pure receiver, 1 = pure rusher)
		var rushYardShare *float64
		totalYds := s.RushYds + s.RecYds
		if totalYds > 0 {
			v := s.RushYds / totalYds
			rushYardShare = &v
		}

		// Age at start of season
		var age *int
		var yearsExp *int
		if m.BirthYear != nil {
			a := s.Season - *m.BirthYear
			age = &a
		}
		if m.YearsExp != nil {
			// years_exp from roster is end-of-most-recent-season; estimate for historical season
			// use entry_year if available, else fall back
			if m.EntryYear != nil {
				ye := s.Season - *m.EntryYear
				yearsExp = &ye
			} else {
				yearsExp = m.YearsExp
			}
		}

		// Team context
		tc := teamCtx[teamKey{s.Team, s.Season}]
		var teamFptsPG, teamPassYdsPG, teamRushYdsPG *float64
		if tc.fptsPG > 0 {
			teamFptsPG = &tc.fptsPG
			teamPassYdsPG = &tc.passYdsPG
			teamRushYdsPG = &tc.rushYdsPG
		}

		// Build the scalar z_scores map placeholder (will fill after all profiles in group+season known).
		// For now insert with empty z_scores; a second pass will update them.

		_, err := pool.Exec(ctx, `
			INSERT INTO nfl_player_season_profiles (
				gsis_id, season, age, years_exp, draft_number, position_group, games_played,
				height, weight,
				pass_att_pg, pass_yds_pg, pass_td_pg, int_pg,
				rush_att_pg, rush_yds_pg, rush_td_pg,
				targets_pg, rec_pg, rec_yds_pg, rec_td_pg,
				fpts_pg, fpts_ppr_pg, fg_made_pg, pat_made_pg,
				pass_ypa, comp_pct, rush_ypc, rec_ypr,
				target_share, wopr, pass_epa_play, rush_epa_play, rec_epa_play,
				rush_yard_share,
				team_fpts_pg, team_pass_yds_pg, team_rush_yds_pg,
				sacks_pg, passing_air_yards_pg, passing_yac_pg, air_yards_share,
				rushing_first_downs_pg, receiving_air_yards_pg, receiving_yac_pg,
				receiving_first_downs_pg, fumbles_pg, fg_pct,
				z_scores
			) VALUES (
				$1,$2,$3,$4,$5,$6,$7,
				$8,$9,
				$10,$11,$12,$13,
				$14,$15,$16,
				$17,$18,$19,$20,
				$21,$22,$23,$24,
				$25,$26,$27,$28,
				$29,$30,$31,$32,$33,
				$34,
				$35,$36,$37,
				$38,$39,$40,$41,
				$42,$43,$44,
				$45,$46,$47,
				'{}'
			)
			ON CONFLICT (gsis_id, season) DO UPDATE SET
				age = EXCLUDED.age,
				years_exp = EXCLUDED.years_exp,
				draft_number = EXCLUDED.draft_number,
				position_group = EXCLUDED.position_group,
				games_played = EXCLUDED.games_played,
				height = EXCLUDED.height,
				weight = EXCLUDED.weight,
				pass_att_pg = EXCLUDED.pass_att_pg,
				pass_yds_pg = EXCLUDED.pass_yds_pg,
				pass_td_pg = EXCLUDED.pass_td_pg,
				int_pg = EXCLUDED.int_pg,
				rush_att_pg = EXCLUDED.rush_att_pg,
				rush_yds_pg = EXCLUDED.rush_yds_pg,
				rush_td_pg = EXCLUDED.rush_td_pg,
				targets_pg = EXCLUDED.targets_pg,
				rec_pg = EXCLUDED.rec_pg,
				rec_yds_pg = EXCLUDED.rec_yds_pg,
				rec_td_pg = EXCLUDED.rec_td_pg,
				fpts_pg = EXCLUDED.fpts_pg,
				fpts_ppr_pg = EXCLUDED.fpts_ppr_pg,
				fg_made_pg = EXCLUDED.fg_made_pg,
				pat_made_pg = EXCLUDED.pat_made_pg,
				pass_ypa = EXCLUDED.pass_ypa,
				comp_pct = EXCLUDED.comp_pct,
				rush_ypc = EXCLUDED.rush_ypc,
				rec_ypr = EXCLUDED.rec_ypr,
				target_share = EXCLUDED.target_share,
				wopr = EXCLUDED.wopr,
				pass_epa_play = EXCLUDED.pass_epa_play,
				rush_epa_play = EXCLUDED.rush_epa_play,
				rec_epa_play = EXCLUDED.rec_epa_play,
				rush_yard_share = EXCLUDED.rush_yard_share,
				team_fpts_pg = EXCLUDED.team_fpts_pg,
				team_pass_yds_pg = EXCLUDED.team_pass_yds_pg,
				team_rush_yds_pg = EXCLUDED.team_rush_yds_pg,
				sacks_pg = EXCLUDED.sacks_pg,
				passing_air_yards_pg = EXCLUDED.passing_air_yards_pg,
				passing_yac_pg = EXCLUDED.passing_yac_pg,
				air_yards_share = EXCLUDED.air_yards_share,
				rushing_first_downs_pg = EXCLUDED.rushing_first_downs_pg,
				receiving_air_yards_pg = EXCLUDED.receiving_air_yards_pg,
				receiving_yac_pg = EXCLUDED.receiving_yac_pg,
				receiving_first_downs_pg = EXCLUDED.receiving_first_downs_pg,
				fumbles_pg = EXCLUDED.fumbles_pg,
				fg_pct = EXCLUDED.fg_pct,
				computed_at = NOW()
		`,
			s.GsisID, s.Season, derefInt(age), derefInt(yearsExp), m.DraftNumber, posGroup, s.Games,
			m.Height, m.Weight,
			passAttPG, passYdsPG, passTdPG, intPG,
			rushAttPG, rushYdsPG, rushTdPG,
			targetsPG, recPG, recYdsPG, recTdPG,
			fptsPG, fptsPPRPG, fgMadePG, patMadePG,
			passYPA, compPct, rushYPC, recYPR,
			s.TargetShare, s.WOPR, s.PassEPA, s.RushEPA, s.RecEPA,
			rushYardShare,
			teamFptsPG, teamPassYdsPG, teamRushYdsPG,
			sacksPG, passingAirYardsPG, passingYACPG, airYardsShare,
			rushingFirstDownsPG, receivingAirYardsPG, receivingYACPG,
			receivingFirstDownsPG, fumblesPG, fgPct,
		)
		if err != nil {
			log.Printf("  profile upsert %s/%d: %v", s.GsisID, s.Season, err)
			continue
		}
		upserted++
	}
	log.Printf("  upserted %d profiles", upserted)

	// 5. Compute and store z-scores within (position_group, season).
	log.Println("  computing z-scores…")
	if err := computeZScores(ctx, pool); err != nil {
		return fmt.Errorf("compute z-scores: %w", err)
	}

	// 6. Generate attribute tags from z-scores.
	log.Println("  generating player tags…")
	if err := generateTags(ctx, pool); err != nil {
		return fmt.Errorf("generate tags: %w", err)
	}

	return nil
}

// derefInt returns the value of an *int, or 0 if nil.
func derefInt(ptr *int) int {
	if ptr == nil {
		return 0
	}
	return *ptr
}

// computeZScores loads all profiles grouped by position_group (globally across
// all seasons), computes per-field z-scores, and updates z_scores JSONB in bulk.
// Global normalization ensures z-scores are comparable across seasons.
func computeZScores(ctx context.Context, pool *pgxpool.Pool) error {
	// The numeric fields we z-score (must match the profile struct and DB columns)
	fields := []string{
		"pass_att_pg", "pass_yds_pg", "pass_td_pg", "int_pg",
		"rush_att_pg", "rush_yds_pg", "rush_td_pg",
		"targets_pg", "rec_pg", "rec_yds_pg", "rec_td_pg",
		"fpts_pg", "fpts_ppr_pg", "fg_made_pg", "pat_made_pg",
		"pass_ypa", "comp_pct", "rush_ypc", "rec_ypr",
		"target_share", "wopr", "pass_epa_play", "rush_epa_play", "rec_epa_play",
		"rush_yard_share",
		"sacks_pg", "passing_air_yards_pg", "passing_yac_pg", "air_yards_share",
		"rushing_first_downs_pg", "receiving_air_yards_pg", "receiving_yac_pg",
		"receiving_first_downs_pg", "fumbles_pg", "fg_pct",
		"team_fpts_pg", "team_pass_yds_pg", "team_rush_yds_pg",
		"age",
		"height", "weight",
		"draft_number",
	}

	// Load all profiles — select the per-game/efficiency columns explicitly.
	// These are the DB columns that correspond to z-scoreable fields (excluding
	// age, height, weight, draft_number which are stored as int columns).
	dbFloatFields := []string{
		"pass_att_pg", "pass_yds_pg", "pass_td_pg", "int_pg",
		"rush_att_pg", "rush_yds_pg", "rush_td_pg",
		"targets_pg", "rec_pg", "rec_yds_pg", "rec_td_pg",
		"fpts_pg", "fpts_ppr_pg", "fg_made_pg", "pat_made_pg",
		"pass_ypa", "comp_pct", "rush_ypc", "rec_ypr",
		"target_share", "wopr", "pass_epa_play", "rush_epa_play", "rec_epa_play",
		"rush_yard_share",
		"sacks_pg", "passing_air_yards_pg", "passing_yac_pg", "air_yards_share",
		"rushing_first_downs_pg", "receiving_air_yards_pg", "receiving_yac_pg",
		"receiving_first_downs_pg", "fumbles_pg", "fg_pct",
		"team_fpts_pg", "team_pass_yds_pg", "team_rush_yds_pg",
	}

	query := `SELECT id, gsis_id, season, position_group, age, height, weight, draft_number, ` +
		strings.Join(dbFloatFields, ", ") +
		` FROM nfl_player_season_profiles`

	rows, err := pool.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("load profiles for z-scores: %w", err)
	}
	defer rows.Close()

	type profileRow struct {
		id            int64
		gsisID        string
		season        int
		positionGroup string
		vals          map[string]float64 // field → value (0 if NULL)
	}

	numFloatFields := len(dbFloatFields)
	var profiles []profileRow
	for rows.Next() {
		var id int64
		var gsisID, posGroup string
		var season int
		var age, height, weight, draftNumber *int
		nums := make([]*float64, numFloatFields)

		dest := []any{&id, &gsisID, &season, &posGroup, &age, &height, &weight, &draftNumber}
		for i := range nums {
			dest = append(dest, &nums[i])
		}

		if err := rows.Scan(dest...); err != nil {
			return fmt.Errorf("scan profile row: %w", err)
		}

		vals := make(map[string]float64, len(fields))
		for i, f := range dbFloatFields {
			if nums[i] != nil {
				vals[f] = *nums[i]
			}
		}
		if age != nil { vals["age"] = float64(*age) }
		if height != nil { vals["height"] = float64(*height) }
		if weight != nil { vals["weight"] = float64(*weight) }
		if draftNumber != nil { vals["draft_number"] = float64(*draftNumber) }

		profiles = append(profiles, profileRow{id, gsisID, season, posGroup, vals})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Group by position_group only (global normalization across all seasons).
	// This makes z-scores comparable across seasons — a z-score of 1.0 always
	// means "1 stdev above the all-time positional mean" regardless of season.
	groups := make(map[string][]int) // positionGroup → indexes into profiles
	for i, p := range profiles {
		groups[p.positionGroup] = append(groups[p.positionGroup], i)
	}

	// For each group, compute mean+stdev per field, then z-scores
	updated := 0
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}

		// Compute mean and stdev per field
		means := make(map[string]float64, len(fields))
		stdevs := make(map[string]float64, len(fields))
		for _, f := range fields {
			vals := make([]float64, 0, len(idxs))
			for _, i := range idxs {
				if v, ok := profiles[i].vals[f]; ok {
					vals = append(vals, v)
				}
			}
			if len(vals) == 0 {
				continue
			}
			mu := mean(vals)
			sd := stdev(vals, mu)
			means[f] = mu
			stdevs[f] = sd
		}

		// Compute z-scores for each profile in the group
		for _, i := range idxs {
			zmap := make(map[string]float64, len(fields))
			for _, f := range fields {
				mu := means[f]
				sd := stdevs[f]
				if sd == 0 {
					zmap[f] = 0
				} else {
					zmap[f] = (profiles[i].vals[f] - mu) / sd
				}
			}

			zJSON, err := json.Marshal(zmap)
			if err != nil {
				continue
			}

			if _, err := pool.Exec(ctx,
				`UPDATE nfl_player_season_profiles SET z_scores = $1 WHERE id = $2`,
				string(zJSON), profiles[i].id,
			); err != nil {
				log.Printf("  z-score update %s: %v", profiles[i].gsisID, err)
				continue
			}
			updated++
		}
	}
	log.Printf("  updated z-scores for %d profiles", updated)
	return nil
}

// ── tag generator ─────────────────────────────────────────────────────────────

// generateTags reads z_scores JSONB from all profiles, applies position-specific
// tag rules, and batch-updates the tags TEXT[] column.
func generateTags(ctx context.Context, pool *pgxpool.Pool) error {
	rows, err := pool.Query(ctx,
		`SELECT id, position_group, z_scores FROM nfl_player_season_profiles WHERE z_scores IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("load profiles for tags: %w", err)
	}
	defer rows.Close()

	type tagRow struct {
		id   int64
		tags []string
	}

	var updates []tagRow
	for rows.Next() {
		var id int64
		var posGroup string
		var zJSON []byte
		if err := rows.Scan(&id, &posGroup, &zJSON); err != nil {
			return fmt.Errorf("scan tag row: %w", err)
		}

		var z map[string]float64
		if err := json.Unmarshal(zJSON, &z); err != nil {
			log.Printf("  skipping id=%d: bad z_scores JSON: %v", id, err)
			continue
		}

		tags := tagsForPosition(posGroup, z)
		updates = append(updates, tagRow{id, tags})
	}
	if err := rows.Err(); err != nil {
		return err
	}

	// Batch update tags
	updated := 0
	for _, u := range updates {
		tagLiteral := "{" + strings.Join(u.tags, ",") + "}"
		if _, err := pool.Exec(ctx,
			`UPDATE nfl_player_season_profiles SET tags = $1 WHERE id = $2`,
			tagLiteral, u.id,
		); err != nil {
			log.Printf("  tag update id=%d: %v", u.id, err)
			continue
		}
		updated++
	}
	log.Printf("  updated tags for %d profiles", updated)
	return nil
}

// z helper: returns the z-score for a key, or 0 if missing.
func zVal(z map[string]float64, key string) float64 {
	return z[key]
}

// zHas returns true if the key exists in the z-score map.
func zHas(z map[string]float64, key string) bool {
	_, ok := z[key]
	return ok
}

// tagsForPosition returns the attribute tags for a player based on position group and z-scores.
func tagsForPosition(posGroup string, z map[string]float64) []string {
	var tags []string

	switch posGroup {
	case "QB":
		tags = qbTags(z)
	case "RB":
		tags = rbTags(z)
	case "WR":
		tags = wrTags(z)
	case "TE":
		// TEs get both WR tags and TE-specific tags
		tags = wrTags(z)
		tags = append(tags, teTags(z)...)
	}

	return tags
}

func qbTags(z map[string]float64) []string {
	var tags []string

	if zVal(z, "rush_yds_pg") > 1.0 && zVal(z, "pass_yds_pg") > 0 {
		tags = append(tags, "Dual-threat")
	}
	if zVal(z, "rush_yds_pg") < 0 && zVal(z, "pass_yds_pg") > 0.5 {
		tags = append(tags, "Pocket passer")
	}
	if zHas(z, "air_yards_share") && zVal(z, "air_yards_share") > 1.0 {
		tags = append(tags, "Deep ball")
	}
	if zVal(z, "air_yards_share") < -1.0 {
		tags = append(tags, "Check-down")
	}
	if zVal(z, "pass_epa_play") > 1.0 {
		tags = append(tags, "Efficient")
	}
	if zVal(z, "pass_td_pg") > 1.0 && zVal(z, "int_pg") > 0.5 {
		tags = append(tags, "Gunslinger")
	}
	if zVal(z, "pass_yds_pg") >= -0.5 && zVal(z, "pass_yds_pg") <= 0.5 && zVal(z, "int_pg") < 0 {
		tags = append(tags, "Game manager")
	}
	if zVal(z, "sacks_pg") > 1.0 {
		tags = append(tags, "Sack-prone")
	}

	return tags
}

func rbTags(z map[string]float64) []string {
	var tags []string

	if zVal(z, "rush_att_pg") > 1.0 {
		tags = append(tags, "Workhorse")
	}
	if zVal(z, "rec_pg") > 1.0 || zVal(z, "targets_pg") > 1.0 {
		tags = append(tags, "Pass-catcher")
	}
	if zVal(z, "weight") > 0.5 && zVal(z, "rush_ypc") < 0.5 {
		tags = append(tags, "Power back")
	}
	if zVal(z, "weight") < -0.5 && zVal(z, "rush_ypc") > 0.5 {
		tags = append(tags, "Speed back")
	}
	if zVal(z, "rush_att_pg") > 0.5 && zVal(z, "targets_pg") > 0.5 {
		tags = append(tags, "Three-down")
	}
	if zVal(z, "fumbles_pg") > 1.0 {
		tags = append(tags, "Fumble risk")
	}

	return tags
}

// wrTags applies to both WR and TE position groups.
func wrTags(z map[string]float64) []string {
	var tags []string

	if zVal(z, "receiving_air_yards_pg") > 1.0 {
		tags = append(tags, "Deep threat")
	}
	if zVal(z, "receiving_air_yards_pg") < 0 && zVal(z, "rec_pg") > 0.5 {
		tags = append(tags, "Slot")
	}
	if zVal(z, "receiving_yac_pg") > 1.0 {
		tags = append(tags, "YAC monster")
	}
	if zVal(z, "target_share") > 1.0 {
		tags = append(tags, "Target hog")
	}
	if zVal(z, "rec_td_pg") > 1.0 {
		tags = append(tags, "Red zone")
	}
	if zVal(z, "rec_pg") > 0.5 && zVal(z, "rec_ypr") < 0 {
		tags = append(tags, "Possession")
	}

	return tags
}

// teTags returns TE-specific tags (applied in addition to wrTags).
func teTags(z map[string]float64) []string {
	var tags []string

	if zVal(z, "targets_pg") > 1.0 {
		tags = append(tags, "Receiving TE")
	}
	if zVal(z, "targets_pg") < -0.5 {
		tags = append(tags, "Blocking TE")
	}

	return tags
}

// ── projection engine ─────────────────────────────────────────────────────────

func computeProjections(ctx context.Context, pool *pgxpool.Pool, baseSeason, targetSeason int, cfg projConfig) error {
	simThresh := cfg.SimilarityThreshold
	// 1. Load all profiles.
	log.Println("  loading profiles…")
	profiles, err := loadAllProfiles(ctx, pool)
	if err != nil {
		return err
	}
	log.Printf("  loaded %d profiles across all seasons", len(profiles))

	// 2. Load player metadata (names, headshots).
	log.Println("  loading player metadata…")
	metaMap, err := loadPlayerMeta(ctx, pool)
	if err != nil {
		return err
	}

	// 2b. Load grade YoY trends for trend adjustment.
	gradeTrends := make(map[string]float64) // gsis_id → yoy_trend (for base season)
	{
		rows, err := pool.Query(ctx,
			`SELECT gsis_id, yoy_trend FROM nfl_player_grades WHERE season = $1 AND yoy_trend IS NOT NULL`,
			baseSeason)
		if err != nil {
			log.Printf("  warning: could not load grade trends: %v", err)
		} else {
			defer rows.Close()
			for rows.Next() {
				var gsis string
				var trend float64
				if err := rows.Scan(&gsis, &trend); err == nil {
					gradeTrends[gsis] = trend
				}
			}
			log.Printf("  loaded %d grade trends for trend adjustment", len(gradeTrends))
		}
	}

	// 3. Index profiles by (gsis_id, season) and by (position_group, season).
	byPlayerSeason := make(map[string]map[int]*seasonProfile, len(profiles))
	byGroupSeason := make(map[string]map[int][]*seasonProfile)

	for i := range profiles {
		p := &profiles[i]
		if byPlayerSeason[p.GsisID] == nil {
			byPlayerSeason[p.GsisID] = make(map[int]*seasonProfile)
		}
		byPlayerSeason[p.GsisID][p.Season] = p

		gs := p.PositionGroup + "|" + fmt.Sprint(p.Season)
		if byGroupSeason[gs] == nil {
			byGroupSeason[gs] = make(map[int][]*seasonProfile)
		}
		byGroupSeason[gs][p.Season] = append(byGroupSeason[gs][p.Season], p)
	}

	// Also build a flat index by position_group for comp searching:
	// key = positionGroup, value = all profiles (all seasons) in that group
	byGroup := make(map[string][]*seasonProfile)
	for i := range profiles {
		p := &profiles[i]
		byGroup[p.PositionGroup] = append(byGroup[p.PositionGroup], p)
	}

	// 4. Identify target players: those with a profile in baseSeason.
	log.Println("  identifying target players…")
	var targets []*seasonProfile
	for _, seasonMap := range byPlayerSeason {
		if p, ok := seasonMap[baseSeason]; ok {
			targets = append(targets, p)
		}
	}
	log.Printf("  found %d players with %d base-season profiles", len(targets), baseSeason)

	// 5. For each target player, find comps and compute projection.
	log.Println("  computing comp-based projections…")
	projs := make([]projection, 0, len(targets))

	for _, target := range targets {
		meta := metaMap[target.GsisID]

		// Get all profiles for this target player (for data quality scoring)
		allTargetSeasons := byPlayerSeason[target.GsisID]
		dataSeasonsCount := len(allTargetSeasons)

		// Find comps: all profiles in same position group, age within ±2, excluding same player
		candidates := byGroup[target.PositionGroup]
		var comps []compResult

		for _, cand := range candidates {
			if cand.GsisID == target.GsisID {
				continue
			}
			if target.Age > 0 && cand.Age > 0 && abs(target.Age-cand.Age) > 2 {
				continue
			}

			groups := groupsFromConfig(cfg, target.PositionGroup, target.YearsExp)
			sim := computeSimilarity(target, cand, groups)
			if sim < simThresh {
				continue
			}

			// Build pre-match career + post-match trajectory
			candMatchSeason := baseSeason - target.Season + cand.Season
			preMatch := buildPreMatch(cand, byPlayerSeason[cand.GsisID], candMatchSeason)
			trajectory := buildTrajectory(cand, byPlayerSeason[cand.GsisID], candMatchSeason)

			matchProfile := map[string]float64{
				"fpts_pg":     cand.FptsPG,
				"fpts_ppr_pg": cand.FptsPPRPG,
				"pass_yds_pg": cand.PassYdsPG,
				"pass_td_pg":  cand.PassTdPG,
				"rush_yds_pg": cand.RushYdsPG,
				"rush_td_pg":  cand.RushTdPG,
				"rec_pg":      cand.RecPG,
				"rec_yds_pg":  cand.RecYdsPG,
				"rec_td_pg":   cand.RecTdPG,
			}

			headshotURL := ""
			if cm, ok := metaMap[cand.GsisID]; ok && cm.HeadshotURL != nil {
				headshotURL = *cm.HeadshotURL
			}
			compName := ""
			if cm, ok := metaMap[cand.GsisID]; ok {
				compName = cm.Name
			}

			matchingDims, divergentDims := computeCompExplanation(target, cand, groups)

			comps = append(comps, compResult{
				GsisID:        cand.GsisID,
				Name:          compName,
				MatchSeason:   cand.Season,
				MatchAge:      cand.Age,
				Similarity:    sim,
				HeadshotURL:   headshotURL,
				MatchProfile:  matchProfile,
				PreMatch:      preMatch,
				Trajectory:    trajectory,
				MatchingDims:  matchingDims,
				DivergentDims: divergentDims,
			})
		}

		// Sort comps by similarity descending
		sort.Slice(comps, func(i, j int) bool {
			return comps[i].Similarity > comps[j].Similarity
		})

		// Assign weights: similarity² / Σsimilarity²
		var simSqSum float64
		for _, c := range comps {
			simSqSum += c.Similarity * c.Similarity
		}
		if simSqSum > 0 {
			for i := range comps {
				comps[i].Weight = (comps[i].Similarity * comps[i].Similarity) / simSqSum
			}
		}

		// Compute weighted average growth rates for year 1 (next season)
		projectedAge := target.Age + (targetSeason - target.Season)
		agingMult := cfg.effectiveAgingMultipliers().Multiplier(target.PositionGroup, projectedAge)
		proj := computeWeightedProjection(target, comps, targetSeason, agingMult)

		// Grade trend adjustment: small bounded nudge (+/- 5%) based on grade trajectory.
		// Player trending up in grade but stats haven't caught up → slight upward nudge.
		// Player trending down → slight downward nudge.
		if trend, ok := gradeTrends[target.GsisID]; ok && trend != 0 {
			// trend is normalized -1..+1; scale to max ±5% adjustment
			adj := 1.0 + clamp(trend*0.05, -0.05, 0.05)
			proj.ProjFptsPPRPG *= adj
			proj.ProjFptsPG *= adj
			proj.ProjPassYdsPG *= adj
			proj.ProjPassTdPG *= adj
			proj.ProjRushYdsPG *= adj
			proj.ProjRushTdPG *= adj
			proj.ProjRecPG *= adj
			proj.ProjRecYdsPG *= adj
			proj.ProjRecTdPG *= adj
			proj.ProjFgMadePG *= adj
			proj.ProjPatMadePG *= adj
		}

		proj.GsisID = target.GsisID
		proj.BaseSeason = baseSeason
		proj.TargetSeason = targetSeason
		proj.CompCount = len(comps)
		if len(comps) > 10 {
			proj.Comps = comps[:10]
		} else {
			proj.Comps = comps
		}

		// Average similarity
		if len(comps) > 0 {
			var sumSim float64
			for _, c := range comps {
				sumSim += c.Similarity
			}
			proj.AvgSimilarity = sumSim / float64(len(comps))
		}

		// Uniqueness label
		proj.Uniqueness = uniquenessLabel(len(comps), proj.AvgSimilarity)

		// Confidence score
		proj.ConfSimilarity = proj.AvgSimilarity
		proj.ConfCompCount = math.Min(1.0, float64(len(comps))/float64(commonArchetype))
		proj.ConfAgreement = computeCompAgreement(comps)
		proj.ConfSampleDepth = computeSampleDepth(comps)
		proj.ConfDataQuality = math.Min(1.0, float64(dataSeasonsCount)/3.0)
		proj.Confidence = 0.25*proj.ConfSimilarity + 0.20*proj.ConfCompCount +
			0.25*proj.ConfAgreement + 0.15*proj.ConfSampleDepth + 0.15*proj.ConfDataQuality

		// Season totals
		proj.ProjGames = defaultProjGames
		proj.ProjFpts = proj.ProjFptsPG * float64(proj.ProjGames)
		proj.ProjFptsPPR = proj.ProjFptsPPRPG * float64(proj.ProjGames)
		proj.ProjFptsHalf = (proj.ProjFptsPPR + proj.ProjFpts) / 2.0

		// Attach meta
		_ = meta

		projs = append(projs, proj)
	}

	log.Printf("  computed %d projections", len(projs))

	// 6. Upsert projections.
	log.Println("  upserting projections…")
	upserted := 0
	for _, p := range projs {
		compsJSON, err := json.Marshal(p.Comps)
		if err != nil {
			log.Printf("  marshal comps %s: %v", p.GsisID, err)
			compsJSON = []byte("[]")
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO nfl_projections (
				gsis_id, base_season, target_season,
				proj_fpts_pg, proj_fpts_ppr_pg,
				proj_pass_yds_pg, proj_pass_td_pg,
				proj_rush_yds_pg, proj_rush_td_pg,
				proj_rec_pg, proj_rec_yds_pg, proj_rec_td_pg,
				proj_fg_made_pg, proj_pat_made_pg,
				proj_games, proj_fpts, proj_fpts_ppr, proj_fpts_half,
				confidence, conf_similarity, conf_comp_count, conf_agreement,
				conf_sample_depth, conf_data_quality,
				comp_count, avg_similarity, uniqueness,
				comps, computed_at
			) VALUES (
				$1,$2,$3,
				$4,$5,
				$6,$7,
				$8,$9,
				$10,$11,$12,
				$13,$14,
				$15,$16,$17,$18,
				$19,$20,$21,$22,
				$23,$24,
				$25,$26,$27,
				$28, NOW()
			)
			ON CONFLICT (gsis_id, base_season, target_season) DO UPDATE SET
				proj_fpts_pg = EXCLUDED.proj_fpts_pg,
				proj_fpts_ppr_pg = EXCLUDED.proj_fpts_ppr_pg,
				proj_pass_yds_pg = EXCLUDED.proj_pass_yds_pg,
				proj_pass_td_pg = EXCLUDED.proj_pass_td_pg,
				proj_rush_yds_pg = EXCLUDED.proj_rush_yds_pg,
				proj_rush_td_pg = EXCLUDED.proj_rush_td_pg,
				proj_rec_pg = EXCLUDED.proj_rec_pg,
				proj_rec_yds_pg = EXCLUDED.proj_rec_yds_pg,
				proj_rec_td_pg = EXCLUDED.proj_rec_td_pg,
				proj_fg_made_pg = EXCLUDED.proj_fg_made_pg,
				proj_pat_made_pg = EXCLUDED.proj_pat_made_pg,
				proj_games = EXCLUDED.proj_games,
				proj_fpts = EXCLUDED.proj_fpts,
				proj_fpts_ppr = EXCLUDED.proj_fpts_ppr,
				proj_fpts_half = EXCLUDED.proj_fpts_half,
				confidence = EXCLUDED.confidence,
				conf_similarity = EXCLUDED.conf_similarity,
				conf_comp_count = EXCLUDED.conf_comp_count,
				conf_agreement = EXCLUDED.conf_agreement,
				conf_sample_depth = EXCLUDED.conf_sample_depth,
				conf_data_quality = EXCLUDED.conf_data_quality,
				comp_count = EXCLUDED.comp_count,
				avg_similarity = EXCLUDED.avg_similarity,
				uniqueness = EXCLUDED.uniqueness,
				comps = EXCLUDED.comps,
				computed_at = NOW()
		`,
			p.GsisID, p.BaseSeason, p.TargetSeason,
			p.ProjFptsPG, p.ProjFptsPPRPG,
			p.ProjPassYdsPG, p.ProjPassTdPG,
			p.ProjRushYdsPG, p.ProjRushTdPG,
			p.ProjRecPG, p.ProjRecYdsPG, p.ProjRecTdPG,
			p.ProjFgMadePG, p.ProjPatMadePG,
			p.ProjGames, p.ProjFpts, p.ProjFptsPPR, p.ProjFptsHalf,
			p.Confidence, p.ConfSimilarity, p.ConfCompCount, p.ConfAgreement,
			p.ConfSampleDepth, p.ConfDataQuality,
			p.CompCount, p.AvgSimilarity, p.Uniqueness,
			string(compsJSON),
		)
		if err != nil {
			log.Printf("  proj upsert %s: %v", p.GsisID, err)
			continue
		}
		upserted++
	}
	log.Printf("  upserted %d projections", upserted)
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func loadAllProfiles(ctx context.Context, pool *pgxpool.Pool) ([]seasonProfile, error) {
	rows, err := pool.Query(ctx, `
		SELECT id, gsis_id, season,
		       COALESCE(age, 0), COALESCE(years_exp, 0),
		       draft_number, position_group, games_played,
		       height, weight,
		       pass_att_pg, pass_yds_pg, pass_td_pg, int_pg,
		       rush_att_pg, rush_yds_pg, rush_td_pg,
		       targets_pg, rec_pg, rec_yds_pg, rec_td_pg,
		       fpts_pg, fpts_ppr_pg, fg_made_pg, pat_made_pg,
		       pass_ypa, comp_pct, rush_ypc, rec_ypr,
		       target_share, wopr, pass_epa_play, rush_epa_play, rec_epa_play,
		       rush_yard_share,
		       sacks_pg, passing_air_yards_pg, passing_yac_pg, air_yards_share,
		       rushing_first_downs_pg, receiving_air_yards_pg, receiving_yac_pg,
		       receiving_first_downs_pg, fumbles_pg, fg_pct,
		       team_fpts_pg, team_pass_yds_pg, team_rush_yds_pg,
		       z_scores::text
		FROM nfl_player_season_profiles
		ORDER BY gsis_id, season
	`)
	if err != nil {
		return nil, fmt.Errorf("load profiles: %w", err)
	}
	defer rows.Close()

	var profiles []seasonProfile
	for rows.Next() {
		var p seasonProfile
		var zJSON string
		if err := rows.Scan(
			&p.ID, &p.GsisID, &p.Season,
			&p.Age, &p.YearsExp,
			&p.DraftNumber, &p.PositionGroup, &p.GamesPlayed,
			&p.Height, &p.Weight,
			&p.PassAttPG, &p.PassYdsPG, &p.PassTdPG, &p.IntPG,
			&p.RushAttPG, &p.RushYdsPG, &p.RushTdPG,
			&p.TargetsPG, &p.RecPG, &p.RecYdsPG, &p.RecTdPG,
			&p.FptsPG, &p.FptsPPRPG, &p.FgMadePG, &p.PatMadePG,
			&p.PassYPA, &p.CompPct, &p.RushYPC, &p.RecYPR,
			&p.TargetShare, &p.WOPR, &p.PassEPAPlay, &p.RushEPAPlay, &p.RecEPAPlay,
			&p.RushYardShare,
			&p.SacksPG, &p.PassingAirYardsPG, &p.PassingYACPG, &p.AirYardsShare,
			&p.RushingFirstDownsPG, &p.ReceivingAirYardsPG, &p.ReceivingYACPG,
			&p.ReceivingFirstDownsPG, &p.FumblesPG, &p.FgPct,
			&p.TeamFptsPG, &p.TeamPassYdsPG, &p.TeamRushYdsPG,
			&zJSON,
		); err != nil {
			return nil, fmt.Errorf("scan profile: %w", err)
		}
		_ = json.Unmarshal([]byte(zJSON), &p.ZScores)
		profiles = append(profiles, p)
	}
	return profiles, rows.Err()
}

func loadPlayerMeta(ctx context.Context, pool *pgxpool.Pool) (map[string]playerMeta, error) {
	rows, err := pool.Query(ctx, `
		SELECT gsis_id, name, position, position_group, team, headshot_url,
		       EXTRACT(YEAR FROM birth_date)::int,
		       height, weight, years_exp, entry_year, rookie_year, draft_number
		FROM nfl_players
	`)
	if err != nil {
		return nil, fmt.Errorf("load player meta: %w", err)
	}
	defer rows.Close()

	result := make(map[string]playerMeta)
	for rows.Next() {
		var m playerMeta
		var birthYear *int
		if err := rows.Scan(
			&m.GsisID, &m.Name, &m.Position, &m.PositionGroup, &m.Team, &m.HeadshotURL,
			&birthYear, &m.Height, &m.Weight, &m.YearsExp, &m.EntryYear, &m.RookieYear, &m.DraftNumber,
		); err != nil {
			return nil, fmt.Errorf("scan meta: %w", err)
		}
		m.BirthYear = birthYear
		result[m.GsisID] = m
	}
	return result, rows.Err()
}

// computeSimilarity computes weighted Euclidean similarity (0–1) between two profiles.
// Each dimGroup contributes one averaged z-score to the distance, so the group weight
// is invariant to how many stats feed it.
func computeSimilarity(target, cand *seasonProfile, groups []dimGroup) float64 {
	if len(target.ZScores) == 0 || len(cand.ZScores) == 0 {
		return 0
	}

	var weightedSumSq float64
	var totalWeight float64
	for _, g := range groups {
		tz, tok := avgGroupZScore(target.ZScores, g.fields)
		cz, cok := avgGroupZScore(cand.ZScores, g.fields)
		if !tok || !cok {
			continue
		}
		diff := tz - cz
		weightedSumSq += g.weight * diff * diff
		totalWeight += g.weight
	}
	if totalWeight == 0 {
		return 0
	}

	// Normalise by total weight so different group counts are comparable
	normDist := math.Sqrt(weightedSumSq / totalWeight)
	return 1.0 / (1.0 + normDist)
}

// buildPreMatch extracts a comp's career seasons before the match season (sorted ascending).
// Always returns a non-nil slice so JSON serialises as [] not null.
func buildPreMatch(cand *seasonProfile, candSeasons map[int]*seasonProfile, matchSeason int) []trajPoint {
	points := []trajPoint{}
	if len(candSeasons) == 0 {
		return points
	}
	// Collect all seasons before the match season
	for s := matchSeason - 15; s < matchSeason; s++ {
		p, ok := candSeasons[s]
		if !ok {
			continue
		}
		growth := 1.0
		if cand.FptsPPRPG > 0 {
			growth = p.FptsPPRPG / cand.FptsPPRPG
		}
		points = append(points, trajPoint{
			Season:    s,
			Age:       p.Age,
			FptsPPRPG: p.FptsPPRPG,
			FptsPG:    p.FptsPG,
			Growth:    growth,
		})
	}
	return points
}

// buildTrajectory extracts a comp's future season profiles starting from matchSeason+1.
// Always returns a non-nil slice (may be empty) so JSON serialises as [] not null.
func buildTrajectory(cand *seasonProfile, candSeasons map[int]*seasonProfile, matchSeason int) []trajPoint {
	points := []trajPoint{} // non-nil so JSON → [] not null
	if len(candSeasons) == 0 {
		return points
	}
	for s := matchSeason + 1; s <= matchSeason+5; s++ {
		p, ok := candSeasons[s]
		if !ok {
			continue
		}
		growth := 1.0
		if cand.FptsPPRPG > 0 {
			growth = p.FptsPPRPG / cand.FptsPPRPG
		}
		points = append(points, trajPoint{
			Season:    s,
			Age:       p.Age,
			FptsPPRPG: p.FptsPPRPG,
			FptsPG:    p.FptsPG,
			Growth:    growth,
		})
	}
	return points
}

// computeWeightedProjection projects the target's next-season stats using comp trajectories.
// agingMult is applied as a post-hoc adjustment based on position-specific career phase.
func computeWeightedProjection(target *seasonProfile, comps []compResult, targetSeason int, agingMult float64) projection {
	var proj projection

	// For each stat, compute weighted average growth rate from year-1 trajectory
	type statGetter func(c *compResult) (growth float64, hasData bool)

	applyGrowth := func(currentVal float64, getter func(c *compResult) (float64, bool)) float64 {
		if len(comps) == 0 {
			return currentVal // no comps: no change
		}
		var weightedGrowth float64
		var totalWeight float64
		for i := range comps {
			g, ok := getter(&comps[i])
			if !ok {
				continue
			}
			// Cap/floor growth rate
			if g > maxGrowthCap { g = maxGrowthCap }
			if g < minGrowthFloor { g = minGrowthFloor }
			weightedGrowth += comps[i].Weight * g
			totalWeight += comps[i].Weight
		}
		if totalWeight == 0 {
			return currentVal
		}
		// Renormalise weight
		growthRate := weightedGrowth / totalWeight
		return currentVal * growthRate
	}

	// Year-1 growth from trajectory (first point)
	getGrowth := func(c *compResult, statIdx int) (float64, bool) {
		if len(c.Trajectory) == 0 {
			return 0, false
		}
		pt := c.Trajectory[0]
		// statIdx: 0=fpts_ppr_pg, 1=fpts_pg
		switch statIdx {
		case 0:
			if c.MatchProfile["fpts_ppr_pg"] == 0 { return 1.0, true }
			return clampGrowth(pt.FptsPPRPG / c.MatchProfile["fpts_ppr_pg"]), true
		case 1:
			if c.MatchProfile["fpts_pg"] == 0 { return 1.0, true }
			return clampGrowth(pt.FptsPG / c.MatchProfile["fpts_pg"]), true
		}
		return pt.Growth, true
	}

	proj.ProjFptsPPRPG = applyGrowth(target.FptsPPRPG, func(c *compResult) (float64, bool) {
		return getGrowth(c, 0)
	})
	proj.ProjFptsPG = applyGrowth(target.FptsPG, func(c *compResult) (float64, bool) {
		return getGrowth(c, 1)
	})

	// Apply position-specific aging curve adjustment
	proj.ProjFptsPPRPG *= agingMult
	proj.ProjFptsPG *= agingMult

	// For position-specific stats, derive from fpts growth ratio
	growthRatioPPR := 1.0
	if target.FptsPPRPG > 0 && proj.ProjFptsPPRPG > 0 {
		growthRatioPPR = proj.ProjFptsPPRPG / target.FptsPPRPG
	}
	growthRatioPPR = clampGrowth(growthRatioPPR)

	proj.ProjPassYdsPG = target.PassYdsPG * growthRatioPPR
	proj.ProjPassTdPG = target.PassTdPG * growthRatioPPR
	proj.ProjRushYdsPG = target.RushYdsPG * growthRatioPPR
	proj.ProjRushTdPG = target.RushTdPG * growthRatioPPR
	proj.ProjRecPG = target.RecPG * growthRatioPPR
	proj.ProjRecYdsPG = target.RecYdsPG * growthRatioPPR
	proj.ProjRecTdPG = target.RecTdPG * growthRatioPPR
	proj.ProjFgMadePG = target.FgMadePG * growthRatioPPR
	proj.ProjPatMadePG = target.PatMadePG * growthRatioPPR

	return proj
}

func clampGrowth(g float64) float64 {
	if g > maxGrowthCap { return maxGrowthCap }
	if g < minGrowthFloor { return minGrowthFloor }
	return g
}

func clamp(v, lo, hi float64) float64 {
	if v < lo { return lo }
	if v > hi { return hi }
	return v
}

func computeCompAgreement(comps []compResult) float64 {
	if len(comps) < 2 {
		return 1.0
	}
	var growths []float64
	for _, c := range comps {
		if len(c.Trajectory) == 0 {
			continue
		}
		growths = append(growths, c.Trajectory[0].Growth)
	}
	if len(growths) < 2 {
		return 1.0
	}
	mu := mean(growths)
	sd := stdev(growths, mu)
	return 1.0 / (1.0 + sd)
}

func computeSampleDepth(comps []compResult) float64 {
	if len(comps) == 0 {
		return 0
	}
	withData := 0
	for _, c := range comps {
		if len(c.Trajectory) > 0 {
			withData++
		}
	}
	return float64(withData) / float64(len(comps))
}

func uniquenessLabel(count int, avgSim float64) string {
	switch {
	case count == 0:
		return "unique"
	case count <= rareProfileMax:
		return "rare"
	case count < commonArchetype:
		return "moderate"
	default:
		if avgSim >= 0.70 {
			return "common"
		}
		return "moderate"
	}
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}

func stdev(vals []float64, mu float64) float64 {
	if len(vals) < 2 {
		return 0
	}
	var sumSq float64
	for _, v := range vals {
		d := v - mu
		sumSq += d * d
	}
	return math.Sqrt(sumSq / float64(len(vals)))
}

func abs(a int) int {
	if a < 0 { return -a }
	return a
}
