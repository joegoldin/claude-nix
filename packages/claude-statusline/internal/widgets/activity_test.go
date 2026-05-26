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

func TestToolsShowsOnlyRunning(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "1", Name: "Read", Completed: true, Timestamp: now},
		{ID: "2", Name: "Read", Completed: true, Timestamp: now},
		{ID: "3", Name: "Edit", Target: "home.nix", Completed: false, Timestamp: now},
		{ID: "4", Name: "Bash", Target: "go test", Completed: false, Timestamp: now},
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
	if strings.Contains(out, "Read") {
		t.Errorf("Tools should not include completed (Read): %q", out)
	}
}

func TestToolsHidesWhenNothingRunning(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "1", Name: "Read", Completed: true, Timestamp: now},
	}}
	if _, vis := (&Tools{}).Render(actCtx(e)); vis {
		t.Errorf("expected hidden when only completed tools")
	}
	if _, vis := (&Tools{}).Render(&Context{}); vis {
		t.Errorf("expected hidden without provider")
	}
}

func TestToolsRecentAggregatesByName(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "1", Name: "Read", Completed: true, Timestamp: now},
		{ID: "2", Name: "Read", Completed: true, Timestamp: now},
		{ID: "3", Name: "Read", Completed: true, Timestamp: now},
		{ID: "4", Name: "Grep", Completed: true, Timestamp: now},
		{ID: "5", Name: "Bash", Completed: false, Timestamp: now}, // running, ignored here
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
	if strings.Contains(out, "Bash") {
		t.Errorf("ToolsRecent should not include running (Bash): %q", out)
	}
}

func TestToolsRecentHidesWhenNothingCompleted(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	e := &transcript.Entries{Tools: []transcript.Tool{
		{ID: "1", Name: "Bash", Completed: false, Timestamp: now},
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

func TestTodosHidesWhenNoTodos(t *testing.T) {
	if _, vis := (&Todos{}).Render(actCtx(&transcript.Entries{})); vis {
		t.Errorf("expected hidden when no todos")
	}
}
