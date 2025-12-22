package adapters

import (
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

var (
	openCodeDiffSummary = regexp.MustCompile(`(?i)(\d+)\s+files?\s+changed(?:,\s+(\d+)\s+insertions?\(\+\))?(?:,\s+(\d+)\s+deletions?\(-\))?`)
	openCodeCommitRef   = regexp.MustCompile(`(?i)\bcommit\s+([0-9a-f]{7,40})\b`)
	openCodeCommitURL   = regexp.MustCompile(`(?i)/commit/([0-9a-f]{7,40})\b`)
	openCodeDiffStat    = regexp.MustCompile(`^(.+?)\s+\|\s+\d+`)
)

// ParseOpenCodeDiffMetadata extracts diff metadata from OpenCode output.
func ParseOpenCodeDiffMetadata(output string) (*models.DiffMetadata, bool, error) {
	lines := strings.Split(output, "\n")
	metrics := &models.DiffMetadata{
		Source:    "opencode.screen",
		UpdatedAt: time.Now().UTC(),
	}

	files := map[string]struct{}{}
	commits := map[string]struct{}{}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if summary := openCodeDiffSummary.FindStringSubmatch(line); len(summary) > 0 {
			if v, ok := parseInt(summary[1]); ok {
				metrics.FilesChanged = v
			}
			if len(summary) > 2 {
				if v, ok := parseInt(summary[2]); ok {
					metrics.Insertions = v
				}
			}
			if len(summary) > 3 {
				if v, ok := parseInt(summary[3]); ok {
					metrics.Deletions = v
				}
			}
		}

		if match := openCodeDiffStat.FindStringSubmatch(line); len(match) > 1 {
			path := strings.TrimSpace(match[1])
			if path != "" {
				files[path] = struct{}{}
			}
		}

		for _, commitMatch := range openCodeCommitRef.FindAllStringSubmatch(line, -1) {
			if len(commitMatch) > 1 {
				commits[strings.ToLower(commitMatch[1])] = struct{}{}
			}
		}
		for _, commitMatch := range openCodeCommitURL.FindAllStringSubmatch(line, -1) {
			if len(commitMatch) > 1 {
				commits[strings.ToLower(commitMatch[1])] = struct{}{}
			}
		}
	}

	for file := range files {
		metrics.Files = append(metrics.Files, file)
	}
	for commit := range commits {
		metrics.Commits = append(metrics.Commits, commit)
	}

	if len(metrics.Files) > 0 {
		sort.Strings(metrics.Files)
		if metrics.FilesChanged == 0 {
			metrics.FilesChanged = int64(len(metrics.Files))
		}
	}
	if len(metrics.Commits) > 0 {
		sort.Strings(metrics.Commits)
	}

	if metrics.FilesChanged == 0 && metrics.Insertions == 0 && metrics.Deletions == 0 && len(metrics.Files) == 0 && len(metrics.Commits) == 0 {
		return nil, false, nil
	}

	return metrics, true, nil
}
