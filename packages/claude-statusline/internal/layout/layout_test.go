package layout

import (
	"strings"
	"testing"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/widgets"
)

type fake struct {
	name string
	out  string
	vis  bool
}

func (f fake) Name() string                           { return f.name }
func (f fake) Render(*widgets.Context) (string, bool) { return f.out, f.vis }

func TestComposeRowSimple(t *testing.T) {
	row := []widgets.Widget{
		fake{"model", "Opus", true},
		fake{"cwd", "claude-nix", true},
	}
	out := ComposeRow(row, nil, &widgets.Context{}, Options{Width: 80, DropPriority: nil})
	if !strings.Contains(out, "Opus") || !strings.Contains(out, "claude-nix") {
		t.Errorf("got %q", out)
	}
	if !strings.Contains(out, " │ ") {
		t.Errorf("expected separator in %q", out)
	}
}

func TestComposeRowSeparatorCollapsesAroundHidden(t *testing.T) {
	row := []widgets.Widget{
		fake{"a", "A", true},
		fake{"b", "B", false},
		fake{"c", "C", true},
	}
	out := ComposeRow(row, nil, &widgets.Context{}, Options{Width: 80})
	if strings.Contains(out, "│ │") {
		t.Errorf("dangling separator in %q", out)
	}
	if !strings.Contains(out, "A") || !strings.Contains(out, "C") {
		t.Errorf("got %q", out)
	}
}

func TestComposeRowFlexSpacerRightAligns(t *testing.T) {
	row := []widgets.Widget{
		fake{"a", "A", true},
		flexMarker{},
		fake{"b", "B", true},
	}
	out := ComposeRow(row, nil, &widgets.Context{}, Options{Width: 20})
	if !strings.HasPrefix(render.StripANSI(out), "A") {
		t.Errorf("not left-anchored: %q", out)
	}
	if !strings.HasSuffix(render.StripANSI(out), "B") {
		t.Errorf("not right-anchored: %q", out)
	}
}

func TestComposeRowOverflowDropsByPriority(t *testing.T) {
	row := []widgets.Widget{
		fake{"low", strings.Repeat("L", 10), true},
		fake{"high", strings.Repeat("H", 10), true},
	}
	priority := []string{"low", "high"}
	out := ComposeRow(row, nil, &widgets.Context{}, Options{Width: 12, DropPriority: priority})
	if strings.Contains(out, "LLLLLLLLLL") {
		t.Errorf("low-priority segment should have dropped: %q", out)
	}
	if !strings.Contains(out, "HHHHHHHHHH") {
		t.Errorf("high-priority segment missing: %q", out)
	}
}
