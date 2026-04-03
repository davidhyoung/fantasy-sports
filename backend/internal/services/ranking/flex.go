package ranking

import "strings"

// FlexAbbrev maps Yahoo FLEX single-letter abbreviations to full position names.
var FlexAbbrev = map[string]string{
	"Q": "QB", "W": "WR", "R": "RB", "T": "TE",
}

// ParseFlexEligible parses a FLEX position string like "W/R/T" into eligible
// position names ["WR", "RB", "TE"]. Returns nil for non-FLEX positions.
func ParseFlexEligible(pos string) []string {
	parts := strings.Split(pos, "/")
	if len(parts) <= 1 {
		return nil
	}
	var eligible []string
	for _, p := range parts {
		if full, ok := FlexAbbrev[p]; ok {
			eligible = append(eligible, full)
		}
	}
	return eligible
}
