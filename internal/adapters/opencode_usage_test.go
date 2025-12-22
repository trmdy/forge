package adapters

import (
	"strings"
	"testing"
)

func TestParseOpenCodeStats(t *testing.T) {
	stats := strings.TrimSpace(`
┌────────────────────────────────────────────────────────┐
│                       OVERVIEW                         │
├────────────────────────────────────────────────────────┤
│Sessions                                              3 │
│Messages                                             12 │
│Days                                                  1 │
└────────────────────────────────────────────────────────┘

┌────────────────────────────────────────────────────────┐
│                    COST & TOKENS                       │
├────────────────────────────────────────────────────────┤
│Total Cost                                        $1.23 │
│Avg Cost/Day                                      $0.61 │
│Avg Tokens/Session                                 456 │
│Median Tokens/Session                              400 │
│Input                                             1200 │
│Output                                            2400 │
│Cache Read                                         300 │
│Cache Write                                        100 │
└────────────────────────────────────────────────────────┘
`)

	metrics, ok, err := ParseOpenCodeStats(stats)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || metrics == nil {
		t.Fatalf("expected metrics to parse")
	}

	if metrics.Sessions != 3 {
		t.Fatalf("sessions: got %d", metrics.Sessions)
	}
	if metrics.Messages != 12 {
		t.Fatalf("messages: got %d", metrics.Messages)
	}
	if metrics.Days != 1 {
		t.Fatalf("days: got %d", metrics.Days)
	}
	if metrics.TotalCostCents != 123 {
		t.Fatalf("total cost: got %d", metrics.TotalCostCents)
	}
	if metrics.AvgCostPerDayCents != 61 {
		t.Fatalf("avg cost/day: got %d", metrics.AvgCostPerDayCents)
	}
	if metrics.AvgTokensPerSession != 456 {
		t.Fatalf("avg tokens/session: got %d", metrics.AvgTokensPerSession)
	}
	if metrics.MedianTokensPerSession != 400 {
		t.Fatalf("median tokens/session: got %d", metrics.MedianTokensPerSession)
	}
	if metrics.InputTokens != 1200 {
		t.Fatalf("input tokens: got %d", metrics.InputTokens)
	}
	if metrics.OutputTokens != 2400 {
		t.Fatalf("output tokens: got %d", metrics.OutputTokens)
	}
	if metrics.CacheReadTokens != 300 {
		t.Fatalf("cache read tokens: got %d", metrics.CacheReadTokens)
	}
	if metrics.CacheWriteTokens != 100 {
		t.Fatalf("cache write tokens: got %d", metrics.CacheWriteTokens)
	}
	if metrics.TotalTokens != 4000 {
		t.Fatalf("total tokens: got %d", metrics.TotalTokens)
	}
}

func TestParseOpenCodeStats_NoMatch(t *testing.T) {
	metrics, ok, err := ParseOpenCodeStats("random output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok || metrics != nil {
		t.Fatalf("expected no metrics")
	}
}
