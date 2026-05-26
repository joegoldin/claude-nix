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

// Bar renders a `width`-cell progress bar for pct (0–100) using █ filled / ░ empty.
func Bar(pct float64, width int) string {
	if width <= 0 {
		return ""
	}
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	filled := int(pct*float64(width)/100 + 0.5)
	if filled > width {
		filled = width
	}
	out := make([]rune, 0, width)
	for i := 0; i < filled; i++ {
		out = append(out, '█')
	}
	for i := filled; i < width; i++ {
		out = append(out, '░')
	}
	return string(out)
}
