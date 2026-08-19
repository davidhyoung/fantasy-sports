package leaguesettings

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/ranking"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
)

// NativeSource resolves a league's settings from its own league_settings row —
// no external league behind it at all. RosterPositions and ScoringMods query
// independently (mirroring YahooSource's two independent Yahoo calls) so
// FetchSettings can still fetch both concurrently.
type NativeSource struct {
	db       *pgxpool.Pool
	leagueID int64
}

// NewNativeSource builds a Source backed by a native league's own settings row.
func NewNativeSource(db *pgxpool.Pool, leagueID int64) *NativeSource {
	return &NativeSource{db: db, leagueID: leagueID}
}

func (s *NativeSource) RosterPositions(ctx context.Context) ([]ranking.RosterPosition, error) {
	var raw []byte
	err := s.db.QueryRow(ctx,
		"SELECT slots FROM league_settings WHERE league_id = $1", s.leagueID,
	).Scan(&raw)
	if err != nil {
		return nil, err
	}
	var slots map[string]int
	if err := json.Unmarshal(raw, &slots); err != nil {
		return nil, err
	}
	var positions []ranking.RosterPosition
	for name, count := range slots {
		if count <= 0 {
			continue
		}
		pos, ok := SlotToPosition[name]
		if !ok {
			continue
		}
		positions = append(positions, ranking.RosterPosition{Position: pos, Count: count})
	}
	return positions, nil
}

func (s *NativeSource) ScoringMods(ctx context.Context) (map[scoring.CanonicalStat]float64, bool) {
	var raw []byte
	err := s.db.QueryRow(ctx,
		"SELECT scoring FROM league_settings WHERE league_id = $1", s.leagueID,
	).Scan(&raw)
	if err != nil {
		return nil, false
	}
	var flat map[string]float64
	if err := json.Unmarshal(raw, &flat); err != nil || len(flat) == 0 {
		return nil, false
	}
	out := make(map[scoring.CanonicalStat]float64, len(flat))
	for k, v := range flat {
		out[scoring.CanonicalStat(k)] = v
	}
	return out, true
}

// Budget loads the league's configured auction budget per team. Returns 0, false
// if no settings row exists (e.g. a league that predates this table).
func Budget(ctx context.Context, db *pgxpool.Pool, leagueID int64) (int, bool) {
	var budget int
	err := db.QueryRow(ctx,
		"SELECT budget FROM league_settings WHERE league_id = $1", leagueID,
	).Scan(&budget)
	if err != nil {
		return 0, false
	}
	return budget, true
}
