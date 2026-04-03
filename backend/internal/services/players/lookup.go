package players

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// YahooKeyToNumericID extracts the numeric ID from a Yahoo player key like "nfl.p.30977".
func YahooKeyToNumericID(playerKey string) string {
	parts := strings.Split(playerKey, ".")
	if len(parts) == 3 {
		return parts[2]
	}
	return playerKey
}

// ResolveAllYahooToGsis loads the entire yahoo_id → gsis_id mapping from nfl_players.
// Returns a resolver function that accepts a full Yahoo player key (e.g. "nfl.p.30977")
// and returns the corresponding gsis_id, or "" if not found.
func ResolveAllYahooToGsis(ctx context.Context, db *pgxpool.Pool) func(playerKey string) string {
	m := map[string]string{}
	rows, err := db.Query(ctx, `SELECT yahoo_id, gsis_id FROM nfl_players WHERE yahoo_id IS NOT NULL`)
	if err != nil {
		return func(string) string { return "" }
	}
	defer rows.Close()
	for rows.Next() {
		var yid, gid string
		if rows.Scan(&yid, &gid) == nil {
			m[yid] = gid
		}
	}
	return func(playerKey string) string {
		return m[YahooKeyToNumericID(playerKey)]
	}
}

// ResolveBatchYahooToGsis loads yahoo_id → gsis_id mappings for a specific set of
// Yahoo player keys (uses ANY($1) for efficiency when the roster is small).
// Returns a map from numeric yahoo_id to gsis_id.
func ResolveBatchYahooToGsis(ctx context.Context, db *pgxpool.Pool, playerKeys []string) map[string]string {
	yahooIDs := make([]string, 0, len(playerKeys))
	for _, key := range playerKeys {
		yahooIDs = append(yahooIDs, YahooKeyToNumericID(key))
	}

	m := map[string]string{}
	if len(yahooIDs) == 0 {
		return m
	}

	rows, err := db.Query(ctx, `SELECT yahoo_id, gsis_id FROM nfl_players WHERE yahoo_id = ANY($1)`, yahooIDs)
	if err != nil {
		return m
	}
	defer rows.Close()
	for rows.Next() {
		var yid, gid string
		if rows.Scan(&yid, &gid) == nil {
			m[yid] = gid
		}
	}
	return m
}
