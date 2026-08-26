package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/davidyoung/fantasy-sports/backend/internal/aging"
)

// Free agency: teams offer contracts, the player signs the best one — no
// FAAB, the salary cap is the only currency. See
// .claude/plans/dynasty-transactions.md. Everything here is native-league
// only, matching rookie-scale pricing's posture — Yahoo leagues have no
// contracts to sign in the first place.

// ── window lifecycle ────────────────────────────────────────────────────────

type faWindowResp struct {
	ID         int64      `json:"id"`
	Season     int        `json:"season"`
	Kind       string     `json:"kind"`
	Week       *int       `json:"week"`
	OpenedAt   time.Time  `json:"opened_at"`
	ResolvedAt *time.Time `json:"resolved_at"`
}

var validFAWindowKinds = map[string]bool{"offseason": true, "weekly": true}

// OpenFAWindow starts a new offer window. A window's season is always the
// league's current one — never client-set — because a signing out of this
// window stamps signed_season = season, and every cap/dead-money
// calculation assumes that season is the one actually being played (the
// same reasoning that made UseLeagueDraftPick reject a future pick's early
// use). Only one window may be open at a time (the partial unique index is
// the real enforcement; the pre-check here is just a clean error message).
//
// POST /api/leagues/{id}/fa/windows
func (h *Handler) OpenFAWindow(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	var req struct {
		Kind string `json:"kind"`
		Week *int   `json:"week"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.Kind == "" {
		req.Kind = "offseason"
	}
	if !validFAWindowKinds[req.Kind] {
		respondError(w, http.StatusBadRequest, "kind must be offseason or weekly")
		return
	}

	var alreadyOpen bool
	if err := h.db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM league_fa_windows WHERE league_id = $1 AND resolved_at IS NULL)", leagueID,
	).Scan(&alreadyOpen); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if alreadyOpen {
		respondError(w, http.StatusConflict, "a free agency window is already open — resolve it before opening another")
		return
	}

	season := h.leagueSeasonInt(r, leagueID)
	var resp faWindowResp
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO league_fa_windows (league_id, season, kind, week)
		VALUES ($1, $2, $3, $4)
		RETURNING id, season, kind, week, opened_at, resolved_at
	`, leagueID, season, req.Kind, req.Week).Scan(&resp.ID, &resp.Season, &resp.Kind, &resp.Week, &resp.OpenedAt, &resp.ResolvedAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, resp)
}

// ListFAWindows returns every window a league has ever opened, newest
// first — read-only, no gate, matching ListLeagueTransactions and the
// other native-league list endpoints.
//
// GET /api/leagues/{id}/fa/windows
func (h *Handler) ListFAWindows(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	rows, err := h.db.Query(r.Context(), `
		SELECT id, season, kind, week, opened_at, resolved_at
		FROM league_fa_windows WHERE league_id = $1 ORDER BY opened_at DESC
	`, leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	windows := []faWindowResp{}
	for rows.Next() {
		var win faWindowResp
		if err := rows.Scan(&win.ID, &win.Season, &win.Kind, &win.Week, &win.OpenedAt, &win.ResolvedAt); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		windows = append(windows, win)
	}
	respondJSON(w, http.StatusOK, windows)
}

// openFAWindowID returns the id of the league's currently open window, if
// any. Shared by every offer-mutating handler, since an offer only ever
// targets "the open window" — there's no client-facing concept of
// targeting an arbitrary past window.
func openFAWindowID(ctx context.Context, db dbtx, leagueID int64) (int64, int, bool, error) {
	var id int64
	var season int
	err := db.QueryRow(ctx,
		"SELECT id, season FROM league_fa_windows WHERE league_id = $1 AND resolved_at IS NULL", leagueID,
	).Scan(&id, &season)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return id, season, true, nil
}

// ── offers ──────────────────────────────────────────────────────────────────

type faOfferResp struct {
	ID         int64     `json:"id"`
	WindowID   int64     `json:"window_id"`
	GsisID     string    `json:"gsis_id"`
	PlayerName string    `json:"player_name"`
	TeamID     int64     `json:"team_id"`
	TeamName   string    `json:"team_name"`
	Salary     int       `json:"salary"`
	Years      int       `json:"years"`
	Priority   int       `json:"priority"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// ListFAOffers lists offers for a window (defaulting to the currently open
// one), optionally filtered to one team — read-only, no gate.
//
// GET /api/leagues/{id}/fa/offers?window_id=&team_id=
func (h *Handler) ListFAOffers(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	q := r.URL.Query()
	var windowID int64
	if wStr := q.Get("window_id"); wStr != "" {
		windowID, _ = strconv.ParseInt(wStr, 10, 64)
	} else {
		id, _, open, err := openFAWindowID(r.Context(), h.db, leagueID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !open {
			respondJSON(w, http.StatusOK, []faOfferResp{})
			return
		}
		windowID = id
	}
	var teamID int64
	if t := q.Get("team_id"); t != "" {
		teamID, _ = strconv.ParseInt(t, 10, 64)
	}

	rows, err := h.db.Query(r.Context(), `
		SELECT o.id, o.window_id, o.gsis_id, p.name, o.team_id, t.name, o.salary, o.years, o.priority, o.status, o.created_at
		FROM league_fa_offers o
		JOIN nfl_players p ON p.gsis_id = o.gsis_id
		JOIN teams t ON t.id = o.team_id
		WHERE o.league_id = $1 AND o.window_id = $2 AND ($3 = 0 OR o.team_id = $3)
		ORDER BY o.team_id, o.priority
	`, leagueID, windowID, teamID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()

	offers := []faOfferResp{}
	for rows.Next() {
		var o faOfferResp
		if err := rows.Scan(&o.ID, &o.WindowID, &o.GsisID, &o.PlayerName, &o.TeamID, &o.TeamName, &o.Salary, &o.Years, &o.Priority, &o.Status, &o.CreatedAt); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		offers = append(offers, o)
	}
	respondJSON(w, http.StatusOK, offers)
}

// CreateOrUpdateFAOffer submits (or edits, if the same team already has a
// pending offer on this player in the open window) an offer. Only ever
// targets the currently open window — there's no way to offer into one
// that's already resolved.
//
// POST /api/leagues/{id}/fa/offers
func (h *Handler) CreateOrUpdateFAOffer(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	var req struct {
		GsisID   string `json:"gsis_id"`
		TeamID   int64  `json:"team_id"`
		Salary   int    `json:"salary"`
		Years    int    `json:"years"`
		Priority int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.GsisID == "" || req.TeamID == 0 {
		respondError(w, http.StatusBadRequest, "gsis_id and team_id are required")
		return
	}
	if req.Salary < 0 {
		respondError(w, http.StatusBadRequest, "salary cannot be negative")
		return
	}
	if req.Years < 1 {
		req.Years = 1
	}

	windowID, _, open, err := openFAWindowID(r.Context(), h.db, leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !open {
		respondError(w, http.StatusUnprocessableEntity, "no free agency window is open")
		return
	}

	var teamInLeague bool
	if err := h.db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM teams WHERE id = $1 AND league_id = $2)", req.TeamID, leagueID,
	).Scan(&teamInLeague); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !teamInLeague {
		respondError(w, http.StatusBadRequest, "team is not in this league")
		return
	}
	var alreadyRostered bool
	if err := h.db.QueryRow(r.Context(),
		"SELECT EXISTS(SELECT 1 FROM league_rosters WHERE league_id = $1 AND gsis_id = $2)", leagueID, req.GsisID,
	).Scan(&alreadyRostered); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if alreadyRostered {
		respondError(w, http.StatusBadRequest, "player is already rostered in this league")
		return
	}

	if req.Priority <= 0 {
		// An omitted priority means "not moving it" for an edit of an
		// existing pending offer (bumping the salary shouldn't silently
		// shove it to the back of the list) and "append to the end" only
		// for a genuinely new offer.
		var existingPriority int
		err := h.db.QueryRow(r.Context(),
			"SELECT priority FROM league_fa_offers WHERE window_id = $1 AND gsis_id = $2 AND team_id = $3 AND status = 'pending'",
			windowID, req.GsisID, req.TeamID,
		).Scan(&existingPriority)
		switch {
		case err == nil:
			req.Priority = existingPriority
		case errors.Is(err, pgx.ErrNoRows):
			if err := h.db.QueryRow(r.Context(),
				"SELECT COALESCE(MAX(priority), 0) + 1 FROM league_fa_offers WHERE window_id = $1 AND team_id = $2 AND status = 'pending'",
				windowID, req.TeamID,
			).Scan(&req.Priority); err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
		default:
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	var resp faOfferResp
	err = h.db.QueryRow(r.Context(), `
		INSERT INTO league_fa_offers (league_id, window_id, gsis_id, team_id, salary, years, priority)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (window_id, gsis_id, team_id) DO UPDATE
		SET salary = excluded.salary, years = excluded.years, priority = excluded.priority
		WHERE league_fa_offers.status = 'pending'
		RETURNING id, window_id, gsis_id, team_id, salary, years, priority, status, created_at
	`, leagueID, windowID, req.GsisID, req.TeamID, req.Salary, req.Years, req.Priority,
	).Scan(&resp.ID, &resp.WindowID, &resp.GsisID, &resp.TeamID, &resp.Salary, &resp.Years, &resp.Priority, &resp.Status, &resp.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		respondError(w, http.StatusConflict, "this offer has already been resolved")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

// WithdrawFAOffer removes a pending offer. Only pending offers in a still-open
// window can be withdrawn — once resolution runs, an offer's outcome is final.
//
// DELETE /api/leagues/{id}/fa/offers/{offerId}
func (h *Handler) WithdrawFAOffer(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	offerID, err := parseID(r, "offerId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid offer id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	tag, err := h.db.Exec(r.Context(), `
		DELETE FROM league_fa_offers
		WHERE id = $1 AND league_id = $2 AND status = 'pending'
		  AND window_id IN (SELECT id FROM league_fa_windows WHERE resolved_at IS NULL)
	`, offerID, leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tag.RowsAffected() == 0 {
		respondError(w, http.StatusNotFound, "no withdrawable offer with that id")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "withdrawn"})
}

// ReorderFAOffers rewrites one team's whole priority order in one call —
// same reasoning as the draft-prep board's reorder endpoint: a priority
// only means something relative to a team's other offers, so a partial
// write would leave the order inconsistent.
//
// PUT /api/leagues/{id}/fa/offers/priority
func (h *Handler) ReorderFAOffers(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	var req struct {
		TeamID   int64   `json:"team_id"`
		OfferIDs []int64 `json:"offer_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.TeamID == 0 || len(req.OfferIDs) == 0 {
		respondError(w, http.StatusBadRequest, "team_id and offer_ids are required")
		return
	}

	windowID, _, open, err := openFAWindowID(r.Context(), h.db, leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !open {
		respondError(w, http.StatusUnprocessableEntity, "no free agency window is open")
		return
	}

	tag, err := h.db.Exec(r.Context(), `
		UPDATE league_fa_offers o
		SET priority = v.ord
		FROM unnest($1::bigint[]) WITH ORDINALITY AS v(id, ord)
		WHERE o.id = v.id AND o.league_id = $2 AND o.window_id = $3 AND o.team_id = $4 AND o.status = 'pending'
	`, req.OfferIDs, leagueID, windowID, req.TeamID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if int(tag.RowsAffected()) != len(req.OfferIDs) {
		respondError(w, http.StatusBadRequest, "one or more offer ids are not this team's pending offers in the open window")
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "reordered"})
}

// ── valuation ────────────────────────────────────────────────────────────────

type faValuationResp struct {
	GsisID           string `json:"gsis_id"`
	Position         string `json:"position"`
	Age              int    `json:"age"`
	AuctionValue     int    `json:"auction_value"`
	ReservationValue int    `json:"reservation_value"`
	PreferredYears   int    `json:"preferred_years"`
}

// preferredYears is a free agent's own preferred contract length — L* in
// the offer-scoring formula. A single global formula, not tuned per league,
// same posture as auctionVORCompressionExponent (draft_values.go) and the
// rookie scale's default percentiles: age pushes it up (an older player
// closer to or past his position's prime wants a security blanket), a high
// quality percentile pulls it down (a young star would rather re-hit the
// market at peak value than get locked in early). qualityPercentile is 1.0
// for the best player at his position on the board, 0.0 for the worst.
func preferredYears(posGroup string, age int, qualityPercentile float64) int {
	ph, ok := aging.DefaultPhases[posGroup]
	if !ok {
		ph = aging.PhaseRange{PrimeStart: 24, PrimeEnd: 30, PostPrimeEnd: 33}
	}
	if age <= 0 {
		age = ph.PrimeStart + (ph.PrimeEnd-ph.PrimeStart)/2 // no age data: assume mid-prime, neutral
	}
	ageRatio := (float64(age) - float64(ph.PrimeStart)) / float64(ph.PostPrimeEnd-ph.PrimeStart)
	if ageRatio < 0 {
		ageRatio = 0
	}
	if ageRatio > 1.3 {
		ageRatio = 1.3
	}
	raw := 2.0 + ageRatio*2.0 - qualityPercentile*2.0
	years := int(math.Round(raw))
	if years < 1 {
		years = 1
	}
	if years > 5 {
		years = 5
	}
	return years
}

// maxOfferYears bounds both a submitted offer's length and the fit-scoring
// formula's denominator — long enough to cover any realistic dynasty deal
// without letting a token 20-year offer dominate on length alone.
const maxOfferYears = 6

// offerScore ranks competing offers on one player: total value, discounted
// by how far the offer's length misses the player's own preferred length.
// See dynasty-transactions.md's "Offer scoring" section.
func offerScore(salary, years, lStar int) float64 {
	const penalty = 0.15
	fit := 1 - penalty*math.Abs(float64(years-lStar))/float64(maxOfferYears)
	if fit < 0.1 {
		fit = 0.1
	}
	return float64(salary) * float64(years) * fit
}

// posMaxRank returns each position group's highest PositionRank on a board —
// the denominator for turning a rank into a percentile.
func posMaxRank(players []draftPlayer) map[string]int {
	out := map[string]int{}
	for _, p := range players {
		pos := primaryPosition(p.PositionGroup)
		if p.PositionRank > out[pos] {
			out[pos] = p.PositionRank
		}
	}
	return out
}

func qualityPercentile(p draftPlayer, maxRank map[string]int) float64 {
	pos := primaryPosition(p.PositionGroup)
	max := maxRank[pos]
	if max <= 1 {
		return 1.0
	}
	return 1.0 - float64(p.PositionRank-1)/float64(max-1)
}

// GetFAValuations returns each requested player's real auction value, the
// reservation floor derived from it, and his own preferred contract
// length — the exact numbers an offer gets scored against, shown so a
// manager can reason about what will actually win rather than guessing at
// a hidden formula.
//
// GET /api/leagues/{id}/fa/valuations?gsis_ids=a,b,c
func (h *Handler) GetFAValuations(w http.ResponseWriter, r *http.Request) {
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	raw := r.URL.Query().Get("gsis_ids")
	if raw == "" {
		respondError(w, http.StatusBadRequest, "gsis_ids is required")
		return
	}
	wanted := strings.Split(raw, ",")

	season := h.leagueSeasonInt(r, leagueID)
	priceSeason, err := h.priceableSeason(r.Context(), season)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	in, err := h.resolveNativeDraftBoardInputs(r.Context(), leagueID, priceSeason)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	players, _, _, err := h.computeDraftBoard(r.Context(), in)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	byGsis := map[string]draftPlayer{}
	for _, p := range players {
		byGsis[p.GsisID] = p
	}
	maxRank := posMaxRank(players)

	var reservationPct float64
	if err := h.db.QueryRow(r.Context(),
		"SELECT fa_reservation_pct FROM league_settings WHERE league_id = $1", leagueID,
	).Scan(&reservationPct); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := make([]faValuationResp, 0, len(wanted))
	var fallback []string
	for _, g := range wanted {
		p, ok := byGsis[g]
		if !ok {
			fallback = append(fallback, g)
			continue
		}
		resp = append(resp, faValuationResp{
			GsisID:           p.GsisID,
			Position:         p.Position,
			Age:              p.Age,
			AuctionValue:     p.AuctionValue,
			ReservationValue: int(math.Round(float64(p.AuctionValue) * reservationPct)),
			PreferredYears:   preferredYears(p.PositionGroup, p.Age, qualityPercentile(p, maxRank)),
		})
	}
	if len(fallback) > 0 {
		rows, err := h.db.Query(r.Context(),
			"SELECT gsis_id, COALESCE(position, '') FROM nfl_players WHERE gsis_id = ANY($1)", fallback,
		)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for rows.Next() {
			var gsisID, position string
			if err := rows.Scan(&gsisID, &position); err != nil {
				rows.Close()
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			// No projection for this season at all — a fallback with no
			// signal to price off, not a computed valuation.
			resp = append(resp, faValuationResp{GsisID: gsisID, Position: position, ReservationValue: 1, PreferredYears: 2})
		}
		rows.Close()
	}
	respondJSON(w, http.StatusOK, resp)
}

// ── resolution ───────────────────────────────────────────────────────────────

type pendingOffer struct {
	id       int64
	gsisID   string
	teamID   int64
	salary   int
	years    int
	priority int
	created  time.Time
}

// ResolveFAWindow runs every pending offer in a window to a conclusion:
// best players first (so cap spent at the top of the market cascades down
// the way real free agency does), each offer checked against its team's
// live cap room *after* reserving for that same team's still-pending,
// higher-priority offers — so a team's stated preference survives even
// when the market doesn't resolve players in that team's preferred order.
// An explicit commissioner action, never a cron, matching ScoreLeagueWeek.
//
// POST /api/leagues/{id}/fa/windows/{windowId}/resolve
func (h *Handler) ResolveFAWindow(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	windowID, err := parseID(r, "windowId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid window id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())

	var season int
	var resolvedAt *time.Time
	if err := tx.QueryRow(r.Context(),
		"SELECT season, resolved_at FROM league_fa_windows WHERE id = $1 AND league_id = $2 FOR UPDATE",
		windowID, leagueID,
	).Scan(&season, &resolvedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			respondError(w, http.StatusNotFound, "window not found in this league")
			return
		}
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if resolvedAt != nil {
		respondError(w, http.StatusConflict, "this window has already been resolved")
		return
	}

	rows, err := tx.Query(r.Context(), `
		SELECT id, gsis_id, team_id, salary, years, priority, created_at
		FROM league_fa_offers WHERE window_id = $1 AND status = 'pending' FOR UPDATE
	`, windowID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var offers []pendingOffer
	for rows.Next() {
		var o pendingOffer
		if err := rows.Scan(&o.id, &o.gsisID, &o.teamID, &o.salary, &o.years, &o.priority, &o.created); err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		offers = append(offers, o)
	}
	rows.Close()

	if len(offers) == 0 {
		if _, err := tx.Exec(r.Context(), "UPDATE league_fa_windows SET resolved_at = NOW() WHERE id = $1", windowID); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := tx.Commit(r.Context()); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{"status": "resolved", "signed": []any{}})
		return
	}

	priceSeason, err := h.priceableSeason(r.Context(), season)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	in, err := h.resolveNativeDraftBoardInputs(r.Context(), leagueID, priceSeason)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	players, _, _, err := h.computeDraftBoard(r.Context(), in)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	byGsis := map[string]draftPlayer{}
	for _, p := range players {
		byGsis[p.GsisID] = p
	}
	maxRank := posMaxRank(players)

	var reservationPct float64
	if err := tx.QueryRow(r.Context(),
		"SELECT fa_reservation_pct FROM league_settings WHERE league_id = $1", leagueID,
	).Scan(&reservationPct); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	byPlayer := map[string][]*pendingOffer{}
	byTeam := map[int64][]*pendingOffer{}
	statusOf := map[int64]string{}
	for i := range offers {
		o := &offers[i]
		byPlayer[o.gsisID] = append(byPlayer[o.gsisID], o)
		byTeam[o.teamID] = append(byTeam[o.teamID], o)
		statusOf[o.id] = "pending"
	}

	gsisIDs := make([]string, 0, len(byPlayer))
	for g := range byPlayer {
		gsisIDs = append(gsisIDs, g)
	}
	sort.Slice(gsisIDs, func(i, j int) bool {
		vi, viok := byGsis[gsisIDs[i]]
		vj, vjok := byGsis[gsisIDs[j]]
		// Off-board players (no projection this season) resolve last, in a
		// stable order, rather than being interleaved arbitrarily among
		// real market values.
		switch {
		case viok && !vjok:
			return true
		case !viok && vjok:
			return false
		case viok && vjok && vi.AuctionValue != vj.AuctionValue:
			return vi.AuctionValue > vj.AuctionValue
		default:
			return gsisIDs[i] < gsisIDs[j]
		}
	})

	type signedEntry struct {
		GsisID string `json:"gsis_id"`
		Name   string `json:"name"`
		TeamID int64  `json:"team_id"`
		Salary int    `json:"salary"`
		Years  int    `json:"years"`
	}
	var signed []signedEntry

	for _, gsisID := range gsisIDs {
		var candidates []*pendingOffer
		for _, o := range byPlayer[gsisID] {
			if statusOf[o.id] == "pending" {
				candidates = append(candidates, o)
			}
		}
		if len(candidates) == 0 {
			continue
		}

		reservation := 1
		if p, ok := byGsis[gsisID]; ok {
			reservation = int(math.Round(float64(p.AuctionValue) * reservationPct))
		}

		var survivors []*pendingOffer
		for _, o := range candidates {
			if o.salary < reservation {
				statusOf[o.id] = "lost"
				continue
			}
			reserved := 0
			for _, other := range byTeam[o.teamID] {
				if other.id != o.id && statusOf[other.id] == "pending" && other.priority < o.priority {
					reserved += other.salary
				}
			}
			cb, err := teamCap(r.Context(), tx, leagueID, o.teamID, season)
			if err != nil {
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if cb.RosterCount+1 > cb.RosterMax || o.salary > cb.Available-reserved {
				statusOf[o.id] = "lost"
				continue
			}
			survivors = append(survivors, o)
		}
		if len(survivors) == 0 {
			continue
		}

		lStar := 2
		if p, ok := byGsis[gsisID]; ok {
			lStar = preferredYears(p.PositionGroup, p.Age, qualityPercentile(p, maxRank))
		}
		best := survivors[0]
		bestScore := offerScore(best.salary, best.years, lStar)
		for _, o := range survivors[1:] {
			s := offerScore(o.salary, o.years, lStar)
			if s > bestScore || (s == bestScore && o.created.Before(best.created)) {
				best, bestScore = o, s
			}
		}

		if err := assignRosterTx(r.Context(), tx, leagueID, best.teamID, gsisID, "BN", "fa", best.salary, season, &best.years); err != nil {
			respondError(w, http.StatusInternalServerError, "signing "+gsisID+": "+err.Error())
			return
		}
		statusOf[best.id] = "won"
		for _, o := range survivors {
			if o.id != best.id {
				statusOf[o.id] = "lost"
			}
		}
		name := gsisID
		if p, ok := byGsis[gsisID]; ok {
			name = p.Name
		}
		signed = append(signed, signedEntry{GsisID: gsisID, Name: name, TeamID: best.teamID, Salary: best.salary, Years: best.years})

		payload, _ := json.Marshal(map[string]any{"gsis_id": gsisID, "team_id": best.teamID, "salary": best.salary, "years": best.years, "window_id": windowID})
		if _, err := tx.Exec(r.Context(),
			`INSERT INTO league_transactions (league_id, season, kind, payload, created_by) VALUES ($1,$2,'sign',$3,$4)`,
			leagueID, season, payload, user.ID,
		); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	for id, st := range statusOf {
		if st == "pending" {
			st = "lost" // defensive: every player with an offer is processed above, so this shouldn't fire
		}
		if _, err := tx.Exec(r.Context(), "UPDATE league_fa_offers SET status = $1 WHERE id = $2", st, id); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if _, err := tx.Exec(r.Context(), "UPDATE league_fa_windows SET resolved_at = NOW() WHERE id = $1", windowID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"status": "resolved", "signed": signed})
}
