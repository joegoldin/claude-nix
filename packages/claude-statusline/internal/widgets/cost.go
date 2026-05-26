package widgets

import (
	"fmt"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
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
	// Claude Max subscribers don't pay for usage inside their plan limits.
	// Only surface cost when rate limits are absent (non-Max user) or when
	// either window has reached 100% (overage territory).
	if !inOverage(ctx.Status.RateLimits) {
		return "", false
	}
	return render.Red(fmt.Sprintf("%s$%.2f", costGlyph, c.TotalCostUSD)), true
}

// inOverage reports whether the user is consuming overage usage. Returns
// true when rate_limits is missing entirely (non-subscriber, billed by usage)
// OR when either the 5-hour or 7-day window has hit 100%.
func inOverage(rl *input.RateLimits) bool {
	if rl == nil {
		return true
	}
	if rl.FiveHour != nil && rl.FiveHour.UsedPercentage >= 100 {
		return true
	}
	if rl.SevenDay != nil && rl.SevenDay.UsedPercentage >= 100 {
		return true
	}
	return false
}

