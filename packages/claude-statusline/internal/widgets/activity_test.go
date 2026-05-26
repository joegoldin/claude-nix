package widgets

import (
	"strings"
	"testing"
	"time"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/transcript"
)

func actCtx(e *transcript.Entries) *Context {
	return &Context{
		TranscriptProvider: func() *transcript.Entries { return e },
		Now:                time.Unix(1_000_000, 0),
	}
}

func TestToolsShowsRunningAndAggregates(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "1", Name: "Read", Completed: true, Timestamp: now},
		{ID: "2", Name: "Read", Completed: true, Timestamp: now},
		{ID: "3", Name: "Grep", Completed: true, Timestamp: now},
		{ID: "4", Name: "Edit", Target: "home.nix", Completed: false, Timestamp: now},
	}}
	out, vis := (&Tools{}).Render(actCtx(e))
	if !vis {
		t.Fatal("expected visible")
	}
	if !strings.Contains(out, "Edit") || !strings.Contains(out, "home.nix") {
		t.Errorf("expected running Edit in %q", out)
	}
	if !strings.Contains(out, "Read") || !strings.Contains(out, "×2") {
		t.Errorf("expected Read ×2 in %q", out)
	}
	if !strings.Contains(out, "Grep") {
		t.Errorf("expected Grep in %q", out)
	}
}

func TestToolsHidesWhenEmpty(t *testing.T) {
	if _, vis := (&Tools{}).Render(actCtx(&transcript.Entries{})); vis {
		t.Errorf("expected hidden")
	}
	if _, vis := (&Tools{}).Render(&Context{}); vis {
		t.Errorf("expected hidden without provider")
	}
}

func TestAgentsShowsRunningAndRecent(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Agents: []transcript.Agent{
		{Name: "explore", Model: "haiku", Description: "Finding LSP", StartedAt: now.Add(-135 * time.Second)},
		{Name: "test", Model: "sonnet", Description: "Done", StartedAt: now.Add(-300 * time.Second), EndedAt: now.Add(-100 * time.Second)},
	}}
	out, vis := (&Agents{}).Render(actCtx(e))
	if !vis {
		t.Fatal("expected visible")
	}
	if !strings.Contains(out, "explore") || !strings.Contains(out, "haiku") {
		t.Errorf("got %q", out)
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

func TestTodosHidesWhenAllDone(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Todos: []transcript.TodoSnapshot{
		{Timestamp: now, Todos: []transcript.TodoItem{
			{Subject: "a", Status: "completed"},
			{Subject: "b", Status: "completed"},
		}},
	}}
	if _, vis := (&Todos{}).Render(actCtx(e)); vis {
		t.Errorf("expected hidden when all done")
	}
}
