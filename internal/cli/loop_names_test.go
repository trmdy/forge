package cli

import (
	"strings"
	"testing"
)

func TestGenerateLoopNameDoesNotMutateExisting(t *testing.T) {
	existing := map[string]struct{}{
		"slick-nelson": {},
	}
	before := len(existing)

	name := generateLoopName(existing)
	if name == "" {
		t.Fatal("expected generated name")
	}
	if name == "slick-nelson" {
		t.Fatal("expected generated name to avoid existing values")
	}
	if !strings.Contains(name, "-") {
		t.Fatalf("expected kebab-case name, got %q", name)
	}
	if len(existing) != before {
		t.Fatalf("expected existing names map to remain unchanged, got %d want %d", len(existing), before)
	}
}
