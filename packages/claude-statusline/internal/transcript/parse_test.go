package transcript

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseTailDeduplicates(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	lines := []string{
		jsonLine("msg-1", "req-A", now.Add(-30*time.Second), 1000, 0, 50),
		jsonLine("msg-1", "req-A", now.Add(-30*time.Second), 1500, 0, 100),
		jsonLine("msg-2", "req-B", now.Add(-10*time.Second), 2000, 0, 200),
		toolUseLine("tool-1", "Edit", "main.go", now.Add(-5*time.Second), false),
		toolUseLine("tool-1", "Edit", "main.go", now.Add(-2*time.Second), true),
	}
	path := writeJSONL(t, lines)
	entries, err := ParseTail(path, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries.Requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(entries.Requests))
	}
	if entries.Requests[0].InputTokens != 1500 {
		t.Errorf("dedup kept wrong copy: %+v", entries.Requests[0])
	}
	if len(entries.Tools) != 1 {
		t.Fatalf("tools = %d, want 1", len(entries.Tools))
	}
	if !entries.Tools[0].Completed {
		t.Errorf("tool should be marked completed")
	}
}

func TestParseTailHandlesPartialFirstLine(t *testing.T) {
	body := strings.Repeat("x", 65*1024) + "\n" + jsonLine("msg-1", "req-A", time.Now(), 100, 0, 10) + "\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
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

func writeJSONL(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func jsonLine(id, reqID string, ts time.Time, input, cacheCreate, output int) string {
	return `{"type":"assistant","id":"` + id + `","request_id":"` + reqID +
		`","timestamp":"` + ts.UTC().Format(time.RFC3339) + `",` +
		`"message":{"usage":{"input_tokens":` + itoa(input) +
		`,"cache_creation_input_tokens":` + itoa(cacheCreate) +
		`,"output_tokens":` + itoa(output) + `}}}`
}

func toolUseLine(id, name, target string, ts time.Time, completed bool) string {
	status := "in_progress"
	if completed {
		status = "completed"
	}
	return `{"type":"tool_use","id":"` + id + `","name":"` + name +
		`","target":"` + target + `","timestamp":"` + ts.UTC().Format(time.RFC3339) +
		`","status":"` + status + `"}`
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
