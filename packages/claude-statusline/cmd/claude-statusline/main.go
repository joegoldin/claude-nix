package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/compaction"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/config"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/gitcache"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/layout"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/transcript"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/voice"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/widgets"
)

// dropPriority — lowest first (dropped first on overflow).
// Conversation-state widgets drop before identity/account widgets, so a
// narrow terminal keeps the "who/where/budget" header intact.
var dropPriority = []string{
	"sessionName", "compaction", "pr", "voice", "cost",
	"burnRate", "duration", "tokens", "effort", "context",
	"usage7d", "usage5h", "git", "cwd", "model",
}

func main() {
	defer func() {
		if r := recover(); r != nil {
			debugLog("panic in main: %v", r)
			os.Exit(0)
		}
	}()

	status, err := input.Decode(os.Stdin)
	if err != nil {
		debugLog("input.Decode: %v", err)
		os.Exit(0)
	}

	cfg, err := config.Load(config.ResolvePath())
	if err != nil {
		debugLog("config.Load: %v", err)
	}

	cacheRoot := userCacheDir()

	ctx := &widgets.Context{
		Status: status,
		Cfg:    cfg,
		Now:    resolveNow(),
	}

	ctx.GitProvider = memoize(func() *gitcache.Git {
		c := &gitcache.Cache{
			Dir:        filepath.Join(cacheRoot, "git"),
			TTLSeconds: cfg.GitCacheTTLSeconds,
			Runner:     gitcache.DefaultRunner,
			Now:        time.Now,
		}
		g, err := c.Query(status.CWD)
		if err != nil {
			debugLog("gitcache.Query: %v", err)
		}
		return g
	})
	ctx.TranscriptProvider = memoize(func() *transcript.Entries {
		if status.TranscriptPath == "" {
			return &transcript.Entries{}
		}
		e, err := transcript.ParseTail(status.TranscriptPath, 64*1024)
		if err != nil {
			debugLog("transcript.ParseTail: %v", err)
			return &transcript.Entries{}
		}
		return e
	})
	ctx.VoiceProvider = memoize(func() *voice.Config {
		return voice.NewReader().Read(status.CWD)
	})
	ctx.CompactionProvider = memoize(func() int {
		if status.SessionID == "" || status.ContextWindow == nil || status.ContextWindow.UsedPercentage == nil {
			return 0
		}
		store := &compaction.Store{Dir: filepath.Join(cacheRoot, "compactions")}
		n, err := store.Track(status.SessionID, *status.ContextWindow.UsedPercentage, status.ContextWindow.ContextWindowSize)
		if err != nil {
			debugLog("compaction.Track: %v", err)
		}
		return n
	})

	registry := buildRegistry()
	width := layout.DetectWidth()

	row1Widgets := resolveRow(cfg.Widgets.Row1, registry)
	row2Widgets := resolveRow(cfg.Widgets.Row2, registry)

	// Two-pass composition: first try at natural width (no truncation) to see
	// if both rows fit on one merged line (row1 left, row2 right). Fall back
	// to two-row mode when the merged form would overflow.
	naturalOpts := layout.Options{Width: 0, DropPriority: dropPriority, Hide: cfg.Widgets.Hide}
	row1Full := layout.ComposeRow(row1Widgets, nil, ctx, naturalOpts)
	row2Full := layout.ComposeRow(row2Widgets, nil, ctx, naturalOpts)
	const mergedGap = 2 // minimum spaces between row1 and row2 when merged
	row1W := render.VisibleWidth(row1Full)
	row2W := render.VisibleWidth(row2Full)

	var row1, row2 string
	merged := ""
	if row1W > 0 && row2W > 0 && row1W+mergedGap+row2W <= width {
		pad := width - row1W - row2W
		if pad < mergedGap {
			pad = mergedGap
		}
		merged = row1Full + strings.Repeat(" ", pad) + row2Full
	} else {
		opts := layout.Options{Width: width, DropPriority: dropPriority, Hide: cfg.Widgets.Hide}
		row1 = layout.ComposeRow(row1Widgets, nil, ctx, opts)
		row2 = layout.ComposeRow(row2Widgets, nil, ctx, opts)
	}

	var activity []string
	if cfg.ActivityRows > 0 {
		actWidgets := []widgets.Widget{widgets.Tools{}, widgets.ToolsRecent{}, widgets.Agents{}, widgets.Todos{}}
		for _, w := range actWidgets {
			if len(activity) >= cfg.ActivityRows {
				break
			}
			text, vis := widgets.SafeRender(w, ctx)
			if !vis {
				continue
			}
			activity = append(activity, text)
		}
	}

	out := strings.Builder{}
	if merged != "" {
		out.WriteString("\x1b[0m")
		out.WriteString(merged)
		if len(activity) > 0 {
			out.WriteString("\n")
		}
	} else {
		if row1 != "" {
			out.WriteString("\x1b[0m")
			out.WriteString(row1)
			out.WriteString("\n")
		}
		if row2 != "" {
			out.WriteString("\x1b[0m")
			out.WriteString(row2)
			if len(activity) > 0 {
				out.WriteString("\n")
			}
		}
	}
	for i, line := range activity {
		out.WriteString("\x1b[0m")
		out.WriteString(line)
		if i < len(activity)-1 {
			out.WriteString("\n")
		}
	}
	_, _ = os.Stdout.WriteString(out.String())
}

func resolveRow(names []string, reg widgets.Registry) []widgets.Widget {
	var out []widgets.Widget
	for _, n := range names {
		if n == layout.FlexName {
			out = append(out, layout.FlexMarker())
			continue
		}
		if w := reg.Lookup(n); w != nil {
			out = append(out, w)
		}
	}
	return out
}

func buildRegistry() widgets.Registry {
	all := []widgets.Widget{
		widgets.Model{}, widgets.CWD{}, widgets.Git{}, widgets.ContextBar{},
		widgets.Cost{}, widgets.Duration{}, widgets.Tokens{},
		widgets.Usage5h{}, widgets.Usage7d{}, widgets.BurnRate{},
		widgets.Effort{}, widgets.Voice{}, widgets.Compaction{}, widgets.PR{},
		widgets.SessionName{},
	}
	r := widgets.Registry{}
	for _, w := range all {
		r[w.Name()] = w
	}
	return r
}

// resolveNow returns time.Now(), or a fixed Unix-seconds time when
// CLAUDE_STATUSLINE_NOW is set (used by golden tests for deterministic output).
func resolveNow() time.Time {
	if s := os.Getenv("CLAUDE_STATUSLINE_NOW"); s != "" {
		var unix int64
		if _, err := fmt.Sscanf(s, "%d", &unix); err == nil && unix > 0 {
			return time.Unix(unix, 0).UTC()
		}
	}
	return time.Now()
}

func memoize[T any](f func() T) func() T {
	var once sync.Once
	var v T
	return func() T {
		once.Do(func() { v = f() })
		return v
	}
}

func userCacheDir() string {
	if d, err := os.UserCacheDir(); err == nil {
		return filepath.Join(d, "claude-statusline")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cache", "claude-statusline")
}

func debugLog(format string, args ...any) {
	if os.Getenv("CLAUDE_STATUSLINE_DEBUG") == "" {
		return
	}
	path := filepath.Join(userCacheDir(), "debug.log")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, time.Now().Format(time.RFC3339)+" "+format+"\n", args...)
}
