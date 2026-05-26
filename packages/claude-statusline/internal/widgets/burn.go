package widgets

import (
	"fmt"
	"strings"
	"time"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/transcript"
)

const burnGlyph = "⚡"

// BurnRate displays the rate at which new tokens are entering the context
// window, as percentage-per-minute (intuitive: 100% / rate ≈ minutes to
// full). Computed via a time-weighted EMA so a single spike doesn't fall
// off a cliff. Cache-read tokens are excluded — they don't grow the context.
type BurnRate struct{}

func (BurnRate) Name() string { return "burnRate" }

func (BurnRate) Render(ctx *Context) (string, bool) {
	entries := ctx.Transcript()
	if entries == nil || len(entries.Requests) == 0 {
		return "", false
	}
	size, ok := contextWindowSize(ctx.Status)
	if !ok || size <= 0 {
		return "", false
	}
	tau := time.Duration(ctx.Cfg.TranscriptWindowSeconds) * time.Second
	if tau <= 0 {
		tau = 60 * time.Second
	}
	tps := transcript.TokensPerSecondEMA(entries.Requests, ctx.Now, tau)
	if tps <= 0 {
		return "", false
	}
	pctPerMin := tps * 60 / float64(size) * 100
	rateStr := formatRate(pctPerMin)
	left := fmt.Sprintf("%s %s", burnGlyph, rateStr)

	pct, _ := contextPercent(ctx.Status)
	eta := etaToFull(size, pct, tps)
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

// contextWindowSize returns the effective context window size, with the
// [1m] model-id fallback that matches contextPercent in context_bar.go.
func contextWindowSize(s input.Status) (int, bool) {
	cw := s.ContextWindow
	if cw == nil {
		return 0, false
	}
	if cw.ContextWindowSize > 0 {
		return cw.ContextWindowSize, true
	}
	if strings.Contains(strings.ToLower(s.Model.ID), "[1m]") {
		return 1_000_000, true
	}
	return 200_000, true
}

// etaToFull projects when context hits 100% given a current new-token rate.
func etaToFull(size int, usedPct float64, tps float64) time.Duration {
	if usedPct >= 100 || tps <= 0 || size <= 0 {
		return 0
	}
	remainingTokens := float64(size) * (100 - usedPct) / 100
	secs := remainingTokens / tps
	return time.Duration(secs * float64(time.Second))
}

// formatRate renders the burn rate as a compact %/min value.
//
// Choices for the smallest readable unit:
//   - ≥10 %/m: integer ("12%/m")
//   - 0.5–10 %/m: one decimal ("2.4%/m")
//   - 0.01–0.5 %/m: two decimals ("0.08%/m")
//   - <0.01 %/m: hundredths-of-percent-per-hour ("3 hpp/h")  — useful when
//     usage is so light the per-minute number rounds to zero
func formatRate(pctPerMin float64) string {
	if pctPerMin >= 10 {
		return fmt.Sprintf("%d%%/m", int(pctPerMin+0.5))
	}
	if pctPerMin >= 0.5 {
		return fmt.Sprintf("%.1f%%/m", pctPerMin)
	}
	if pctPerMin >= 0.01 {
		return fmt.Sprintf("%.2f%%/m", pctPerMin)
	}
	// hundredths of percent per hour: pctPerMin * 60 * 100
	hpph := pctPerMin * 60 * 100
	return fmt.Sprintf("%d hpp/h", int(hpph+0.5))
}

// formatTokens compacts large numbers (e.g. 1234 → "1.2k").
// Retained because the tokens widget uses it.
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
