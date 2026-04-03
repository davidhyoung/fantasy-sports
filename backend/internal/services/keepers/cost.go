package keepers

// KeeperRules holds the league's keeper cost configuration.
type KeeperRules struct {
	CostIncrease  int  // annual cost increase per year kept
	UndraftedBase int  // base cost for undrafted (FA pickup) players
	MaxYears      *int // nil = unlimited
}

// ComputeKeeperCost returns the projected keeper cost and whether the player
// is ineligible due to exceeding max years.
//
// yearsKept=0 is treated as yearsKept=1 (first year keeping).
// Undrafted: cost = UndraftedBase + CostIncrease*(yearsKept-1).
// Drafted:   cost = draftCost + CostIncrease*yearsKept.
func ComputeKeeperCost(rules KeeperRules, draftCost int, undrafted bool, yearsKept int) (cost int, notKeepable bool) {
	if yearsKept < 1 {
		yearsKept = 1
	}
	if rules.MaxYears != nil && yearsKept >= *rules.MaxYears {
		return 0, true
	}
	if undrafted {
		cost = rules.UndraftedBase + rules.CostIncrease*(yearsKept-1)
	} else {
		cost = draftCost + rules.CostIncrease*yearsKept
	}
	if cost < 1 {
		cost = 1
	}
	return cost, false
}
