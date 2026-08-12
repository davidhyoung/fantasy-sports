// Package tiers groups players whose value is close enough that the choice
// between them is a coin flip.
//
// The question a tier answers during a draft is "if I wait, do I still get
// someone like this?" — so a tier has to mean "these players are
// interchangeable", not "these players are ranked 5th through 9th". That makes
// it a 1-D clustering problem over projected value, and the cluster's *width* is
// the thing that has to be bounded.
//
// See docs/stats/tiering.md.
package tiers

import "sort"

// Player is one entry to be tiered. Value is whatever the caller ranks by —
// league-scored projected points, in this codebase.
type Player struct {
	ID    string
	Value float64
}

// Options tunes the tiering. The zero value is usable: DefaultOptions() is
// applied for any field left at zero.
type Options struct {
	// TargetTiers is roughly how many tiers should cover the draftable range.
	// It sets the width of a tier rather than the count directly, so a position
	// whose talent is bunched gets fewer, wider-spanning tiers and one with real
	// separation gets more — which is the information a tier is meant to carry.
	TargetTiers int
	// Draftable is how many players at this position are realistically drafted.
	// The tier width is derived from this range, so the long tail of replacement
	// level players can't flatten it. 0 = use every player supplied.
	Draftable int
}

func DefaultOptions() Options {
	// Eight tiers across a position's draftable range puts a typical 12-team
	// starting pool into groups of two or three — small enough to act on, big
	// enough that a tier break means something.
	return Options{TargetTiers: 8}
}

// Assign returns a tier number per player ID, 1 being the best tier.
//
// Players are sorted by value descending and walked once: a player joins the
// current tier while the tier's total spread (best minus worst) stays within a
// width budget, and starts a new tier when it wouldn't. That single rule gives
// the guarantee the UI promises — everyone inside a tier is within one budget of
// everyone else in it — and it drops a natural break wherever there's a cliff,
// because a cliff is exactly what blows the budget.
//
// The budget is the draftable range divided by TargetTiers, so it scales with
// the position and with the league's scoring rather than being a magic number of
// points. Ties and equal values naturally land together.
func Assign(players []Player, opts Options) map[string]int {
	if len(players) == 0 {
		return map[string]int{}
	}
	def := DefaultOptions()
	if opts.TargetTiers <= 0 {
		opts.TargetTiers = def.TargetTiers
	}

	sorted := make([]Player, len(players))
	copy(sorted, players)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Value > sorted[j].Value })

	// Width comes from the draftable range only. Including the tail — where
	// hundreds of players sit within a few points of zero — would stretch the
	// budget so wide that the top of the board collapsed into one or two tiers.
	pool := len(sorted)
	if opts.Draftable > 0 && opts.Draftable < pool {
		pool = opts.Draftable
	}
	span := sorted[0].Value - sorted[pool-1].Value
	budget := span / float64(opts.TargetTiers)

	out := make(map[string]int, len(sorted))
	tier := 1
	tierTop := sorted[0].Value
	for _, p := range sorted {
		// A non-positive budget means every value is identical; one tier is correct.
		if budget > 0 && tierTop-p.Value > budget {
			tier++
			tierTop = p.Value
		}
		out[p.ID] = tier
	}
	return out
}
