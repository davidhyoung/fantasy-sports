package espnnba

import (
	"strconv"
	"strings"
)

// PlayerGameStats is one athlete's parsed stat line for one game, ready to
// upsert into nba_player_stats.
type PlayerGameStats struct {
	Minutes           float64
	Points            float64
	Rebounds          float64
	OffensiveRebounds float64
	DefensiveRebounds float64
	Assists           float64
	Steals            float64
	Blocks            float64
	Turnovers         float64
	Fouls             float64
	FGMade, FGAtt     float64
	FG3Made, FG3Att   float64
	FTMade, FTAtt     float64
}

func statValues(keys, stats []string) map[string]string {
	out := make(map[string]string, len(keys))
	for i, k := range keys {
		if i < len(stats) {
			out[k] = stats[i]
		}
	}
	return out
}

func num(v string) float64 {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

// firstOfComposite parses the first number out of an ESPN composite stat
// string like "6-17" (made-attempted), returning the value before the
// separator. Same helper shape as espnlive's NFL composite parser
// ("23/33" completions/attempts) — ESPN just uses "-" instead of "/" here.
func firstOfComposite(v string, sep byte) float64 {
	if i := strings.IndexByte(v, sep); i >= 0 {
		return num(v[:i])
	}
	return num(v)
}

func secondOfComposite(v string, sep byte) float64 {
	if i := strings.IndexByte(v, sep); i >= 0 {
		return num(v[i+1:])
	}
	return 0
}

// ParseAthleteStats turns one athlete's box-score stat line (a StatGroup's
// Keys zipped with that athlete's Stats) into PlayerGameStats. Unknown keys
// are ignored — ESPN's NBA box score is one flat group (unlike NFL's
// passing/rushing/receiving split), so there's no category dispatch needed,
// just a direct key lookup.
func ParseAthleteStats(keys, stats []string) PlayerGameStats {
	v := statValues(keys, stats)
	return PlayerGameStats{
		Minutes:           num(v["minutes"]),
		Points:            num(v["points"]),
		Rebounds:          num(v["rebounds"]),
		OffensiveRebounds: num(v["offensiveRebounds"]),
		DefensiveRebounds: num(v["defensiveRebounds"]),
		Assists:           num(v["assists"]),
		Steals:            num(v["steals"]),
		Blocks:            num(v["blocks"]),
		Turnovers:         num(v["turnovers"]),
		Fouls:             num(v["fouls"]),
		FGMade:            firstOfComposite(v["fieldGoalsMade-fieldGoalsAttempted"], '-'),
		FGAtt:             secondOfComposite(v["fieldGoalsMade-fieldGoalsAttempted"], '-'),
		FG3Made:           firstOfComposite(v["threePointFieldGoalsMade-threePointFieldGoalsAttempted"], '-'),
		FG3Att:            secondOfComposite(v["threePointFieldGoalsMade-threePointFieldGoalsAttempted"], '-'),
		FTMade:            firstOfComposite(v["freeThrowsMade-freeThrowsAttempted"], '-'),
		FTAtt:             secondOfComposite(v["freeThrowsMade-freeThrowsAttempted"], '-'),
	}
}
