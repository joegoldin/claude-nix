package render

// ThresholdColor returns the colorizer for a 0–100 percentage: green <70,
// yellow 70–85, red >=85. Universal across context/usage bars.
func ThresholdColor(pct float64) func(string) string {
	switch {
	case pct >= 85:
		return Red
	case pct >= 70:
		return Yellow
	default:
		return Green
	}
}

// DottedRamp is a progressive braille-dot ramp. Index 0 is a thin baseline
// ("track"), index N-1 is a fully filled cell. Cells grow upward as fill
// increases — fine-grained (7 levels) and visually light. Good for slow-
// moving secondary values like rate-limit windows.
var DottedRamp = []rune{'⣀', '⣄', '⣤', '⣦', '⣶', '⣷', '⣿'}

// ShadedRamp is a 4-level shade ramp from light to full block. Cells
// darken as fill increases — visually heavy and easy to read at a glance.
// Good for focal/primary values like the context-window bar.
var ShadedRamp = []rune{'░', '▒', '▓', '█'}

// TriangleRamp is a 2-level (binary) ramp using right-pointing triangles
// — discrete tick-marks ticking forward through a slow window. Fits a
// long-term budget indicator like 7-day account usage.
var TriangleRamp = []rune{'▷', '▶'}

// Bar renders a `width`-cell progress bar for pct (0–100) using the
// dotted ramp. Equivalent to BarWithRamp(pct, width, DottedRamp).
func Bar(pct float64, width int) string {
	return BarWithRamp(pct, width, DottedRamp)
}

// BarWithRamp renders a progress bar using an arbitrary fill ramp. Each
// cell has (len(ramp)-1) partial-fill steps; the effective resolution is
// width × (len(ramp)-1) units.
func BarWithRamp(pct float64, width int, ramp []rune) string {
	if width <= 0 || len(ramp) < 2 {
		return ""
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	levels := len(ramp) - 1
	totalUnits := width * levels
	filledUnits := int(pct/100*float64(totalUnits) + 0.5)
	if filledUnits > totalUnits {
		filledUnits = totalUnits
	}

	out := make([]rune, 0, width)
	remaining := filledUnits
	for i := 0; i < width; i++ {
		step := remaining
		if step > levels {
			step = levels
		}
		if step < 0 {
			step = 0
		}
		out = append(out, ramp[step])
		remaining -= levels
		if remaining < 0 {
			remaining = 0
		}
	}
	return string(out)
}
