package render

import "testing"

func TestThresholdColor(t *testing.T) {
	tests := []struct {
		pct  float64
		want func(string) string
	}{
		{0, Green},
		{50, Green},
		{69.9, Green},
		{70, Yellow},
		{80, Yellow},
		{84.9, Yellow},
		{85, Red},
		{100, Red},
	}
	for _, tc := range tests {
		got := ThresholdColor(tc.pct)
		wantSample := tc.want("x")
		gotSample := got("x")
		if gotSample != wantSample {
			t.Errorf("ThresholdColor(%v) returned wrong color: got %q want %q",
				tc.pct, gotSample, wantSample)
		}
	}
}

func TestBarDotted(t *testing.T) {
	tests := []struct {
		pct   float64
		width int
		want  string
	}{
		{0, 8, "⣀⣀⣀⣀⣀⣀⣀⣀"},
		{50, 8, "⣿⣿⣿⣿⣀⣀⣀⣀"},
		{100, 8, "⣿⣿⣿⣿⣿⣿⣿⣿"},
		{-10, 4, "⣀⣀⣀⣀"},
		{150, 4, "⣿⣿⣿⣿"},
		{44, 8, "⣿⣿⣿⣦⣀⣀⣀⣀"},
		{13, 8, "⣿⣀⣀⣀⣀⣀⣀⣀"},
		{5, 8, "⣤⣀⣀⣀⣀⣀⣀⣀"},
	}
	for _, tc := range tests {
		if got := Bar(tc.pct, tc.width); got != tc.want {
			t.Errorf("Bar(%v, %d) = %q, want %q", tc.pct, tc.width, got, tc.want)
		}
	}
}

func TestBarShaded(t *testing.T) {
	// 4-level ramp: 3 steps per cell. For 8 cells: 24 total units.
	tests := []struct {
		pct   float64
		width int
		want  string
	}{
		{0, 8, "░░░░░░░░"},
		// 50% × 24 = 12 units → 4 full cells, 4 empty
		{50, 8, "████░░░░"},
		{100, 8, "████████"},
		// 44% × 24 = 10.56 → 11 units = 3 full (3*3=9) + level-2 (▓) + 4 empty
		{44, 8, "███▓░░░░"},
		// 75% × 24 = 18 → 6 full + 0 partial + 2 empty
		{75, 8, "██████░░"},
	}
	for _, tc := range tests {
		if got := BarWithRamp(tc.pct, tc.width, ShadedRamp); got != tc.want {
			t.Errorf("BarShaded(%v, %d) = %q, want %q", tc.pct, tc.width, got, tc.want)
		}
	}
}
