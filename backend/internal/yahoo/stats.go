package yahoo

// nflStatNames maps Yahoo stat IDs to short display labels for NFL fantasy leagues.
// Only stats in this map are surfaced to users; all others are silently dropped.
var nflStatNames = map[string]string{
	"4":  "Pass Yds",
	"5":  "Pass TD",
	"6":  "INT",
	"9":  "Rush Yds",
	"10": "Rush TD",
	"11": "Rec",
	"12": "Rec Yds",
	"13": "Rec TD",
	"45": "Sacks",
	"46": "INT (D)",
	"47": "FR",
	"48": "TD (D)",
	"57": "FG 0-19",
	"58": "FG 20-29",
	"59": "FG 30-39",
	"60": "FG 40-49",
	"61": "FG 50+",
	"63": "XP",
}

// nbaStatNames maps Yahoo stat IDs to short display labels for NBA fantasy leagues.
var nbaStatNames = map[string]string{
	"12": "Pts",
	"13": "Ast",
	"14": "Reb",
	"17": "Stl",
	"18": "Blk",
	"19": "TO",
	"21": "FGM",
	"22": "FG%",
	"24": "FTM",
	"25": "FT%",
	"26": "3PM",
}

// StatNamesForSport returns the stat ID→label map for the given sport code
// (e.g. "nfl", "nba"). Returns nil for unrecognised sports; callers should
// treat nil as "no stat mapping available" and omit the stats field entirely.
func StatNamesForSport(sport string) map[string]string {
	switch sport {
	case "nfl":
		return nflStatNames
	case "nba":
		return nbaStatNames
	default:
		return nil
	}
}
