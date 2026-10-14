package keepers

import "testing"

func TestComputeNBAKeeperCost(t *testing.T) {
	tests := []struct {
		name       string
		lastSalary int
		sameTeam   bool
		want       int
	}{
		{"same team, cheap keeper stays cheap", 7, true, 12},
		{"same team, zero salary (e.g. a $0 waiver add) still just +5", 0, true, 5},
		{"different team reclaim floors at 10", 2, false, 10},
		{"never rostered floors at 10", 0, false, 10},
		{"expensive player, same team, no floor needed", 55, true, 60},
		{"expensive player, different team (poach), no floor needed since already above 10", 55, false, 60},
		{"boundary: same team lands exactly on 10, no floor applied either way", 5, true, 10},
		{"boundary: different team lands exactly on 10, floor is a no-op", 5, false, 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeNBAKeeperCost(tt.lastSalary, tt.sameTeam)
			if got != tt.want {
				t.Errorf("ComputeNBAKeeperCost(%d, %v) = %d, want %d", tt.lastSalary, tt.sameTeam, got, tt.want)
			}
		})
	}
}
