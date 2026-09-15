package espnlive

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
)

// LoadLiveWeekStats reads this week's best-effort ESPN-derived stats for the
// given gsis_ids from nfl_live_player_stats — the live-preview sibling of
// nflstats.LoadWeekStats, same shape and same "absent = zero points"
// contract, just backed by the poller's isolated table instead of the real
// nflverse import. Players with no live row yet (game hasn't started, no
// stats recorded, or no live game this week) are simply absent.
func LoadLiveWeekStats(ctx context.Context, db *pgxpool.Pool, season, week int, gsisIDs []string) (map[string]map[scoring.CanonicalStat]float64, error) {
	out := map[string]map[scoring.CanonicalStat]float64{}
	if len(gsisIDs) == 0 {
		return out, nil
	}

	rows, err := db.Query(ctx, `
		SELECT gsis_id, stats FROM nfl_live_player_stats
		WHERE gsis_id = ANY($1) AND season = $2 AND week = $3
	`, gsisIDs, season, week)
	if err != nil {
		return nil, fmt.Errorf("load live week stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var gsisID string
		var raw []byte
		if err := rows.Scan(&gsisID, &raw); err != nil {
			return nil, fmt.Errorf("scan live week stats: %w", err)
		}
		var flat map[string]float64
		if err := json.Unmarshal(raw, &flat); err != nil {
			return nil, fmt.Errorf("unmarshal live stats for %s: %w", gsisID, err)
		}
		values := make(map[scoring.CanonicalStat]float64, len(flat))
		for k, v := range flat {
			values[scoring.CanonicalStat(k)] = v
		}
		out[gsisID] = values
	}
	return out, rows.Err()
}
