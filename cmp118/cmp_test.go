package cmp118

import (
	"math"
	"testing"
)

func TestCompare(t *testing.T) {
	nan := math.NaN()
	tests := []struct {
		x, y float64
		want int
	}{
		{1, 2, -1},
		{2, 1, 1},
		{1, 1, 0},
		{nan, nan, 0},
		{nan, 1, -1},
		{1, nan, 1},
	}
	for _, tt := range tests {
		if got := Compare(tt.x, tt.y); got != tt.want {
			t.Errorf("Compare(%v, %v) = %d, want %d", tt.x, tt.y, got, tt.want)
		}
	}
	if got := Compare("a", "b"); got != -1 {
		t.Errorf("Compare(a, b) = %d, want -1", got)
	}
}
