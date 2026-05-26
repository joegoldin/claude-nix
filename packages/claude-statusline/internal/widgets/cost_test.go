package widgets

import (
	"strings"
	"testing"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
)

func TestCostFormats(t *testing.T) {
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
