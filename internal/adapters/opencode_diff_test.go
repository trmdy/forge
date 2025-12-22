package adapters

import (
	"testing"
)

func TestParseOpenCodeDiffMetadata(t *testing.T) {
	input := `
 file1.txt | 2 +-
 src/main.go | 10 +++++++---
 2 files changed, 9 insertions(+), 3 deletions(-)
 commit a1b2c3d4e5f6a7b8c9d0
 https://github.com/example/repo/commit/abcdef1234567890
`

	metrics, ok, err := ParseOpenCodeDiffMetadata(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || metrics == nil {
		t.Fatalf("expected diff metadata")
	}
	if metrics.FilesChanged != 2 {
		t.Fatalf("files changed: got %d", metrics.FilesChanged)
	}
	if metrics.Insertions != 9 {
		t.Fatalf("insertions: got %d", metrics.Insertions)
	}
	if metrics.Deletions != 3 {
		t.Fatalf("deletions: got %d", metrics.Deletions)
	}
	if len(metrics.Files) != 2 {
		t.Fatalf("files len: got %d", len(metrics.Files))
	}
	if len(metrics.Commits) != 2 {
		t.Fatalf("commits len: got %d", len(metrics.Commits))
	}
}

func TestParseOpenCodeDiffMetadata_NoMatch(t *testing.T) {
	metrics, ok, err := ParseOpenCodeDiffMetadata("no diff here")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok || metrics != nil {
		t.Fatalf("expected no diff metadata")
	}
}
