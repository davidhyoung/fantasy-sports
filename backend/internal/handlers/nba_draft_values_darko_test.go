package handlers

import (
	"math"
	"testing"
)

func TestNBADarkoPerGame(t *testing.T) {
	// 96 pace × 30 min / 48 = 60 possessions, so per-100 rates scale by 0.6.
	r := nbaDarkoRates{
		minutes: 30, pace: 96,
		pts100: 30, ast100: 10, orb100: 2, drb100: 8, stl100: 2, blk100: 1, tov100: 5,
		fga100: 20, fg3a100: 10, fta100: 10, fgPct: 0.5, fg3Pct: 0.4, ftPct: 0.8,
	}
	s := r.perGame()
	want := map[string]float64{
		"points": 18, "assists": 6, "rebounds": 6, "steals": 1.2, "blocks": 0.6,
		"turnovers": 3, "three_pm": 2.4, "fg_pct": 0.5, "ft_pct": 0.8,
	}
	for cat, w := range want {
		if got := s.value(cat); math.Abs(got-w) > 1e-9 {
			t.Errorf("%s = %v, want %v", cat, got, w)
		}
	}
	if math.Abs(s.fga-12) > 1e-9 || math.Abs(s.fta-6) > 1e-9 {
		t.Errorf("attempts = %v/%v, want 12/6", s.fga, s.fta)
	}
}
