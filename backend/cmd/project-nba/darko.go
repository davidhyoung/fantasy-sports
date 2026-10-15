// darko.go imports DARKO (darko.app) player projections into
// nba_darko_projections, the projection source for native-NBA auction values
// (internal/handlers/nba_draft_values.go). DARKO has no API this backend can
// call, so the data is snapshotted to a compact JSON file first:
//
//	{"columns": [...], "rows": [[...], ...]}
//
// where columns names darkoColumns' fields in order. Rows that can't be
// matched to an nba_players.espn_id are kept with espn_id = NULL.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
)

type darkoFile struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

var darkoNumeric = []string{
	"darko_id", "age", "x_minutes", "x_pace", "pts_100", "ast_100", "orb_100", "drb_100",
	"stl_100", "blk_100", "tov_100", "fga_100", "fg3a_100", "fta_100", "fg_pct", "fg3_pct", "ft_pct",
}

// darkoNameAliases maps DARKO's (NBA.com) legal names onto the names ESPN
// uses, keyed by normalizePlayerName output. Add to it when an import logs an
// "unmatched" player whose ESPN spelling differs.
var darkoNameAliases = map[string]string{
	"nicolas claxton":    "nic claxton",
	"david jones":        "david jones garcia",
	"alexandre sarr":     "alex sarr",
	"carlton carrington": "bub carrington",
}

// loadRecentNameIndex maps normalized name -> espn_ids of players with real
// stats in the last two seasons, so a retired namesake can't shadow an active
// player. A name that is still ambiguous among active players is left
// unmatched rather than guessed.
func loadRecentNameIndex(ctx context.Context, pool *pgxpool.Pool, season int) (map[string][]string, error) {
	rows, err := pool.Query(ctx, `
		SELECT p.espn_id, p.name FROM nba_players p
		WHERE EXISTS (SELECT 1 FROM nba_player_stats s WHERE s.espn_id = p.espn_id AND s.season >= $1)
	`, season-1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	idx := map[string][]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		n := normalizePlayerName(name)
		idx[n] = append(idx[n], id)
	}
	return idx, rows.Err()
}

func importDarko(ctx context.Context, pool *pgxpool.Pool, path string, season int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var f darkoFile
	if err := json.Unmarshal(data, &f); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	col := map[string]int{}
	for i, c := range f.Columns {
		col[c] = i
	}
	for _, c := range append([]string{"name", "team", "position", "snapshot_date"}, darkoNumeric...) {
		if _, ok := col[c]; !ok {
			return fmt.Errorf("%s: missing column %q", path, c)
		}
	}

	idx, err := loadRecentNameIndex(ctx, pool, season)
	if err != nil {
		return fmt.Errorf("load name index: %w", err)
	}

	var matched, unmatched, failed int
	for _, row := range f.Rows {
		if len(row) != len(f.Columns) {
			failed++
			continue
		}
		str := func(c string) string { s, _ := row[col[c]].(string); return s }
		num := func(c string) float64 { v, _ := row[col[c]].(float64); return v }

		name := str("name")
		var espnArg any
		key := normalizePlayerName(name)
		if alias, ok := darkoNameAliases[key]; ok {
			key = alias
		}
		if ids := idx[key]; len(ids) == 1 {
			espnArg = ids[0]
			matched++
		} else {
			unmatched++
			log.Printf("  unmatched: %s (%s) — %d candidates", name, str("team"), len(ids))
		}
		snap, err := parseDate(str("snapshot_date"))
		if err != nil || snap == nil {
			failed++
			continue
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO nba_darko_projections
				(darko_id, season, espn_id, player_name, team, position, age, snapshot_date,
				 x_minutes, x_pace, pts_100, ast_100, orb_100, drb_100, stl_100, blk_100, tov_100,
				 fga_100, fg3a_100, fta_100, fg_pct, fg3_pct, ft_pct)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
			ON CONFLICT (darko_id, season) DO UPDATE SET
				espn_id = EXCLUDED.espn_id, player_name = EXCLUDED.player_name, team = EXCLUDED.team,
				position = EXCLUDED.position, age = EXCLUDED.age, snapshot_date = EXCLUDED.snapshot_date,
				x_minutes = EXCLUDED.x_minutes, x_pace = EXCLUDED.x_pace, pts_100 = EXCLUDED.pts_100,
				ast_100 = EXCLUDED.ast_100, orb_100 = EXCLUDED.orb_100, drb_100 = EXCLUDED.drb_100,
				stl_100 = EXCLUDED.stl_100, blk_100 = EXCLUDED.blk_100, tov_100 = EXCLUDED.tov_100,
				fga_100 = EXCLUDED.fga_100, fg3a_100 = EXCLUDED.fg3a_100, fta_100 = EXCLUDED.fta_100,
				fg_pct = EXCLUDED.fg_pct, fg3_pct = EXCLUDED.fg3_pct, ft_pct = EXCLUDED.ft_pct,
				imported_at = NOW()
		`, int64(num("darko_id")), season, espnArg, name, str("team"), str("position"), num("age"), *snap,
			num("x_minutes"), num("x_pace"), num("pts_100"), num("ast_100"), num("orb_100"), num("drb_100"),
			num("stl_100"), num("blk_100"), num("tov_100"), num("fga_100"), num("fg3a_100"), num("fta_100"),
			num("fg_pct"), num("fg3_pct"), num("ft_pct"))
		if err != nil {
			log.Printf("  import darko row %q: %v", name, err)
			failed++
		}
	}
	log.Printf("  imported %d DARKO rows: %d matched to espn_id, %d unmatched, %d failed",
		len(f.Rows), matched, unmatched, failed)
	return nil
}

// darkoExtrasEntry is one player's comps and trend in the -import-darko-extras
// file: {"<darko_id>": {"c": [["Name",season,age,sim],...], "t": [[season,dpm],...]}}
// — comps are DARKO's nearest historical player-seasons, t is season-end DPM.
type darkoExtrasEntry struct {
	C [][]any `json:"c"`
	T [][]any `json:"t"`
}

// importDarkoExtras attaches comps and trend to already-imported
// nba_darko_projections rows (matched on darko_id + season).
func importDarkoExtras(ctx context.Context, pool *pgxpool.Pool, path string, season int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var in map[string]darkoExtrasEntry
	if err := json.Unmarshal(data, &in); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	f := func(v any) float64 { x, _ := v.(float64); return x }
	var updated, missing int
	for idStr, e := range in {
		var id int64
		if _, err := fmt.Sscan(idStr, &id); err != nil {
			continue
		}
		comps := make([]map[string]any, 0, len(e.C))
		for _, c := range e.C {
			if len(c) < 4 {
				continue
			}
			name, _ := c[0].(string)
			comps = append(comps, map[string]any{"name": name, "season": int(f(c[1])), "age": f(c[2]), "similarity": f(c[3])})
		}
		trend := make([]map[string]any, 0, len(e.T))
		for _, t := range e.T {
			if len(t) < 2 {
				continue
			}
			trend = append(trend, map[string]any{"season": int(f(t[0])), "dpm": f(t[1])})
		}
		cj, _ := json.Marshal(comps)
		tj, _ := json.Marshal(trend)
		tag, err := pool.Exec(ctx, `UPDATE nba_darko_projections SET comps = $3, trend = $4 WHERE darko_id = $1 AND season = $2`, id, season, cj, tj)
		if err != nil {
			return fmt.Errorf("update %d: %w", id, err)
		}
		if tag.RowsAffected() == 0 {
			missing++
		} else {
			updated++
		}
	}
	log.Printf("  attached comps/trend to %d players (%d ids had no projection row)", updated, missing)
	return nil
}
