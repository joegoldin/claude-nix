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
	runningGlyph = "◐"
	doneGlyph    = "✓"
	todoGlyph    = "▸"
	allDoneGlyph = "✓"
)

// todoCompleteGrace is how long an all-complete todo list keeps showing before
// the line drops — long enough to register the completion, short enough not to
// linger as stale state.
const todoCompleteGrace = 60 * time.Second

// ----- Tools (running) -----
//
// Shows up to two currently-running tools (no matching tool_result yet),
// each rendered as `◐ Name: target`. Hides when nothing is in flight.

type Tools struct{}

func (Tools) Name() string { return "tools" }

func (Tools) Render(ctx *Context) (string, bool) {
	entries := ctx.Transcript()
	if entries == nil || len(entries.Tools) == 0 {
		return "", false
	}
	// entries.Tools is already just the running (uncompleted) tools.
	const maxRunning = 2
	running := entries.Tools
	start := 0
	if len(running) > maxRunning {
		start = len(running) - maxRunning
	}
	parts := make([]string, 0, maxRunning)
	for _, t := range running[start:] {
		label := t.Name
		if t.Target != "" {
			label += ": " + t.Target
		}
		parts = append(parts, render.Yellow(runningGlyph+" "+label))
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
	icon := render.Yellow(runningGlyph)
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
