package espnlive

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
)

const pollInterval = 90 * time.Second

// Run starts the live-stats poller and blocks until ctx is cancelled. Meant
// to be started as `go espnlive.Run(ctx, pool)` once, from cmd/api, only
// when Config.LiveStatsPoll is set.
//
// This is the first long-running background goroutine in this codebase —
// everything else is either request-scoped or a separate cmd/ binary run
// out-of-band. Each tick is wrapped in its own recover() so a bad ESPN
// response (malformed JSON, an unexpected field, a network blip) logs and
// the loop keeps going, rather than taking down the whole API server.
func Run(ctx context.Context, db *pgxpool.Pool) {
	log.Println("espnlive: live-stats poller started")
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("espnlive: live-stats poller stopped")
			return
		case <-ticker.C:
			if !isGameWindow(time.Now().UTC()) {
				continue
			}
			tick(ctx, db)
		}
	}
}

// tick runs one poll pass with a panic guard, so a single bad response never
// crashes the server (there's no per-goroutine equivalent of
// middleware.Recoverer for a background loop like this one).
func tick(ctx context.Context, db *pgxpool.Pool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("espnlive: recovered from panic: %v", r)
		}
	}()
	if err := pollOnce(ctx, db); err != nil {
		log.Printf("espnlive: poll failed: %v", err)
	}
}

// pollOnce fetches the current scoreboard and refreshes live stats for every
// game that's currently in progress.
func pollOnce(ctx context.Context, db *pgxpool.Pool) error {
	events, err := fetchScoreboard(ctx)
	if err != nil {
		return err
	}

	for _, ev := range events {
		if ev.state() != "in" {
			continue
		}
		if err := processGame(ctx, db, ev); err != nil {
			// Log and keep going — one bad game shouldn't stop the others.
			log.Printf("espnlive: game %s failed: %v", ev.ID, err)
		}
	}
	return nil
}

// processGame fetches one live game's box score, maps every player's stat
// line into canonical totals, and upserts them into nfl_live_player_stats.
func processGame(ctx context.Context, db *pgxpool.Pool, ev scoreboardEvent) error {
	teams, err := fetchBoxscore(ctx, ev.ID)
	if err != nil {
		return err
	}

	perAthlete := map[string]map[scoring.CanonicalStat]float64{}
	for _, team := range teams {
		for _, cat := range team.Statistics {
			for _, a := range cat.Athletes {
				id := a.Athlete.ID
				if id == "" {
					continue
				}
				totals, ok := perAthlete[id]
				if !ok {
					totals = map[scoring.CanonicalStat]float64{}
					perAthlete[id] = totals
				}
				addCanonicalStats(totals, cat.Name, cat.Keys, a.Stats)
			}
		}
	}
	if len(perAthlete) == 0 {
		return nil
	}

	espnIDs := make([]string, 0, len(perAthlete))
	for id := range perAthlete {
		espnIDs = append(espnIDs, id)
	}
	gsisByEspn, err := espnToGsis(ctx, db, espnIDs)
	if err != nil {
		return err
	}

	for espnID, totals := range perAthlete {
		gsisID, ok := gsisByEspn[espnID]
		if !ok {
			continue // not a player nflverse/our nfl_players table tracks
		}
		finalizeFGDistribution(totals)
		if err := upsertLiveStats(ctx, db, gsisID, ev.Season.Year, ev.Week.Number, totals); err != nil {
			log.Printf("espnlive: upsert failed for %s: %v", gsisID, err)
		}
	}
	return nil
}

func upsertLiveStats(ctx context.Context, db *pgxpool.Pool, gsisID string, season, week int, totals map[scoring.CanonicalStat]float64) error {
	flat := make(map[string]float64, len(totals))
	for k, v := range totals {
		flat[string(k)] = v
	}
	raw, err := json.Marshal(flat)
	if err != nil {
		return err
	}
	_, err = db.Exec(ctx, `
		INSERT INTO nfl_live_player_stats (gsis_id, season, week, stats, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (gsis_id, season, week)
		DO UPDATE SET stats = EXCLUDED.stats, updated_at = NOW()
	`, gsisID, season, week, raw)
	return err
}

// isGameWindow is a deliberately approximate heuristic for "NFL games are
// probably being played right now" — Thursday/Sunday/Monday evenings ET,
// with generous padding on both ends wide enough to cover both EDT and EST
// (DST isn't tracked precisely; erring toward polling a bit more rather than
// missing a window is the safe direction for a best-effort preview feature).
// Occasional early international/Saturday games fall outside it — an
// accepted gap, not a correctness bug: worst case, the live preview for
// those games simply starts a little late.
func isGameWindow(t time.Time) bool {
	minuteOfWeek := int(t.Weekday())*24*60 + t.Hour()*60 + t.Minute()
	for _, w := range gameWindows {
		if minuteOfWeek >= w.start && minuteOfWeek < w.end {
			return true
		}
	}
	return false
}

type window struct{ start, end int } // minutes since Sunday 00:00 UTC

var gameWindows = []window{
	{start: 16*60 + 30, end: 24*60 + 5*60},           // Sun 16:30 UTC -> Mon 05:00 UTC
	{start: 4*1440 + 23*60, end: 5*1440 + 5*60},      // Thu 23:00 UTC -> Fri 05:00 UTC
	{start: 1*1440 + 23*60 + 45, end: 2*1440 + 5*60}, // Mon 23:45 UTC -> Tue 05:00 UTC
}
