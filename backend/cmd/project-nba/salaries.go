// salaries.go imports one league's historical auction salaries into
// league_rosters_nba.salary — the real cost basis the keeper-cost formula
// (internal/services/keepers.ComputeNBAKeeperCost) reads. Unlike
// -import-consensus (external market data, global across every league),
// salary is inherently one league's own internal history, so this requires
// -league-id and writes against that league's existing roster rows rather
// than inserting new ones — a row must already exist at
// (league_id, season, espn_id) in league_rosters_nba for the update to land.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// nbaSalaryInput is one row of the -import-nba-salaries JSON file.
type nbaSalaryInput struct {
	FantasyTeam string `json:"fantasy_team"`
	PlayerName  string `json:"player_name"`
	NBATeam     string `json:"nba_team,omitempty"`
	Salary      *int   `json:"player_salary_2025,omitempty"`
}

func loadLeagueTeamIndex(ctx context.Context, pool *pgxpool.Pool, leagueID int64) (map[string]int64, error) {
	rows, err := pool.Query(ctx, `SELECT id, name FROM teams WHERE league_id = $1`, leagueID)
	if err != nil {
		return nil, fmt.Errorf("load league teams: %w", err)
	}
	defer rows.Close()

	idx := map[string]int64{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("scan team row: %w", err)
		}
		idx[strings.ToLower(strings.TrimSpace(name))] = id
	}
	return idx, rows.Err()
}

func importNBASalaries(ctx context.Context, pool *pgxpool.Pool, path string, leagueID int64, season int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var inputRows []nbaSalaryInput
	if err := json.Unmarshal(data, &inputRows); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	// Player identity resolution reuses the same playerIndex/resolve chain
	// consensus.go already proved works for this exact dataset (diacritic
	// stripping handles Jokić/Vučević/Şengün/Dëmin/Porziņģis/Diabaté).
	idx, err := loadPlayerIndex(ctx, pool)
	if err != nil {
		return err
	}
	teams, err := loadLeagueTeamIndex(ctx, pool, leagueID)
	if err != nil {
		return err
	}

	var matched, unmatchedPlayer, unmatchedTeam, noRosterRow, skipped int
	for _, r := range inputRows {
		if r.Salary == nil {
			skipped++ // no recorded salary (e.g. a mid-season waiver add with no cost on file)
			continue
		}
		if _, ok := teams[strings.ToLower(strings.TrimSpace(r.FantasyTeam))]; !ok {
			log.Printf("  skipping %q: unknown fantasy team %q in league %d", r.PlayerName, r.FantasyTeam, leagueID)
			unmatchedTeam++
			continue
		}
		espnID, ok := idx.resolve(r.PlayerName, "", r.NBATeam, "")
		if !ok {
			log.Printf("  skipping %q: no matching nba_players row", r.PlayerName)
			unmatchedPlayer++
			continue
		}
		tag, err := pool.Exec(ctx, `
			UPDATE league_rosters_nba SET salary = $1
			WHERE league_id = $2 AND season = $3 AND espn_id = $4
		`, *r.Salary, leagueID, season, espnID)
		if err != nil {
			log.Printf("  update salary for %q: %v", r.PlayerName, err)
			continue
		}
		if tag.RowsAffected() == 0 {
			log.Printf("  skipping %q: no league_rosters_nba row at league=%d season=%d", r.PlayerName, leagueID, season)
			noRosterRow++
			continue
		}
		matched++
	}
	log.Printf("  imported %d/%d salary rows: %d unmatched player, %d unmatched team, %d no roster row, %d skipped (no salary)",
		matched, len(inputRows), unmatchedPlayer, unmatchedTeam, noRosterRow, skipped)
	return nil
}
