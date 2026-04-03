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
	starterSlots := map[string]float64{"QB": 1.0, "RB": 2.0}
	numTeams := 2

	levels := ComputeReplacementLevels(posGroups, starterSlots, numTeams)

	// QB: threshold = ceil(1.0 * 2) = 2, replacement = posGroups["QB"][2] = 200
	if got := levels["QB"].Points; got != 200 {
		t.Errorf("QB replacement = %.0f, want 200", got)
	}
	if got := levels["QB"].Threshold; got != 2 {
		t.Errorf("QB threshold = %d, want 2", got)
	}

	// RB: threshold = ceil(2.0 * 2) = 4, but only 2 players → worst = 180
	if got := levels["RB"].Points; got != 180 {
		t.Errorf("RB replacement = %.0f, want 180 (fewer players than threshold)", got)
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
