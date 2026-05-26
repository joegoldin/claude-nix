package widgets

import (
	"fmt"
	"time"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
)

const durationGlyph = " " // nf-fa-clock-o

type Duration struct{}

func (Duration) Name() string { return "duration" }

func (Duration) Render(ctx *Context) (string, bool) {
	c := ctx.Status.Cost
	if c == nil || c.TotalDurationMS <= 0 {
		return "", false
	}
	d := time.Duration(c.TotalDurationMS) * time.Millisecond
	return render.Dim(durationGlyph + formatDuration(d)), true
}

// formatDuration returns "45s", "5m30s", or "1h12m" depending on magnitude.
func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
	if d < time.Hour {
		m := int(d / time.Minute)
		s := int(d/time.Second) - m*60
		return fmt.Sprintf("%dm%ds", m, s)
	}
	h := int(d / time.Hour)
	m := int(d/time.Minute) - h*60
	return fmt.Sprintf("%dh%dm", h, m)
}
