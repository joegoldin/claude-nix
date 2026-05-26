package transcript

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

// ParseTail reads the last tailBytes of path and decodes well-formed JSONL
// entries from it. The first line may be a partial fragment if the tail
// window started mid-record; it is skipped on JSON decode failure.
// Returns (&Entries{}, nil) when path is missing.
func ParseTail(path string, tailBytes int64) (*Entries, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Entries{}, nil
		}
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	offset := int64(0)
	if info.Size() > tailBytes {
		offset = info.Size() - tailBytes
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	return decodeStream(f, offset > 0)
}

func decodeStream(r io.Reader, skipFirst bool) (*Entries, error) {
	br := bufio.NewReaderSize(r, 64*1024)
	if skipFirst {
		_, _ = br.ReadBytes('\n')
	}
	requestByKey := map[string]Request{}
	toolByID := map[string]Tool{}
	agentByID := map[string]Agent{}
	taskByID := map[string]TodoItem{}
	var requestOrder, toolOrder, agentOrder, taskOrder []string
	var todos []TodoSnapshot
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\n")
			classify(line, requestByKey, toolByID, &requestOrder, &toolOrder,
				agentByID, &agentOrder, &todos, &taskOrder, taskByID)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	e := &Entries{Todos: todos}
	for _, k := range requestOrder {
		e.Requests = append(e.Requests, requestByKey[k])
	}
	for _, k := range toolOrder {
		e.Tools = append(e.Tools, toolByID[k])
	}
	for _, k := range agentOrder {
		e.Agents = append(e.Agents, agentByID[k])
	}
	// Promote the FleetView TaskCreate/TaskUpdate stream to a final
	// TodoSnapshot so the Todos widget can treat both schemas uniformly.
	if len(taskOrder) > 0 {
		snap := TodoSnapshot{}
		for _, id := range taskOrder {
			t := taskByID[id]
			snap.Todos = append(snap.Todos, t)
		}
		e.Todos = append(e.Todos, snap)
	}
	return e, nil
}

// envelope captures the top-level wrapper for every JSONL line Claude Code
// writes. The interesting payload lives in `.message`, whose shape depends
// on `.type`.
type envelope struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	UUID      string          `json:"uuid"`
	SessionID string           `json:"sessionId"`
	Message   json.RawMessage `json:"message"`
}

// assistantMessage is the shape of `.message` on `type=="assistant"` lines.
// It holds the API-level message id (used for token-usage dedup) plus the
// `content` array where tool_use entries live.
type assistantMessage struct {
	ID      string                   `json:"id"`
	Model   string                   `json:"model"`
	Role    string                   `json:"role"`
	Content []contentBlock           `json:"content"`
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
	ID        string          `json:"id"`         // tool_use
	Name      string          `json:"name"`       // tool_use
	Input     json.RawMessage `json:"input"`      // tool_use
	ToolUseID string          `json:"tool_use_id"` // tool_result
	IsError   *bool           `json:"is_error"`    // tool_result
}

// todoWriteInput is the .input payload of a TodoWrite tool call.
type todoWriteInput struct {
	Todos []rawTodo `json:"todos"`
}

// agentInput is the .input payload of an Agent (or Task) tool call —
// matches what claude-hud extracts: subagent_type + optional model +
// description + run_in_background.
type agentInput struct {
	SubagentType string `json:"subagent_type"`
	Model        *string `json:"model"`
	Description  string `json:"description"`
	Background   bool    `json:"run_in_background"`
}

// taskCreateInput is the FleetView TaskCreate payload (used by some
// Claude variants like FleetView; standard Claude uses TodoWrite).
type taskCreateInput struct {
	Subject     string `json:"subject"`
	Description string `json:"description"`
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

func classify(
	line []byte,
	requestByKey map[string]Request,
	toolByID map[string]Tool,
	requestOrder, toolOrder *[]string,
	agentByID map[string]Agent,
	agentOrder *[]string,
	todos *[]TodoSnapshot,
	taskOrder *[]string,
	taskByID map[string]TodoItem,
) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return
	}
	ts := parseTime(env.Timestamp)
	switch env.Type {
	case "assistant":
		var msg assistantMessage
		if err := json.Unmarshal(env.Message, &msg); err != nil {
			return
		}
		// Token usage for burn rate.
		if msg.ID != "" {
			key := msg.ID
			if _, seen := requestByKey[key]; !seen {
				*requestOrder = append(*requestOrder, key)
			}
			requestByKey[key] = Request{
				ID:           msg.ID,
				Timestamp:    ts,
				InputTokens:  msg.Usage.InputTokens,
				CacheCreate:  msg.Usage.CacheCreationInputTokens,
				CacheRead:    msg.Usage.CacheReadInputTokens,
				OutputTokens: msg.Usage.OutputTokens,
			}
		}
		// Tool invocations live in content blocks. Classify each tool_use:
		//   - Agent / Task  → subagent launch (goes to agentByID)
		//   - TodoWrite     → todo snapshot
		//   - TaskCreate    → adds a new tracked task (FleetView model)
		//   - TaskUpdate    → updates status of an existing tracked task
		//   - anything else → regular tool (goes to toolByID)
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
				if _, seen := agentByID[c.ID]; !seen {
					*agentOrder = append(*agentOrder, c.ID)
				}
				agentByID[c.ID] = Agent{
					ID:          c.ID,
					Name:        name,
					Model:       model,
					Description: ai.Description,
					StartedAt:   ts,
					Background:  ai.Background,
				}
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
					if len(snap.Todos) > 0 {
						*todos = append(*todos, snap)
					}
				}
			case c.Name == "TaskCreate":
				var ti taskCreateInput
				if json.Unmarshal(c.Input, &ti) == nil && ti.Subject != "" {
					// Sequential 1-based id; matches the format Claude's
					// task tracker returns in subsequent TaskUpdate calls.
					id := itoa(len(*taskOrder) + 1)
					*taskOrder = append(*taskOrder, id)
					taskByID[id] = TodoItem{Subject: ti.Subject, Status: "pending"}
				}
			case c.Name == "TaskUpdate":
				var ti taskUpdateInput
				if json.Unmarshal(c.Input, &ti) == nil && ti.TaskID != "" {
					if t, ok := taskByID[ti.TaskID]; ok {
						if ti.Status != "" {
							t.Status = ti.Status
						}
						taskByID[ti.TaskID] = t
					}
				}
			default:
				if _, seen := toolByID[c.ID]; !seen {
					*toolOrder = append(*toolOrder, c.ID)
				}
				toolByID[c.ID] = Tool{
					ID:        c.ID,
					Name:      c.Name,
					Target:    toolTarget(c.Name, c.Input),
					Timestamp: ts,
					Completed: false,
				}
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
			if t, ok := toolByID[c.ToolUseID]; ok {
				t.Completed = true
				toolByID[c.ToolUseID] = t
			}
			// Foreground agents complete via tool_result. Background agents
			// emit their tool_result at launch time so we can't use it for
			// completion — leave EndedAt zero so they read as "still running".
			if a, ok := agentByID[c.ToolUseID]; ok && !a.Background {
				a.EndedAt = ts
				agentByID[c.ToolUseID] = a
			}
		}
	}
}

// itoa avoids strconv (we already have it via formatTokens) so the parser
// stays self-contained.
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
