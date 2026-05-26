package transcript

import "time"

// TokensInWindow returns the sum of input + cache_create + cache_read + output
// tokens for requests whose Timestamp falls within [now-window, now]. Subagent
// activity (rows with a non-empty ParentAgentID) is included.
func TokensInWindow(reqs []Request, now time.Time, window time.Duration) int {
	cutoff := now.Add(-window)
	total := 0
	for _, r := range reqs {
		if r.Timestamp.Before(cutoff) || r.Timestamp.After(now) {
			continue
		}
		total += r.InputTokens + r.CacheCreate + r.CacheRead + r.OutputTokens
	}
	return total
}

// TokensPerSecond returns the average tok/s over the window. Returns 0 if no
// requests fall inside the window.
func TokensPerSecond(reqs []Request, now time.Time, window time.Duration) float64 {
	tokens := TokensInWindow(reqs, now, window)
	if tokens == 0 || window <= 0 {
		return 0
	}
	return float64(tokens) / window.Seconds()
}
