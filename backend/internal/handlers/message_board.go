package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/davidyoung/fantasy-sports/backend/internal/models"

	"github.com/jackc/pgx/v5"
)

// League message board (native leagues only). "Author" is a team, not a
// user — this app has no concept of an individual manager beyond team
// ownership, so posts display team identity the same way every other
// native-league surface (Standings, Roster, Trade builder) already does.
//
// Deliberate v1 scope cuts, all noted in .claude/plans/native-leagues.md:
//   - Reactions are a single "+1" glyph, not the six emoji the design
//     mockup used — this app has a standing "no emoji outside the sport
//     label" rule enforced everywhere else, and the mockup itself offered
//     +1 as the fallback.
//   - Mentions (@TeamName, @all) are plain text with no notification
//     system behind them — nothing in this app sends notifications today.
//     The "Mentions" filter is computed by a substring match, not a stored
//     mentions table.
//   - Image attachment is a pasted URL rendered inline — no upload
//     pipeline, matching the design doc's own cheapest-v1 fallback.
//   - Trade-card attachment on a reply was not built — the composer
//     toolbar in the spec only lists Mention/Player/Image/Poll, so the
//     trade-card example in the mockup reads as illustrative content, not
//     a described interaction.
//   - The activity rail surfaces trade and scored-week events only — native
//     roster adds/drops don't write to league_transactions today, and
//     wiring that up was out of scope for this pass.

// ── response types ──────────────────────────────────────────────────────

type postAuthorResp struct {
	TeamID int64  `json:"team_id"`
	Name   string `json:"name"`
}

type reactionResp struct {
	Reaction string `json:"reaction"`
	Count    int    `json:"count"`
	Mine     bool   `json:"mine"`
}

type attachedPlayerResp struct {
	GsisID      string `json:"gsis_id"`
	Name        string `json:"name"`
	Position    string `json:"position"`
	Team        string `json:"team"`
	HeadshotURL string `json:"headshot_url,omitempty"`
}

type pollOptionResp struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
	Votes int    `json:"votes"`
}

type pollResp struct {
	Options      []pollOptionResp `json:"options"`
	TotalVotes   int              `json:"total_votes"`
	MyOptionID   *int64           `json:"my_option_id,omitempty"`
	ClosesAt     *time.Time       `json:"closes_at,omitempty"`
	VotesVisible string           `json:"votes_visible"`
}

type postResp struct {
	ID             int64               `json:"id"`
	ParentID       *int64              `json:"parent_id,omitempty"`
	Author         postAuthorResp      `json:"author"`
	Title          *string             `json:"title,omitempty"`
	Body           string              `json:"body"`
	ImageURL       *string             `json:"image_url,omitempty"`
	AttachedPlayer *attachedPlayerResp `json:"attached_player,omitempty"`
	IsPinned       bool                `json:"is_pinned"`
	IsDeleted      bool                `json:"is_deleted"`
	Mine           bool                `json:"mine"`
	EditedAt       *time.Time          `json:"edited_at,omitempty"`
	CreatedAt      time.Time           `json:"created_at"`
	Reactions      []reactionResp      `json:"reactions,omitempty"`
	ReplyCount     int                 `json:"reply_count,omitempty"`
	LastActivityAt time.Time           `json:"last_activity_at,omitempty"`
	Unread         bool                `json:"unread,omitempty"`
	Poll           *pollResp           `json:"poll,omitempty"`
}

// ── shared loaders ──────────────────────────────────────────────────────

// postRow is the raw scan target shared by every query that reads league_posts.
type postRow struct {
	ID             int64
	ParentID       *int64
	AuthorTeamID   int64
	AuthorTeamName string
	Title          *string
	Body           string
	ImageURL       *string
	AttachedGsisID *string
	IsPinned       bool
	IsDeleted      bool
	EditedAt       *time.Time
	CreatedAt      time.Time
	PollClosesAt   *time.Time
	PollVotesVis   string
}

const postSelectCols = `
	p.id, p.parent_id, p.author_team_id, t.name,
	p.title, p.body, p.image_url, p.attached_gsis_id,
	p.is_pinned, p.is_deleted, p.edited_at, p.created_at,
	p.poll_closes_at, p.poll_votes_visible
`

func scanPostRow(row pgx.Row) (postRow, error) {
	var pr postRow
	err := row.Scan(
		&pr.ID, &pr.ParentID, &pr.AuthorTeamID, &pr.AuthorTeamName,
		&pr.Title, &pr.Body, &pr.ImageURL, &pr.AttachedGsisID,
		&pr.IsPinned, &pr.IsDeleted, &pr.EditedAt, &pr.CreatedAt,
		&pr.PollClosesAt, &pr.PollVotesVis,
	)
	return pr, err
}

// attachEnrichment fills in reactions, attached player, poll, reply count,
// and unread state for a batch of posts. Loading these in a handful of
// batched queries (rather than one query per post) is what keeps a 25-row
// thread list or a thread-with-replies page cheap.
func (h *Handler) attachEnrichment(ctx context.Context, viewerUserID int64, myTeamID int64, rows []postRow, withReplyCounts bool) ([]postResp, error) {
	out := make([]postResp, len(rows))
	ids := make([]int64, len(rows))
	for i, pr := range rows {
		ids[i] = pr.ID
		title := pr.Title
		body := pr.Body
		if pr.IsDeleted {
			body = "[deleted]"
			title = nil
		}
		out[i] = postResp{
			ID: pr.ID, ParentID: pr.ParentID,
			Author: postAuthorResp{TeamID: pr.AuthorTeamID, Name: pr.AuthorTeamName},
			Title:  title, Body: body,
			ImageURL: pr.ImageURL,
			IsPinned: pr.IsPinned, IsDeleted: pr.IsDeleted,
			Mine:     pr.AuthorTeamID == myTeamID,
			EditedAt: pr.EditedAt, CreatedAt: pr.CreatedAt,
			LastActivityAt: pr.CreatedAt,
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	byID := map[int64]int{}
	for i, id := range ids {
		byID[id] = i
	}

	// Reactions
	rrows, err := h.db.Query(ctx, `
		SELECT post_id, reaction, COUNT(*), COALESCE(bool_or(user_id = $2), false)
		FROM league_post_reactions WHERE post_id = ANY($1) GROUP BY post_id, reaction
	`, ids, viewerUserID)
	if err != nil {
		return nil, err
	}
	for rrows.Next() {
		var postID int64
		var reaction string
		var count int
		var mine bool
		if err := rrows.Scan(&postID, &reaction, &count, &mine); err != nil {
			rrows.Close()
			return nil, err
		}
		i := byID[postID]
		out[i].Reactions = append(out[i].Reactions, reactionResp{Reaction: reaction, Count: count, Mine: mine})
	}
	rrows.Close()
	if err := rrows.Err(); err != nil {
		return nil, err
	}

	// Attached players
	var gsisIDs []string
	postForGsis := map[string][]int64{}
	for i, pr := range rows {
		if pr.AttachedGsisID != nil {
			gsisIDs = append(gsisIDs, *pr.AttachedGsisID)
			postForGsis[*pr.AttachedGsisID] = append(postForGsis[*pr.AttachedGsisID], int64(i))
		}
	}
	if len(gsisIDs) > 0 {
		prows, err := h.db.Query(ctx, `
			SELECT gsis_id, name, COALESCE(position,''), COALESCE(team,''), COALESCE(headshot_url,'')
			FROM nfl_players WHERE gsis_id = ANY($1)
		`, gsisIDs)
		if err != nil {
			return nil, err
		}
		for prows.Next() {
			var ap attachedPlayerResp
			if err := prows.Scan(&ap.GsisID, &ap.Name, &ap.Position, &ap.Team, &ap.HeadshotURL); err != nil {
				prows.Close()
				return nil, err
			}
			for _, i := range postForGsis[ap.GsisID] {
				apCopy := ap
				out[i].AttachedPlayer = &apCopy
			}
		}
		prows.Close()
		if err := prows.Err(); err != nil {
			return nil, err
		}
	}

	// Polls (options + vote counts + my vote)
	porows, err := h.db.Query(ctx, `
		SELECT o.post_id, o.id, o.label,
		       COUNT(v.user_id), COALESCE(bool_or(v.user_id = $2), false)
		FROM league_poll_options o
		LEFT JOIN league_poll_votes v ON v.option_id = o.id
		WHERE o.post_id = ANY($1)
		GROUP BY o.post_id, o.id, o.label, o.position
		ORDER BY o.post_id, o.position
	`, ids, viewerUserID)
	if err != nil {
		return nil, err
	}
	polls := map[int64]*pollResp{}
	for porows.Next() {
		var postID, optionID int64
		var label string
		var votes int
		var myVoteOnThisOption bool
		if err := porows.Scan(&postID, &optionID, &label, &votes, &myVoteOnThisOption); err != nil {
			porows.Close()
			return nil, err
		}
		p := polls[postID]
		if p == nil {
			p = &pollResp{}
			polls[postID] = p
		}
		p.Options = append(p.Options, pollOptionResp{ID: optionID, Label: label, Votes: votes})
		p.TotalVotes += votes
		if myVoteOnThisOption {
			id := optionID
			p.MyOptionID = &id
		}
	}
	porows.Close()
	if err := porows.Err(); err != nil {
		return nil, err
	}
	for i, pr := range rows {
		if p, ok := polls[pr.ID]; ok {
			p.ClosesAt = pr.PollClosesAt
			p.VotesVisible = pr.PollVotesVis
			out[i].Poll = p
		}
	}

	if withReplyCounts {
		crows, err := h.db.Query(ctx, `
			SELECT parent_id, COUNT(*), MAX(created_at)
			FROM league_posts WHERE parent_id = ANY($1) AND NOT is_deleted
			GROUP BY parent_id
		`, ids)
		if err != nil {
			return nil, err
		}
		for crows.Next() {
			var threadID int64
			var count int
			var lastAt time.Time
			if err := crows.Scan(&threadID, &count, &lastAt); err != nil {
				crows.Close()
				return nil, err
			}
			i := byID[threadID]
			out[i].ReplyCount = count
			if lastAt.After(out[i].LastActivityAt) {
				out[i].LastActivityAt = lastAt
			}
		}
		crows.Close()
		if err := crows.Err(); err != nil {
			return nil, err
		}

		// Unread: last_activity_at > this viewer's last_read_at (or never read at all).
		readRows, err := h.db.Query(ctx, `
			SELECT thread_id, last_read_at FROM league_thread_reads
			WHERE thread_id = ANY($1) AND user_id = $2
		`, ids, viewerUserID)
		if err != nil {
			return nil, err
		}
		reads := map[int64]time.Time{}
		for readRows.Next() {
			var threadID int64
			var lastReadAt time.Time
			if err := readRows.Scan(&threadID, &lastReadAt); err != nil {
				readRows.Close()
				return nil, err
			}
			reads[threadID] = lastReadAt
		}
		readRows.Close()
		if err := readRows.Err(); err != nil {
			return nil, err
		}
		for i := range out {
			lastRead, ok := reads[out[i].ID]
			out[i].Unread = !ok || out[i].LastActivityAt.After(lastRead)
		}
	}

	return out, nil
}

// myTeamID resolves the caller's claimed team in a league, or 0 if they
// haven't claimed one — every native-league mutation this app has already
// tolerates that (the single-user model means the commissioner can act
// regardless), and posting here is no different.
func (h *Handler) myTeamID(ctx context.Context, leagueID, userID int64) int64 {
	var teamID int64
	h.db.QueryRow(ctx, "SELECT id FROM teams WHERE league_id = $1 AND user_id = $2", leagueID, userID).Scan(&teamID) //nolint:errcheck
	return teamID
}

// ── feed (activity rail) ────────────────────────────────────────────────

type feedItemResp struct {
	Kind string    `json:"kind"` // "post" | "event"
	Post *postResp `json:"post,omitempty"`
	// Event fields (kind == "event")
	EventText string    `json:"event_text,omitempty"`
	EventLink string    `json:"event_link,omitempty"`
	EventAt   time.Time `json:"event_at,omitempty"`
}

// GetLeagueFeed returns the Standings activity rail: the pinned thread (if
// any) followed by up to four more posts/events, interleaved by time.
//
// GET /api/leagues/{id}/feed
func (h *Handler) GetLeagueFeed(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	ctx := r.Context()
	myTeam := h.myTeamID(ctx, leagueID, user.ID)

	// Pinned thread, if any.
	var pinnedRow *postRow
	pr, err := scanPostRow(h.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM league_posts p JOIN teams t ON t.id = p.author_team_id
		WHERE p.league_id = $1 AND p.is_pinned AND p.parent_id IS NULL AND NOT p.is_deleted
	`, postSelectCols), leagueID))
	if err == nil {
		pinnedRow = &pr
	} else if err != pgx.ErrNoRows {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Recent (non-pinned) threads, enriched with reply counts for last-activity.
	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM league_posts p JOIN teams t ON t.id = p.author_team_id
		WHERE p.league_id = $1 AND p.parent_id IS NULL AND NOT p.is_deleted
		  AND NOT (p.is_pinned)
		ORDER BY p.created_at DESC LIMIT 10
	`, postSelectCols), leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var postRows []postRow
	for rows.Next() {
		pr, err := scanPostRow(rows)
		if err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		postRows = append(postRows, pr)
	}
	rows.Close()

	posts, err := h.attachEnrichment(ctx, user.ID, myTeam, postRows, true)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := []feedItemResp{}
	for i := range posts {
		p := posts[i]
		items = append(items, feedItemResp{Kind: "post", Post: &p, EventAt: p.LastActivityAt})
	}

	// Events: recent trades + scored-week headlines.
	events, err := h.recentLeagueEvents(ctx, leagueID, 10)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items = append(items, events...)

	sort.Slice(items, func(i, j int) bool { return items[i].EventAt.After(items[j].EventAt) })
	if len(items) > 4 {
		items = items[:4]
	}

	if pinnedRow != nil {
		pinned, err := h.attachEnrichment(ctx, user.ID, myTeam, []postRow{*pinnedRow}, true)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		items = append([]feedItemResp{{Kind: "post", Post: &pinned[0], EventAt: pinned[0].LastActivityAt}}, items...)
	}

	unread := 0
	for _, it := range items {
		if it.Kind == "post" && it.Post.Unread {
			unread++
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{"items": items, "unread_count": unread})
}

// recentLeagueEvents reads trade and scored-week history straight from
// existing tables — no new storage for events, per the design doc.
func (h *Handler) recentLeagueEvents(ctx context.Context, leagueID int64, limit int) ([]feedItemResp, error) {
	var events []feedItemResp

	trows, err := h.db.Query(ctx, `
		SELECT lt.payload, lt.created_at
		FROM league_transactions lt
		WHERE lt.league_id = $1 AND lt.kind = 'trade'
		ORDER BY lt.created_at DESC LIMIT $2
	`, leagueID, limit)
	if err != nil {
		return nil, err
	}
	type move struct {
		FromTeamID int64 `json:"from_team_id"`
		ToTeamID   int64 `json:"to_team_id"`
	}
	teamNameCache := map[int64]string{}
	teamName := func(id int64) string {
		if n, ok := teamNameCache[id]; ok {
			return n
		}
		var n string
		h.db.QueryRow(ctx, "SELECT name FROM teams WHERE id = $1", id).Scan(&n) //nolint:errcheck
		teamNameCache[id] = n
		return n
	}
	for trows.Next() {
		var raw []byte
		var createdAt time.Time
		if err := trows.Scan(&raw, &createdAt); err != nil {
			trows.Close()
			return nil, err
		}
		var payload struct {
			Moves []move `json:"moves"`
		}
		if err := json.Unmarshal(raw, &payload); err == nil && len(payload.Moves) > 0 {
			teamsInvolved := map[int64]bool{}
			for _, m := range payload.Moves {
				teamsInvolved[m.FromTeamID] = true
				teamsInvolved[m.ToTeamID] = true
			}
			names := make([]string, 0, len(teamsInvolved))
			for id := range teamsInvolved {
				names = append(names, teamName(id))
			}
			sort.Strings(names)
			text := fmt.Sprintf("Trade accepted — %s — %d asset%s", strings.Join(names, " & "), len(payload.Moves), plural(len(payload.Moves)))
			events = append(events, feedItemResp{Kind: "event", EventText: text, EventLink: "transactions", EventAt: createdAt})
		}
	}
	trows.Close()
	if err := trows.Err(); err != nil {
		return nil, err
	}

	// One headline event per scored week: the highest-combined-points matchup.
	wrows, err := h.db.Query(ctx, `
		SELECT DISTINCT season, week FROM league_matchups
		WHERE league_id = $1 AND computed_at IS NOT NULL
		ORDER BY season DESC, week DESC LIMIT $2
	`, leagueID, limit)
	if err != nil {
		return nil, err
	}
	type weekKey struct{ season, week int }
	var weeks []weekKey
	for wrows.Next() {
		var wk weekKey
		if err := wrows.Scan(&wk.season, &wk.week); err != nil {
			wrows.Close()
			return nil, err
		}
		weeks = append(weeks, wk)
	}
	wrows.Close()

	for _, wk := range weeks {
		var t1Name, t2Name string
		var p1, p2 float64
		var scoredAt time.Time
		err := h.db.QueryRow(ctx, `
			SELECT t1.name, m.team1_points, t2.name, m.team2_points, m.computed_at
			FROM league_matchups m
			JOIN teams t1 ON t1.id = m.team1_id
			JOIN teams t2 ON t2.id = m.team2_id
			WHERE m.league_id = $1 AND m.season = $2 AND m.week = $3
			ORDER BY (m.team1_points + m.team2_points) DESC LIMIT 1
		`, leagueID, wk.season, wk.week).Scan(&t1Name, &p1, &t2Name, &p2, &scoredAt)
		if err != nil {
			continue
		}
		winner, winPts, loser, losePts := t1Name, p1, t2Name, p2
		if p2 > p1 {
			winner, winPts, loser, losePts = t2Name, p2, t1Name, p1
		}
		text := fmt.Sprintf("Week %d scored — %s %.1f def. %s %.1f, high score of the week", wk.week, winner, winPts, loser, losePts)
		events = append(events, feedItemResp{Kind: "event", EventText: text, EventLink: "scoreboard", EventAt: scoredAt})
	}

	return events, nil
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// ── threads list ────────────────────────────────────────────────────────

// ListLeagueThreads returns root posts (threads), newest activity first,
// pinned thread always first. filter: all|unread|mentions|mine.
//
// GET /api/leagues/{id}/threads?filter=&offset=&limit=
func (h *Handler) ListLeagueThreads(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	ctx := r.Context()
	myTeam := h.myTeamID(ctx, leagueID, user.ID)

	q := r.URL.Query()
	filter := q.Get("filter")
	limit := 25
	if l, err := strconv.Atoi(q.Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	offset := 0
	if o, err := strconv.Atoi(q.Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM league_posts p JOIN teams t ON t.id = p.author_team_id
		WHERE p.league_id = $1 AND p.parent_id IS NULL AND NOT p.is_deleted
		ORDER BY p.is_pinned DESC, p.created_at DESC
	`, postSelectCols), leagueID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var all []postRow
	for rows.Next() {
		pr, err := scanPostRow(rows)
		if err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		all = append(all, pr)
	}
	rows.Close()

	threads, err := h.attachEnrichment(ctx, user.ID, myTeam, all, true)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	myName := ""
	if myTeam != 0 {
		h.db.QueryRow(ctx, "SELECT name FROM teams WHERE id = $1", myTeam).Scan(&myName) //nolint:errcheck
	}

	// Mentions can live in a reply, not just the thread's root post — so
	// "mentioned in this thread" is computed over every post in it, not
	// just the root's own title+body.
	mentionedThreads := map[int64]bool{}
	if filter == "mentions" {
		needles := []string{"@all"}
		if myName != "" {
			needles = append(needles, "@"+strings.ToLower(myName))
		}
		mrows, err := h.db.Query(ctx, `
			SELECT COALESCE(parent_id, id), COALESCE(title,'') || ' ' || body
			FROM league_posts WHERE league_id = $1 AND NOT is_deleted
		`, leagueID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for mrows.Next() {
			var threadID int64
			var text string
			if err := mrows.Scan(&threadID, &text); err != nil {
				mrows.Close()
				respondError(w, http.StatusInternalServerError, err.Error())
				return
			}
			text = strings.ToLower(text)
			for _, n := range needles {
				if strings.Contains(text, n) {
					mentionedThreads[threadID] = true
					break
				}
			}
		}
		mrows.Close()
		if err := mrows.Err(); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}

	// Re-sort by last activity (reply counts can reorder threads relative to
	// the created_at-only DB order), pinned still first.
	sort.SliceStable(threads, func(i, j int) bool {
		if threads[i].IsPinned != threads[j].IsPinned {
			return threads[i].IsPinned
		}
		return threads[i].LastActivityAt.After(threads[j].LastActivityAt)
	})

	filtered := make([]postResp, 0, len(threads))
	for _, t := range threads {
		switch filter {
		case "unread":
			if !t.Unread {
				continue
			}
		case "mine":
			if !t.Mine {
				continue
			}
		case "mentions":
			if !mentionedThreads[t.ID] {
				continue
			}
		}
		filtered = append(filtered, t)
	}

	total := len(filtered)
	end := offset + limit
	if end > total {
		end = total
	}
	page := []postResp{}
	if offset < total {
		page = filtered[offset:end]
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"threads":  page,
		"total":    total,
		"has_more": end < total,
	})
}

// ── thread detail ───────────────────────────────────────────────────────

// GetLeagueThread returns a thread's root post plus its (one level of)
// replies, oldest first.
//
// GET /api/leagues/{id}/threads/{threadId}
func (h *Handler) GetLeagueThread(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	threadID, err := parseID(r, "threadId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid thread id")
		return
	}
	ctx := r.Context()
	myTeam := h.myTeamID(ctx, leagueID, user.ID)

	root, err := scanPostRow(h.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM league_posts p JOIN teams t ON t.id = p.author_team_id
		WHERE p.id = $1 AND p.league_id = $2 AND p.parent_id IS NULL
	`, postSelectCols), threadID, leagueID))
	if err == pgx.ErrNoRows {
		respondError(w, http.StatusNotFound, "thread not found")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rows, err := h.db.Query(ctx, fmt.Sprintf(`
		SELECT %s FROM league_posts p JOIN teams t ON t.id = p.author_team_id
		WHERE p.parent_id = $1 ORDER BY p.created_at ASC
	`, postSelectCols), threadID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var replyRows []postRow
	for rows.Next() {
		pr, err := scanPostRow(rows)
		if err != nil {
			rows.Close()
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		replyRows = append(replyRows, pr)
	}
	rows.Close()

	all, err := h.attachEnrichment(ctx, user.ID, myTeam, append([]postRow{root}, replyRows...), false)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if _, err := h.db.Exec(ctx, `
		INSERT INTO league_thread_reads (thread_id, user_id, last_read_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (thread_id, user_id) DO UPDATE SET last_read_at = NOW()
	`, threadID, user.ID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"thread": all[0], "replies": all[1:]})
}

// MarkThreadRead lets the client mark a thread read without opening it
// (e.g. the "mark all read" affordance in the design doc).
//
// POST /api/leagues/{id}/threads/{threadId}/read
func (h *Handler) MarkThreadRead(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	threadID, err := parseID(r, "threadId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid thread id")
		return
	}
	if _, err := h.db.Exec(r.Context(), `
		INSERT INTO league_thread_reads (thread_id, user_id, last_read_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (thread_id, user_id) DO UPDATE SET last_read_at = NOW()
	`, threadID, user.ID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ── create / edit / delete ──────────────────────────────────────────────

type pollOptionReq struct {
	Label string `json:"label"`
}

type createPostReq struct {
	Title            *string         `json:"title"`
	Body             string          `json:"body"`
	ImageURL         *string         `json:"image_url"`
	AttachedGsisID   *string         `json:"attached_gsis_id"`
	PollOptions      []pollOptionReq `json:"poll_options"`
	PollClosesAt     *time.Time      `json:"poll_closes_at"`
	PollVotesVisible string          `json:"poll_votes_visible"`
}

// resolveAuthorTeam returns the caller's claimed team, falling back to the
// league's first team when the caller hasn't claimed one — commissioners
// post on behalf of the league in every other native-league flow (trades,
// picks) when acting outside their own team's context, and this matches.
func (h *Handler) resolveAuthorTeam(ctx context.Context, leagueID, userID int64) (int64, error) {
	if id := h.myTeamID(ctx, leagueID, userID); id != 0 {
		return id, nil
	}
	var id int64
	err := h.db.QueryRow(ctx, "SELECT id FROM teams WHERE league_id = $1 ORDER BY id LIMIT 1", leagueID).Scan(&id)
	return id, err
}

// CreateLeagueThread creates a new top-level thread, optionally with a
// poll. Any team owner may post — this isn't commissioner-gated, unlike
// roster/trade mutations, since a message board is meant for every manager.
//
// POST /api/leagues/{id}/threads
func (h *Handler) CreateLeagueThread(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	var req createPostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		respondError(w, http.StatusBadRequest, "body is required")
		return
	}
	if len(req.PollOptions) == 1 {
		respondError(w, http.StatusBadRequest, "a poll needs at least two options")
		return
	}
	votesVisible := req.PollVotesVisible
	if votesVisible == "" {
		votesVisible = "after_voting"
	}

	ctx := r.Context()
	authorTeam, err := h.resolveAuthorTeam(ctx, leagueID, user.ID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "league has no teams to post as")
		return
	}

	tx, err := h.db.Begin(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var postID int64
	err = tx.QueryRow(ctx, `
		INSERT INTO league_posts (league_id, author_team_id, title, body, image_url, attached_gsis_id, poll_closes_at, poll_votes_visible)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING id
	`, leagueID, authorTeam, req.Title, req.Body, req.ImageURL, req.AttachedGsisID, req.PollClosesAt, votesVisible).Scan(&postID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	for i, opt := range req.PollOptions {
		if strings.TrimSpace(opt.Label) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO league_poll_options (post_id, label, position) VALUES ($1, $2, $3)
		`, postID, opt.Label, i); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(ctx); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.getPostByID(w, r, postID, user.ID, leagueID)
}

// CreateLeagueReply adds a reply to a thread. One level of nesting only —
// replying to a reply still attaches to the thread's root post_id, which
// the frontend enforces by always passing the thread id, not a reply id.
//
// POST /api/leagues/{id}/threads/{threadId}/replies
func (h *Handler) CreateLeagueReply(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	threadID, err := parseID(r, "threadId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid thread id")
		return
	}
	var req createPostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		respondError(w, http.StatusBadRequest, "body is required")
		return
	}
	ctx := r.Context()

	var exists bool
	if err := h.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM league_posts WHERE id = $1 AND league_id = $2 AND parent_id IS NULL AND NOT is_deleted)
	`, threadID, leagueID).Scan(&exists); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		respondError(w, http.StatusNotFound, "thread not found")
		return
	}

	authorTeam, err := h.resolveAuthorTeam(ctx, leagueID, user.ID)
	if err != nil {
		respondError(w, http.StatusBadRequest, "league has no teams to post as")
		return
	}

	var postID int64
	err = h.db.QueryRow(ctx, `
		INSERT INTO league_posts (league_id, parent_id, author_team_id, body, image_url, attached_gsis_id)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id
	`, leagueID, threadID, authorTeam, req.Body, req.ImageURL, req.AttachedGsisID).Scan(&postID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}

	h.getPostByID(w, r, postID, user.ID, leagueID)
}

func (h *Handler) getPostByID(w http.ResponseWriter, r *http.Request, postID, userID, leagueID int64) {
	ctx := r.Context()
	myTeam := h.myTeamID(ctx, leagueID, userID)
	pr, err := scanPostRow(h.db.QueryRow(ctx, fmt.Sprintf(`
		SELECT %s FROM league_posts p JOIN teams t ON t.id = p.author_team_id WHERE p.id = $1
	`, postSelectCols), postID))
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	posts, err := h.attachEnrichment(ctx, userID, myTeam, []postRow{pr}, true)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusCreated, posts[0])
}

type editPostReq struct {
	Body string `json:"body"`
}

// EditLeaguePost lets a post's author edit its body — title, poll, and
// attachments are fixed at creation (the mockup only shows body edits).
//
// PUT /api/leagues/{id}/posts/{postId}
func (h *Handler) EditLeaguePost(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	postID, err := parseID(r, "postId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid post id")
		return
	}
	var req editPostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if strings.TrimSpace(req.Body) == "" {
		respondError(w, http.StatusBadRequest, "body is required")
		return
	}
	ctx := r.Context()

	if ok, status, msg := h.canModifyPost(r, ctx, leagueID, postID, user); !ok {
		respondError(w, status, msg)
		return
	}

	if _, err := h.db.Exec(ctx, `
		UPDATE league_posts SET body = $1, edited_at = NOW() WHERE id = $2
	`, req.Body, postID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.getPostByID(w, r, postID, user.ID, leagueID)
}

// DeleteLeaguePost soft-deletes a post — its row (and any replies' parent
// pointer) stays, but the body is masked as "[deleted]" so a thread's
// reply count and mention history don't retroactively change.
//
// DELETE /api/leagues/{id}/posts/{postId}
func (h *Handler) DeleteLeaguePost(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	postID, err := parseID(r, "postId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid post id")
		return
	}
	ctx := r.Context()

	if ok, status, msg := h.canModifyPost(r, ctx, leagueID, postID, user); !ok {
		respondError(w, status, msg)
		return
	}

	if _, err := h.db.Exec(ctx, `UPDATE league_posts SET is_deleted = true WHERE id = $1`, postID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// canModifyPost allows a post's own author, or the league commissioner
// (moderation), to edit/delete it.
func (h *Handler) canModifyPost(r *http.Request, ctx context.Context, leagueID, postID int64, user *models.User) (ok bool, status int, msg string) {
	var authorTeamID int64
	var postLeagueID int64
	err := h.db.QueryRow(ctx, `SELECT league_id, author_team_id FROM league_posts WHERE id = $1`, postID).Scan(&postLeagueID, &authorTeamID)
	if err == pgx.ErrNoRows {
		return false, http.StatusNotFound, "post not found"
	} else if err != nil {
		return false, http.StatusInternalServerError, err.Error()
	}
	if postLeagueID != leagueID {
		return false, http.StatusNotFound, "post not found"
	}
	if h.myTeamID(ctx, leagueID, user.ID) == authorTeamID {
		return true, 0, ""
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		return false, http.StatusForbidden, "only the author or commissioner can do that"
	} else {
		_ = msg
	}
	return true, 0, ""
}

// PinLeagueThread sets or clears a thread's pin — commissioner-only, since
// it's a league-wide broadcast slot, and the DB enforces at most one.
//
// PUT /api/leagues/{id}/threads/{threadId}/pin  {"pinned": true}
func (h *Handler) PinLeagueThread(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	threadID, err := parseID(r, "threadId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid thread id")
		return
	}
	if status, msg := h.requireCommissioner(r, user, leagueID); status != 0 {
		respondError(w, status, msg)
		return
	}
	var req struct {
		Pinned bool `json:"pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	ctx := r.Context()

	if req.Pinned {
		tx, err := h.db.Begin(ctx)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		defer tx.Rollback(ctx) //nolint:errcheck
		if _, err := tx.Exec(ctx, `
			UPDATE league_posts SET is_pinned = false WHERE league_id = $1 AND is_pinned AND parent_id IS NULL
		`, leagueID); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		res, err := tx.Exec(ctx, `
			UPDATE league_posts SET is_pinned = true WHERE id = $1 AND league_id = $2 AND parent_id IS NULL
		`, threadID, leagueID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if res.RowsAffected() == 0 {
			respondError(w, http.StatusNotFound, "thread not found")
			return
		}
		if err := tx.Commit(ctx); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	} else {
		if _, err := h.db.Exec(ctx, `
			UPDATE league_posts SET is_pinned = false WHERE id = $1 AND league_id = $2
		`, threadID, leagueID); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	h.getPostByID(w, r, threadID, user.ID, leagueID)
}

// ── reactions & polls ────────────────────────────────────────────────────

// ToggleLeaguePostReaction adds the caller's "+1" if absent, removes it if
// present — a single reaction type, per the v1 scope cut noted at the top
// of this file.
//
// POST /api/leagues/{id}/posts/{postId}/react
func (h *Handler) ToggleLeaguePostReaction(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	postID, err := parseID(r, "postId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid post id")
		return
	}
	ctx := r.Context()

	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM league_posts WHERE id = $1 AND league_id = $2)`, postID, leagueID).Scan(&exists); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !exists {
		respondError(w, http.StatusNotFound, "post not found")
		return
	}

	res, err := h.db.Exec(ctx, `
		DELETE FROM league_post_reactions WHERE post_id = $1 AND user_id = $2 AND reaction = '+1'
	`, postID, user.ID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if res.RowsAffected() == 0 {
		if _, err := h.db.Exec(ctx, `
			INSERT INTO league_post_reactions (post_id, user_id, reaction) VALUES ($1, $2, '+1')
		`, postID, user.ID); err != nil {
			respondError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	h.getPostByID(w, r, postID, user.ID, leagueID)
}

// VoteLeaguePoll casts (or changes) the caller's vote on a poll — one vote
// per user per poll, enforced by the table's primary key.
//
// POST /api/leagues/{id}/posts/{postId}/vote  {"option_id": 123}
func (h *Handler) VoteLeaguePoll(w http.ResponseWriter, r *http.Request) {
	user := requireUser(r)
	leagueID, err := parseID(r, "id")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid league id")
		return
	}
	postID, err := parseID(r, "postId")
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid post id")
		return
	}
	var req struct {
		OptionID int64 `json:"option_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid body")
		return
	}
	ctx := r.Context()

	var optionPostID int64
	var closesAt *time.Time
	err = h.db.QueryRow(ctx, `
		SELECT o.post_id, p.poll_closes_at FROM league_poll_options o
		JOIN league_posts p ON p.id = o.post_id
		WHERE o.id = $1
	`, req.OptionID).Scan(&optionPostID, &closesAt)
	if err == pgx.ErrNoRows || optionPostID != postID {
		respondError(w, http.StatusBadRequest, "invalid poll option")
		return
	} else if err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if closesAt != nil && closesAt.Before(time.Now()) {
		respondError(w, http.StatusConflict, "this poll has closed")
		return
	}

	var league int64
	if err := h.db.QueryRow(ctx, `SELECT league_id FROM league_posts WHERE id = $1`, postID).Scan(&league); err != nil || league != leagueID {
		respondError(w, http.StatusNotFound, "post not found")
		return
	}

	if _, err := h.db.Exec(ctx, `
		INSERT INTO league_poll_votes (post_id, option_id, user_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (post_id, user_id) DO UPDATE SET option_id = $2, created_at = NOW()
	`, postID, req.OptionID, user.ID); err != nil {
		respondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.getPostByID(w, r, postID, user.ID, leagueID)
}
