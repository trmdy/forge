// Package components provides reusable TUI components.
package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
	"github.com/opencode-ai/swarm/internal/tui/styles"
)

// UsagePanelData contains data for rendering usage information.
type UsagePanelData struct {
	Metrics   *models.UsageMetrics
	AgentName string
	UpdatedAt time.Time
}

// RenderUsagePanel renders a panel displaying usage metrics.
func RenderUsagePanel(styleSet styles.Styles, data UsagePanelData, width int) string {
	if data.Metrics == nil {
		lines := []string{
			styleSet.Muted.Render("No usage data available."),
			styleSet.Muted.Render("Usage is collected from adapter output."),
		}
		return renderUsagePanelContainer(styleSet, "Usage", lines, width)
	}

	lines := renderUsageMetricsLines(styleSet, data.Metrics)

	// Add update timestamp
	if !data.UpdatedAt.IsZero() {
		ago := time.Since(data.UpdatedAt)
		timestamp := formatUsageTimestamp(ago)
		lines = append(lines, "", styleSet.Muted.Render(fmt.Sprintf("Updated: %s", timestamp)))
	}

	return renderUsagePanelContainer(styleSet, "Usage", lines, width)
}

// RenderCompactUsage renders a compact single-line usage summary.
// Format: "Tokens: 1.2K | Cost: $0.15"
func RenderCompactUsage(styleSet styles.Styles, metrics *models.UsageMetrics) string {
	if metrics == nil {
		return styleSet.Muted.Render("Usage: --")
	}

	var parts []string

	// Token count
	if metrics.TotalTokens > 0 {
		parts = append(parts, fmt.Sprintf("Tokens: %s", formatTokenCount(metrics.TotalTokens)))
	}

	// Cost
	if metrics.TotalCostCents > 0 {
		parts = append(parts, fmt.Sprintf("Cost: %s", formatCostCents(metrics.TotalCostCents)))
	}

	if len(parts) == 0 {
		return styleSet.Muted.Render("Usage: --")
	}

	return styleSet.Text.Render("Usage: " + strings.Join(parts, " | "))
}

// RenderUsageBar renders a visual bar showing token breakdown.
// Format: "▓▓▓▓░░░░░░ In:60% Out:30% Cache:10%"
func RenderUsageBar(styleSet styles.Styles, metrics *models.UsageMetrics, width int) string {
	if metrics == nil || metrics.TotalTokens == 0 {
		return ""
	}

	total := metrics.TotalTokens
	inPct := int(metrics.InputTokens * 100 / total)
	outPct := int(metrics.OutputTokens * 100 / total)
	cachePct := int((metrics.CacheReadTokens + metrics.CacheWriteTokens) * 100 / total)

	// Normalize percentages
	if inPct+outPct+cachePct > 100 {
		scale := 100.0 / float64(inPct+outPct+cachePct)
		inPct = int(float64(inPct) * scale)
		outPct = int(float64(outPct) * scale)
		cachePct = 100 - inPct - outPct
	}

	// Build bar (use 20 chars for bar)
	barWidth := 20
	if width > 0 && width < 40 {
		barWidth = 10
	}

	inChars := inPct * barWidth / 100
	outChars := outPct * barWidth / 100
	cacheChars := barWidth - inChars - outChars

	bar := strings.Repeat("▓", inChars) +
		strings.Repeat("▒", outChars) +
		strings.Repeat("░", cacheChars)

	label := fmt.Sprintf(" In:%d%% Out:%d%% Cache:%d%%", inPct, outPct, cachePct)

	return styleSet.Text.Render(bar) + styleSet.Muted.Render(label)
}

func renderUsageMetricsLines(styleSet styles.Styles, m *models.UsageMetrics) []string {
	lines := []string{}

	// Cost section
	if m.TotalCostCents > 0 || m.AvgCostPerDayCents > 0 {
		lines = append(lines, styleSet.Accent.Render("Cost"))
		if m.TotalCostCents > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Total", formatCostCents(m.TotalCostCents)))
		}
		if m.AvgCostPerDayCents > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Avg/Day", formatCostCents(m.AvgCostPerDayCents)))
		}
		lines = append(lines, "")
	}

	// Token section
	if m.TotalTokens > 0 || m.InputTokens > 0 || m.OutputTokens > 0 {
		lines = append(lines, styleSet.Accent.Render("Tokens"))
		if m.TotalTokens > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Total", formatTokenCount(m.TotalTokens)))
		}
		if m.InputTokens > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Input", formatTokenCount(m.InputTokens)))
		}
		if m.OutputTokens > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Output", formatTokenCount(m.OutputTokens)))
		}
		if m.CacheReadTokens > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Cache Read", formatTokenCount(m.CacheReadTokens)))
		}
		if m.CacheWriteTokens > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Cache Write", formatTokenCount(m.CacheWriteTokens)))
		}
		lines = append(lines, "")
	}

	// Session stats
	if m.Sessions > 0 || m.Messages > 0 {
		lines = append(lines, styleSet.Accent.Render("Activity"))
		if m.Sessions > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Sessions", fmt.Sprintf("%d", m.Sessions)))
		}
		if m.Messages > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Messages", fmt.Sprintf("%d", m.Messages)))
		}
		if m.Days > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Days", fmt.Sprintf("%d", m.Days)))
		}
		if m.AvgTokensPerSession > 0 {
			lines = append(lines, renderMetricLine(styleSet, "Avg Tokens/Session", formatTokenCount(m.AvgTokensPerSession)))
		}
	}

	// Source info
	if m.Source != "" {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, styleSet.Muted.Render(fmt.Sprintf("Source: %s", m.Source)))
	}

	return lines
}

func renderMetricLine(styleSet styles.Styles, label, value string) string {
	return fmt.Sprintf("  %s: %s", styleSet.Muted.Render(label), styleSet.Text.Render(value))
}

func renderUsagePanelContainer(styleSet styles.Styles, title string, lines []string, width int) string {
	header := styleSet.Accent.Render(title)
	content := strings.Join(append([]string{header}, lines...), "\n")
	return styleSet.Panel.Copy().Width(width).Padding(0, 1).Render(content)
}

// formatTokenCount formats a token count with K/M suffixes.
func formatTokenCount(count int64) string {
	if count < 1000 {
		return fmt.Sprintf("%d", count)
	}
	if count < 1_000_000 {
		return fmt.Sprintf("%.1fK", float64(count)/1000)
	}
	return fmt.Sprintf("%.2fM", float64(count)/1_000_000)
}

// formatCostCents formats a cost in cents as dollars.
func formatCostCents(cents int64) string {
	dollars := float64(cents) / 100
	if dollars < 1 {
		return fmt.Sprintf("$%.2f", dollars)
	}
	if dollars < 10 {
		return fmt.Sprintf("$%.2f", dollars)
	}
	if dollars < 100 {
		return fmt.Sprintf("$%.1f", dollars)
	}
	return fmt.Sprintf("$%.0f", dollars)
}

// formatUsageTimestamp formats a duration as a human-readable timestamp.
func formatUsageTimestamp(d time.Duration) string {
	if d < time.Minute {
		return "just now"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// RenderUsageSummaryLine renders a single-line usage summary for card display.
func RenderUsageSummaryLine(styleSet styles.Styles, metrics *models.UsageMetrics) string {
	if metrics == nil {
		return ""
	}

	var parts []string

	if metrics.TotalCostCents > 0 {
		parts = append(parts, formatCostCents(metrics.TotalCostCents))
	}

	if metrics.TotalTokens > 0 {
		parts = append(parts, formatTokenCount(metrics.TotalTokens)+" tokens")
	}

	if len(parts) == 0 {
		return ""
	}

	label := styleSet.Muted.Render("Usage:")
	value := styleSet.Text.Render(strings.Join(parts, " | "))
	return fmt.Sprintf("%s %s", label, value)
}
