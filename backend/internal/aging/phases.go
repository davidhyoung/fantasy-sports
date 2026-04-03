package aging

// PhaseRange defines the career phase boundaries for a position.
// Ages below PrimeStart are "developing"; PrimeStart–PrimeEnd are "prime";
// PrimeEnd+1 through PostPrimeEnd are "post-prime"; above that is "late career".
type PhaseRange struct {
	PrimeStart   int
	PrimeEnd     int
	PostPrimeEnd int // ages above this are "late career"
}

// DefaultPhases maps position group to career phase boundaries.
var DefaultPhases = map[string]PhaseRange{
	"QB": {PrimeStart: 25, PrimeEnd: 34, PostPrimeEnd: 37},
	"RB": {PrimeStart: 22, PrimeEnd: 26, PostPrimeEnd: 29},
	"WR": {PrimeStart: 24, PrimeEnd: 30, PostPrimeEnd: 33},
	"TE": {PrimeStart: 25, PrimeEnd: 30, PostPrimeEnd: 33},
	"K":  {PrimeStart: 25, PrimeEnd: 38, PostPrimeEnd: 41},
}

var fallbackPhase = PhaseRange{PrimeStart: 24, PrimeEnd: 30, PostPrimeEnd: 33}

// Phase returns the career phase for a player at a given age and position.
func Phase(posGroup string, age int) string {
	p, ok := DefaultPhases[posGroup]
	if !ok {
		p = fallbackPhase
	}
	switch {
	case age < p.PrimeStart:
		return "developing"
	case age <= p.PrimeEnd:
		return "prime"
	case age <= p.PostPrimeEnd:
		return "post-prime"
	default:
		return "late-career"
	}
}

// AgingMultipliers holds per-phase multipliers applied to projected stats.
type AgingMultipliers struct {
	Developing float64 `json:"developing"`
	Prime      float64 `json:"prime"`
	PostPrime  float64 `json:"post_prime"`
	LateCareer float64 `json:"late_career"`
}

// DefaultAgingMultipliers returns conservative default multipliers.
// These are intentionally modest since the comp system already partially
// accounts for age through age-window constraints.
var DefaultAgingMultipliers = AgingMultipliers{
	Developing: 1.02,
	Prime:      1.00,
	PostPrime:  0.97,
	LateCareer: 0.93,
}

// DefaultDraftMultipliers returns draft-value multipliers, which are slightly
// more aggressive because draft capital pricing benefits from age awareness.
var DefaultDraftMultipliers = AgingMultipliers{
	Developing: 1.05,
	Prime:      1.00,
	PostPrime:  0.93,
	LateCareer: 0.85,
}

// Multiplier returns the aging multiplier for a given position and age.
func (m AgingMultipliers) Multiplier(posGroup string, age int) float64 {
	if age <= 0 {
		return 1.0 // missing data — no adjustment
	}
	switch Phase(posGroup, age) {
	case "developing":
		return m.Developing
	case "prime":
		return m.Prime
	case "post-prime":
		return m.PostPrime
	case "late-career":
		return m.LateCareer
	default:
		return 1.0
	}
}
