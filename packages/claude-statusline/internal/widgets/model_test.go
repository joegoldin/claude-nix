package widgets

import (
	"strings"
	"testing"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
)

func TestModelWidgetBasic(t *testing.T) {
	w := &Model{}
	ctx := &Context{Status: input.Status{Model: input.Model{DisplayName: "Opus"}}}
	out, vis := w.Render(ctx)
	if !vis {
		t.Fatal("expected visible")
	}
	if !strings.Contains(out, "Opus") {
		t.Errorf("output missing Opus: %q", out)
	}
}

func TestModelWidgetStripsContextSuffix(t *testing.T) {
	w := &Model{}
	ctx := &Context{Status: input.Status{Model: input.Model{DisplayName: "Opus (1M context)"}}}
	out, _ := w.Render(ctx)
	if strings.Contains(out, "1M") {
		t.Errorf("expected suffix stripped: %q", out)
	}
}

func TestModelWidgetHidesWhenAbsent(t *testing.T) {
	w := &Model{}
	if _, vis := w.Render(&Context{}); vis {
		t.Errorf("expected hidden when display_name empty")
	}
}
