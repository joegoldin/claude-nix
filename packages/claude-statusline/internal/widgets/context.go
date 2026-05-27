package widgets

import (
	"time"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/config"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/gitcache"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/transcript"
	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/voice"
)

// Context bundles everything a widget might need. Heavy fields (transcript,
// git) are computed lazily via the Provider funcs so unused widgets never
// pay the cost.
type Context struct {
	Status input.Status
	Cfg    config.Config
	Now    time.Time
	// Width is the detected terminal width in cells, for widgets that size
	// their content to the line (e.g. the running-tools row). Zero means
	// unknown; width-aware widgets fall back to a sensible default.
	Width int

	GitProvider        func() *gitcache.Git
	TranscriptProvider func() *transcript.Entries
	VoiceProvider      func() *voice.Config
	CompactionProvider func() int
}

func (c *Context) Git() *gitcache.Git {
	if c.GitProvider == nil {
		return nil
	}
	return c.GitProvider()
}

func (c *Context) Transcript() *transcript.Entries {
	if c.TranscriptProvider == nil {
		return nil
	}
	return c.TranscriptProvider()
}

func (c *Context) Voice() *voice.Config {
	if c.VoiceProvider == nil {
		return nil
	}
	return c.VoiceProvider()
}

func (c *Context) Compactions() int {
	if c.CompactionProvider == nil {
		return 0
	}
	return c.CompactionProvider()
}
