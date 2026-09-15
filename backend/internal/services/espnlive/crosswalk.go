package espnlive

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// espnToGsis resolves a batch of ESPN athlete ids to gsis_id via
// nfl_players.espn_id, populated from nflverse's own roster CSV
// (cmd/import's importRosters) — confirmed to match ESPN's athlete ids
// verbatim, no format conversion needed. Athletes with no match (not on an
// NFL roster nflverse tracks, or a not-yet-synced espn_id) are simply
// absent from the result and skipped by the caller.
func espnToGsis(ctx context.Context, db *pgxpool.Pool, espnIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(espnIDs) == 0 {
		return out, nil
	}

	rows, err := db.Query(ctx,
		"SELECT espn_id, gsis_id FROM nfl_players WHERE espn_id = ANY($1)",
		espnIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("espn->gsis crosswalk: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var espnID, gsisID string
		if err := rows.Scan(&espnID, &gsisID); err != nil {
			return nil, fmt.Errorf("scan crosswalk row: %w", err)
		}
		out[espnID] = gsisID
	}
	return out, rows.Err()
}
