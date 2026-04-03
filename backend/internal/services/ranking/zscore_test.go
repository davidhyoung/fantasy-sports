package ranking

import (
	"math"
	"testing"
)

func TestRankByCategories(t *testing.T) {
	players := []PlayerData{
		{PlayerKey: "p1", Name: "Player A", PrimaryPos: "PG", Position: "PG", StatValues: map[string]float64{"pts": 30, "reb": 5}},
		{PlayerKey: "p2", Name: "Player B", PrimaryPos: "PG", Position: "PG", StatValues: map[string]float64{"pts": 20, "reb": 10}},
		{PlayerKey: "p3", Name: "Player C", PrimaryPos: "C", Position: "C", StatValues: map[string]float64{"pts": 10, "reb": 15}},
	}
	catMeta := []CategoryMeta{
		{ID: "pts", Label: "Points", SortOrder: "1"},
		{ID: "reb", Label: "Rebounds", SortOrder: "1"},
	}

	result := RankByCategories(players, catMeta, nil)

	if len(result.Players) != 3 {
		t.Fatalf("expected 3 players, got %d", len(result.Players))
	}

	// Verify category stats were computed
	if len(result.CategoryStats) != 2 {
		t.Fatalf("expected 2 category stats, got %d", len(result.CategoryStats))
	}

	// First ranked player should have highest overall z-score
	if result.Players[0].OverallRank != 1 {
		t.Errorf("first player should have rank 1, got %d", result.Players[0].OverallRank)
	}

	// Verify z-scores have correct sign: higher values → positive z-scores for SortOrder=1
	for _, sp := range result.Players {
		for _, cs := range sp.CategoryScores {
			if sp.PlayerKey == "p1" && cs.Label == "Points" && cs.ZScore <= 0 {
				t.Error("Player A has highest points, should have positive z-score for Points")
			}
			if sp.PlayerKey == "p3" && cs.Label == "Points" && cs.ZScore >= 0 {
				t.Error("Player C has lowest points, should have negative z-score for Points")
			}
		}
	}
}

func TestRankByCategoriesLowerIsBetter(t *testing.T) {
	players := []PlayerData{
		{PlayerKey: "p1", Name: "Low TO", PrimaryPos: "PG", Position: "PG", StatValues: map[string]float64{"to": 1}},
		{PlayerKey: "p2", Name: "High TO", PrimaryPos: "PG", Position: "PG", StatValues: map[string]float64{"to": 5}},
	}
	catMeta := []CategoryMeta{
		{ID: "to", Label: "Turnovers", SortOrder: "0"}, // lower is better
	}

	result := RankByCategories(players, catMeta, nil)

	// Low turnovers should rank higher (positive z-score after flip)
	if result.Players[0].PlayerKey != "p1" {
		t.Errorf("player with fewer turnovers should rank first, got %s", result.Players[0].PlayerKey)
	}
}

func TestRankByCategoriesWeightNormalization(t *testing.T) {
	// With equal stats spread, all weights should normalize to 1.0
	players := []PlayerData{
		{PlayerKey: "p1", PrimaryPos: "PG", Position: "PG", StatValues: map[string]float64{"a": 10, "b": 100}},
		{PlayerKey: "p2", PrimaryPos: "PG", Position: "PG", StatValues: map[string]float64{"a": 20, "b": 200}},
		{PlayerKey: "p3", PrimaryPos: "PG", Position: "PG", StatValues: map[string]float64{"a": 30, "b": 300}},
	}
	catMeta := []CategoryMeta{
		{ID: "a", Label: "A", SortOrder: "1"},
		{ID: "b", Label: "B", SortOrder: "1"},
	}

	result := RankByCategories(players, catMeta, nil)

	// Both categories have same CV (same relative spread), no FA data → weights should be equal
	var totalWeight float64
	for _, cs := range result.CategoryStats {
		totalWeight += cs.Weight
	}
	meanWeight := totalWeight / float64(len(result.CategoryStats))
	if math.Abs(meanWeight-1.0) > 0.01 {
		t.Errorf("mean weight = %.3f, want 1.0", meanWeight)
	}
}
