package widgets

import (
	"fmt"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/render"
)

const costGlyph = " " // nf-fa-dollar

type Cost struct{}

func (Cost) Name() string { return "cost" }

func (Cost) Render(ctx *Context) (string, bool) {
	c := ctx.Status.Cost
	if c == nil || c.TotalCostUSD <= 0 {
		return "", false
	}
	return render.Yellow(fmt.Sprintf("%s$%.2f", costGlyph, c.TotalCostUSD)), true
}
