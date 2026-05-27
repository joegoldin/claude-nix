package transcript

import (
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strings"
	"time"
)

// ParseTail parses the whole file at path into a fresh accumulator and
// returns the projected Entries. It is the non-cached convenience entry
// point (used by tests); production rendering uses Load, which parses
// incrementally and persists state across renders. Returns (&Entries{},
// nil) when path is missing.
func ParseTail(path string, _ int64) (*Entries, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Entries{}, nil
		}
		return nil, err
	}
	defer f.Close()
	acc := newAccumulator(path)
	if _, err := acc.consume(f); err != nil {
		return nil, err
	}
	return acc.toEntries(), nil
}

// envelope captures the top-level wrapper for every JSONL line Claude Code
// writes. The interesting payload lives in `.message`, whose shape depends
// on `.type`.
type envelope struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	UUID      string          `json:"uuid"`
	SessionID string          `json:"sessionId"`
	Message   json.RawMessage `json:"message"`
	// IsCompactSummary flags the injected summary line Claude Code writes at a
	// /compact (or auto-compact) boundary. It's the authoritative, immediate
	// compaction signal — present the moment compaction happens, so a bare 1s
	// refresh resets epoch-scoped activity without waiting for the next turn.
	IsCompactSummary bool `json:"isCompactSummary"`
}

// assistantMessage is the shape of `.message` on `type=="assistant"` lines.
// It holds the API-level message id (used for token-usage dedup) plus the
// `content` array where tool_use entries live.
type assistantMessage struct {
	ID      string         `json:"id"`
	Model   string         `json:"model"`
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
	Usage   struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

// userMessage is the shape of `.message` on `type=="user"` lines. Tool
// completions arrive as tool_result content blocks here.
type userMessage struct {
	Role    string         `json:"role"`
	Content []contentBlock `json:"content"`
}

// contentBlock unifies the shapes we care about inside `.message.content[]`
// — tool_use and tool_result. Unrelated fields (e.g. text) are ignored.
type contentBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`          // tool_use
	Name      string          `json:"name"`        // tool_use
	Input     json.RawMessage `json:"input"`       // tool_use
	ToolUseID string          `json:"tool_use_id"` // tool_result
	IsError   *bool           `json:"is_error"`    // tool_result
	Content   json.RawMessage `json:"content"`     // tool_result (string or [{type,text}])
}

// taskCreatedRE matches the FleetView TaskCreate result text, e.g.
// "Task #22 created successfully: Run the full Go test suite". The real
// task id lives here (the tool_use input has no id), and TaskUpdate later
// references that same id.
var taskCreatedRE = regexp.MustCompile(`Task #(\d+) created successfully:\s*(.*)`)

// todoWriteInput is the .input payload of a TodoWrite tool call.
type todoWriteInput struct {
	Todos []rawTodo `json:"todos"`
}

// agentInput is the .input payload of an Agent (or Task) tool call —
// matches what claude-hud extracts: subagent_type + optional model +
// description + run_in_background.
type agentInput struct {
	SubagentType string  `json:"subagent_type"`
	Model        *string `json:"model"`
	Description  string  `json:"description"`
	Background   bool    `json:"run_in_background"`
}

// taskUpdateInput is the FleetView TaskUpdate payload.
type taskUpdateInput struct {
	TaskID string `json:"taskId"`
	Status string `json:"status"`
}

type rawTodo struct {
	Subject string `json:"subject"`
	Status  string `json:"status"`
	// Standard Claude Code TodoWrite uses "content" for the task text.
	Content    string `json:"content"`
	ActiveForm string `json:"activeForm"`
}

// isAgentTool returns true when a tool_use name represents launching a
// subagent (Agent or the older Task name).
func isAgentTool(name string) bool {
	return name == "Agent" || name == "Task"
}

// classifyLine decodes one JSONL record and folds it into the accumulator.
func (a *accumulator) classifyLine(line []byte) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return
	}
	// A compaction summary line resets epoch-scoped activity immediately and
	// re-baselines the prompt peak so the observePrompt heuristic doesn't
	// double-fire on the first (now-small) post-compaction message.
	if env.IsCompactSummary {
		a.resetEpoch()
		a.PeakPromptSize = 0
		return
	}
	ts := parseTime(env.Timestamp)
	switch env.Type {
	case "assistant":
		var msg assistantMessage
		if err := json.Unmarshal(env.Message, &msg); err != nil {
			return
		}
		if msg.ID != "" {
			// Detect a compaction/resume boundary from a sharp drop in prompt
			// size and reset epoch-scoped activity BEFORE counting this
			// message's tools (which belong to the new epoch).
			promptSize := msg.Usage.InputTokens + msg.Usage.CacheReadInputTokens + msg.Usage.CacheCreationInputTokens
			if a.observePrompt(promptSize) {
				a.resetEpoch()
			}
			a.addRequest(Request{
				ID:           msg.ID,
				Timestamp:    ts,
				InputTokens:  msg.Usage.InputTokens,
				CacheCreate:  msg.Usage.CacheCreationInputTokens,
				CacheRead:    msg.Usage.CacheReadInputTokens,
				OutputTokens: msg.Usage.OutputTokens,
			})
		}
		// Tool invocations live in content blocks. Classify each tool_use:
		//   - Agent / Task  → subagent launch
		//   - TodoWrite     → todo snapshot (standard Claude)
		//   - TaskCreate    → registered later from its result (FleetView)
		//   - TaskUpdate    → status change on a tracked task
		//   - anything else → a regular running tool
		for _, c := range msg.Content {
			if c.Type != "tool_use" || c.ID == "" {
				continue
			}
			switch {
			case isAgentTool(c.Name):
				var ai agentInput
				_ = json.Unmarshal(c.Input, &ai)
				name := ai.SubagentType
				if name == "" {
					name = c.Name
				}
				model := ""
				if ai.Model != nil {
					model = *ai.Model
				}
				a.addAgent(Agent{
					ID:          c.ID,
					Name:        name,
					Model:       model,
					Description: ai.Description,
					StartedAt:   ts,
					Background:  ai.Background,
				})
			case c.Name == "TodoWrite":
				var ti todoWriteInput
				if json.Unmarshal(c.Input, &ti) == nil {
					snap := TodoSnapshot{Timestamp: ts}
					for _, raw := range ti.Todos {
						subj := raw.Subject
						if subj == "" {
							subj = raw.Content
						}
						snap.Todos = append(snap.Todos, TodoItem{Subject: subj, Status: raw.Status})
					}
					// Record every write, including an empty list — that's
					// Claude clearing the todos, which must drop the line.
					s := snap
					a.LastTodoWrite = &s
				}
			case c.Name == "TaskCreate":
				// Real id is assigned by the tracker and reported in the
				// tool_result; we register the task when we see that result.
			case c.Name == "TaskUpdate":
				var ti taskUpdateInput
				if json.Unmarshal(c.Input, &ti) == nil && ti.TaskID != "" {
					if t, ok := a.TaskByID[ti.TaskID]; ok {
						if ti.Status != "" {
							t.Status = ti.Status
						}
						a.TaskByID[ti.TaskID] = t
						a.LastTaskActivity = ts
					}
				}
			default:
				a.addPendingTool(Tool{
					ID:        c.ID,
					Name:      c.Name,
					Target:    toolTarget(c.Name, c.Input),
					Timestamp: ts,
				})
			}
		}
	case "user":
		var msg userMessage
		if err := json.Unmarshal(env.Message, &msg); err != nil {
			return
		}
		for _, c := range msg.Content {
			if c.Type != "tool_result" || c.ToolUseID == "" {
				continue
			}
			a.completeTool(c.ToolUseID)
			a.completeAgent(c.ToolUseID, ts)
			// FleetView TaskCreate result carries the real task id + subject.
			if m := taskCreatedRE.FindStringSubmatch(resultText(c.Content)); m != nil {
				id, subject := m[1], strings.TrimSpace(m[2])
				if _, seen := a.TaskByID[id]; !seen {
					a.TaskOrder = append(a.TaskOrder, id)
				}
				a.TaskByID[id] = TodoItem{Subject: subject, Status: "pending"}
				a.LastTaskActivity = ts
			}
		}
	}
}

// resultText flattens a tool_result `content` field, which Claude Code
// writes either as a plain JSON string or as an array of {type,text}
// blocks, into a single searchable string.
func resultText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var b strings.Builder
		for _, blk := range blocks {
			b.WriteString(blk.Text)
			b.WriteByte('\n')
		}
		return b.String()
	}
	return ""
}

// toolTarget produces a short, glanceable display string from a tool's
// .input — file_path for Read/Edit/Write/etc., command for Bash, pattern
// for Glob/Grep. Empty when nothing useful is identifiable.
func toolTarget(name string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var fields struct {
		FilePath    string `json:"file_path"`
		Path        string `json:"path"`
		Command     string `json:"command"`
		Pattern     string `json:"pattern"`
		Description string `json:"description"`
		URL         string `json:"url"`
		Subject     string `json:"subject"`
	}
	if json.Unmarshal(raw, &fields) != nil {
		return ""
	}
	switch {
	case fields.FilePath != "":
		return basenameOf(fields.FilePath)
	case fields.Path != "":
		return basenameOf(fields.Path)
	case fields.Command != "":
		return clip(fields.Command, 40)
	case fields.Pattern != "":
		return clip(fields.Pattern, 40)
	case fields.URL != "":
		return clip(fields.URL, 40)
	case fields.Description != "":
		return clip(fields.Description, 40)
	case fields.Subject != "":
		return clip(fields.Subject, 40)
	}
	return ""
}

func basenameOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}

func clip(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, _ = time.Parse(time.RFC3339, s)
	}
	return t
}
