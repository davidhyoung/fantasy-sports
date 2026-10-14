// consensus.go is the NBA analog of cmd/projections/consensus.go: importing
// external consensus rankings/ADP, then diffing our own comp-based projection
// rank against them. Diagnostic only — never mutates nba_projections.
// See docs/stats/nba-consensus-rankings.md.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/unicode/norm"
)

// consensusRankingInput is one row of the -import-consensus JSON file.
type consensusRankingInput struct {
	PlayerName   string  `json:"player_name"`
	Position     string  `json:"position"`
	Team         string  `json:"team"`
	Season       int     `json:"season,omitempty"` // falls back to -season flag if omitted
	Source       string  `json:"source"`
	MetricType   string  `json:"metric_type"` // rank | adp
	Value        float64 `json:"value"`
	CapturedDate string  `json:"captured_date"` // YYYY-MM-DD
	ESPNID       string  `json:"espn_id,omitempty"`
}

// ── player identity resolution ──────────────────────────────────────────────
//
// nba_players.espn_id is the row's own primary key (not a crosswalk column
// the way nfl_players.espn_id is), so this resolver is simpler than the NFL
// one: an explicit espn_id on the input row (rare — few outlets publish it)
// resolves directly; otherwise normalized name+team, then normalized
// name+position (catches players traded since the source snapshot was
// captured). Unmatched rows are kept with espn_id = NULL for manual review,
// same convention as the NFL importer.

type playerIndex struct {
	byESPNID   map[string]string // identity map, confirms a given id is real
	byNameTeam map[string]string
	byNamePos  map[string]string
}

var suffixRE = regexp.MustCompile(`\b(jr|sr|ii|iii|iv|v)\.?$`)
var punctRE = regexp.MustCompile(`[.'’]`)
var spaceRE = regexp.MustCompile(`\s+`)

// stripDiacritics folds accented characters to their base ASCII form (e.g.
// "Dončić" -> "Doncic") — ported verbatim from cmd/projections/consensus.go,
// sport-agnostic.
func stripDiacritics(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func normalizePlayerName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = stripDiacritics(s)
	s = punctRE.ReplaceAllString(s, "")
	s = suffixRE.ReplaceAllString(strings.TrimSpace(s), "")
	s = spaceRE.ReplaceAllString(strings.TrimSpace(s), " ")
	return strings.TrimSpace(s)
}

func loadPlayerIndex(ctx context.Context, pool *pgxpool.Pool) (*playerIndex, error) {
	rows, err := pool.Query(ctx, `SELECT espn_id, name, position, team FROM nba_players`)
	if err != nil {
		return nil, fmt.Errorf("load player index: %w", err)
	}
	defer rows.Close()

	idx := &playerIndex{
		byESPNID:   make(map[string]string),
		byNameTeam: make(map[string]string),
		byNamePos:  make(map[string]string),
	}
	for rows.Next() {
		var espnID, name string
		var position, team *string
		if err := rows.Scan(&espnID, &name, &position, &team); err != nil {
			return nil, fmt.Errorf("scan player index row: %w", err)
		}
		idx.byESPNID[espnID] = espnID
		norm := normalizePlayerName(name)
		if team != nil && *team != "" {
			idx.byNameTeam[norm+"|"+strings.ToUpper(*team)] = espnID
		}
		if position != nil && *position != "" {
			idx.byNamePos[norm+"|"+strings.ToUpper(*position)] = espnID
		}
	}
	return idx, rows.Err()
}

// teamAliases maps common external-source team abbreviations onto the ones
// nba_players.team actually uses (ESPN's own convention — e.g. "GS" not
// "GSW", "NY" not "NYK", "WSH" not "WAS") — same purpose as NFL's own
// teamAliases in cmd/import/main.go, just a different alias set.
var teamAliases = map[string]string{
	"GSW": "GS", "NYK": "NY", "SAS": "SA", "UTA": "UTAH", "NOP": "NO",
	"WAS": "WSH", "BRK": "BKN", "PHO": "PHX",
}

func normalizeTeam(team string) string {
	team = strings.ToUpper(strings.TrimSpace(team))
	if alias, ok := teamAliases[team]; ok {
		return alias
	}
	return team
}

func (idx *playerIndex) resolve(name, position, team, espnID string) (string, bool) {
	if espnID != "" {
		if g, ok := idx.byESPNID[espnID]; ok {
			return g, true
		}
	}
	norm := normalizePlayerName(name)
	team = normalizeTeam(team)
	if team != "" {
		if g, ok := idx.byNameTeam[norm+"|"+strings.ToUpper(team)]; ok {
			return g, true
		}
	}
	if position != "" {
		if g, ok := idx.byNamePos[norm+"|"+strings.ToUpper(position)]; ok {
			return g, true
		}
	}
	return "", false
}

func parseDate(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil, fmt.Errorf("parse date %q: %w", s, err)
	}
	return &t, nil
}

// ── import ───────────────────────────────────────────────────────────────────

func importConsensusRankings(ctx context.Context, pool *pgxpool.Pool, path string, defaultSeason int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var inputRows []consensusRankingInput
	if err := json.Unmarshal(data, &inputRows); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}

	idx, err := loadPlayerIndex(ctx, pool)
	if err != nil {
		return err
	}

	var matched, unmatched, failed int
	for _, r := range inputRows {
		season := r.Season
		if season == 0 {
			season = defaultSeason
		}
		captured, err := parseDate(r.CapturedDate)
		if err != nil || captured == nil {
			log.Printf("  skipping %q: %v", r.PlayerName, err)
			failed++
			continue
		}

		espnID, ok := idx.resolve(r.PlayerName, r.Position, r.Team, r.ESPNID)
		var espnArg any
		if ok {
			espnArg = espnID
			matched++
		} else {
			espnArg = nil
			unmatched++
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO nba_consensus_rankings
				(espn_id, player_name_raw, position, team, season, source, metric_type, value, captured_date)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT (player_name_raw, season, source, metric_type, captured_date)
			DO UPDATE SET espn_id = EXCLUDED.espn_id, position = EXCLUDED.position,
			              team = EXCLUDED.team, value = EXCLUDED.value
		`, espnArg, r.PlayerName, r.Position, r.Team, season, r.Source, r.MetricType, r.Value, *captured)
		if err != nil {
			log.Printf("  import consensus row %q (%s): %v", r.PlayerName, r.Source, err)
			failed++
			continue
		}
	}
	log.Printf("  imported %d consensus rows: %d matched to espn_id, %d unmatched, %d failed",
		len(inputRows), matched, unmatched, failed)
	return nil
}

// ── divergence computation ──────────────────────────────────────────────────

func median(vals []float64) float64 {
	n := len(vals)
	if n == 0 {
		return 0
	}
	sorted := append([]float64(nil), vals...)
	sort.Float64s(sorted)
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}

type divergenceRow struct {
	espnID        string
	ourRank       int
	consensusRank float64
	sourceCount   int
	delta         float64
}

// computeConsensusDivergences ranks overall on both sides (no position split —
// NBA category-league rankings are published overall, unlike NFL where raw
// point volume differs structurally by position). our_rank reads straight
// from nba_projections.our_rank (already overall); source_rank is
// RANK() OVER (PARTITION BY source ORDER BY value ASC), recovering a
// comparable rank from each source's list regardless of how many players it
// actually covers.
func computeConsensusDivergences(ctx context.Context, pool *pgxpool.Pool, season int) error {
	rows, err := pool.Query(ctx, `
		SELECT espn_id, our_rank FROM nba_projections WHERE target_season = $1 AND our_rank IS NOT NULL
	`, season)
	if err != nil {
		return fmt.Errorf("load our projections: %w", err)
	}
	ourRank := make(map[string]int)
	for rows.Next() {
		var espnID string
		var rank int
		if err := rows.Scan(&espnID, &rank); err != nil {
			rows.Close()
			return fmt.Errorf("scan projection: %w", err)
		}
		ourRank[espnID] = rank
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	log.Printf("  loaded %d of our own ranked projections", len(ourRank))

	crows, err := pool.Query(ctx, `
		SELECT espn_id, RANK() OVER (PARTITION BY source ORDER BY value ASC) AS src_rank
		FROM nba_consensus_rankings
		WHERE season = $1 AND espn_id IS NOT NULL AND metric_type IN ('rank','adp')
	`, season)
	if err != nil {
		return fmt.Errorf("load consensus rankings: %w", err)
	}
	rankLike := make(map[string][]float64)
	for crows.Next() {
		var espnID string
		var srcRank int
		if err := crows.Scan(&espnID, &srcRank); err != nil {
			crows.Close()
			return fmt.Errorf("scan consensus row: %w", err)
		}
		rankLike[espnID] = append(rankLike[espnID], float64(srcRank))
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return err
	}

	var divergences []divergenceRow
	for espnID, vals := range rankLike {
		our, ok := ourRank[espnID]
		if !ok {
			continue
		}
		consensusRank := median(vals)
		divergences = append(divergences, divergenceRow{
			espnID: espnID, ourRank: our, consensusRank: consensusRank,
			sourceCount: len(vals), delta: float64(our) - consensusRank,
		})
	}
	log.Printf("  computed %d divergences (players present in both our projections and consensus data)", len(divergences))

	for _, d := range divergences {
		if _, err := pool.Exec(ctx, `
			INSERT INTO nba_projection_divergences
				(espn_id, season, our_rank, consensus_rank_median, source_count, rank_delta)
			VALUES ($1,$2,$3,$4,$5,$6)
			ON CONFLICT (espn_id, season) DO UPDATE SET
				our_rank = EXCLUDED.our_rank, consensus_rank_median = EXCLUDED.consensus_rank_median,
				source_count = EXCLUDED.source_count, rank_delta = EXCLUDED.rank_delta, computed_at = NOW()
		`, d.espnID, season, d.ourRank, d.consensusRank, d.sourceCount, d.delta); err != nil {
			log.Printf("  divergence upsert %s: %v", d.espnID, err)
		}
	}

	printDivergenceReport(ctx, pool, divergences)
	return nil
}

func printDivergenceReport(ctx context.Context, pool *pgxpool.Pool, divergences []divergenceRow) {
	if len(divergences) == 0 {
		return
	}
	names := make(map[string]string)
	rows, err := pool.Query(ctx, `SELECT espn_id, name FROM nba_players`)
	if err == nil {
		for rows.Next() {
			var id, name string
			if rows.Scan(&id, &name) == nil {
				names[id] = name
			}
		}
		rows.Close()
	}

	printRow := func(d divergenceRow) {
		log.Printf("    %-25s our=%-4d consensus=%-6.1f delta=%+.1f (n=%d sources)",
			names[d.espnID], d.ourRank, d.consensusRank, d.delta, d.sourceCount)
	}

	sorted := append([]divergenceRow(nil), divergences...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].delta > sorted[j].delta })
	log.Println("  --- we rank players LOWER than consensus (top 20) ---")
	for i := 0; i < len(sorted) && i < 20; i++ {
		printRow(sorted[i])
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].delta < sorted[j].delta })
	log.Println("  --- we rank players HIGHER than consensus (top 20) ---")
	for i := 0; i < len(sorted) && i < 20; i++ {
		printRow(sorted[i])
	}
}
