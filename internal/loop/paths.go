package loop

import (
	"path/filepath"
	"strings"
)

// LogPath returns the log path for a loop.
func LogPath(dataDir, name, id string) string {
	slug := loopSlug(name)
	if slug == "" {
		slug = id
	}
	return filepath.Join(dataDir, "logs", "loops", slug+".log")
}

// LedgerPath returns the ledger path for a loop.
func LedgerPath(repoPath, name, id string) string {
	slug := loopSlug(name)
	if slug == "" {
		slug = id
	}
	return filepath.Join(repoPath, ".forge", "ledgers", slug+".md")
}

func loopSlug(name string) string {
	trimmed := strings.TrimSpace(strings.ToLower(name))
	if trimmed == "" {
		return ""
	}

	builder := strings.Builder{}
	prevDash := false
	for _, r := range trimmed {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			builder.WriteRune(r)
			prevDash = false
			continue
		}
		if r == ' ' || r == '-' || r == '_' {
			if !prevDash {
				builder.WriteRune('-')
				prevDash = true
			}
		}
	}

	slug := strings.Trim(builder.String(), "-")
	return slug
}
