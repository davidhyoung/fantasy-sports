package ranking

import (
	"math"
	"testing"
)

func TestComputeStarterSlots(t *testing.T) {
	positions := []RosterPosition{
		{Position: "QB", Count: 1},
		{Position: "RB", Count: 2},
		{Position: "WR", Count: 2},
		{Position: "TE", Count: 1},
		{Position: "W/R/T", Count: 1}, // FLEX: distributes 0.33 each to WR, RB, TE
		{Position: "BN", Count: 6},
		{Position: "IR", Count: 1},
	}
	slots := ComputeStarterSlots(positions)

	assertClose := func(pos string, want float64) {
		t.Helper()
		if got := slots[pos]; math.Abs(got-want) > 0.01 {
			t.Errorf("slots[%q] = %.2f, want %.2f", pos, got, want)
		}
	}
	assertClose("QB", 1.0)
	assertClose("RB", 2.33)
	assertClose("WR", 2.33)
	assertClose("TE", 1.33)

	if _, ok := slots["BN"]; ok {
		t.Error("BN should be excluded from starter slots")
	}
}

func TestComputeReplacementLevels(t *testing.T) {
	posGroups := map[string][]PlayerData{
		"QB": {
			{PrimaryPos: "QB", TotalPoints: 300},
			{PrimaryPos: "QB", TotalPoints: 250},
			{PrimaryPos: "QB", TotalPoints: 200},
			{PrimaryPos: "QB", TotalPoints: 150},
		},
		"RB": {
			{PrimaryPos: "RB", TotalPoints: 200},
			{PrimaryPos: "RB", TotalPoints: 180},
		},
	}
	positions := []RosterPosition{
		{Position: "QB", Count: 1},
		{Position: "RB", Count: 2},
	}
	numTeams := 2

	levels := ComputeReplacementLevels(positions, posGroups, numTeams)

	// QB: threshold = 1 dedicated slot × 2 teams = 2, replacement = posGroups["QB"][2] = 200
	if got := levels["QB"].Points; got != 200 {
		t.Errorf("QB replacement = %.0f, want 200", got)
	}
	if got := levels["QB"].Threshold; got != 2 {
		t.Errorf("QB threshold = %d, want 2", got)
	}

	// RB: threshold = 2 dedicated slots × 2 teams = 4, but only 2 players → worst = 180
	if got := levels["RB"].Points; got != 180 {
		t.Errorf("RB replacement = %.0f, want 180 (fewer players than threshold)", got)
	}
}

// A regular FLEX (W/R/T) among comparably-valued positions should still land
// close to an even split, since that's the case the old flat-share model got
// right.
func TestComputeReplacementLevelsFlexPool(t *testing.T) {
	mk := func(pos string, vals ...float64) []PlayerData {
		out := make([]PlayerData, len(vals))
		for i, v := range vals {
			out[i] = PlayerData{PrimaryPos: pos, TotalPoints: v}
		}
		return out
	}
	posGroups := map[string][]PlayerData{
		"RB": mk("RB", 200, 190, 180, 170, 160, 150, 140, 130),
		"WR": mk("WR", 195, 185, 175, 165, 155, 145, 135, 125),
		"TE": mk("TE", 150, 100, 90, 80, 70, 60, 50, 40),
	}
	positions := []RosterPosition{
		{Position: "RB", Count: 1},
		{Position: "WR", Count: 1},
		{Position: "TE", Count: 1},
		{Position: "W/R/T", Count: 1},
	}
	numTeams := 2

	levels := ComputeReplacementLevels(positions, posGroups, numTeams)

	// Dedicated: RB=2, WR=2, TE=2 (1 slot × 2 teams each). FLEX pools the
	// remaining top players across RB/WR/TE (2 spots): RB[2]=180, WR[2]=175
	// are next-best and comparable, TE[2]=90 is far behind, so FLEX should go
	// to RB and WR, not TE.
	if got := levels["RB"].Threshold; got != 3 {
		t.Errorf("RB threshold = %d, want 3 (2 dedicated + 1 flex)", got)
	}
	if got := levels["WR"].Threshold; got != 3 {
		t.Errorf("WR threshold = %d, want 3 (2 dedicated + 1 flex)", got)
	}
	if got := levels["TE"].Threshold; got != 2 {
		t.Errorf("TE threshold = %d, want 2 (dedicated only, lost the flex pool)", got)
	}
}

// A superflex slot pooled against a real value gap should go almost entirely
// to QB, not split evenly — this is the fix for undervaluing QB in superflex
// leagues (a flat 1/4 split only credited QB with 0.25 of a starter slot).
func TestComputeReplacementLevelsSuperflexFavorsQB(t *testing.T) {
	// decline generates n descending values starting at start, dropping by step
	// each rank — enough depth (40 players) that the dedicated slots below
	// don't exhaust any position before the flex pool gets a look.
	decline := func(pos string, start, step float64, n int) []PlayerData {
		out := make([]PlayerData, n)
		for i := 0; i < n; i++ {
			v := start - step*float64(i)
			if v < 0 {
				v = 0
			}
			out[i] = PlayerData{PrimaryPos: pos, TotalPoints: v}
		}
		return out
	}
	// QBs comfortably outscore WR/RB/TE at every comparable depth, as in a real
	// points league (e.g. QB13-24 streamers still outscore RB/WR/TE waiver options).
	posGroups := map[string][]PlayerData{
		"QB": decline("QB", 380, 8, 40),
		"RB": decline("RB", 280, 8, 40),
		"WR": decline("WR", 260, 8, 40),
		"TE": decline("TE", 180, 8, 40),
	}
	positions := []RosterPosition{
		{Position: "QB", Count: 1},
		{Position: "RB", Count: 2},
		{Position: "WR", Count: 3},
		{Position: "TE", Count: 1},
		{Position: "Q/W/R/T", Count: 1},
	}
	numTeams := 12

	levels := ComputeReplacementLevels(positions, posGroups, numTeams)

	// Dedicated QB demand = 1 × 12 = 12. SFLEX pools 12 more spots league-wide;
	// QB13-24 (296 down to 208) still clears every RB/WR/TE at the equivalent
	// depth (RB25-36, WR37-48 — both already claimed by dedicated slots, so
	// their next-available is far lower on the curve), so QB should claim most
	// or all of the pool — well above the naive even-split's 1.25 slots/team.
	slotsPerTeam := float64(levels["QB"].Threshold) / float64(numTeams)
	if slotsPerTeam <= 1.25 {
		t.Errorf("QB slots/team = %.2f, want > 1.25 (the old naive-split value) — SFLEX should skew toward QB", slotsPerTeam)
	}
	if levels["QB"].Threshold <= 12 {
		t.Errorf("QB threshold = %d, want > 12 (SFLEX should award QB some of its pooled spots)", levels["QB"].Threshold)
	}
}

func TestRankByPoints(t *testing.T) {
	rostered := []PlayerData{
		{PlayerKey: "qb1", Name: "QB One", PrimaryPos: "QB", Position: "QB", TotalPoints: 300, StatValues: map[string]float64{"1": 4000}, IsRostered: true, OwnerTeamKey: "t1"},
		{PlayerKey: "rb1", Name: "RB One", PrimaryPos: "RB", Position: "RB", TotalPoints: 250, StatValues: map[string]float64{"1": 500}, IsRostered: true, OwnerTeamKey: "t1"},
		{PlayerKey: "rb2", Name: "RB Two", PrimaryPos: "RB", Position: "RB", TotalPoints: 200, StatValues: map[string]float64{"1": 400}, IsRostered: true, OwnerTeamKey: "t2"},
		{PlayerKey: "rb3", Name: "RB Three", PrimaryPos: "RB", Position: "RB", TotalPoints: 100, StatValues: map[string]float64{"1": 200}, IsRostered: true, OwnerTeamKey: "t2"},
	}
	fas := []PlayerData{
		{PlayerKey: "fa1", Name: "FA Guy", PrimaryPos: "RB", Position: "RB", TotalPoints: 50, StatValues: map[string]float64{"1": 100}, IsRostered: false},
	}
	catMeta := []CategoryMeta{
		{ID: "1", Label: "Yards", SortOrder: "1", Modifier: 0.1},
	}
	positions := []RosterPosition{
		{Position: "QB", Count: 1},
		{Position: "RB", Count: 1},
	}
	numTeams := 2

	result := RankByPoints(rostered, fas, catMeta, positions, numTeams)

	if len(result.Players) != 5 {
		t.Fatalf("expected 5 players, got %d", len(result.Players))
	}

	// QB: threshold=ceil(1*2)=2, only 1 QB → repl=300; VORP=0
	// RB: threshold=ceil(1*2)=2, repl=rb3(100 index=2); VORP: rb1=150, rb2=100, rb3=0, fa=-50
	// Highest VORP should be rb1 (150), then rb2 (100), then qb1 (0) or rb3 (0), then fa1 (-50)

	first := result.Players[0]
	if first.PlayerKey != "rb1" {
		t.Errorf("expected rb1 first (highest VORP), got %s", first.PlayerKey)
	}
	if first.OverallRank != 1 {
		t.Errorf("expected rank 1, got %d", first.OverallRank)
	}

	// Check replacement levels exist
	if len(result.ReplacementLevels) == 0 {
		t.Error("expected replacement levels in result")
	}
}
