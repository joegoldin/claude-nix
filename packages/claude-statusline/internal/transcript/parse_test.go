package transcript

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTailExtractsTokensFromAssistantMessages(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	lines := []string{
		assistantLine("msg-1", now.Add(-30*time.Second), usage{input: 1500, cacheCreate: 200, cacheRead: 8000, output: 100}, nil),
		assistantLine("msg-2", now.Add(-10*time.Second), usage{input: 2000, output: 200}, nil),
	}
	path := writeJSONL(t, lines)
	entries, err := ParseTail(path, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries.Requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(entries.Requests))
	}
	if entries.Requests[0].InputTokens != 1500 || entries.Requests[0].CacheRead != 8000 {
		t.Errorf("first request: %+v", entries.Requests[0])
	}
}

func TestParseTailDedupesAssistantByMessageID(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	// Streaming can write the same message id multiple times with growing
	// token counts — the latest copy wins.
	lines := []string{
		assistantLine("msg-1", now.Add(-30*time.Second), usage{input: 1000, output: 50}, nil),
		assistantLine("msg-1", now.Add(-30*time.Second), usage{input: 1500, output: 100}, nil),
		assistantLine("msg-2", now.Add(-10*time.Second), usage{input: 2000, output: 200}, nil),
	}
	entries, err := ParseTail(writeJSONL(t, lines), 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries.Requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(entries.Requests))
	}
	if entries.Requests[0].InputTokens != 1500 {
		t.Errorf("dedup kept wrong copy: got %d input tokens", entries.Requests[0].InputTokens)
	}
}

func TestParseTailExtractsToolUsesFromContentBlocks(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	lines := []string{
		assistantLine("msg-1", now.Add(-20*time.Second), usage{input: 100}, []block{
			{Type: "tool_use", ID: "toolu_1", Name: "Read", Input: `{"file_path":"/foo/bar/main.go"}`},
		}),
		userResultLine(now.Add(-15*time.Second), "toolu_1", false),
		assistantLine("msg-2", now.Add(-10*time.Second), usage{input: 200}, []block{
			{Type: "tool_use", ID: "toolu_2", Name: "Bash", Input: `{"command":"go test ./...","description":"run tests"}`},
		}),
	}
	entries, err := ParseTail(writeJSONL(t, lines), 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	// Read completed (has a tool_result) → ToolCounts; Bash is still running → Tools.
	if len(entries.Tools) != 1 {
		t.Fatalf("running tools = %d, want 1 (%+v)", len(entries.Tools), entries.Tools)
	}
	if entries.Tools[0].Name != "Bash" || !strings.Contains(entries.Tools[0].Target, "go test") {
		t.Errorf("running tool = %+v", entries.Tools[0])
	}
	if len(entries.ToolCounts) != 1 || entries.ToolCounts[0].Name != "Read" || entries.ToolCounts[0].Count != 1 {
		t.Errorf("tool counts = %+v, want Read ×1", entries.ToolCounts)
	}
}

func TestParseTailToolCountsAccumulate(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	lines := []string{}
	// Three completed Reads + one completed Bash, across separate turns.
	for i, n := range []string{"r1", "r2", "r3", "b1"} {
		name := "Read"
		if n == "b1" {
			name = "Bash"
		}
		ts := now.Add(time.Duration(-40+i*5) * time.Second)
		lines = append(lines,
			assistantLine("m"+n, ts, usage{input: 10}, []block{
				{Type: "tool_use", ID: "t" + n, Name: name, Input: `{}`},
			}),
			userResultLine(ts.Add(time.Second), "t"+n, false),
		)
	}
	entries, err := ParseTail(writeJSONL(t, lines), 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, c := range entries.ToolCounts {
		got[c.Name] = c.Count
	}
	if got["Read"] != 3 || got["Bash"] != 1 {
		t.Errorf("counts = %+v, want Read 3 / Bash 1", got)
	}
}

func TestParseTailExtractsTodosFromTodoWrite(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	input := `{"todos":[{"subject":"refactor parser","status":"completed"},{"subject":"add tests","status":"in_progress"},{"subject":"docs","status":"pending"}]}`
	lines := []string{
		assistantLine("msg-1", now, usage{input: 100}, []block{
			{Type: "tool_use", ID: "toolu_t", Name: "TodoWrite", Input: input},
		}),
	}
	entries, err := ParseTail(writeJSONL(t, lines), 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries.Todos) != 1 {
		t.Fatalf("todos snapshots = %d, want 1", len(entries.Todos))
	}
	if got := entries.Todos[0].Todos; len(got) != 3 || got[1].Status != "in_progress" {
		t.Errorf("todos = %+v", got)
	}
}

func TestParseTailTracksTasksFromCreateResults(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	lines := []string{
		// Create two tasks; their real ids come back in the results.
		assistantLine("a1", now.Add(-50*time.Second), usage{input: 50}, []block{
			{Type: "tool_use", ID: "tc1", Name: "TaskCreate", Input: `{"subject":"refactor parser"}`},
		}),
		userResultTextLine(now.Add(-49*time.Second), "tc1", "Task #22 created successfully: refactor parser"),
		assistantLine("a2", now.Add(-48*time.Second), usage{input: 50}, []block{
			{Type: "tool_use", ID: "tc2", Name: "TaskCreate", Input: `{"subject":"write tests"}`},
		}),
		userResultTextLine(now.Add(-47*time.Second), "tc2", "Task #23 created successfully: write tests"),
		// Now flip the second task to in_progress via its REAL id (23).
		assistantLine("a3", now.Add(-10*time.Second), usage{input: 50}, []block{
			{Type: "tool_use", ID: "tu1", Name: "TaskUpdate", Input: `{"taskId":"23","status":"in_progress"}`},
		}),
	}
	entries, err := ParseTail(writeJSONL(t, lines), 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries.Todos) != 1 {
		t.Fatalf("expected 1 normalized todo snapshot, got %d", len(entries.Todos))
	}
	todos := entries.Todos[0].Todos
	if len(todos) != 2 {
		t.Fatalf("expected 2 tracked tasks, got %d (%+v)", len(todos), todos)
	}
	// Task #23 ("write tests") should be in_progress; #22 still pending.
	if todos[0].Subject != "refactor parser" || todos[0].Status != "pending" {
		t.Errorf("task[0] = %+v", todos[0])
	}
	if todos[1].Subject != "write tests" || todos[1].Status != "in_progress" {
		t.Errorf("task[1] = %+v, want write tests/in_progress", todos[1])
	}
}

func TestParseTailHandlesPartialFirstLine(t *testing.T) {
	body := strings.Repeat("x", 65*1024) + "\n" +
		assistantLine("msg-1", time.Now(), usage{input: 100}, nil) + "\n"
	path := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := ParseTail(path, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries.Requests) != 1 {
		t.Errorf("requests = %d, want 1", len(entries.Requests))
	}
}

func TestParseTailMissingFile(t *testing.T) {
	entries, err := ParseTail(filepath.Join(t.TempDir(), "nope.jsonl"), 64*1024)
	if err != nil {
		t.Errorf("missing file should not error, got %v", err)
	}
	if entries != nil && (len(entries.Requests) != 0 || len(entries.Tools) != 0) {
		t.Errorf("expected empty entries, got %+v", entries)
	}
}

// ---- helpers ----

type usage struct {
	input       int
	cacheCreate int
	cacheRead   int
	output      int
}

type block struct {
	Type      string
	ID        string
	Name      string
	Input     string // raw JSON
	ToolUseID string
}

func writeJSONL(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "transcript.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func assistantLine(msgID string, ts time.Time, u usage, content []block) string {
	contentJSON := "[]"
	if len(content) > 0 {
		parts := make([]string, 0, len(content))
		for _, b := range content {
			input := b.Input
			if input == "" {
				input = "{}"
			}
			parts = append(parts, fmt.Sprintf(
				`{"type":%q,"id":%q,"name":%q,"input":%s}`,
				b.Type, b.ID, b.Name, input))
		}
		contentJSON = "[" + strings.Join(parts, ",") + "]"
	}
	return fmt.Sprintf(
		`{"type":"assistant","timestamp":%q,"message":{"id":%q,"role":"assistant","content":%s,"usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,"output_tokens":%d}}}`,
		ts.UTC().Format(time.RFC3339Nano), msgID, contentJSON, u.input, u.cacheCreate, u.cacheRead, u.output)
}

func userResultLine(ts time.Time, toolUseID string, isError bool) string {
	return fmt.Sprintf(
		`{"type":"user","timestamp":%q,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":%q,"is_error":%t,"content":"ok"}]}}`,
		ts.UTC().Format(time.RFC3339Nano), toolUseID, isError)
}

func userResultTextLine(ts time.Time, toolUseID, text string) string {
	tb, _ := json.Marshal(text)
	return fmt.Sprintf(
		`{"type":"user","timestamp":%q,"message":{"role":"user","content":[{"type":"tool_result","tool_use_id":%q,"content":%s}]}}`,
		ts.UTC().Format(time.RFC3339Nano), toolUseID, string(tb))
}
