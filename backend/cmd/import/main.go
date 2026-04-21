// cmd/import downloads nflverse CSV data (player rosters + weekly stats) and
// upserts it into the nfl_players / nfl_player_stats tables.
//
// Usage:
//
//	go run ./cmd/import                     # imports 2020-2024 (default)
//	go run ./cmd/import -from 2015 -to 2024 # custom range
//	go run ./cmd/import -rosters-only       # just player metadata
//	go run ./cmd/import -stats-only         # just weekly stats
package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"os"
)

const (
	rosterURL = "https://github.com/nflverse/nflverse-data/releases/download/rosters/roster_%d.csv"

	// Pre-2025 naming convention (player_stats release).
	statsURLLegacy = "https://github.com/nflverse/nflverse-data/releases/download/player_stats/player_stats_%d.csv"

	// 2025+ naming convention — nflverse reorganised into stats_player release with
	// renamed columns: team (was recent_team), passing_interceptions (was interceptions),
	// sacks_suffered (was sacks), sack_yards_lost (was sack_yards).
	statsURLNew = "https://github.com/nflverse/nflverse-data/releases/download/stats_player/stats_player_week_%d.csv"
)

func statsURL(year int) string {
	if year >= 2025 {
		return fmt.Sprintf(statsURLNew, year)
	}
	return fmt.Sprintf(statsURLLegacy, year)
}

func main() {
	fromYear := flag.Int("from", 2020, "first season to import")
	toYear := flag.Int("to", 2024, "last season to import")
	rostersOnly := flag.Bool("rosters-only", false, "import only roster/player data")
	statsOnly := flag.Bool("stats-only", false, "import only weekly stats")
	flag.Parse()

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

	for year := *fromYear; year <= *toYear; year++ {
		if !*statsOnly {
			log.Printf("importing rosters for %d…", year)
			if err := importRosters(ctx, pool, year); err != nil {
				log.Printf("  roster %d error: %v", year, err)
			}
		}
		if !*rostersOnly {
			log.Printf("importing stats for %d…", year)
			if err := importStats(ctx, pool, year); err != nil {
				log.Printf("  stats %d error: %v", year, err)
			}
		}
	}
	log.Println("done")
}

// fetchCSV downloads a CSV from the given URL and returns a map-based reader
// that yields rows as map[string]string keyed by header name.
func fetchCSV(url string) ([]map[string]string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch %s: status %d", url, resp.StatusCode)
	}

	r := csv.NewReader(resp.Body)
	r.LazyQuotes = true

	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	var rows []map[string]string
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read row: %w", err)
		}
		row := make(map[string]string, len(header))
		for i, col := range header {
			if i < len(record) {
				row[col] = record[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func importRosters(ctx context.Context, pool *pgxpool.Pool, year int) error {
	url := fmt.Sprintf(rosterURL, year)
	rows, err := fetchCSV(url)
	if err != nil {
		return err
	}

	// Only keep the latest week per player (highest week = most current data)
	latest := make(map[string]map[string]string)
	for _, row := range rows {
		id := row["gsis_id"]
		if id == "" || id == "NA" {
			continue
		}
		existing, ok := latest[id]
		if !ok || atoi(row["week"]) > atoi(existing["week"]) {
			latest[id] = row
		}
	}

	playerRows := make([]map[string]string, 0, len(latest))
	for _, row := range latest {
		playerRows = append(playerRows, row)
	}

	const rosterBatchSize = 200
	count := 0
	for i := 0; i < len(playerRows); i += rosterBatchSize {
		end := i + rosterBatchSize
		if end > len(playerRows) {
			end = len(playerRows)
		}
		chunk := playerRows[i:end]

		batch := &pgx.Batch{}
		for _, row := range chunk {
			batch.Queue(`
				INSERT INTO nfl_players (
					gsis_id, name, position, position_group, team,
					birth_date, height, weight, college,
					years_exp, entry_year, rookie_year,
					draft_club, draft_number, jersey_number, headshot_url,
					yahoo_id, espn_id, sportradar_id, rotowire_id, sleeper_id, pfr_id,
					updated_at
				) VALUES (
					$1, $2, $3, $4, $5,
					$6, $7, $8, $9,
					$10, $11, $12,
					$13, $14, $15, $16,
					$17, $18, $19, $20, $21, $22,
					NOW()
				)
				ON CONFLICT (gsis_id) DO UPDATE SET
					name = EXCLUDED.name,
					position = EXCLUDED.position,
					position_group = EXCLUDED.position_group,
					team = EXCLUDED.team,
					birth_date = COALESCE(EXCLUDED.birth_date, nfl_players.birth_date),
					height = COALESCE(EXCLUDED.height, nfl_players.height),
					weight = COALESCE(EXCLUDED.weight, nfl_players.weight),
					college = COALESCE(EXCLUDED.college, nfl_players.college),
					years_exp = EXCLUDED.years_exp,
					entry_year = COALESCE(EXCLUDED.entry_year, nfl_players.entry_year),
					rookie_year = COALESCE(EXCLUDED.rookie_year, nfl_players.rookie_year),
					draft_club = COALESCE(EXCLUDED.draft_club, nfl_players.draft_club),
					draft_number = COALESCE(EXCLUDED.draft_number, nfl_players.draft_number),
					jersey_number = EXCLUDED.jersey_number,
					headshot_url = COALESCE(EXCLUDED.headshot_url, nfl_players.headshot_url),
					yahoo_id = COALESCE(EXCLUDED.yahoo_id, nfl_players.yahoo_id),
					espn_id = COALESCE(EXCLUDED.espn_id, nfl_players.espn_id),
					sportradar_id = COALESCE(EXCLUDED.sportradar_id, nfl_players.sportradar_id),
					rotowire_id = COALESCE(EXCLUDED.rotowire_id, nfl_players.rotowire_id),
					sleeper_id = COALESCE(EXCLUDED.sleeper_id, nfl_players.sleeper_id),
					pfr_id = COALESCE(EXCLUDED.pfr_id, nfl_players.pfr_id),
					updated_at = NOW()
			`,
				row["gsis_id"],
				row["full_name"],
				nullIfEmpty(row["position"]),
				nullIfEmpty(row["depth_chart_position"]),
				nullIfEmpty(row["team"]),
				parseDate(row["birth_date"]),
				parseHeight(row["height"]),
				atoi_ptr(row["weight"]),
				nullIfEmpty(row["college"]),
				atoi_ptr(row["years_exp"]),
				atoi_ptr(row["entry_year"]),
				atoi_ptr(row["rookie_year"]),
				nullIfEmpty(row["draft_club"]),
				atoi_ptr(row["draft_number"]),
				atoi_ptr(row["jersey_number"]),
				nullIfEmpty(row["headshot_url"]),
				nullIfEmpty(row["yahoo_id"]),
				nullIfEmpty(row["espn_id"]),
				nullIfEmpty(row["sportradar_id"]),
				nullIfEmpty(row["rotowire_id"]),
				nullIfEmpty(row["sleeper_id"]),
				nullIfEmpty(row["pfr_id"]),
			)
		}

		br := pool.SendBatch(ctx, batch)
		for _, row := range chunk {
			if _, err := br.Exec(); err != nil {
				log.Printf("  roster upsert %s: %v", row["gsis_id"], err)
			} else {
				count++
			}
		}
		br.Close()
	}
	log.Printf("  upserted %d players from %d roster", count, year)
	return nil
}

func importStats(ctx context.Context, pool *pgxpool.Pool, year int) error {
	rows, err := fetchCSV(statsURL(year))
	if err != nil {
		return err
	}

	// Filter to valid rows once, then reuse the slice for both passes.
	valid := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		id := row["player_id"]
		if id == "" || id == "NA" {
			continue
		}
		valid = append(valid, row)
	}

	const statsBatchSize = 200

	// Ensure all referenced players exist (some stats may reference players
	// not in the roster file for that year, e.g. players cut mid-season).
	for i := 0; i < len(valid); i += statsBatchSize {
		end := i + statsBatchSize
		if end > len(valid) {
			end = len(valid)
		}
		chunk := valid[i:end]

		batch := &pgx.Batch{}
		for _, row := range chunk {
			batch.Queue(`
				INSERT INTO nfl_players (gsis_id, name, position, position_group, team, updated_at)
				VALUES ($1, $2, $3, $4, $5, NOW())
				ON CONFLICT (gsis_id) DO NOTHING
			`,
				row["player_id"],
				row["player_display_name"],
				nullIfEmpty(row["position"]),
				nullIfEmpty(row["position_group"]),
				nullIfEmpty(firstCol(row, "recent_team", "team")),
			)
		}

		br := pool.SendBatch(ctx, batch)
		for _, row := range chunk {
			if _, err := br.Exec(); err != nil {
				log.Printf("  player ensure %s: %v", row["player_id"], err)
			}
		}
		br.Close()
	}

	count := 0
	for i := 0; i < len(valid); i += statsBatchSize {
		end := i + statsBatchSize
		if end > len(valid) {
			end = len(valid)
		}
		chunk := valid[i:end]

		batch := &pgx.Batch{}
		for _, row := range chunk {
			batch.Queue(`
				INSERT INTO nfl_player_stats (
				gsis_id, season, week, season_type, team, opponent_team,
				completions, pass_attempts, passing_yards, passing_tds, interceptions,
				sacks, sack_yards, passing_air_yards, passing_yac,
				passing_first_downs, passing_epa, passing_2pt,
				carries, rushing_yards, rushing_tds,
				rushing_fumbles, rushing_fumbles_lost, rushing_first_downs,
				rushing_epa, rushing_2pt,
				receptions, targets, receiving_yards, receiving_tds,
				receiving_fumbles, receiving_fumbles_lost,
				receiving_air_yards, receiving_yac, receiving_first_downs,
				receiving_epa, receiving_2pt, target_share, wopr,
				fg_made, fg_att, fg_missed, fg_long, pat_made, pat_att,
				special_teams_tds, fantasy_points, fantasy_points_ppr
			) VALUES (
				$1,$2,$3,$4,$5,$6,
				$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,
				$19,$20,$21,$22,$23,$24,$25,$26,
				$27,$28,$29,$30,$31,$32,$33,$34,$35,$36,$37,$38,$39,
				$40,$41,$42,$43,$44,$45,$46,$47,$48
			)
			ON CONFLICT (gsis_id, season, week, season_type) DO UPDATE SET
				team = EXCLUDED.team,
				opponent_team = EXCLUDED.opponent_team,
				completions = EXCLUDED.completions,
				pass_attempts = EXCLUDED.pass_attempts,
				passing_yards = EXCLUDED.passing_yards,
				passing_tds = EXCLUDED.passing_tds,
				interceptions = EXCLUDED.interceptions,
				sacks = EXCLUDED.sacks,
				sack_yards = EXCLUDED.sack_yards,
				passing_air_yards = EXCLUDED.passing_air_yards,
				passing_yac = EXCLUDED.passing_yac,
				passing_first_downs = EXCLUDED.passing_first_downs,
				passing_epa = EXCLUDED.passing_epa,
				passing_2pt = EXCLUDED.passing_2pt,
				carries = EXCLUDED.carries,
				rushing_yards = EXCLUDED.rushing_yards,
				rushing_tds = EXCLUDED.rushing_tds,
				rushing_fumbles = EXCLUDED.rushing_fumbles,
				rushing_fumbles_lost = EXCLUDED.rushing_fumbles_lost,
				rushing_first_downs = EXCLUDED.rushing_first_downs,
				rushing_epa = EXCLUDED.rushing_epa,
				rushing_2pt = EXCLUDED.rushing_2pt,
				receptions = EXCLUDED.receptions,
				targets = EXCLUDED.targets,
				receiving_yards = EXCLUDED.receiving_yards,
				receiving_tds = EXCLUDED.receiving_tds,
				receiving_fumbles = EXCLUDED.receiving_fumbles,
				receiving_fumbles_lost = EXCLUDED.receiving_fumbles_lost,
				receiving_air_yards = EXCLUDED.receiving_air_yards,
				receiving_yac = EXCLUDED.receiving_yac,
				receiving_first_downs = EXCLUDED.receiving_first_downs,
				receiving_epa = EXCLUDED.receiving_epa,
				receiving_2pt = EXCLUDED.receiving_2pt,
				target_share = EXCLUDED.target_share,
				wopr = EXCLUDED.wopr,
				fg_made = EXCLUDED.fg_made,
				fg_att = EXCLUDED.fg_att,
				fg_missed = EXCLUDED.fg_missed,
				fg_long = EXCLUDED.fg_long,
				pat_made = EXCLUDED.pat_made,
				pat_att = EXCLUDED.pat_att,
				special_teams_tds = EXCLUDED.special_teams_tds,
				fantasy_points = EXCLUDED.fantasy_points,
				fantasy_points_ppr = EXCLUDED.fantasy_points_ppr
		`,
				row["player_id"],
				atoi(row["season"]),
				atoi(row["week"]),
				defaultStr(row["season_type"], "REG"),
				nullIfEmpty(firstCol(row, "recent_team", "team")),
				nullIfEmpty(row["opponent_team"]),
				atoi(row["completions"]),
				atoi(row["attempts"]),
				parseFloat(row["passing_yards"]),
				atoi(row["passing_tds"]),
				atoi(firstCol(row, "interceptions", "passing_interceptions")),
				atoi(firstCol(row, "sacks", "sacks_suffered")),
				parseFloat(firstCol(row, "sack_yards", "sack_yards_lost")),
				parseFloat(row["passing_air_yards"]),
				parseFloat(row["passing_yards_after_catch"]),
				atoi(row["passing_first_downs"]),
				parseFloat_ptr(row["passing_epa"]),
				atoi(row["passing_2pt_conversions"]),
				atoi(row["carries"]),
				parseFloat(row["rushing_yards"]),
				atoi(row["rushing_tds"]),
				atoi(row["rushing_fumbles"]),
				atoi(row["rushing_fumbles_lost"]),
				atoi(row["rushing_first_downs"]),
				parseFloat_ptr(row["rushing_epa"]),
				atoi(row["rushing_2pt_conversions"]),
				atoi(row["receptions"]),
				atoi(row["targets"]),
				parseFloat(row["receiving_yards"]),
				atoi(row["receiving_tds"]),
				atoi(row["receiving_fumbles"]),
				atoi(row["receiving_fumbles_lost"]),
				parseFloat(row["receiving_air_yards"]),
				parseFloat(row["receiving_yards_after_catch"]),
				atoi(row["receiving_first_downs"]),
				parseFloat_ptr(row["receiving_epa"]),
				atoi(row["receiving_2pt_conversions"]),
				parseFloat_ptr(row["target_share"]),
				parseFloat_ptr(row["wopr"]),
				atoi(row["fg_made"]),
				atoi(row["fg_att"]),
				atoi(row["fg_missed"]),
				atoi(row["fg_long"]),
				atoi(row["pat_made"]),
				atoi(row["pat_att"]),
				atoi(row["special_teams_tds"]),
				parseFloat(row["fantasy_points"]),
				parseFloat(row["fantasy_points_ppr"]),
			)
		}

		br := pool.SendBatch(ctx, batch)
		for _, row := range chunk {
			if _, err := br.Exec(); err != nil {
				log.Printf("  stat upsert %s wk%s: %v", row["player_id"], row["week"], err)
			} else {
				count++
			}
		}
		br.Close()
	}
	log.Printf("  upserted %d stat rows for %d", count, year)
	return nil
}

// --- helpers ---

// firstCol returns the value of the first named column that is non-empty and non-"NA".
// Used to handle nflverse column renames between the legacy (≤2024) and new (≥2025) formats.
func firstCol(row map[string]string, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(row[name]); v != "" && v != "NA" {
			return v
		}
	}
	return ""
}

func atoi(s string) int {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" {
		return 0
	}
	// Handle floats like "167.0" by parsing as float first
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return int(f)
}

func atoi_ptr(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	v := int(f)
	return &v
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return f
}

func parseFloat_ptr(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" {
		return nil
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseDate(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return &t
}

func parseHeight(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" {
		return nil
	}
	// nflverse height is in inches as an integer string, e.g. "76"
	v, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &v
}

func nullIfEmpty(s string) *string {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" {
		return nil
	}
	return &s
}

func defaultStr(s, def string) string {
	s = strings.TrimSpace(s)
	if s == "" || s == "NA" {
		return def
	}
	return s
}
