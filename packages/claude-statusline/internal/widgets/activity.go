package widgets

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/transcript"
)

const (
	doneGlyph    = "✓"
	todoGlyph    = "▸"
	allDoneGlyph = "✓"
)

// runningSpinnerFrames are cycled by runningGlyph so the running indicator
// visibly animates between statusline refreshes (Claude Code re-invokes the
// statusline on refreshInterval, default 1s).
var runningSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerStepMs is the per-frame interval (ms) for the running spinner.
// 13 is prime — coprime with every round refresh interval (1000/500/250/
// 100/50 ms) — and small enough to advance under any realistic refresh
// rate. A divisor of the refresh interval (e.g. the original 100 ms ×
// 10 frames = 1 s cycle) would stall the spinner on second-aligned ticks.
const spinnerStepMs = 13

// runningGlyph picks the spinner frame for the given moment.
func runningGlyph(now time.Time) string {
	if len(runningSpinnerFrames) == 0 {
		return "◐"
	}
	idx := int(now.UnixMilli()/spinnerStepMs) % len(runningSpinnerFrames)
	if idx < 0 {
		idx += len(runningSpinnerFrames)
	}
	return runningSpinnerFrames[idx]
}

// todoCompleteGrace is how long an all-complete todo list keeps showing before
// the line drops — long enough to register the completion, short enough not to
// linger as stale state.
const todoCompleteGrace = 60 * time.Second

// ----- Tools (running + just-finished) -----
//
// Shows currently-running tools (`◐ Name: target`, yellow) plus commands that
// finished within the last toolCompleteGrace (`✓ Name: target`, green) so a
// completed command lingers instead of vanishing instantly. Most-recent first
// — newer activity pushes older off the row. Caps at 2 (or 3 on a wide
// terminal); each command is middle-truncated to its share of the line.

type Tools struct{}

func (Tools) Name() string { return "tools" }

// toolCompleteGrace is how long a finished command keeps showing after its
// result lands, before it drops off the running row.
const toolCompleteGrace = 30 * time.Second

func (Tools) Render(ctx *Context) (string, bool) {
	entries := ctx.Transcript()
	if entries == nil {
		return "", false
	}
	type item struct {
		t       transcript.Tool
		running bool
	}
	var items []item
	for _, t := range entries.Tools {
		items = append(items, item{t: t, running: true})
	}
	for _, t := range entries.RecentTools {
		if t.EndedAt.IsZero() || ctx.Now.Sub(t.EndedAt) > toolCompleteGrace {
			continue
		}
		items = append(items, item{t: t, running: false})
	}
	if len(items) == 0 {
		return "", false
	}

	// Most-recent first: a running tool counts as "now" so it stays on top,
	// finished commands sort by completion time. Newer pushes older off.
	recency := func(it item) time.Time {
		if it.running {
			return ctx.Now
		}
		return it.t.EndedAt
	}
	sort.SliceStable(items, func(i, j int) bool {
		return recency(items[i]).After(recency(items[j]))
	})

	width := ctx.Width
	if width <= 0 {
		width = 80
	}
	maxShow := 2
	if width >= 120 {
		maxShow = 3
	}
	if len(items) > maxShow {
		items = items[:maxShow]
	}
	n := len(items)

	// Split the line evenly between shown tools; middle-truncate each so its
	// start and end stay readable.
	sepW := render.VisibleWidth("  ·  ")
	perTool := (width - (n-1)*sepW) / n

	parts := make([]string, 0, n)
	for _, it := range items {
		glyph := runningGlyph(ctx.Now)
		if !it.running {
			glyph = doneGlyph
		}
		// Running tools show the elapsed counter right after the spinner
		// (e.g. "⠋ (7s) Bash: …") so it tracks with the animated glyph
		// instead of trailing off the end of a long command.
		prefix := glyph + " "
		if it.running && !it.t.Timestamp.IsZero() {
			if elapsed := ctx.Now.Sub(it.t.Timestamp); elapsed >= time.Second {
				prefix += "(" + formatDuration(elapsed) + ") "
			}
		}
		label := it.t.Name
		if it.t.Target != "" {
			label += ": " + it.t.Target
		}
		// Budget against the prefix's actual cell width — the done glyph (✓)
		// is two cells, so a fixed assumption would overflow and trip the
		// outer end-truncate into a spurious trailing ellipsis.
		budget := perTool - render.VisibleWidth(prefix)
		if budget < 1 {
			budget = 1
		}
		label = render.TruncateMiddle(label, budget)
		seg := prefix + label
		if it.running {
			parts = append(parts, render.Yellow(seg))
		} else {
			parts = append(parts, render.Green(seg))
		}
	}
	return strings.Join(parts, "  ·  "), true
}

// ----- Tools (recent / completed aggregates) -----
//
// Shows up to five completed tool names with their counts: `✓ Read ×3`. All
// MCP tool calls (names prefixed `mcp__`) collapse into a single `MCP ×N`
// aggregate. Hides when nothing has completed.

type ToolsRecent struct{}

func (ToolsRecent) Name() string { return "toolsRecent" }

func (ToolsRecent) Render(ctx *Context) (string, bool) {
	entries := ctx.Transcript()
	if entries == nil || len(entries.ToolCounts) == 0 {
		return "", false
	}
	const maxAggregates = 5
	// Fold MCP calls together, then surface the busiest first.
	counts := foldMCPCounts(entries.ToolCounts)
	sort.SliceStable(counts, func(i, j int) bool {
		return counts[i].Count > counts[j].Count
	})
	if len(counts) > maxAggregates {
		counts = counts[:maxAggregates]
	}
	parts := make([]string, 0, len(counts))
	for _, c := range counts {
		parts = append(parts, render.Green(fmt.Sprintf("%s %s ×%d", doneGlyph, c.Name, c.Count)))
	}
	return strings.Join(parts, "  ·  "), true
}

// foldMCPCounts collapses every mcp__server__tool count into one "MCP" entry
// so MCP usage shows as a single column (e.g. "MCP ×12") instead of crowding
// out other tools with many low-count server-specific names.
func foldMCPCounts(in []transcript.ToolCount) []transcript.ToolCount {
	out := make([]transcript.ToolCount, 0, len(in))
	mcp := 0
	for _, c := range in {
		if strings.HasPrefix(c.Name, "mcp__") {
			mcp += c.Count
			continue
		}
		out = append(out, c)
	}
	if mcp > 0 {
		out = append(out, transcript.ToolCount{Name: "MCP", Count: mcp})
	}
	return out
}

// ----- Agents -----
//
// Up to three: prefer running over completed, newest first. Format:
// `◐ <type> [<model>]: <description> (<elapsed>)`.

type Agents struct{}

func (Agents) Name() string { return "agents" }

func (Agents) Render(ctx *Context) (string, bool) {
	entries := ctx.Transcript()
	if entries == nil || len(entries.Agents) == 0 {
		return "", false
	}
	const maxShown = 3
	const maxCompleted = 2

	// Most-recent first.
	sorted := append([]transcript.Agent(nil), entries.Agents...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].StartedAt.After(sorted[j].StartedAt)
	})

	var running []transcript.Agent
	var completed []transcript.Agent
	for _, a := range sorted {
		if a.EndedAt.IsZero() {
			running = append(running, a)
		} else {
			completed = append(completed, a)
		}
	}
	if len(completed) > maxCompleted {
		completed = completed[:maxCompleted]
	}

	pick := append([]transcript.Agent(nil), running...)
	pick = append(pick, completed...)
	if len(pick) > maxShown {
		pick = pick[:maxShown]
	}
	if len(pick) == 0 {
		return "", false
	}

	parts := make([]string, 0, len(pick))
	for _, a := range pick {
		parts = append(parts, formatAgent(a, ctx))
	}
	return strings.Join(parts, "  ·  "), true
}

func formatAgent(a transcript.Agent, ctx *Context) string {
	var elapsed string
	if a.EndedAt.IsZero() {
		elapsed = formatDuration(ctx.Now.Sub(a.StartedAt))
	} else {
		elapsed = formatDuration(a.EndedAt.Sub(a.StartedAt))
	}
	icon := render.Yellow(runningGlyph(ctx.Now))
	statusColor := render.Yellow
	if !a.EndedAt.IsZero() {
		icon = render.Green(doneGlyph)
		statusColor = render.Green
	}
	name := render.Magenta(a.Name)
	model := ""
	if a.Model != "" {
		model = " " + render.Dim("["+a.Model+"]")
	}
	desc := ""
	if a.Description != "" {
		desc = render.Dim(": " + clipForAgent(a.Description, 40))
	}
	return fmt.Sprintf("%s %s%s%s %s", icon, name, model, desc, statusColor("("+elapsed+")"))
}

func clipForAgent(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

// ----- Todos -----
//
// Shows the in-progress todo plus done/total. When all todos in the latest
// snapshot are complete, renders an "all done" line instead. Supports both
// the TodoWrite snapshot model and the TaskCreate/TaskUpdate id-based
// model — the parser normalizes both into TodoSnapshot.

type Todos struct{}

func (Todos) Name() string { return "todos" }

func (Todos) Render(ctx *Context) (string, bool) {
	entries := ctx.Transcript()
	if entries == nil || len(entries.Todos) == 0 {
		return "", false
	}
	latest := entries.Todos[len(entries.Todos)-1]
	if len(latest.Todos) == 0 {
		return "", false
	}
	done := 0
	var current *transcript.TodoItem
	for i := range latest.Todos {
		t := &latest.Todos[i]
		if t.Status == "completed" {
			done++
		}
		if t.Status == "in_progress" && current == nil {
			current = t
		}
	}
	total := len(latest.Todos)
	if current != nil {
		return render.Cyan(fmt.Sprintf("%s %s %s", todoGlyph, clipForAgent(current.Subject, 50),
			render.Dim(fmt.Sprintf("(%d/%d)", done, total)))), true
	}
	if done == total && total > 0 {
		// "All complete" is a brief celebration; drop it once the grace period
		// elapses (or immediately if we can't tell when it finished).
		if latest.Timestamp.IsZero() || ctx.Now.Sub(latest.Timestamp) > todoCompleteGrace {
			return "", false
		}
		return render.Green(fmt.Sprintf("%s all todos complete %s", allDoneGlyph,
			render.Dim(fmt.Sprintf("(%d/%d)", done, total)))), true
	}
	return "", false
}
