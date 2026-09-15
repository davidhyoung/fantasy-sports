package espnlive

import (
	"context"
	"os"
	"testing"
)

// TestFetchScoreboard_Live and TestFetchBoxscore_Live hit the real,
// unofficial ESPN endpoints — skipped by default (this codebase's tests
// never call live external services otherwise) and only run when
// ESPN_LIVE_TEST=1 is set, as a manual sanity check that ESPN hasn't
// changed the response shape this package's parser depends on.
func skipUnlessLive(t *testing.T) {
	if os.Getenv("ESPN_LIVE_TEST") == "" {
		t.Skip("set ESPN_LIVE_TEST=1 to run this against the real ESPN API")
	}
}

func TestFetchScoreboard_Live(t *testing.T) {
	skipUnlessLive(t)

	events, err := fetchScoreboard(context.Background())
	if err != nil {
		t.Fatalf("fetchScoreboard: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	for _, ev := range events {
		if ev.ID == "" {
			t.Errorf("event missing id: %+v", ev)
		}
		if ev.Season.Year == 0 {
			t.Errorf("event %s missing season.year", ev.ID)
		}
		if ev.Week.Number == 0 {
			t.Errorf("event %s missing week.number", ev.ID)
		}
		if s := ev.state(); s != "pre" && s != "in" && s != "post" {
			t.Errorf("event %s has unexpected state %q", ev.ID, s)
		}
	}
}

func TestFetchBoxscore_Live(t *testing.T) {
	skipUnlessLive(t)

	// Week 1, 2026: Patriots @ Seahawks — a completed game, always available
	// as a boxscore fixture regardless of what's live when this test runs.
	teams, err := fetchBoxscore(context.Background(), "401872656")
	if err != nil {
		t.Fatalf("fetchBoxscore: %v", err)
	}
	if len(teams) != 2 {
		t.Fatalf("expected 2 teams, got %d", len(teams))
	}

	var sawPassing bool
	for _, team := range teams {
		for _, cat := range team.Statistics {
			if cat.Name == "passing" {
				sawPassing = true
				if len(cat.Athletes) == 0 {
					t.Error("passing category has no athletes")
				}
				for _, a := range cat.Athletes {
					if a.Athlete.ID == "" {
						t.Error("athlete missing id")
					}
					if len(a.Stats) != len(cat.Keys) {
						t.Errorf("athlete %s has %d stats but category has %d keys", a.Athlete.ID, len(a.Stats), len(cat.Keys))
					}
				}
			}
		}
	}
	if !sawPassing {
		t.Error("expected a passing category in this box score")
	}
}
