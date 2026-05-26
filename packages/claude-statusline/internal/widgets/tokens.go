package widgets

import (
	"fmt"
	"strings"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
)

const tokensGlyph = " " // nf-fa-hashtag

// Tokens renders the current input-token count in context. Matches the
// number Claude Code's --verbose mode shows in the corner, so users can
// turn the built-in counter off.
type Tokens struct{}

func (Tokens) Name() string { return "tokens" }

func (Tokens) Render(ctx *Context) (string, bool) {
	cw := ctx.Status.ContextWindow
	if cw == nil {
		return "", false
	}
	total := cw.TotalInputTokens
	if total <= 0 && cw.CurrentUsage != nil {
		total = cw.CurrentUsage.InputTokens +
			cw.CurrentUsage.CacheCreationInputTokens +
			cw.CurrentUsage.CacheReadInputTokens
	}
	if total <= 0 {
		return "", false
	}
	var formatted string
	if strings.EqualFold(ctx.Cfg.TokenFormat, "compact") {
		formatted = formatTokens(total)
	} else {
		formatted = fmt.Sprintf("%d", total)
	}
	return render.Dim(tokensGlyph + formatted + " tokens"), true
}
