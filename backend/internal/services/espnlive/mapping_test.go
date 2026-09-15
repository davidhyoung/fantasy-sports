package espnlive

import (
	"testing"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
)

// Fixtures below are real values captured from ESPN's
// summary?event=401872656 (Patriots @ Seahawks, 2026 week 1) box score, to
// pin the parser against actual ESPN output rather than a made-up shape.

func TestAddCanonicalStats_Passing(t *testing.T) {
	totals := map[scoring.CanonicalStat]float64{}
	keys := []string{"completions/passingAttempts", "passingYards", "yardsPerPassAttempt", "passingTouchdowns", "interceptions", "sacks-sackYardsLost", "adjQBR", "QBRating"}
	stats := []string{"23/33", "178", "5.4", "1", "3", "3-10", "58.5", "54.9"} // Drake Maye

	addCanonicalStats(totals, "passing", keys, stats)

	want := map[scoring.CanonicalStat]float64{
		scoring.StatPassComp: 23,
		scoring.StatPassAtt:  33,
		scoring.StatPassInc:  10,
		scoring.StatPassYds:  178,
		scoring.StatPassTD:   1,
		scoring.StatPassInt:  3,
		scoring.StatSacks:    3,
	}
	for stat, v := range want {
		if got := totals[stat]; got != v {
			t.Errorf("%s = %v, want %v", stat, got, v)
		}
	}
}

func TestAddCanonicalStats_RushingAndReceiving_Accumulate(t *testing.T) {
	totals := map[scoring.CanonicalStat]float64{}

	// Rhamondre Stevenson — rushing then receiving, same player, same map.
	addCanonicalStats(totals, "rushing",
		[]string{"rushingAttempts", "rushingYards", "yardsPerRushAttempt", "rushingTouchdowns", "longRushing"},
		[]string{"18", "51", "2.8", "0", "12"},
	)
	addCanonicalStats(totals, "receiving",
		[]string{"receptions", "receivingYards", "yardsPerReception", "receivingTouchdowns", "longReception", "receivingTargets"},
		[]string{"5", "44", "8.8", "0", "17", "6"},
	)

	want := map[scoring.CanonicalStat]float64{
		scoring.StatRushAtt: 18,
		scoring.StatRushYds: 51,
		scoring.StatRushTD:  0,
		scoring.StatRec:     5,
		scoring.StatRecYds:  44,
		scoring.StatRecTD:   0,
		scoring.StatTargets: 6,
	}
	for stat, v := range want {
		if got := totals[stat]; got != v {
			t.Errorf("%s = %v, want %v", stat, got, v)
		}
	}
}

func TestAddCanonicalStats_Kicking(t *testing.T) {
	totals := map[scoring.CanonicalStat]float64{}
	keys := []string{"fieldGoalsMade/fieldGoalAttempts", "fieldGoalPct", "longFieldGoalMade", "extraPointsMade/extraPointAttempts", "totalKickingPoints"}
	stats := []string{"1/1", "100.0", "50", "1/1", "4"} // Andy Borregales

	addCanonicalStats(totals, "kicking", keys, stats)

	if got := totals[scoring.StatFGMade]; got != 1 {
		t.Errorf("StatFGMade = %v, want 1", got)
	}
	if got := totals[scoring.StatPATMade]; got != 1 {
		t.Errorf("StatPATMade = %v, want 1", got)
	}
}

func TestAddCanonicalStats_ReturnTDsFromBothCategories(t *testing.T) {
	totals := map[scoring.CanonicalStat]float64{}
	addCanonicalStats(totals, "kickReturns",
		[]string{"kickReturns", "kickReturnYards", "yardsPerKickReturn", "longKickReturn", "kickReturnTouchdowns"},
		[]string{"2", "40", "20.0", "25", "1"},
	)
	addCanonicalStats(totals, "puntReturns",
		[]string{"puntReturns", "puntReturnYards", "yardsPerPuntReturn", "longPuntReturn", "puntReturnTouchdowns"},
		[]string{"1", "10", "10.0", "10", "1"},
	)
	if got := totals[scoring.StatReturnTD]; got != 2 {
		t.Errorf("StatReturnTD = %v, want 2 (accumulated across kick + punt returns)", got)
	}
}

func TestFinalizeFGDistribution(t *testing.T) {
	totals := map[scoring.CanonicalStat]float64{scoring.StatFGMade: 2}
	finalizeFGDistribution(totals)

	var sum float64
	for bucket := range scoring.FGDistribution {
		sum += totals[bucket]
	}
	if want := 2.0; sum < want-0.001 || sum > want+0.001 {
		t.Errorf("distance buckets sum to %v, want %v (fg_made total preserved)", sum, want)
	}
}

func TestUnknownCategory_NoPanic(t *testing.T) {
	totals := map[scoring.CanonicalStat]float64{}
	addCanonicalStats(totals, "defensive", []string{"soloTackles"}, []string{"5"})
	if len(totals) != 0 {
		t.Errorf("expected no canonical stats added for an unhandled category, got %v", totals)
	}
}
