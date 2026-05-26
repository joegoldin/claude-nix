package transcript

import (
	"testing"
	"time"
)

func TestTokensInWindow(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	reqs := []Request{
		{Timestamp: now.Add(-120 * time.Second), InputTokens: 1000, CacheRead: 0, OutputTokens: 200},
		{Timestamp: now.Add(-30 * time.Second), InputTokens: 500, CacheRead: 1500, OutputTokens: 100},
		{Timestamp: now.Add(-5 * time.Second), InputTokens: 300, CacheRead: 700, OutputTokens: 50},
	}
	tokens := TokensInWindow(reqs, now, 60*time.Second)
	if tokens != 3150 {
		t.Errorf("tokens = %d, want 3150", tokens)
	}
}

func TestTokensPerSecond(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	reqs := []Request{
		{Timestamp: now.Add(-30 * time.Second), InputTokens: 1000, OutputTokens: 200},
		{Timestamp: now.Add(-10 * time.Second), InputTokens: 500, OutputTokens: 100},
	}
	tps := TokensPerSecond(reqs, now, 60*time.Second)
	if tps != 30.0 {
		t.Errorf("tps = %f, want 30.0", tps)
	}
}

func TestTokensPerSecondNoData(t *testing.T) {
	if tps := TokensPerSecond(nil, time.Now(), 60*time.Second); tps != 0 {
		t.Errorf("tps with no data = %f, want 0", tps)
	}
}
