package widgets

import (
	"fmt"
	"strings"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
)

type ContextBar struct{}

func (ContextBar) Name() string { return "context" }

func (ContextBar) Render(ctx *Context) (string, bool) {
	pct, ok := contextPercent(ctx.Status)
	if !ok {
		return "", false
	}
	width := ctx.Cfg.BarWidth
	if width <= 0 {
		width = 8
	}
	color := render.ThresholdColor(pct)
	bar := color(render.Bar(pct, width))
	return fmt.Sprintf("%s %s", bar, color(fmt.Sprintf("%d%%", int(pct+0.5)))), true
}

// contextPercent computes the effective context percentage from Status,
// with a [1m] model-id fallback when context_window_size is missing.
func contextPercent(s input.Status) (float64, bool) {
	cw := s.ContextWindow
	if cw == nil {
		return 0, false
	}
	if cw.UsedPercentage != nil {
		return *cw.UsedPercentage, true
	}
	size := cw.ContextWindowSize
	if size == 0 && strings.Contains(strings.ToLower(s.Model.ID), "[1m]") {
		size = 1_000_000
	}
	if size == 0 {
		return 0, false
	}
	used := cw.TotalInputTokens
	if cw.CurrentUsage != nil {
		used = cw.CurrentUsage.InputTokens +
			cw.CurrentUsage.CacheCreationInputTokens +
			cw.CurrentUsage.CacheReadInputTokens
	}
	if used == 0 {
		return 0, false
	}
	return float64(used) / float64(size) * 100, true
}
