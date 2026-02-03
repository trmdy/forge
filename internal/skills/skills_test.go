package skills

import (
	"path/filepath"
	"testing"

	"github.com/tOgg1/forge/internal/models"
)

func TestResolveHarnessDestPrefersAuthHome(t *testing.T) {
	dest, ok := resolveHarnessDest("/repo", models.HarnessCodex, "/custom")
	if !ok {
		t.Fatalf("expected destination to resolve")
	}
	if dest != "/custom/skills" {
		t.Fatalf("expected auth_home destination, got %q", dest)
	}
}

func TestResolveHarnessDestUsesBaseDir(t *testing.T) {
	dest, ok := resolveHarnessDest("/repo", models.HarnessCodex, "")
	if !ok {
		t.Fatalf("expected destination to resolve")
	}
	if dest != "/repo/.codex/skills" {
		t.Fatalf("expected baseDir destination, got %q", dest)
	}
}

func TestResolveHarnessDestUsesHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dest, ok := resolveHarnessDest("", models.HarnessCodex, "")
	if !ok {
		t.Fatalf("expected destination to resolve")
	}
	expected := filepath.Join(home, ".codex", "skills")
	if dest != expected {
		t.Fatalf("expected %q, got %q", expected, dest)
	}
}
