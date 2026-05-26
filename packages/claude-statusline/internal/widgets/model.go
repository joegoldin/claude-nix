package widgets

import (
	"regexp"
	"strings"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
)

const modelGlyph = " " // nf-fa-cogs

// Model renders the model name with the trailing "(1M context)" stripped.
type Model struct{}

func (Model) Name() string { return "model" }

var modelSuffixRE = regexp.MustCompile(`\s*\(1M context\)\s*$`)

func (Model) Render(ctx *Context) (string, bool) {
	name := strings.TrimSpace(ctx.Status.Model.DisplayName)
	if name == "" {
		return "", false
	}
	name = modelSuffixRE.ReplaceAllString(name, "")
	return render.Cyan(modelGlyph + name), true
}
