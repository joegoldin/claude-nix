package widgets

import (
	"strings"
	"testing"
	"time"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/transcript"
)

func actCtx(e *transcript.Entries) *Context {
	return &Context{
		TranscriptProvider: func() *transcript.Entries { return e },
		Now:                time.Unix(1_000_000, 0),
	}
}

func TestToolsShowsOnlyRunning(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "3", Name: "Edit", Target: "home.nix", Timestamp: now},
		{ID: "4", Name: "Bash", Target: "go test", Timestamp: now},
	}}
	out, vis := (&Tools{}).Render(actCtx(e))
	if !vis {
		t.Fatal("expected visible")
	}
	if !strings.Contains(out, "Edit") || !strings.Contains(out, "home.nix") {
		t.Errorf("expected running Edit in %q", out)
	}
	if !strings.Contains(out, "Bash") || !strings.Contains(out, "go test") {
		t.Errorf("expected running Bash in %q", out)
	}
}

func TestToolsRunningShowsSpinnerAndElapsed(t *testing.T) {
	start := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "1", Name: "Bash", Target: "sleep 30", Timestamp: start},
	}}
	ctx := &Context{
		TranscriptProvider: func() *transcript.Entries { return e },
		Now:                start.Add(7 * time.Second),
		Width:              80,
	}
	out, vis := (&Tools{}).Render(ctx)
	if !vis {
		t.Fatal("expected visible")
	}
	plain := render.StripANSI(out)
	if strings.Contains(plain, "◐") {
		t.Errorf("static half-circle should be replaced with a spinner frame: %q", plain)
	}
	frameSeen := false
	for _, f := range runningSpinnerFrames {
		if strings.Contains(plain, f) {
			frameSeen = true
			break
		}
	}
	if !frameSeen {
		t.Errorf("expected a spinner frame in %q", plain)
	}
	if !strings.Contains(plain, "(7s) Bash") {
		t.Errorf("expected '(7s)' between spinner and label in %q", plain)
	}
}

func TestToolsRunningOmitsElapsedUnderOneSecond(t *testing.T) {
	start := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "1", Name: "Read", Target: "main.go", Timestamp: start},
	}}
	ctx := &Context{
		TranscriptProvider: func() *transcript.Entries { return e },
		Now:                start.Add(500 * time.Millisecond),
		Width:              80,
	}
	out, _ := (&Tools{}).Render(ctx)
	plain := render.StripANSI(out)
	if strings.Contains(plain, "(0s)") {
		t.Errorf("sub-second elapsed shouldn't render a noisy (0s): %q", plain)
	}
}

func TestRunningGlyphAdvancesAtRefreshIntervals(t *testing.T) {
	// The original 100ms step × 10 frames had a 1s period, so under the
	// default 1s refresh the spinner landed on the same frame each tick.
	// Verify the current step survives every refresh interval Claude Code
	// is likely to use — and a few faster ones for good measure.
	intervals := []time.Duration{
		1000 * time.Millisecond,
		500 * time.Millisecond,
		250 * time.Millisecond,
		100 * time.Millisecond,
		50 * time.Millisecond,
	}
	base := time.UnixMilli(1_780_000_000_000)
	for _, d := range intervals {
		a := runningGlyph(base)
		b := runningGlyph(base.Add(d))
		if a == b {
			t.Errorf("spinner stalled across a %s refresh: %q == %q", d, a, b)
		}
	}
}

func TestToolsSingleRunningUsesFullWidth(t *testing.T) {
	long := "cd /Users/joe/Development/dotfiles/agent-skills && nix flake update claude-nix && git commit -am bump"
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "1", Name: "Bash", Target: long, Timestamp: time.Unix(1_000_000, 0)},
	}}
	ctx := &Context{
		TranscriptProvider: func() *transcript.Entries { return e },
		Now:                time.Unix(1_000_000, 0),
		Width:              80,
	}
	out, vis := (&Tools{}).Render(ctx)
	if !vis {
		t.Fatal("expected visible")
	}
	if w := render.VisibleWidth(out); w > 80 {
		t.Errorf("row width %d exceeds 80: %q", w, out)
	} else if w < 70 {
		t.Errorf("a single tool should fill most of the line, got %d: %q", w, out)
	}
	plain := render.StripANSI(out)
	if !strings.Contains(plain, "Bash: cd /Users") {
		t.Errorf("expected command start in %q", plain)
	}
	if !strings.HasSuffix(plain, "bump") {
		t.Errorf("expected command end (middle truncation) in %q", plain)
	}
	if !strings.Contains(plain, "…") {
		t.Errorf("expected middle ellipsis in %q", plain)
	}
}

func TestToolsMultipleSplitWidthEvenly(t *testing.T) {
	long := strings.Repeat("x", 200)
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "1", Name: "Bash", Target: long, Timestamp: time.Unix(1_000_000, 0)},
		{ID: "2", Name: "Grep", Target: long, Timestamp: time.Unix(1_000_000, 0)},
	}}
	ctx := &Context{
		TranscriptProvider: func() *transcript.Entries { return e },
		Now:                time.Unix(1_000_000, 0),
		Width:              80,
	}
	out, _ := (&Tools{}).Render(ctx)
	if w := render.VisibleWidth(out); w > 80 {
		t.Errorf("row width %d exceeds 80: %q", w, out)
	}
	plain := render.StripANSI(out)
	if !strings.Contains(plain, "Bash") || !strings.Contains(plain, "Grep") {
		t.Errorf("expected both tools present in %q", plain)
	}
	// Both targets exceed their share → each is middle-truncated.
	if n := strings.Count(plain, "…"); n != 2 {
		t.Errorf("expected 2 truncated segments (even split), got %d: %q", n, plain)
	}
}

func TestToolsShowsRecentlyCompletedThenDrops(t *testing.T) {
	done := time.Unix(1_000_000, 0)
	e := &transcript.Entries{RecentTools: []transcript.Tool{
		{ID: "t1", Name: "Bash", Target: "go test", Timestamp: done.Add(-5 * time.Second), EndedAt: done},
	}}
	// Within the grace window → the finished command lingers, marked done.
	within := &Context{
		TranscriptProvider: func() *transcript.Entries { return e },
		Now:                done.Add(3 * time.Second),
		Width:              80,
	}
	out, vis := (&Tools{}).Render(within)
	if !vis {
		t.Fatal("expected the just-completed command to stay visible within grace")
	}
	plain := render.StripANSI(out)
	if !strings.Contains(plain, "Bash") || !strings.Contains(plain, "go test") {
		t.Errorf("expected the completed command shown, got %q", plain)
	}
	if !strings.Contains(plain, doneGlyph) {
		t.Errorf("a completed command should use the done glyph, got %q", plain)
	}
	// Past the grace window → it drops.
	after := &Context{
		TranscriptProvider: func() *transcript.Entries { return e },
		Now:                done.Add(toolCompleteGrace + time.Second),
		Width:              80,
	}
	if _, vis := (&Tools{}).Render(after); vis {
		t.Errorf("a completed command should drop after the grace window")
	}
}

func TestToolsCompletedCommandFitsWidth(t *testing.T) {
	done := time.Unix(1_000_000, 0)
	long := strings.Repeat("x", 300)
	e := &transcript.Entries{RecentTools: []transcript.Tool{
		{ID: "t1", Name: "Bash", Target: long, Timestamp: done.Add(-2 * time.Second), EndedAt: done},
	}}
	ctx := &Context{
		TranscriptProvider: func() *transcript.Entries { return e },
		Now:                done.Add(time.Second),
		Width:              80,
	}
	out, vis := (&Tools{}).Render(ctx)
	if !vis {
		t.Fatal("expected visible")
	}
	// The done glyph (✓) is two cells wide; if the budget doesn't account for
	// that, the row overflows by one and the outer end-truncate adds a stray ….
	if w := render.VisibleWidth(out); w > 80 {
		t.Errorf("completed-command row width %d exceeds 80: %q", w, out)
	}
}

func TestToolsHidesWhenNothingRunning(t *testing.T) {
	// Only completed tools (ToolCounts) → the running row hides.
	e := &transcript.Entries{ToolCounts: []transcript.ToolCount{{Name: "Read", Count: 1}}}
	if _, vis := (&Tools{}).Render(actCtx(e)); vis {
		t.Errorf("expected hidden when nothing running")
	}
	if _, vis := (&Tools{}).Render(&Context{}); vis {
		t.Errorf("expected hidden without provider")
	}
}

func TestToolsRecentAggregatesByName(t *testing.T) {
	e := &transcript.Entries{ToolCounts: []transcript.ToolCount{
		{Name: "Read", Count: 3},
		{Name: "Grep", Count: 1},
	}}
	out, vis := (&ToolsRecent{}).Render(actCtx(e))
	if !vis {
		t.Fatal("expected visible")
	}
	if !strings.Contains(out, "Read") || !strings.Contains(out, "×3") {
		t.Errorf("expected Read ×3 in %q", out)
	}
	if !strings.Contains(out, "Grep") || !strings.Contains(out, "×1") {
		t.Errorf("expected Grep ×1 in %q", out)
	}
}

func TestToolsRecentFoldsMCPCalls(t *testing.T) {
	e := &transcript.Entries{ToolCounts: []transcript.ToolCount{
		{Name: "mcp__server_a__foo", Count: 3},
		{Name: "mcp__server_b__bar", Count: 2},
		{Name: "Skill", Count: 2},
		{Name: "Bash", Count: 39},
	}}
	out, vis := (&ToolsRecent{}).Render(actCtx(e))
	if !vis {
		t.Fatal("expected visible")
	}
	if !strings.Contains(out, "MCP ×5") {
		t.Errorf("expected all mcp__* folded into MCP ×5 in %q", out)
	}
	if !strings.Contains(out, "Skill ×2") {
		t.Errorf("expected Skill ×2 alongside MCP in %q", out)
	}
	if strings.Contains(out, "mcp__") {
		t.Errorf("raw mcp__ tool names should not appear: %q", out)
	}
}

func TestToolsRecentShowsUpToFive(t *testing.T) {
	e := &transcript.Entries{ToolCounts: []transcript.ToolCount{
		{Name: "Bash", Count: 39}, {Name: "Edit", Count: 18}, {Name: "Read", Count: 11},
		{Name: "Skill", Count: 5}, {Name: "Write", Count: 3}, {Name: "Grep", Count: 2},
	}}
	out, _ := (&ToolsRecent{}).Render(actCtx(e))
	if got := strings.Count(out, "×"); got != 5 {
		t.Errorf("expected 5 columns, got %d in %q", got, out)
	}
	if strings.Contains(out, "Grep") {
		t.Errorf("lowest-count tool should be dropped past 5 columns: %q", out)
	}
}

func TestToolsRecentHidesWhenNothingCompleted(t *testing.T) {
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "1", Name: "Bash", Timestamp: time.Unix(1_000_000, 0)},
	}}
	if _, vis := (&ToolsRecent{}).Render(actCtx(e)); vis {
		t.Errorf("expected hidden when only running tools")
	}
}

func TestAgentsShowsRunningAndRecent(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Agents: []transcript.Agent{
		{Name: "Explore", Model: "haiku", Description: "Finding LSP",
			StartedAt: now.Add(-135 * time.Second)},
		{Name: "general-purpose", Description: "Done",
			StartedAt: now.Add(-300 * time.Second), EndedAt: now.Add(-100 * time.Second)},
	}}
	out, vis := (&Agents{}).Render(actCtx(e))
	if !vis {
		t.Fatal("expected visible")
	}
	if !strings.Contains(out, "Explore") {
		t.Errorf("expected running Explore in %q", out)
	}
	if !strings.Contains(out, "haiku") {
		t.Errorf("expected model haiku in %q", out)
	}
	if !strings.Contains(out, "general-purpose") {
		t.Errorf("expected completed general-purpose in %q", out)
	}
}

func TestAgentsHidesWhenEmpty(t *testing.T) {
	if _, vis := (&Agents{}).Render(actCtx(&transcript.Entries{})); vis {
		t.Errorf("expected hidden")
	}
}

func TestTodosShowsCurrentInProgress(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Todos: []transcript.TodoSnapshot{
		{Timestamp: now, Todos: []transcript.TodoItem{
			{Subject: "wire up", Status: "completed"},
			{Subject: "write tests", Status: "in_progress"},
			{Subject: "ship it", Status: "pending"},
		}},
	}}
	out, vis := (&Todos{}).Render(actCtx(e))
	if !vis {
		t.Fatal("expected visible")
	}
	if !strings.Contains(out, "write tests") {
		t.Errorf("expected current todo in %q", out)
	}
	if !strings.Contains(out, "1/3") {
		t.Errorf("expected 1/3 in %q", out)
	}
}

func TestTodosShowsCelebrationWhenAllDone(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Todos: []transcript.TodoSnapshot{
		{Timestamp: now, Todos: []transcript.TodoItem{
			{Subject: "a", Status: "completed"},
			{Subject: "b", Status: "completed"},
		}},
	}}
	out, vis := (&Todos{}).Render(actCtx(e))
	if !vis {
		t.Fatal("expected visible — all-done is its own affirmation")
	}
	if !strings.Contains(out, "all todos complete") {
		t.Errorf("expected 'all todos complete' message in %q", out)
	}
	if !strings.Contains(out, "2/2") {
		t.Errorf("expected 2/2 in %q", out)
	}
}

func TestTodosDropsCelebrationAfterGrace(t *testing.T) {
	done := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Todos: []transcript.TodoSnapshot{
		{Timestamp: done, Todos: []transcript.TodoItem{
			{Subject: "a", Status: "completed"},
			{Subject: "b", Status: "completed"},
		}},
	}}
	ctx := &Context{
		TranscriptProvider: func() *transcript.Entries { return e },
		Now:                done.Add(todoCompleteGrace + time.Second),
	}
	if _, vis := (&Todos{}).Render(ctx); vis {
		t.Errorf("all-complete todos should drop after the grace period")
	}
}

func TestTodosHidesWhenNoTodos(t *testing.T) {
	if _, vis := (&Todos{}).Render(actCtx(&transcript.Entries{})); vis {
		t.Errorf("expected hidden when no todos")
	}
}
