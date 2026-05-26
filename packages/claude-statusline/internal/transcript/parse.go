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
	var requestOrder []string
	var toolOrder []string
	var agents []Agent
	var todos []TodoSnapshot
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			line = bytes.TrimRight(line, "\n")
			classify(line, requestByKey, toolByID, &requestOrder, &toolOrder, &agents, &todos)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	e := &Entries{
		Agents: agents,
		Todos:  todos,
	}
	for _, k := range requestOrder {
		e.Requests = append(e.Requests, requestByKey[k])
	}
	for _, k := range toolOrder {
		e.Tools = append(e.Tools, toolByID[k])
	}
	return e, nil
}

type envelope struct {
	Type        string          `json:"type"`
	ID          string          `json:"id"`
	RequestID   string          `json:"request_id"`
	Timestamp   string          `json:"timestamp"`
	Name        string          `json:"name"`
	Target      string          `json:"target"`
	Status      string          `json:"status"`
	Model       string          `json:"model"`
	Description string          `json:"description"`
	ParentAgent string          `json:"parent_agent_id"`
	Message     json.RawMessage `json:"message"`
	Todos       []rawTodo       `json:"todos"`
}

type rawTodo struct {
	Subject string `json:"subject"`
	Status  string `json:"status"`
}

type usageEnvelope struct {
	Usage struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	} `json:"usage"`
}

func classify(
	line []byte,
	requestByKey map[string]Request,
	toolByID map[string]Tool,
	requestOrder, toolOrder *[]string,
	agents *[]Agent,
	todos *[]TodoSnapshot,
) {
	var env envelope
	if err := json.Unmarshal(line, &env); err != nil {
		return
	}
	ts := parseTime(env.Timestamp)
	switch env.Type {
	case "assistant":
		var u usageEnvelope
		_ = json.Unmarshal(env.Message, &u)
		key := env.ID + "|" + env.RequestID
		if _, seen := requestByKey[key]; !seen {
			*requestOrder = append(*requestOrder, key)
		}
		requestByKey[key] = Request{
			ID:            env.ID,
			RequestID:     env.RequestID,
			Timestamp:     ts,
			InputTokens:   u.Usage.InputTokens,
			CacheCreate:   u.Usage.CacheCreationInputTokens,
			CacheRead:     u.Usage.CacheReadInputTokens,
			OutputTokens:  u.Usage.OutputTokens,
			ParentAgentID: env.ParentAgent,
		}
	case "tool_use":
		if _, seen := toolByID[env.ID]; !seen {
			*toolOrder = append(*toolOrder, env.ID)
		}
		toolByID[env.ID] = Tool{
			ID:        env.ID,
			Name:      env.Name,
			Target:    env.Target,
			Timestamp: ts,
			Completed: env.Status == "completed",
		}
	case "agent":
		ended := time.Time{}
		if env.Status == "completed" {
			ended = ts
		}
		*agents = append(*agents, Agent{
			ID:          env.ID,
			Name:        env.Name,
			Model:       env.Model,
			Description: env.Description,
			StartedAt:   ts,
			EndedAt:     ended,
		})
	case "todo_snapshot":
		snap := TodoSnapshot{Timestamp: ts}
		for _, raw := range env.Todos {
			snap.Todos = append(snap.Todos, TodoItem{Subject: raw.Subject, Status: raw.Status})
		}
		*todos = append(*todos, snap)
	}
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
