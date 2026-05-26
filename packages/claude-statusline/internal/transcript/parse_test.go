package transcript

import (
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
	if len(entries.Tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(entries.Tools))
	}
	if entries.Tools[0].Name != "Read" || entries.Tools[0].Target != "main.go" {
		t.Errorf("tool[0] = %+v", entries.Tools[0])
	}
	if !entries.Tools[0].Completed {
		t.Errorf("tool[0] should be completed (matching tool_result)")
	}
	if entries.Tools[1].Name != "Bash" || !strings.Contains(entries.Tools[1].Target, "go test") {
		t.Errorf("tool[1] = %+v", entries.Tools[1])
	}
	if entries.Tools[1].Completed {
		t.Errorf("tool[1] should still be running (no matching tool_result)")
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
