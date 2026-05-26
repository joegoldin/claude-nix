package widgets

import (
	"regexp"
	"strings"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
)

const modelGlyph = " " // nf-fa-cogs

// Model renders the model name with the trailing "(1M context)" suffix
// stripped and (when present) the current reasoning effort appended inline.
// Rendered effort follows the model name with a single space, matching the
// "Opus 4.7 xhigh" look.
type Model struct{}

func (Model) Name() string { return "model" }

var modelSuffixRE = regexp.MustCompile(`\s*\(1M context\)\s*$`)

func (Model) Render(ctx *Context) (string, bool) {
	name := strings.TrimSpace(ctx.Status.Model.DisplayName)
	if name == "" {
		return "", false
	}
	name = modelSuffixRE.ReplaceAllString(name, "")
	out := modelGlyph + name
	if e := ctx.Status.Effort; e != nil && e.Level != "" {
		out += " " + e.Level
	}
	return render.Cyan(out), true
}
