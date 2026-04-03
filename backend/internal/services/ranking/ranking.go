package ranking

// PlayerData is the input type for ranking computations.
type PlayerData struct {
	PlayerKey    string
	Name         string
	Position     string             // raw display position (may be "PG,SG" etc.)
	PrimaryPos   string             // first token, used for position grouping
	TeamAbbr     string
	OwnerTeamKey string             // empty = unowned FA
	StatValues   map[string]float64 // statID → value
	TotalPoints  float64            // only used in points mode
	IsRostered   bool
}

// CategoryMeta describes a single scoring category.
type CategoryMeta struct {
	ID        string
	Label     string
	SortOrder string  // "1" = higher better, "0" = lower better
	Modifier  float64 // points per unit (NFL only)
}

// ScoredPlayer is the output of ranking computation.
type ScoredPlayer struct {
	PlayerData
	OverallScore   float64
	OverallRank    int
	PositionScore  float64
	PositionRank   int
	VORP           float64
	CategoryScores []CategoryScore
}

// CategoryScore holds per-category scoring details for a single player.
type CategoryScore struct {
	Label      string
	Value      float64
	ZScore     float64
	Percentile int
}

// ReplacementLevel describes the replacement threshold for a position.
type ReplacementLevel struct {
	Position  string
	Threshold int
	Points    float64
}

// CategoryStats holds aggregate statistics for a single scoring category.
type CategoryStats struct {
	Label     string
	SortOrder string
	Mean      float64
	Stdev     float64
	Weight    float64
}

// RosterPosition mirrors yahoo.RosterPosition for decoupling from the Yahoo package.
type RosterPosition struct {
	Position string
	Count    int
}

// RankCategoriesResult is returned by RankByCategories.
type RankCategoriesResult struct {
	Players       []ScoredPlayer
	CategoryStats []CategoryStats
}

// RankPointsResult is returned by RankByPoints.
type RankPointsResult struct {
	Players           []ScoredPlayer
	CategoryStats     []CategoryStats
	ReplacementLevels []ReplacementLevel
}
