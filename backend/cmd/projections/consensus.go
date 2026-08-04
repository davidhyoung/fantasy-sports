// consensus.go implements the divergence-flag layer described in
// docs/stats/consensus-ensemble.md: importing external consensus rankings/ADP and
// situational news (injuries, camp battles, etc.), then diffing our own comp-based
// projection rank against them. This is diagnostic only — it never mutates
// nfl_projections.
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

	"github.com/jackc/pgx/v5/pgxpool"
)

// ── input JSON shapes ───────────────────────────────────────────────────────

// consensusRankingInput is one row of the -import-consensus JSON file.
type consensusRankingInput struct {
	PlayerName   string  `json:"player_name"`
	Position     string  `json:"position"`
	Team         string  `json:"team"`
	Season       int     `json:"season,omitempty"` // falls back to -season flag if omitted
	Source       string  `json:"source"`
	Format       string  `json:"format"`      // standard | half_ppr | ppr | dynasty | superflex
	MetricType   string  `json:"metric_type"` // rank | adp | dynasty_value
	Value        float64 `json:"value"`
	CapturedDate string  `json:"captured_date"` // YYYY-MM-DD
	SleeperID    string  `json:"sleeper_id,omitempty"`
	ESPNID       string  `json:"espn_id,omitempty"`
}

// situationalNoteInput is one row of the -import-notes JSON file.
type situationalNoteInput struct {
	PlayerName      string `json:"player_name"`
	Position        string `json:"position"`
	Team            string `json:"team"`
	Season          int    `json:"season,omitempty"`
	Category        string `json:"category"`
	Summary         string `json:"summary"`
	ImpactDirection string `json:"impact_direction"`
	ImpactMagnitude string `json:"impact_magnitude"`
	Confidence      string `json:"confidence"`
	Source          string `json:"source"`
	ReportedDate    string `json:"reported_date,omitempty"`
	SleeperID       string `json:"sleeper_id,omitempty"`
	ESPNID          string `json:"espn_id,omitempty"`
	// Scope is "player" (default, about this player's own value) or "team"
	// (team-wide context — QB competitions, scheme changes, coaching hires —
	// that a human reviewing any teammate's projection should see, without the
	// system guessing which direction/magnitude it applies to that teammate).
	Scope string `json:"scope,omitempty"`
}

// ── player identity resolution ──────────────────────────────────────────────
//
// Almost no external ranking source shares a stable ID with nfl_players, so
// resolution falls back through: sleeper_id/espn_id (exact) → normalized
// name+team (handles the common case) → normalized name+position (handles
// players who were traded since the source's data was captured). Unmatched
// rows are kept with gsis_id = NULL rather than dropped, so they can be
// reviewed and the resolver improved later.

type playerIndex struct {
	bySleeperID map[string]string
	byESPNID    map[string]string
	byNameTeam  map[string]string
	byNamePos   map[string]string
}

var suffixRE = regexp.MustCompile(`\b(jr|sr|ii|iii|iv|v)\.?$`)
var punctRE = regexp.MustCompile(`[.'’]`)
var spaceRE = regexp.MustCompile(`\s+`)

// normalizePlayerName lowercases, strips punctuation/suffixes, and collapses
// whitespace so "A.J. Brown Jr." and "aj brown" match.
func normalizePlayerName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = punctRE.ReplaceAllString(s, "")
	s = suffixRE.ReplaceAllString(strings.TrimSpace(s), "")
	s = spaceRE.ReplaceAllString(strings.TrimSpace(s), " ")
	return strings.TrimSpace(s)
}

func loadPlayerIndex(ctx context.Context, pool *pgxpool.Pool) (*playerIndex, error) {
	rows, err := pool.Query(ctx, `
		SELECT gsis_id, name, position_group, team, sleeper_id, espn_id
		FROM nfl_players
	`)
	if err != nil {
		return nil, fmt.Errorf("load player index: %w", err)
	}
	defer rows.Close()

	idx := &playerIndex{
		bySleeperID: make(map[string]string),
		byESPNID:    make(map[string]string),
		byNameTeam:  make(map[string]string),
		byNamePos:   make(map[string]string),
	}
	for rows.Next() {
		var gsisID, name string
		var posGroup, team, sleeperID, espnID *string
		if err := rows.Scan(&gsisID, &name, &posGroup, &team, &sleeperID, &espnID); err != nil {
			return nil, fmt.Errorf("scan player index row: %w", err)
		}
		if sleeperID != nil && *sleeperID != "" {
			idx.bySleeperID[*sleeperID] = gsisID
		}
		if espnID != nil && *espnID != "" {
			idx.byESPNID[*espnID] = gsisID
		}
		norm := normalizePlayerName(name)
		if team != nil && *team != "" {
			idx.byNameTeam[norm+"|"+strings.ToUpper(*team)] = gsisID
		}
		if posGroup != nil && *posGroup != "" {
			idx.byNamePos[norm+"|"+strings.ToUpper(*posGroup)] = gsisID
		}
	}
	return idx, rows.Err()
}

func (idx *playerIndex) resolve(name, position, team, sleeperID, espnID string) (gsisID string, matched bool) {
	if sleeperID != "" {
		if g, ok := idx.bySleeperID[sleeperID]; ok {
			return g, true
		}
	}
	if espnID != "" {
		if g, ok := idx.byESPNID[espnID]; ok {
			return g, true
		}
	}
	norm := normalizePlayerName(name)
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

// parseDate parses a YYYY-MM-DD string, returning nil for an empty input.
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

// ── import: consensus rankings ──────────────────────────────────────────────

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

		gsisID, ok := idx.resolve(r.PlayerName, r.Position, r.Team, r.SleeperID, r.ESPNID)
		var gsisArg any
		if ok {
			gsisArg = gsisID
			matched++
		} else {
			gsisArg = nil
			unmatched++
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO nfl_consensus_rankings
				(gsis_id, player_name_raw, position, team, season, source, format, metric_type, value, captured_date)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
			ON CONFLICT (player_name_raw, season, source, format, metric_type, captured_date)
			DO UPDATE SET gsis_id = EXCLUDED.gsis_id, position = EXCLUDED.position,
			              team = EXCLUDED.team, value = EXCLUDED.value
		`, gsisArg, r.PlayerName, r.Position, r.Team, season, r.Source, r.Format, r.MetricType, r.Value, *captured)
		if err != nil {
			log.Printf("  import consensus row %q (%s): %v", r.PlayerName, r.Source, err)
			failed++
			continue
		}
	}
	log.Printf("  imported %d consensus rows: %d matched to gsis_id, %d unmatched, %d failed",
		len(inputRows), matched, unmatched, failed)
	return nil
}

// ── import: situational notes ───────────────────────────────────────────────

func importSituationalNotes(ctx context.Context, pool *pgxpool.Pool, path string, defaultSeason int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	var inputRows []situationalNoteInput
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
		reported, err := parseDate(r.ReportedDate)
		if err != nil {
			log.Printf("  skipping %q: %v", r.PlayerName, err)
			failed++
			continue
		}

		gsisID, ok := idx.resolve(r.PlayerName, r.Position, r.Team, r.SleeperID, r.ESPNID)
		var gsisArg any
		if ok {
			gsisArg = gsisID
			matched++
		} else {
			gsisArg = nil
			unmatched++
		}

		scope := r.Scope
		if scope == "" {
			scope = "player"
		}

		_, err = pool.Exec(ctx, `
			INSERT INTO nfl_player_situational_notes
				(gsis_id, player_name_raw, team, scope, season, category, summary, impact_direction,
				 impact_magnitude, confidence, source, reported_date)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT ON CONSTRAINT nfl_player_situational_notes_dedup
			DO UPDATE SET gsis_id = EXCLUDED.gsis_id, team = EXCLUDED.team, scope = EXCLUDED.scope,
			              summary = EXCLUDED.summary, impact_direction = EXCLUDED.impact_direction,
			              impact_magnitude = EXCLUDED.impact_magnitude, confidence = EXCLUDED.confidence
		`, gsisArg, r.PlayerName, r.Team, scope, season, r.Category, r.Summary, r.ImpactDirection,
			r.ImpactMagnitude, r.Confidence, r.Source, reported)
		if err != nil {
			log.Printf("  import note row %q (%s): %v", r.PlayerName, r.Category, err)
			failed++
			continue
		}
	}
	log.Printf("  imported %d situational notes: %d matched to gsis_id, %d unmatched, %d failed",
		len(inputRows), matched, unmatched, failed)
	return nil
}

// ── divergence computation ──────────────────────────────────────────────────

// projectionRankColumn maps a redraft scoring format to the nfl_projections
// column that ranks players in that format. Dynasty and superflex are
// deliberately unsupported here: our engine only produces a one-season-ahead
// redraft projection, so there is no model-native rank to diff a dynasty
// (multi-year) or superflex (QB-premium) consensus against — see the
// "sources must be format-matched" assumption in docs/stats/consensus-ensemble.md.
func projectionRankColumn(format string) (string, error) {
	switch format {
	case "standard":
		return "proj_fpts", nil
	case "half_ppr", "half":
		return "proj_fpts_half", nil
	case "ppr":
		return "proj_fpts_ppr", nil
	case "dynasty", "superflex":
		return "", fmt.Errorf("format %q has no comparable model output: our engine projects one redraft season, not multi-year/QB-premium value — dynasty/superflex data is still imported but not diffed", format)
	default:
		return "", fmt.Errorf("unknown format %q (want standard|half_ppr|ppr)", format)
	}
}

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
	gsisID        string
	positionGroup string
	ourRank       int
	consensusRank float64
	sourceCount   int
	delta         float64
}

// computeConsensusDivergences ranks players WITHIN their position group on both
// sides before comparing. Ranking overall (QB vs RB vs WR combined) would
// conflate raw fantasy-point volume — which structurally favors QBs, who score
// more points than any other position — with consensus draft value, which is
// scarcity-adjusted (a 1-QB league starts one QB but 2-3 RB/WR). That mismatch
// alone accounted for ~a fifth of all outliers in the first real run. See
// docs/stats/consensus-ensemble.md.
func computeConsensusDivergences(ctx context.Context, pool *pgxpool.Pool, season int, format string) error {
	rankCol, err := projectionRankColumn(format)
	if err != nil {
		return err
	}

	// 1. Rank our own projections 1..N within each position group.
	log.Printf("  ranking our projections by %s within position group…", rankCol)
	rows, err := pool.Query(ctx, fmt.Sprintf(`
		SELECT pr.gsis_id, p.position_group, pr.%s
		FROM nfl_projections pr
		JOIN nfl_players p ON p.gsis_id = pr.gsis_id
		WHERE pr.target_season = $1 AND p.position_group IS NOT NULL
	`, rankCol), season)
	if err != nil {
		return fmt.Errorf("load projections: %w", err)
	}
	type projVal struct {
		gsisID   string
		posGroup string
		val      float64
	}
	var projVals []projVal
	posGroupOf := make(map[string]string)
	for rows.Next() {
		var pv projVal
		if err := rows.Scan(&pv.gsisID, &pv.posGroup, &pv.val); err != nil {
			rows.Close()
			return fmt.Errorf("scan projection: %w", err)
		}
		projVals = append(projVals, pv)
		posGroupOf[pv.gsisID] = pv.posGroup
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	byPosition := make(map[string][]projVal)
	for _, pv := range projVals {
		byPosition[pv.posGroup] = append(byPosition[pv.posGroup], pv)
	}
	ourRank := make(map[string]int, len(projVals))
	for _, group := range byPosition {
		sort.Slice(group, func(i, j int) bool { return group[i].val > group[j].val })
		for i, pv := range group {
			ourRank[pv.gsisID] = i + 1
		}
	}
	log.Printf("  ranked %d players across %d position groups", len(ourRank), len(byPosition))

	// 2. Load matched consensus rows, ranked WITHIN position by each source
	// (RANK() OVER a source+position partition) — this recovers a position-
	// relative rank regardless of whether the source published an overall list
	// or a position-specific one, since only the relative order within the
	// partition matters. rank/adp already sort ascending (1=best); dynasty_value
	// is a value scale and gets rank-ordered separately (only relevant once a
	// diffable format carries dynasty_value rows, which none currently does).
	crows, err := pool.Query(ctx, `
		SELECT gsis_id, position,
		       RANK() OVER (PARTITION BY source, position ORDER BY value ASC) AS pos_rank
		FROM nfl_consensus_rankings
		WHERE season = $1 AND format = $2 AND gsis_id IS NOT NULL
		  AND metric_type IN ('rank','adp') AND position IS NOT NULL
	`, season, format)
	if err != nil {
		return fmt.Errorf("load consensus rankings: %w", err)
	}
	rankLike := make(map[string][]float64)
	for crows.Next() {
		var gsisID, position string
		var posRank int
		if err := crows.Scan(&gsisID, &position, &posRank); err != nil {
			crows.Close()
			return fmt.Errorf("scan consensus row: %w", err)
		}
		rankLike[gsisID] = append(rankLike[gsisID], float64(posRank))
	}
	crows.Close()
	if err := crows.Err(); err != nil {
		return err
	}

	// 3. Join, compute median consensus position-rank + delta, upsert.
	var divergences []divergenceRow
	for gsisID, vals := range rankLike {
		our, ok := ourRank[gsisID]
		if !ok {
			continue
		}
		consensusRank := median(vals)
		divergences = append(divergences, divergenceRow{
			gsisID:        gsisID,
			positionGroup: posGroupOf[gsisID],
			ourRank:       our,
			consensusRank: consensusRank,
			sourceCount:   len(vals),
			delta:         float64(our) - consensusRank,
		})
	}
	log.Printf("  computed %d divergences (players present in both our projections and consensus data)", len(divergences))

	for _, d := range divergences {
		if _, err := pool.Exec(ctx, `
			INSERT INTO nfl_projection_divergences
				(gsis_id, season, format, position_group, our_rank, consensus_rank_median, source_count, rank_delta)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (gsis_id, season, format) DO UPDATE SET
				position_group = EXCLUDED.position_group,
				our_rank = EXCLUDED.our_rank,
				consensus_rank_median = EXCLUDED.consensus_rank_median,
				source_count = EXCLUDED.source_count,
				rank_delta = EXCLUDED.rank_delta,
				computed_at = NOW()
		`, d.gsisID, season, format, d.positionGroup, d.ourRank, d.consensusRank, d.sourceCount, d.delta); err != nil {
			log.Printf("  divergence upsert %s: %v", d.gsisID, err)
		}
	}

	printDivergenceReport(ctx, pool, divergences, season)
	return nil
}

// situationalContext is one note surfaced alongside a divergence row — either
// specific to the player or team-scoped for whichever team they're on.
type situationalContext struct {
	category string
	summary  string
	impact   string
}

// loadSituationalContext returns, for each gsis_id in scope, both their own
// player-scoped notes and any team-scoped notes for their team+season. This is
// a plain join, not inference — direction/magnitude for a given teammate is
// left to whoever is reading the report.
func loadSituationalContext(ctx context.Context, pool *pgxpool.Pool, season int) (map[string][]situationalContext, error) {
	rows, err := pool.Query(ctx, `
		SELECT n.gsis_id, n.category, n.summary, n.impact_direction, n.impact_magnitude
		FROM nfl_player_situational_notes n
		WHERE n.season = $1 AND n.scope = 'player' AND n.gsis_id IS NOT NULL
	`, season)
	if err != nil {
		return nil, fmt.Errorf("load player-scoped notes: %w", err)
	}
	byPlayer := make(map[string][]situationalContext)
	for rows.Next() {
		var gsisID, category, summary, direction, magnitude string
		if err := rows.Scan(&gsisID, &category, &summary, &direction, &magnitude); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan player-scoped note: %w", err)
		}
		byPlayer[gsisID] = append(byPlayer[gsisID], situationalContext{category, summary, direction + "/" + magnitude})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	teamRows, err := pool.Query(ctx, `
		SELECT n.team, n.category, n.summary, n.impact_direction, n.impact_magnitude
		FROM nfl_player_situational_notes n
		WHERE n.season = $1 AND n.scope = 'team' AND n.team IS NOT NULL
	`, season)
	if err != nil {
		return nil, fmt.Errorf("load team-scoped notes: %w", err)
	}
	byTeam := make(map[string][]situationalContext)
	for teamRows.Next() {
		var team, category, summary, direction, magnitude string
		if err := teamRows.Scan(&team, &category, &summary, &direction, &magnitude); err != nil {
			teamRows.Close()
			return nil, fmt.Errorf("scan team-scoped note: %w", err)
		}
		byTeam[team] = append(byTeam[team], situationalContext{category, summary, direction + "/" + magnitude})
	}
	teamRows.Close()
	if err := teamRows.Err(); err != nil {
		return nil, err
	}

	playerTeamRows, err := pool.Query(ctx, `SELECT gsis_id, team FROM nfl_players WHERE team IS NOT NULL`)
	if err != nil {
		return nil, fmt.Errorf("load player teams: %w", err)
	}
	result := make(map[string][]situationalContext)
	for playerTeamRows.Next() {
		var gsisID, team string
		if err := playerTeamRows.Scan(&gsisID, &team); err != nil {
			playerTeamRows.Close()
			return nil, fmt.Errorf("scan player team: %w", err)
		}
		ctxs := append([]situationalContext(nil), byPlayer[gsisID]...)
		ctxs = append(ctxs, byTeam[team]...)
		if len(ctxs) > 0 {
			result[gsisID] = ctxs
		}
	}
	playerTeamRows.Close()
	return result, playerTeamRows.Err()
}

// printDivergenceReport prints the top 20 players in each direction of
// disagreement, for immediate manual review alongside the persisted table.
// Any situational notes (player-specific or team-scoped) matching a printed
// player are shown inline as unweighted context, not applied to the numbers.
func printDivergenceReport(ctx context.Context, pool *pgxpool.Pool, divergences []divergenceRow, season int) {
	if len(divergences) == 0 {
		return
	}
	names := make(map[string]string)
	rows, err := pool.Query(ctx, `SELECT gsis_id, name FROM nfl_players`)
	if err == nil {
		for rows.Next() {
			var id, name string
			if rows.Scan(&id, &name) == nil {
				names[id] = name
			}
		}
		rows.Close()
	}

	context, err := loadSituationalContext(ctx, pool, season)
	if err != nil {
		log.Printf("  warning: could not load situational context: %v", err)
		context = nil
	}

	printRow := func(d divergenceRow) {
		log.Printf("    %-25s %-3s our=%-4d consensus=%-6.1f delta=%+.1f (n=%d sources)",
			names[d.gsisID], d.positionGroup, d.ourRank, d.consensusRank, d.delta, d.sourceCount)
		for _, c := range context[d.gsisID] {
			log.Printf("        context [%s, %s]: %s", c.category, c.impact, c.summary)
		}
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
