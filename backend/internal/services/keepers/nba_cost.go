package keepers

// ComputeNBAKeeperCost prices a native-NBA keeper designation. Unlike the NFL
// formula above (cost.go), which compounds from an original draft cost via a
// persisted years-kept counter, this one just reads whatever salary was
// actually recorded for the player last season and adds the league's flat
// $5 increase — there's no years-kept counter to maintain at all, since
// each season's recorded salary already reflects any prior escalation.
//
// sameTeam = the team designating this keeper is the same team that held
// the player on the final league_rosters_nba row from last season.
// lastSalary = 0 for a player who was never rostered in this league before
// (a pure free-agent pickup).
//
// A continuously-held cheap player can stay cheap (no floor). Anyone else —
// never rostered, or reclaimed from a different team after being dropped
// ("the Brian Rule") — floors at $10, discouraging cheaply poaching a
// rival's cheap drop.
func ComputeNBAKeeperCost(lastSalary int, sameTeam bool) int {
	cost := lastSalary + 5
	if !sameTeam && cost < 10 {
		cost = 10
	}
	return cost
}
