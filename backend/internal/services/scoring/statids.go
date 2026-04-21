// Package scoring defines the canonical stat vocabulary used internally and
// provides mappings to/from external stat ID systems (Yahoo, nflverse columns).
//
// Canonical IDs decouple the rest of the app from Yahoo stat IDs. A ranking or
// projection computation references StatPassYds rather than "4", and we pick up
// data either from Yahoo (in-season live stats) or nfl_player_stats (historical)
// depending on the code path.
package scoring

// CanonicalStat is our internal stat ID. One per meaningful fantasy stat.
type CanonicalStat string

const (
	StatPassAtt     CanonicalStat = "pass_att"
	StatPassComp    CanonicalStat = "pass_comp"
	StatPassInc     CanonicalStat = "pass_inc"
	StatPassYds     CanonicalStat = "pass_yds"
	StatPassTD      CanonicalStat = "pass_td"
	StatPassInt     CanonicalStat = "pass_int"
	StatSacks       CanonicalStat = "sacks"
	StatRushAtt     CanonicalStat = "rush_att"
	StatRushYds     CanonicalStat = "rush_yds"
	StatRushTD      CanonicalStat = "rush_td"
	StatRec         CanonicalStat = "rec"
	StatTargets     CanonicalStat = "targets"
	StatRecYds      CanonicalStat = "rec_yds"
	StatRecTD       CanonicalStat = "rec_td"
	StatReturnTD    CanonicalStat = "return_td"
	StatTwoPt       CanonicalStat = "two_pt"
	StatFumbles     CanonicalStat = "fumbles"
	StatFumblesLost CanonicalStat = "fumbles_lost"
	StatFG0_19      CanonicalStat = "fg_0_19"
	StatFG20_29     CanonicalStat = "fg_20_29"
	StatFG30_39     CanonicalStat = "fg_30_39"
	StatFG40_49     CanonicalStat = "fg_40_49"
	StatFG50Plus    CanonicalStat = "fg_50_plus"
	StatFGMade      CanonicalStat = "fg_made"
	StatPATMade     CanonicalStat = "pat_made"
)

// yahooToCanonical maps Yahoo NFL stat IDs to our canonical vocabulary.
// Only stats we can source or derive from nfl_player_stats are mapped.
var yahooToCanonical = map[string]CanonicalStat{
	"1":  StatPassAtt,
	"2":  StatPassComp,
	"3":  StatPassInc,
	"4":  StatPassYds,
	"5":  StatPassTD,
	"6":  StatPassInt,
	"7":  StatSacks,
	"8":  StatRushAtt,
	"9":  StatRushYds,
	"10": StatRushTD,
	"11": StatRec,
	"12": StatRecYds,
	"13": StatRecTD,
	"15": StatReturnTD,
	"16": StatTwoPt,
	"17": StatFumbles,
	"18": StatFumblesLost,
	"19": StatFG0_19,
	"20": StatFG20_29,
	"21": StatFG30_39,
	"22": StatFG40_49,
	"23": StatFG50Plus,
	"29": StatPATMade,
}

// YahooToCanonical returns the canonical stat for a Yahoo stat ID, or "" if unmapped.
func YahooToCanonical(yahooStatID string) CanonicalStat {
	return yahooToCanonical[yahooStatID]
}

// FGDistribution approximates the share of FG attempts by distance bucket,
// based on recent NFL kicker patterns. Used when converting a total fg_made
// into the per-distance buckets Yahoo uses for scoring.
var FGDistribution = map[CanonicalStat]float64{
	StatFG0_19:   0.01,
	StatFG20_29:  0.14,
	StatFG30_39:  0.28,
	StatFG40_49:  0.33,
	StatFG50Plus: 0.24,
}
