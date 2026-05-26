package widgets

import (
	"strings"
	"testing"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
)

func TestCostHidesForMaxSubscriberInsideLimits(t *testing.T) {
	w := &Cost{}
	ctx := &Context{Status: input.Status{
		Cost: &input.Cost{TotalCostUSD: 1.42},
		RateLimits: &input.RateLimits{
			FiveHour: &input.Window{UsedPercentage: 40},
			SevenDay: &input.Window{UsedPercentage: 65},
		},
	}}
	if _, vis := w.Render(ctx); vis {
		t.Errorf("expected hidden when inside Max plan limits")
	}
}

func TestCostShowsAtFiveHourOverage(t *testing.T) {
	w := &Cost{}
	ctx := &Context{Status: input.Status{
		Cost: &input.Cost{TotalCostUSD: 1.42},
		RateLimits: &input.RateLimits{
			FiveHour: &input.Window{UsedPercentage: 100},
			SevenDay: &input.Window{UsedPercentage: 65},
		},
	}}
	out, vis := w.Render(ctx)
	if !vis || !strings.Contains(out, "$1.42") {
		t.Errorf("got %q", out)
	}
}

func TestCostShowsAtSevenDayOverage(t *testing.T) {
	w := &Cost{}
	ctx := &Context{Status: input.Status{
		Cost: &input.Cost{TotalCostUSD: 1.42},
		RateLimits: &input.RateLimits{
			FiveHour: &input.Window{UsedPercentage: 40},
			SevenDay: &input.Window{UsedPercentage: 100},
		},
	}}
	out, vis := w.Render(ctx)
	if !vis || !strings.Contains(out, "$1.42") {
		t.Errorf("got %q", out)
	}
}

func TestCostShowsWhenNoRateLimits(t *testing.T) {
	// Non-Max user (no rate_limits field) → cost always shows when positive.
	w := &Cost{}
	ctx := &Context{Status: input.Status{Cost: &input.Cost{TotalCostUSD: 1.42}}}
	out, vis := w.Render(ctx)
	if !vis || !strings.Contains(out, "$1.42") {
		t.Errorf("got %q", out)
	}
}

func TestCostHidesWhenAbsent(t *testing.T) {
	if _, vis := (&Cost{}).Render(&Context{}); vis {
		t.Errorf("expected hidden")
	}
}

func TestCostHidesAtZero(t *testing.T) {
	w := &Cost{}
	ctx := &Context{Status: input.Status{Cost: &input.Cost{TotalCostUSD: 0}}}
	if _, vis := w.Render(ctx); vis {
		t.Errorf("expected hidden at 0")
	}
}
