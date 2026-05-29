package widgets

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/toolclock"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/transcript"
)

const (
	doneGlyph    = "✓"
	todoGlyph    = "▸"
	allDoneGlyph = "✓"
	// waitingGlyph marks a tool that has been emitted but hasn't started
	// running yet — queued behind another tool or sitting on a permission
	// prompt. An hourglass (instead of the animated spinner, and with no
	// elapsed counter) so the row doesn't imply work is underway while the
	// tool just waits in line.
	waitingGlyph = "" // nf-fa-hourglass-half
)

// runningSpinnerFrames are cycled by runningGlyph so the running indicator
// visibly animates between statusline refreshes (Claude Code re-invokes the
// statusline on refreshInterval, default 1s).
var runningSpinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// runningGlyph picks the spinner frame for the given moment. Indexed by
// whole seconds so each statusline refresh at the default 1s interval
// advances the spinner by exactly one frame — matching the elapsed
// counter's tick. Sub-second refreshes legitimately re-render the same
// frame; that's the cost of a clean 1-Hz rotation.
func runningGlyph(now time.Time) string {
	if len(runningSpinnerFrames) == 0 {
		return "◐"
	}
	idx := int(now.Unix() % int64(len(runningSpinnerFrames)))
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
	type toolState int
	const (
		stateRunning toolState = iota
		stateWaiting
		stateDone
	)
	type item struct {
		t      transcript.Tool
		state  toolState
		timing toolclock.Entry // real start/end from hooks; zero when unavailable
	}

	// timing maps tool_use_id → real execution window, populated by the
	// PermissionRequest / PostToolUse hooks. The transcript can't tell a
	// queued tool from one running in parallel (both are just "emitted, not
	// completed"), so we lean on the sidecar:
	//   - StartedAt set                → the tool has actually begun → running.
	//   - no StartedAt, a live runner   → genuinely queued behind it → waiting.
	//   - no StartedAt, nothing running → unhooked / nothing to wait behind →
	//     fall back to running so a missed hook never strands a tool as a
	//     perpetual hourglass (and so the row still works with no hooks at all).
	timing := ctx.ToolTiming()
	liveRunner := false
	for _, e := range timing {
		if !e.StartedAt.IsZero() && e.EndedAt.IsZero() {
			liveRunner = true
			break
		}
	}

	var items []item
	for _, t := range entries.Tools {
		e := timing[t.ID]
		st := stateRunning
		if e.StartedAt.IsZero() && liveRunner {
			st = stateWaiting
		}
		items = append(items, item{t: t, state: st, timing: e})
	}
	for _, t := range entries.RecentTools {
		if t.EndedAt.IsZero() || ctx.Now.Sub(t.EndedAt) > toolCompleteGrace {
			continue
		}
		items = append(items, item{t: t, state: stateDone, timing: timing[t.ID]})
	}
	if len(items) == 0 {
		return "", false
	}

	// Most-recent first: running/waiting tools count as "now" so they stay on
	// top, finished commands sort by completion time. Newer pushes older off.
	recency := func(it item) time.Time {
		if it.state == stateDone {
			return it.t.EndedAt
		}
		return ctx.Now
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
		var glyph string
		switch it.state {
		case stateWaiting:
			glyph = waitingGlyph
		case stateDone:
			glyph = doneGlyph
		default:
			glyph = runningGlyph(ctx.Now)
		}
		// Running tools show a live elapsed counter right after the spinner
		// (e.g. "⠋ (7s) Bash: …") so it tracks with the animated glyph; on
		// completion the counter freezes at the final run length so the
		// just-finished line records how long the command actually took. A
		// waiting tool hasn't started, so it shows no counter at all.
		//
		// Elapsed is measured from the tool's real execution start (the hook's
		// StartedAt) when we have it, which excludes queue + permission wait;
		// otherwise we fall back to the tool_use emission time so the counter
		// still works without hooks.
		var elapsedText string
		if it.state != stateWaiting {
			var elapsed time.Duration
			switch it.state {
			case stateRunning:
				switch {
				case !it.timing.StartedAt.IsZero():
					elapsed = ctx.Now.Sub(it.timing.StartedAt)
				case !it.t.Timestamp.IsZero():
					elapsed = ctx.Now.Sub(it.t.Timestamp)
				}
			case stateDone:
				switch {
				case !it.timing.StartedAt.IsZero() && !it.timing.EndedAt.IsZero():
					elapsed = it.timing.EndedAt.Sub(it.timing.StartedAt)
				case !it.t.Timestamp.IsZero() && !it.t.EndedAt.IsZero():
					elapsed = it.t.EndedAt.Sub(it.t.Timestamp)
				}
			}
			if elapsed >= time.Second {
				elapsedText = "(" + formatDuration(elapsed) + ") "
			}
		}
		label := it.t.Name
		if it.t.Target != "" {
			label += ": " + it.t.Target
		}
		// Budget against the prefix's actual cell width — the done glyph (✓)
		// and the waiting hourglass are two cells, so a fixed assumption would
		// overflow and trip the outer end-truncate into a spurious trailing
		// ellipsis.
		budget := perTool - render.VisibleWidth(glyph+" "+elapsedText)
		if budget < 1 {
			budget = 1
		}
		label = render.TruncateMiddle(label, budget)
		// Color the elapsed counter dim independently so it reads as metadata
		// against the spinner+command. Waiting tools render fully dim so they
		// recede behind the active one.
		var color func(string) string
		switch it.state {
		case stateWaiting:
			color = render.Dim
		case stateDone:
			color = render.Green
		default:
			color = render.Yellow
		}
		parts = append(parts, color(glyph+" ")+render.Dim(elapsedText)+color(label))
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

// agentCompleteGrace is how long a finished agent keeps showing after its
// result lands, mirroring toolCompleteGrace so completed agents don't linger
// in the statusline for the rest of the session.
const agentCompleteGrace = 30 * time.Second

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
			continue
		}
		// Drop long-completed agents so they don't linger for the rest of
		// the session, same as the running-tools row drops finished bash
		// commands past their grace window.
		if ctx.Now.Sub(a.EndedAt) > agentCompleteGrace {
			continue
		}
		completed = append(completed, a)
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
	// Elapsed renders right after the glyph (dim, "(7s) ") so it tracks with
	// the spinner the same way Tools do — instead of trailing off the end
	// where it gets lost behind a long description.
	var elapsed time.Duration
	if a.EndedAt.IsZero() {
		elapsed = ctx.Now.Sub(a.StartedAt)
	} else {
		elapsed = a.EndedAt.Sub(a.StartedAt)
	}
	var elapsedText string
	if elapsed >= time.Second {
		elapsedText = "(" + formatDuration(elapsed) + ") "
	}
	icon := render.Yellow(runningGlyph(ctx.Now))
	if !a.EndedAt.IsZero() {
		icon = render.Green(doneGlyph)
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
	return fmt.Sprintf("%s %s%s%s%s", icon, render.Dim(elapsedText), name, model, desc)
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
