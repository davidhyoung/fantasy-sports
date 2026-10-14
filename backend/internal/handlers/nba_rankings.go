package handlers

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// nbaRankingSource is one external outlet's rank for a player — the piece
// NFL's own /divergences never surfaces (only the aggregated median), built
// here specifically so a viewer can see "ESPN said X, RotoWire said Y," not
// just an opaque consensus number. See docs/stats/nba-consensus-rankings.md.
type nbaRankingSource struct {
	Source string  `json:"source"`
	Rank   float64 `json:"rank"`
}

type nbaRankingItem struct {
	ESPNID              string             `json:"espn_id"`
	Name                string             `json:"name"`
	Position            string             `json:"position"`
	Team                string             `json:"team"`
	HeadshotURL         string             `json:"headshot_url"`
	OurRank             int                `json:"our_rank"`
	ConsensusRankMedian float64            `json:"consensus_rank_median"`
	SourceCount         int                `json:"source_count"`
	Sources             []nbaRankingSource `json:"sources"`
	Stats               []nbaCategoryEntry `json:"stats"`
}

type nbaRankingListResp struct {
	Season  int              `json:"season"`
	Players []nbaRankingItem `json:"players"`
	Total   int              `json:"total"`
}

// ListNBARankings returns our own comp-based projection rank for every
// player we've batch-projected (cmd/project-nba -project), alongside the
// external consensus median AND every individual source's rank — unlike the
// NFL /divergences endpoint, which only ever exposes the aggregated median.
// Ranking is overall, not within position group: NBA category-league
// rankings are published overall (no QB-style raw-volume skew the way NFL's
// points differ structurally by position), so there's nothing to correct
// for by splitting.
//
// GET /api/nba/rankings?season=&position=&limit=&offset=
func (h *Handler) ListNBARankings(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	season := h.config.DefaultSeason
	if s := q.Get("season"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			season = v
		}
	}
	limit := 300
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
		SELECT pr.espn_id, p.name, COALESCE(p.position, ''), COALESCE(p.team, ''), COALESCE(p.headshot_url, ''),
		       pr.our_rank, pr.points, pr.rebounds, pr.assists, pr.steals, pr.blocks, pr.turnovers, pr.three_pm, pr.fg_pct, pr.ft_pct
		FROM nba_projections pr
		JOIN nba_players p ON p.espn_id = pr.espn_id
		WHERE pr.target_season = $1 AND pr.our_rank IS NOT NULL`
	args := []any{season}
	if len(positions) > 0 {
		sql += ` AND p.position = ANY($2)`
		args = append(args, positions)
	}
	sql += ` ORDER BY pr.our_rank ASC LIMIT ` + strconv.Itoa(limit) + ` OFFSET ` + strconv.Itoa(offset)

	rows, err := h.db.Query(ctx, sql, args...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := []nbaRankingItem{}
	espnIDs := []string{}
	for rows.Next() {
		var it nbaRankingItem
		var pts, reb, ast, stl, blk, tov, fg3m, fgPct, ftPct float64
		if err := rows.Scan(&it.ESPNID, &it.Name, &it.Position, &it.Team, &it.HeadshotURL,
			&it.OurRank, &pts, &reb, &ast, &stl, &blk, &tov, &fg3m, &fgPct, &ftPct); err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		statVals := map[string]float64{
			"points": pts, "rebounds": reb, "assists": ast, "steals": stl, "blocks": blk,
			"turnovers": tov, "three_pm": fg3m, "fg_pct": fgPct, "ft_pct": ftPct,
		}
		it.Stats = make([]nbaCategoryEntry, 0, len(nbaCategories))
		for _, cat := range nbaCategories {
			it.Stats = append(it.Stats, nbaCategoryEntry{Category: cat, Value: statVals[cat]})
		}
		it.Sources = []nbaRankingSource{}
		items = append(items, it)
		espnIDs = append(espnIDs, it.ESPNID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Per-source ranks + consensus median computed live from
	// nba_consensus_rankings (cheap at this row count — same "derive on
	// read" approach draft_consensus.go already uses for NFL) rather than
	// persisting a denormalized per-source table.
	srows, err := h.db.Query(ctx, `
		SELECT espn_id, source, RANK() OVER (PARTITION BY source ORDER BY value ASC) AS src_rank
		FROM nba_consensus_rankings
		WHERE season = $1 AND espn_id = ANY($2) AND metric_type IN ('rank','adp')
	`, season, espnIDs)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sourcesByPlayer := make(map[string][]nbaRankingSource)
	for srows.Next() {
		var espnID, source string
		var rank int
		if err := srows.Scan(&espnID, &source, &rank); err != nil {
			srows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sourcesByPlayer[espnID] = append(sourcesByPlayer[espnID], nbaRankingSource{Source: source, Rank: float64(rank)})
	}
	srows.Close()
	if err := srows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	for i := range items {
		srcs := sourcesByPlayer[items[i].ESPNID]
		sort.Slice(srcs, func(a, b int) bool { return srcs[a].Source < srcs[b].Source })
		if srcs == nil {
			srcs = []nbaRankingSource{}
		}
		items[i].Sources = srcs
		items[i].SourceCount = len(srcs)
		if len(srcs) > 0 {
			vals := make([]float64, len(srcs))
			for j, s := range srcs {
				vals[j] = s.Rank
			}
			items[i].ConsensusRankMedian = medianOf(vals)
		}
	}

	respondJSON(w, http.StatusOK, nbaRankingListResp{Season: season, Players: items, Total: len(items)})
}
