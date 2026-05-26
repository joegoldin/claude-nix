package widgets

import (
	"strings"
	"testing"

	"github.com/joegoldin/claude-nix/packages/claude-statusline/internal/input"
)

func TestDurationFormatsSeconds(t *testing.T) {
	tests := []struct {
		ms   int64
		want string
	}{
		{45_000, "45s"},
		{90_000, "1m30s"},
		{60_000, "1m0s"},
		{3_660_000, "1h1m"},
		{0, ""},
	}
	for _, tc := range tests {
		w := &Duration{}
		ctx := &Context{Status: input.Status{Cost: &input.Cost{TotalDurationMS: tc.ms}}}
		out, vis := w.Render(ctx)
		if tc.want == "" {
			if vis {
				t.Errorf("ms=%d: expected hidden", tc.ms)
			}
			continue
		}
		if !vis {
			t.Errorf("ms=%d: expected visible", tc.ms)
			continue
		}
		if !strings.Contains(out, tc.want) {
			t.Errorf("ms=%d: out=%q want %q", tc.ms, out, tc.want)
		}
	}
}
