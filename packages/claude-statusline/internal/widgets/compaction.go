package widgets

import (
	"fmt"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
)

const compactionGlyph = " " // nf-fa-compress

type Compaction struct{}

func (Compaction) Name() string { return "compaction" }

func (Compaction) Render(ctx *Context) (string, bool) {
	n := ctx.Compactions()
	if n <= 0 {
		return "", false
	}
	return render.Dim(fmt.Sprintf("%s%dc", compactionGlyph, n)), true
}
