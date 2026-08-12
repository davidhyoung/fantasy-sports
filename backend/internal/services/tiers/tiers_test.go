package tiers

import "testing"

func p(id string, v float64) Player { return Player{ID: id, Value: v} }

func TestAssignSplitsAtCliffs(t *testing.T) {
	// Two obvious clusters with a gulf between them.
	got := Assign([]Player{
		p("a", 300), p("b", 295), p("c", 290),
		p("d", 200), p("e", 195),
	}, Options{TargetTiers: 4})

	if got["a"] != got["b"] || got["b"] != got["c"] {
		t.Errorf("the bunched top three should share a tier: %v", got)
	}
	if got["d"] != got["e"] {
		t.Errorf("the bunched bottom two should share a tier: %v", got)
	}
	if got["c"] == got["d"] {
		t.Errorf("a 90-point cliff must break the tier: %v", got)
	}
}

// The promise the UI makes is about width, not count: everyone inside a tier is
// within one budget of everyone else in it.
func TestTierSpreadStaysWithinBudget(t *testing.T) {
	var players []Player
	for i := 0; i < 60; i++ {
		players = append(players, p(string(rune('a'+i%26))+string(rune('0'+i/26)), 300-float64(i)*3.7))
	}
	opts := Options{TargetTiers: 8}
	got := Assign(players, opts)

	span := 300 - (300 - 59*3.7)
	budget := span / 8

	byTier := map[int][]float64{}
	for _, pl := range players {
		byTier[got[pl.ID]] = append(byTier[got[pl.ID]], pl.Value)
	}
	for tier, vals := range byTier {
		lo, hi := vals[0], vals[0]
		for _, v := range vals {
			if v < lo {
				lo = v
			}
			if v > hi {
				hi = v
			}
		}
		if hi-lo > budget+1e-9 {
			t.Errorf("tier %d spans %.1f, over the %.1f budget", tier, hi-lo, budget)
		}
	}
}

// A long replacement-level tail must not set the width for the top of the board.
func TestDraftableRangeSetsTheWidth(t *testing.T) {
	players := []Player{p("elite", 300), p("good", 260), p("ok", 220)}
	for i := 0; i < 200; i++ {
		players = append(players, p("scrub"+string(rune(i)), 5-float64(i)*0.01))
	}

	withTail := Assign(players, Options{TargetTiers: 8})
	if withTail["elite"] == withTail["ok"] {
		t.Error("without a draftable limit the tail flattens the top into one tier — " +
			"this test documents why Draftable exists")
	}

	limited := Assign(players, Options{TargetTiers: 8, Draftable: 3})
	if limited["elite"] == limited["good"] || limited["good"] == limited["ok"] {
		t.Errorf("across a 3-player draftable range these should separate: %v",
			map[string]int{"elite": limited["elite"], "good": limited["good"], "ok": limited["ok"]})
	}
}

func TestEdgeCases(t *testing.T) {
	if got := Assign(nil, Options{}); len(got) != 0 {
		t.Errorf("nil input should give no tiers, got %v", got)
	}
	if got := Assign([]Player{p("solo", 10)}, Options{}); got["solo"] != 1 {
		t.Errorf("a single player is tier 1, got %v", got)
	}
	// Identical values can't be separated — one tier, not one tier each.
	same := Assign([]Player{p("a", 10), p("b", 10), p("c", 10)}, Options{})
	if same["a"] != 1 || same["b"] != 1 || same["c"] != 1 {
		t.Errorf("identical values belong together: %v", same)
	}
}

func TestTiersAreOrdered(t *testing.T) {
	got := Assign([]Player{p("c", 100), p("a", 300), p("b", 200)}, Options{TargetTiers: 3})
	if !(got["a"] <= got["b"] && got["b"] <= got["c"]) {
		t.Errorf("tier numbers must follow value order regardless of input order: %v", got)
	}
	if got["a"] != 1 {
		t.Errorf("the best player is in tier 1, got %d", got["a"])
	}
}
