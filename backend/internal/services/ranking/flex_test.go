package ranking

import (
	"reflect"
	"testing"
)

func TestParseFlexEligible(t *testing.T) {
	tests := []struct {
		name string
		pos  string
		want []string
	}{
		{"standard flex", "W/R/T", []string{"WR", "RB", "TE"}},
		{"superflex", "Q/W/R/T", []string{"QB", "WR", "RB", "TE"}},
		{"single position", "QB", nil},
		{"bench", "BN", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFlexEligible(tt.pos)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ParseFlexEligible(%q) = %v, want %v", tt.pos, got, tt.want)
			}
		})
	}
}
