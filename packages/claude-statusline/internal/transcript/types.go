// Package transcript reads the trailing portion of Claude Code's JSONL
// transcript file. We only ever need recent entries: enough to compute a
// rolling burn rate plus the most recent tool/agent/todo activity.
package transcript

import "time"

// Entries is the result of ParseTail.
type Entries struct {
	Requests []Request
	Tools    []Tool
	Agents   []Agent
	Todos    []TodoSnapshot
}

type Request struct {
	ID            string
	RequestID     string
	Timestamp     time.Time
	InputTokens   int
	CacheCreate   int
	CacheRead     int
	OutputTokens  int
	ParentAgentID string
}

type Tool struct {
	ID        string
	Name      string
	Target    string
	Timestamp time.Time
	Completed bool
}

type Agent struct {
	ID          string
	Name        string // subagent_type (e.g. "Explore", "general-purpose")
	Model       string // optional model override; empty when default
	Description string
	StartedAt   time.Time
	EndedAt     time.Time
	Background  bool // run_in_background — tool_result on launch can't be used as completion
}

type TodoSnapshot struct {
	Timestamp time.Time
	Todos     []TodoItem
}

type TodoItem struct {
	Subject string
	Status  string
}
