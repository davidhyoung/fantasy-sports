package espnlive

import (
	"strconv"
	"strings"

	"github.com/davidyoung/fantasy-sports/backend/internal/services/scoring"
)

// statValues turns one athlete's stat line (a statCategory's Keys zipped
// with that athlete's Stats) into a lookup by ESPN key name, e.g.
// {"passingYards": "178", "completions/passingAttempts": "23/33"}.
func statValues(keys, stats []string) map[string]string {
	out := make(map[string]string, len(keys))
	for i, k := range keys {
		if i < len(stats) {
			out[k] = stats[i]
		}
	}
	return out
}

// num parses a plain numeric ESPN stat value, defaulting to 0 for "--" or
// anything unparsable — matching the rest of this app's convention that an
// absent/unreadable stat simply contributes zero rather than erroring out.
func num(v string) float64 {
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

// firstOfComposite parses the first number out of an ESPN composite stat
// string like "23/33" (completions/attempts) or "3-10" (sacks-sackYardsLost),
// returning the value before the separator.
func firstOfComposite(v string, sep byte) float64 {
	if i := strings.IndexByte(v, sep); i >= 0 {
		return num(v[:i])
	}
	return num(v)
}

// secondOfComposite parses the second number out of an ESPN composite stat
// string like "23/33", returning the value after the separator.
func secondOfComposite(v string, sep byte) float64 {
	if i := strings.IndexByte(v, sep); i >= 0 {
		return num(v[i+1:])
	}
	return 0
}

// addCanonicalStats maps one athlete's category stat line into the shared
// per-player canonical totals map, accumulating across categories (a player
// can appear in both kickReturns and puntReturns, for example).
//
// Categories/keys ESPN doesn't expose here (e.g. two-point conversions have
// no dedicated box-score category) are simply never added — same "absent
// stat contributes zero" convention scoring.ScoreWithModifiers already
// relies on elsewhere.
func addCanonicalStats(totals map[scoring.CanonicalStat]float64, category string, keys, stats []string) {
	v := statValues(keys, stats)

	switch category {
	case "passing":
		comp := firstOfComposite(v["completions/passingAttempts"], '/')
		att := secondOfComposite(v["completions/passingAttempts"], '/')
		totals[scoring.StatPassComp] += comp
		totals[scoring.StatPassAtt] += att
		totals[scoring.StatPassInc] += att - comp
		totals[scoring.StatPassYds] += num(v["passingYards"])
		totals[scoring.StatPassTD] += num(v["passingTouchdowns"])
		totals[scoring.StatPassInt] += num(v["interceptions"])
		totals[scoring.StatSacks] += firstOfComposite(v["sacks-sackYardsLost"], '-')

	case "rushing":
		totals[scoring.StatRushAtt] += num(v["rushingAttempts"])
		totals[scoring.StatRushYds] += num(v["rushingYards"])
		totals[scoring.StatRushTD] += num(v["rushingTouchdowns"])

	case "receiving":
		totals[scoring.StatRec] += num(v["receptions"])
		totals[scoring.StatTargets] += num(v["receivingTargets"])
		totals[scoring.StatRecYds] += num(v["receivingYards"])
		totals[scoring.StatRecTD] += num(v["receivingTouchdowns"])

	case "fumbles":
		totals[scoring.StatFumbles] += num(v["fumbles"])
		totals[scoring.StatFumblesLost] += num(v["fumblesLost"])

	case "kicking":
		fgMade := firstOfComposite(v["fieldGoalsMade/fieldGoalAttempts"], '/')
		totals[scoring.StatFGMade] += fgMade
		totals[scoring.StatPATMade] += firstOfComposite(v["extraPointsMade/extraPointAttempts"], '/')

	case "kickReturns":
		totals[scoring.StatReturnTD] += num(v["kickReturnTouchdowns"])

	case "puntReturns":
		totals[scoring.StatReturnTD] += num(v["puntReturnTouchdowns"])
	}
}

// finalizeFGDistribution spreads a player's total fg_made across Yahoo's
// distance buckets, same as nflstats.LoadWeekStats does for real stats —
// ESPN's box score only gives us a made/attempt total, not per-kick
// distance, so this is an approximation either way.
func finalizeFGDistribution(totals map[scoring.CanonicalStat]float64) {
	fgMade := totals[scoring.StatFGMade]
	if fgMade == 0 {
		return
	}
	for bucket, share := range scoring.FGDistribution {
		totals[bucket] = fgMade * share
	}
}
