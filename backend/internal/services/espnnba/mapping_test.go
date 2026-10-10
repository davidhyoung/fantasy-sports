package espnnba

import "testing"

// Fixture captured from a real ESPN summary?event=401898392 (Celtics @
// Cavaliers) box score — Luka Garza's actual line — to pin the parser
// against real ESPN output rather than a made-up shape.
func TestParseAthleteStats_RealLine(t *testing.T) {
	keys := []string{
		"minutes", "points", "fieldGoalsMade-fieldGoalsAttempted",
		"threePointFieldGoalsMade-threePointFieldGoalsAttempted",
		"freeThrowsMade-freeThrowsAttempted", "rebounds", "assists",
		"turnovers", "steals", "blocks", "offensiveRebounds",
		"defensiveRebounds", "fouls", "plusMinus",
	}
	stats := []string{"27", "22", "6-17", "3-10", "7-8", "10", "2", "0", "0", "3", "4", "6", "2", "+5"}

	got := ParseAthleteStats(keys, stats)

	want := PlayerGameStats{
		Minutes: 27, Points: 22,
		FGMade: 6, FGAtt: 17,
		FG3Made: 3, FG3Att: 10,
		FTMade: 7, FTAtt: 8,
		Rebounds: 10, Assists: 2, Turnovers: 0, Steals: 0, Blocks: 3,
		OffensiveRebounds: 4, DefensiveRebounds: 6, Fouls: 2,
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestParseAthleteStats_UnknownKeysIgnored(t *testing.T) {
	got := ParseAthleteStats([]string{"someFutureStat"}, []string{"99"})
	if got != (PlayerGameStats{}) {
		t.Errorf("expected zero-value stats for unrecognized keys, got %+v", got)
	}
}
