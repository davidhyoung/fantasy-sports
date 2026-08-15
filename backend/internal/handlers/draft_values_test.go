package handlers

import (
	"strconv"
	"testing"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/ranking"
	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
	"github.com/davidyoung/fantasy-sports/backend/internal/yahoo"
)

func TestYahooToSlot(t *testing.T) {
	tests := []struct {
		pos       string
		want      string
		startable bool
	}{
		{"QB", "QB", true},
		{"RB", "RB", true},
		{"WR", "WR", true},
		{"TE", "TE", true},
		{"K", "K", true},
		{"DEF", "DEF", true},
		{"DST", "DEF", true},
		{"W/R/T", "FLEX", true},
		{"W/R", "FLEX", true},
		{"Q/W/R/T", "SFLEX", true},
		{"BN", "BN", true},
		{"IR", "", false},
		{"IL+", "", false},
	}
	for _, tt := range tests {
		got, ok := yahooToSlot(tt.pos)
		if ok != tt.startable || got != tt.want {
			t.Errorf("yahooToSlot(%q) = (%q, %t), want (%q, %t)", tt.pos, got, ok, tt.want, tt.startable)
		}
	}
}

func TestSlotsFromYahoo(t *testing.T) {
	slots := slotsFromYahoo([]yahoo.RosterPosition{
		{Position: "QB", Count: 1},
		{Position: "RB", Count: 2},
		{Position: "WR", Count: 3},
		{Position: "TE", Count: 1},
		{Position: "W/R/T", Count: 1},
		{Position: "Q/W/R/T", Count: 1},
		{Position: "K", Count: 1},
		{Position: "DEF", Count: 1},
		{Position: "BN", Count: 6},
	})
	want := map[string]int{"QB": 1, "RB": 2, "WR": 3, "TE": 1, "FLEX": 1, "SFLEX": 1, "K": 1, "DEF": 1, "BN": 6}
	if len(slots) != len(want) {
		t.Fatalf("got %d slot kinds, want %d: %v", len(slots), len(want), slots)
	}
	for k, v := range want {
		if slots[k] != v {
			t.Errorf("slot %s = %d, want %d", k, slots[k], v)
		}
	}
}

// A round trip through the two vocabularies must land on the same starter
// demand, since that's what replacement levels — and so every auction value —
// are computed from.
func TestSlotOverrideRoundTrip(t *testing.T) {
	yahooRoster := []yahoo.RosterPosition{
		{Position: "QB", Count: 1},
		{Position: "RB", Count: 2},
		{Position: "WR", Count: 3},
		{Position: "TE", Count: 1},
		{Position: "W/R/T", Count: 2},
		{Position: "Q/W/R/T", Count: 1},
		{Position: "K", Count: 1},
		{Position: "DEF", Count: 1},
		{Position: "BN", Count: 7},
	}

	direct := make([]ranking.RosterPosition, len(yahooRoster))
	for i, rp := range yahooRoster {
		direct[i] = ranking.RosterPosition{Position: rp.Position, Count: rp.Count}
	}
	want := ranking.ComputeStarterSlots(direct)

	// Report the roster as the API would, feed it back as an override, rescore.
	reported := slotsFromYahoo(yahooRoster)
	var encoded string
	for name, count := range reported {
		if encoded != "" {
			encoded += ","
		}
		encoded += name + ":" + strconv.Itoa(count)
	}
	_, positions, ok := parseSlotOverride(encoded)
	if !ok {
		t.Fatalf("parseSlotOverride(%q) reported no startable slots", encoded)
	}
	got := ranking.ComputeStarterSlots(positions)

	if len(got) != len(want) {
		t.Fatalf("got %d positions, want %d (%v vs %v)", len(got), len(want), got, want)
	}
	for pos, wantSlots := range want {
		if diff := got[pos] - wantSlots; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("starter slots for %s = %v, want %v", pos, got[pos], wantSlots)
		}
	}
}

func TestParseSlotOverrideRejectsJunk(t *testing.T) {
	// Unknown names, negatives, absurd counts and malformed pairs are dropped;
	// what survives still parses.
	slots, positions, ok := parseSlotOverride("QB:1,BOGUS:3,RB:-2,WR:999,TE:x,FLEX:1")
	if !ok {
		t.Fatal("expected the valid entries to survive")
	}
	want := map[string]int{"QB": 1, "FLEX": 1}
	if len(slots) != len(want) {
		t.Errorf("slots = %v, want %v", slots, want)
	}
	for k, v := range want {
		if slots[k] != v {
			t.Errorf("slot %s = %d, want %d", k, slots[k], v)
		}
	}
	if len(positions) != 2 {
		t.Errorf("positions = %v, want 2 entries", positions)
	}

	// A parse with nothing startable must report false, so the caller falls back to
	// the league's roster rather than scoring against an empty lineup.
	if _, _, ok := parseSlotOverride("BOGUS:3,QB:0"); ok {
		t.Error("expected no startable slots to report false")
	}
	if _, _, ok := parseSlotOverride("garbage"); ok {
		t.Error("expected unparseable override to report false")
	}
}

func TestParseScoringOverride(t *testing.T) {
	// Unknown keys, distance-bucketed FG stats (not in the editable set) and
	// unparsable values are dropped; what survives still parses.
	got, ok := parseScoringOverride("pass_yds:0.04,pass_td:4,bogus:99,fg_0_19:5,rec:x")
	if !ok {
		t.Fatal("expected the valid entries to survive")
	}
	want := map[scoring.CanonicalStat]float64{
		scoring.StatPassYds: 0.04,
		scoring.StatPassTD:  4,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("scoring[%s] = %v, want %v", k, got[k], v)
		}
	}

	if _, ok := parseScoringOverride("bogus:1,fg_0_19:3"); ok {
		t.Error("expected no editable keys to report false")
	}
	if _, ok := parseScoringOverride("garbage"); ok {
		t.Error("expected unparseable override to report false")
	}
}
