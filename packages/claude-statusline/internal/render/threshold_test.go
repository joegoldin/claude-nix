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

func TestBar(t *testing.T) {
	if got := Bar(0, 8); got != "░░░░░░░░" {
		t.Errorf("got %q", got)
	}
	if got := Bar(50, 8); got != "████░░░░" {
		t.Errorf("got %q", got)
	}
	if got := Bar(100, 8); got != "████████" {
		t.Errorf("got %q", got)
	}
	if got := Bar(-10, 4); got != "░░░░" {
		t.Errorf("got %q", got)
	}
	if got := Bar(150, 4); got != "████" {
		t.Errorf("got %q", got)
	}
}
