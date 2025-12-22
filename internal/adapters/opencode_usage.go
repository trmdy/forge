package adapters

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

var openCodeStatsSplit = regexp.MustCompile(`\s{2,}`)

var errNoUsageMetrics = errors.New("no usage metrics found")

// ParseOpenCodeStats parses `opencode stats` output into usage metrics.
func ParseOpenCodeStats(output string) (*models.UsageMetrics, bool, error) {
	if !strings.Contains(output, "COST & TOKENS") && !strings.Contains(output, "OVERVIEW") {
		return nil, false, nil
	}

	metrics := &models.UsageMetrics{
		Source:    "opencode.stats",
		UpdatedAt: time.Now().UTC(),
	}

	parsed := 0
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "│") || !strings.HasSuffix(line, "│") {
			continue
		}

		line = strings.TrimPrefix(line, "│")
		line = strings.TrimSuffix(line, "│")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := openCodeStatsSplit.Split(line, -1)
		if len(parts) < 2 {
			continue
		}

		label := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[len(parts)-1])
		if label == "" || value == "" {
			continue
		}

		if applyOpenCodeStat(metrics, label, value) {
			parsed++
		}
	}

	if parsed == 0 {
		return nil, false, errNoUsageMetrics
	}

	if metrics.TotalTokens == 0 {
		metrics.TotalTokens = metrics.InputTokens + metrics.OutputTokens + metrics.CacheReadTokens + metrics.CacheWriteTokens
	}

	return metrics, true, nil
}

func applyOpenCodeStat(metrics *models.UsageMetrics, label, value string) bool {
	switch label {
	case "Sessions":
		if v, ok := parseInt(value); ok {
			metrics.Sessions = v
			return true
		}
	case "Messages":
		if v, ok := parseInt(value); ok {
			metrics.Messages = v
			return true
		}
	case "Days":
		if v, ok := parseInt(value); ok {
			metrics.Days = v
			return true
		}
	case "Total Cost":
		if v, ok := parseCostCents(value); ok {
			metrics.TotalCostCents = v
			return true
		}
	case "Avg Cost/Day":
		if v, ok := parseCostCents(value); ok {
			metrics.AvgCostPerDayCents = v
			return true
		}
	case "Avg Tokens/Session":
		if v, ok := parseInt(value); ok {
			metrics.AvgTokensPerSession = v
			return true
		}
	case "Median Tokens/Session":
		if v, ok := parseInt(value); ok {
			metrics.MedianTokensPerSession = v
			return true
		}
	case "Input":
		if v, ok := parseInt(value); ok {
			metrics.InputTokens = v
			return true
		}
	case "Output":
		if v, ok := parseInt(value); ok {
			metrics.OutputTokens = v
			return true
		}
	case "Cache Read":
		if v, ok := parseInt(value); ok {
			metrics.CacheReadTokens = v
			return true
		}
	case "Cache Write":
		if v, ok := parseInt(value); ok {
			metrics.CacheWriteTokens = v
			return true
		}
	}

	return false
}

func parseInt(value string) (int64, bool) {
	clean := strings.ReplaceAll(value, ",", "")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return 0, false
	}
	parsed, err := strconv.ParseInt(clean, 10, 64)
	if err != nil {
		return 0, false
	}
	return parsed, true
}

func parseCostCents(value string) (int64, bool) {
	clean := strings.ReplaceAll(value, ",", "")
	clean = strings.TrimSpace(strings.TrimPrefix(clean, "$"))
	if clean == "" {
		return 0, false
	}
	amount, err := strconv.ParseFloat(clean, 64)
	if err != nil {
		return 0, false
	}
	return int64(amount*100 + 0.5), true
}
