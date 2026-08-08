package yahoo

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// mock.go serves synthetic Yahoo Fantasy responses when YAHOO_MOCK=1, so the
// league side of the app is usable without live Yahoo access.
//
// The interception point is the HTTP transport rather than the Client methods:
// all 18 methods funnel through Client.get, so one RoundTripper covers every
// endpoint, and the real XML decoding in decode() still runs. That means these
// fixtures are continuously validated against the production types — a fixture
// that didn't match what the parser expects would fail exactly like a malformed
// Yahoo response.

// NewMockClient returns a Client backed by synthetic data instead of the Yahoo
// API. It needs no tokens, no network, and no database.
func NewMockClient() *Client {
	return &Client{httpClient: &http.Client{Transport: &mockTransport{}}}
}

type mockTransport struct{}

func (t *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Yahoo puts its parameters in path segments (";out=stats", ";week=3"),
	// not the query string, so routing keys off the raw path.
	path := req.URL.EscapedPath()
	if unescaped, err := req.URL.Parse(req.URL.String()); err == nil {
		path = unescaped.Path
	}

	content, ok := mockRoute(path)
	if !ok {
		// A 404 with the path in the body — an unmocked endpoint should be
		// immediately obvious rather than silently returning empty data.
		log.Printf("[yahoo/mock] no fixture for path: %s", path)
		return textResponse(http.StatusNotFound,
			fmt.Sprintf("mock: no fixture for %s", path)), nil
	}

	body, err := xml.Marshal(content)
	if err != nil {
		return nil, fmt.Errorf("mock: marshalling fixture for %s: %w", path, err)
	}
	return xmlResponse(append([]byte(xml.Header), body...)), nil
}

// mockRoute maps a Yahoo API path to the fixture that serves it.
func mockRoute(path string) (FantasyContent, bool) {
	switch {
	// Order matters: the leagues path is a superset of the bare users path.
	case strings.Contains(path, "/users;use_login=1/games"):
		return mockUserLeaguesContent(), true

	case strings.Contains(path, "/users;use_login=1"):
		return mockUsersContent(), true

	case strings.Contains(path, "/settings"):
		return mockSettingsContent(), true

	case strings.Contains(path, "/draftresults"):
		return mockDraftResultsContent(), true

	case strings.Contains(path, "/standings"):
		return mockStandingsContent(), true

	case strings.Contains(path, "/scoreboard"):
		return mockScoreboardContent(weekFromPath(path)), true

	// A team roster ("/team/{key}/roster/...") must be matched before the
	// league-wide "/teams/roster" case below.
	case strings.Contains(path, "/team/") && strings.Contains(path, "/roster"):
		return mockTeamRosterContent(teamKeyFromPath(path)), true

	case strings.Contains(path, "/teams/roster"):
		return mockTeamsContent(true), true

	case strings.Contains(path, "/teams"):
		return mockTeamsContent(false), true

	case strings.Contains(path, "/players"):
		pos, search, count, faOnly := playerFiltersFromPath(path)
		return mockPlayersContent(pos, search, count, faOnly), true
	}
	return FantasyContent{}, false
}

// weekFromPath pulls N out of a ";week=N" segment, returning 0 when absent
// (which the fixture treats as "current week").
func weekFromPath(path string) int {
	if v := segmentParam(path, "week"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return 0
}

// teamKeyFromPath extracts the team key from ".../team/{key}/roster...".
func teamKeyFromPath(path string) string {
	const marker = "/team/"
	i := strings.Index(path, marker)
	if i < 0 {
		return ""
	}
	rest := path[i+len(marker):]
	if j := strings.Index(rest, "/"); j >= 0 {
		return rest[:j]
	}
	return rest
}

// playerFiltersFromPath interprets the subset of Yahoo's player-query syntax the
// handlers actually use: ;position=, ;search=, ;count=, and ;status=.
func playerFiltersFromPath(path string) (position, search string, count int, freeAgentsOnly bool) {
	position = segmentParam(path, "position")
	search = segmentParam(path, "search")
	count = 25
	if v := segmentParam(path, "count"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			count = n
		}
	}
	// status=A is Yahoo's "available" (free agents + waivers) filter.
	// status=K ("keepers") intentionally falls through to the full pool.
	freeAgentsOnly = segmentParam(path, "status") == "A"
	return position, search, count, freeAgentsOnly
}

// segmentParam finds ";key=value" in a Yahoo-style path and returns the value.
func segmentParam(path, key string) string {
	marker := ";" + key + "="
	i := strings.Index(path, marker)
	if i < 0 {
		return ""
	}
	rest := path[i+len(marker):]
	end := strings.IndexAny(rest, ";/")
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func xmlResponse(body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/xml; charset=utf-8"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func textResponse(status int, msg string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(msg)),
	}
}
