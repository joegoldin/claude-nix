package widgets

import (
	"fmt"
	"sort"
	"strings"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/transcript"
)

const (
	runningGlyph = "◐"
	doneGlyph    = "✓"
	todoGlyph    = "▸"
)

// ----- Tools -----

type Tools struct{}

func (Tools) Name() string { return "tools" }

func (Tools) Render(ctx *Context) (string, bool) {
	entries := ctx.Transcript()
	if entries == nil || len(entries.Tools) == 0 {
		return "", false
	}
	var running *transcript.Tool
	counts := map[string]int{}
	var order []string
	for i := range entries.Tools {
		t := entries.Tools[i]
		if !t.Completed && running == nil {
			running = &t
			continue
		}
		if _, seen := counts[t.Name]; !seen {
			order = append(order, t.Name)
		}
		counts[t.Name]++
	}
	parts := []string{}
	if running != nil {
		label := running.Name
		if running.Target != "" {
			label += ": " + running.Target
		}
		parts = append(parts, render.Yellow(runningGlyph+" "+label))
	}
	for i, name := range order {
		if i >= 4 {
			break
		}
		parts = append(parts, render.Green(fmt.Sprintf("%s %s ×%d", doneGlyph, name, counts[name])))
	}
	if len(parts) == 0 {
		return "", false
	}
	return strings.Join(parts, "  ·  "), true
}

// ----- Agents -----

type Agents struct{}

func (Agents) Name() string { return "agents" }

func (Agents) Render(ctx *Context) (string, bool) {
	entries := ctx.Transcript()
	if entries == nil || len(entries.Agents) == 0 {
		return "", false
	}
	sorted := append([]transcript.Agent(nil), entries.Agents...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartedAt.After(sorted[j].StartedAt)
	})
	var lines []string
	var running *transcript.Agent
	for i := range sorted {
		if sorted[i].EndedAt.IsZero() && running == nil {
			a := sorted[i]
			running = &a
			break
		}
	}
	if running != nil {
		elapsed := ctx.Now.Sub(running.StartedAt)
		line := fmt.Sprintf("%s %s [%s]: %s (%s)",
			runningGlyph, running.Name, running.Model, running.Description, formatDuration(elapsed))
		lines = append(lines, render.Yellow(line))
	}
	added := 0
	for i := range sorted {
		if !sorted[i].EndedAt.IsZero() {
			a := sorted[i]
			elapsed := a.EndedAt.Sub(a.StartedAt)
			line := fmt.Sprintf("%s %s [%s]: %s (%s)",
				doneGlyph, a.Name, a.Model, a.Description, formatDuration(elapsed))
			lines = append(lines, render.Green(line))
			added++
			if added >= 2 {
				break
			}
		}
	}
	if len(lines) == 0 {
		return "", false
	}
	return strings.Join(lines, "  ·  "), true
}

// ----- Todos -----

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
	if current == nil {
		return "", false
	}
	out := fmt.Sprintf("%s %s (%d/%d)", todoGlyph, current.Subject, done, len(latest.Todos))
	return render.Cyan(out), true
}
