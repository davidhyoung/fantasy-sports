package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
)

// ── response types ────────────────────────────────────────────────────────────

// situationalNote is one piece of external context about a player (injury,
// depth-chart battle, scheme change, …). Scope is "player" when the note is
// about this player specifically, or "team" when it's team-wide context that
// happens to affect them — see docs/stats/consensus-ensemble.md.
type situationalNote struct {
	Category        string `json:"category"`
	Summary         string `json:"summary"`
	ImpactDirection string `json:"impact_direction"`
	ImpactMagnitude string `json:"impact_magnitude"`
	Confidence      string `json:"confidence"`
	Source          string `json:"source"`
	ReportedDate    string `json:"reported_date"`
	Scope           string `json:"scope"`
}

type divergenceItem struct {
	GsisID              string            `json:"gsis_id"`
	Name                string            `json:"name"`
	Position            string            `json:"position"`
	PositionGroup       string            `json:"position_group"`
	Team                string            `json:"team"`
	HeadshotURL         string            `json:"headshot_url"`
	OurRank             int               `json:"our_rank"`
	ConsensusRankMedian float64           `json:"consensus_rank_median"`
	SourceCount         int               `json:"source_count"`
	RankDelta           float64           `json:"rank_delta"`
	ProjFptsPPR         float64           `json:"proj_fpts_ppr"`
	Notes               []situationalNote `json:"notes"`
}

type divergenceListResp struct {
	Season  int              `json:"season"`
	Format  string           `json:"format"`
	Players []divergenceItem `json:"players"`
	Total   int              `json:"total"`
}

// ── shared note loading ───────────────────────────────────────────────────────

// loadNotesForPlayers returns situational notes keyed by gsis_id for the given
// players. Each player receives their own notes plus any team-scoped notes for
// their team — a plain join, with no inference about how a team-wide event
// (e.g. a QB competition) affects that specific player. That judgment stays
// with whoever reads the result. Mirrors loadSituationalContext in
// cmd/projections/consensus.go, which does the same for the CLI report.
func (h *Handler) loadNotesForPlayers(ctx context.Context, season int, gsisIDs []string) (map[string][]situationalNote, error) {
	result := make(map[string][]situationalNote)
	if len(gsisIDs) == 0 {
		return result, nil
	}

	// Player-scoped notes attach directly to their gsis_id. Team-scoped notes
	// are matched through nfl_players.team so every teammate picks them up.
	rows, err := h.db.Query(ctx, `
		SELECT p.gsis_id, n.category, n.summary, n.impact_direction,
		       n.impact_magnitude, n.confidence, COALESCE(n.source, ''),
		       COALESCE(n.reported_date::text, ''), n.scope
		FROM nfl_players p
		JOIN nfl_player_situational_notes n
		  ON (n.scope = 'player' AND n.gsis_id = p.gsis_id)
		  OR (n.scope = 'team' AND n.team IS NOT NULL AND n.team = p.team)
		WHERE p.gsis_id = ANY($1) AND n.season = $2
		ORDER BY n.scope, n.reported_date DESC NULLS LAST
	`, gsisIDs, season)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var gsisID string
		var n situationalNote
		if err := rows.Scan(&gsisID, &n.Category, &n.Summary, &n.ImpactDirection,
			&n.ImpactMagnitude, &n.Confidence, &n.Source, &n.ReportedDate, &n.Scope); err != nil {
			return nil, err
		}
		result[gsisID] = append(result[gsisID], n)
	}
	return result, rows.Err()
}

// ── handler ───────────────────────────────────────────────────────────────────

// ListDivergences returns players ranked by how far our projection's
// within-position rank sits from the median external consensus rank, largest
// disagreement first, with any situational notes that might explain the gap.
//
// This is a review surface, not a correction: a large divergence is a prompt to
// look, not evidence that either side is wrong. Nothing here feeds back into
// nfl_projections.
//
// GET /api/divergences?season=&format=&position=&limit=&offset=
func (h *Handler) ListDivergences(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	season := h.config.DefaultSeason
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}

	format := q.Get("format")
	if format == "" {
		format = "ppr"
	}
	switch format {
	case "ppr", "half_ppr", "standard":
		// ok — these are the formats the diff step computes. Dynasty/superflex
		// consensus data is stored but never diffed, since our engine has no
		// multi-year or QB-premium output to compare against.
	default:
		respondError(w, http.StatusBadRequest, "invalid format (want ppr|half_ppr|standard)")
		return
	}

	limit := 200
	if l := q.Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	offset := 0
	if o := q.Get("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	// position accepts a comma-separated list (e.g. "RB,WR,TE") to match the
	// grades endpoint's Flex/Superflex filters.
	var positions []string
	if p := q.Get("position"); p != "" {
		for _, part := range strings.Split(p, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				positions = append(positions, strings.ToUpper(trimmed))
			}
		}
	}

	ctx := r.Context()

	sql := `
		SELECT d.gsis_id, p.name, COALESCE(p.position, ''), COALESCE(p.position_group, ''),
		       COALESCE(p.team, ''), COALESCE(p.headshot_url, ''),
		       d.our_rank, d.consensus_rank_median, d.source_count, d.rank_delta,
		       COALESCE(pr.proj_fpts_ppr, 0)
		FROM nfl_projection_divergences d
		JOIN nfl_players p ON p.gsis_id = d.gsis_id
		LEFT JOIN nfl_projections pr
		       ON pr.gsis_id = d.gsis_id AND pr.target_season = d.season
		WHERE d.season = $1 AND d.format = $2`
	args := []any{season, format}
	if len(positions) > 0 {
		sql += ` AND p.position_group = ANY($3)`
		args = append(args, positions)
	}
	sql += ` ORDER BY ABS(d.rank_delta) DESC, p.name
	         LIMIT ` + strconv.Itoa(limit) + ` OFFSET ` + strconv.Itoa(offset)

	rows, err := h.db.Query(ctx, sql, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	items := []divergenceItem{}
	gsisIDs := []string{}
	for rows.Next() {
		var it divergenceItem
		if err := rows.Scan(
			&it.GsisID, &it.Name, &it.Position, &it.PositionGroup,
			&it.Team, &it.HeadshotURL,
			&it.OurRank, &it.ConsensusRankMedian, &it.SourceCount, &it.RankDelta,
			&it.ProjFptsPPR,
		); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		it.Notes = []situationalNote{}
		items = append(items, it)
		gsisIDs = append(gsisIDs, it.GsisID)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Notes are fetched in a second batch query rather than joined above, since
	// a player can have several and the join would multiply divergence rows.
	notes, err := h.loadNotesForPlayers(ctx, season, gsisIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i := range items {
		if n, ok := notes[items[i].GsisID]; ok {
			items[i].Notes = n
		}
	}

	respondJSON(w, http.StatusOK, divergenceListResp{
		Season:  season,
		Format:  format,
		Players: items,
		Total:   len(items),
	})
}
