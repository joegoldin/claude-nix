package widgets

import (
	"fmt"
	"time"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/transcript"
)

const burnGlyph = "⚡"

type BurnRate struct{}

func (BurnRate) Name() string { return "burnRate" }

func (BurnRate) Render(ctx *Context) (string, bool) {
	entries := ctx.Transcript()
	if entries == nil || len(entries.Requests) == 0 {
		return "", false
	}
	window := time.Duration(ctx.Cfg.TranscriptWindowSeconds) * time.Second
	if window <= 0 {
		window = 60 * time.Second
	}
	tps := transcript.TokensPerSecond(entries.Requests, ctx.Now, window)
	if tps <= 0 {
		return "", false
	}
	left := fmt.Sprintf("%s %s tok/s", burnGlyph, formatTokens(int(tps+0.5)))
	pct, ok := contextPercent(ctx.Status)
	if !ok {
		return render.Magenta(left), true
	}
	eta := etaToFull(ctx.Status, entries.Requests, ctx.Now, window, pct)
	if eta <= 0 {
		return render.Magenta(left), true
	}
	etaStr := fmt.Sprintf("ETA %s", formatDuration(eta))
	if eta < 15*time.Minute {
		etaStr = render.Red(etaStr)
	} else {
		etaStr = render.Dim(etaStr)
	}
	return render.Magenta(left) + " " + etaStr, true
}

// etaToFull estimates time to 100% context based on tokens consumed in the
// recent window relative to total window capacity.
func etaToFull(s input.Status, reqs []transcript.Request, now time.Time, window time.Duration, usedPct float64) time.Duration {
	if usedPct >= 100 {
		return 0
	}
	tokens := transcript.TokensInWindow(reqs, now, window)
	if tokens <= 0 {
		return 0
	}
	if s.ContextWindow == nil {
		return 0
	}
	size := s.ContextWindow.ContextWindowSize
	if size == 0 {
		return 0
	}
	tokensPerSec := float64(tokens) / window.Seconds()
	remainingTokens := float64(size) * (100 - usedPct) / 100
	if tokensPerSec <= 0 {
		return 0
	}
	secs := remainingTokens / tokensPerSec
	return time.Duration(secs * float64(time.Second))
}

// formatTokens compacts large numbers (e.g. 1234 → "1.2k").
func formatTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
