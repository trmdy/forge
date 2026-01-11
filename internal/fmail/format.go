package fmail

import (
	"fmt"
	"time"
)

const activeWindow = time.Minute

func isActive(now, t time.Time) bool {
	if t.IsZero() {
		return false
	}
	return now.Sub(t) <= activeWindow
}

func formatLastSeen(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	if isActive(now, t) {
		return "active"
	}
	return formatRelative(now, t)
}

func formatRelative(now, t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	if t.After(now) {
		t = now
	}
	diff := now.Sub(t)
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	}
}
