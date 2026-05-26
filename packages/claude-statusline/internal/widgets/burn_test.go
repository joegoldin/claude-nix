package widgets

import (
	"strings"
	"testing"
	"time"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/config"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/transcript"
)

func burnCtx(now time.Time, reqs []transcript.Request, usedPct float64) *Context {
	pct := usedPct
	return &Context{
		Status: input.Status{ContextWindow: &input.ContextWindow{
			ContextWindowSize: 200_000,
			UsedPercentage:    &pct,
		}},
		Cfg: config.Config{TranscriptWindowSeconds: 60},
		Now: now,
		TranscriptProvider: func() *transcript.Entries {
			return &transcript.Entries{Requests: reqs}
		},
	}
}

func TestBurnRendersTokensPerSecond(t *testing.T) {
	w := &BurnRate{}
	now := time.Unix(1_000_000, 0)
	reqs := []transcript.Request{
		{Timestamp: now.Add(-30 * time.Second), InputTokens: 1500, OutputTokens: 300},
	}
	out, vis := w.Render(burnCtx(now, reqs, 47))
	if !vis {
		t.Fatal("expected visible")
	}
	if !strings.Contains(out, "tok/s") {
		t.Errorf("expected tok/s in %q", out)
	}
}

func TestBurnHidesWithoutTranscript(t *testing.T) {
	w := &BurnRate{}
	if _, vis := w.Render(&Context{}); vis {
		t.Errorf("expected hidden")
	}
}

func TestBurnETARed(t *testing.T) {
	w := &BurnRate{}
	now := time.Unix(1_000_000, 0)
	reqs := []transcript.Request{
		{Timestamp: now.Add(-60 * time.Second), InputTokens: 10000, OutputTokens: 0},
	}
	out, vis := w.Render(burnCtx(now, reqs, 80))
	if !vis {
		t.Fatal("expected visible")
	}
	if !strings.Contains(out, "ETA") {
		t.Errorf("expected ETA in %q", out)
	}
}
