package keepers

import "testing"

func intPtr(n int) *int { return &n }

func TestComputeKeeperCost(t *testing.T) {
	tests := []struct {
		name        string
		rules       KeeperRules
		draftCost   int
		undrafted   bool
		yearsKept   int
		wantCost    int
		wantBlocked bool
	}{
		{
			name:      "drafted, 1 year kept",
			rules:     KeeperRules{CostIncrease: 5, UndraftedBase: 10},
			draftCost: 20, undrafted: false, yearsKept: 1,
			wantCost: 25, // 20 + 5*1
		},
		{
			name:      "drafted, 3 years kept",
			rules:     KeeperRules{CostIncrease: 5, UndraftedBase: 10},
			draftCost: 20, undrafted: false, yearsKept: 3,
			wantCost: 35, // 20 + 5*3
		},
		{
			name:      "undrafted, 0 years (normalized to 1)",
			rules:     KeeperRules{CostIncrease: 5, UndraftedBase: 10},
			draftCost: 0, undrafted: true, yearsKept: 0,
			wantCost: 10, // 10 + 5*(1-1) = 10
		},
		{
			name:      "undrafted, 2 years kept",
			rules:     KeeperRules{CostIncrease: 5, UndraftedBase: 10},
			draftCost: 0, undrafted: true, yearsKept: 2,
			wantCost: 15, // 10 + 5*(2-1) = 15
		},
		{
			name:        "max years reached — not keepable",
			rules:       KeeperRules{CostIncrease: 5, UndraftedBase: 10, MaxYears: intPtr(3)},
			draftCost:   20, undrafted: false, yearsKept: 3,
			wantCost:    0,
			wantBlocked: true,
		},
		{
			name:      "max years not reached — keepable",
			rules:     KeeperRules{CostIncrease: 5, UndraftedBase: 10, MaxYears: intPtr(3)},
			draftCost: 20, undrafted: false, yearsKept: 2,
			wantCost:  30, // 20 + 5*2
		},
		{
			name:      "nil max years — always keepable",
			rules:     KeeperRules{CostIncrease: 5, UndraftedBase: 10, MaxYears: nil},
			draftCost: 20, undrafted: false, yearsKept: 10,
			wantCost:  70, // 20 + 5*10
		},
		{
			name:      "cost floor — minimum $1",
			rules:     KeeperRules{CostIncrease: 0, UndraftedBase: 0},
			draftCost: 0, undrafted: true, yearsKept: 1,
			wantCost:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, blocked := ComputeKeeperCost(tt.rules, tt.draftCost, tt.undrafted, tt.yearsKept)
			if cost != tt.wantCost {
				t.Errorf("cost = %d, want %d", cost, tt.wantCost)
			}
			if blocked != tt.wantBlocked {
				t.Errorf("notKeepable = %v, want %v", blocked, tt.wantBlocked)
			}
		})
	}
}
