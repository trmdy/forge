package hooks

import (
	"path/filepath"
	"testing"
)

func TestStoreAddAndList(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "hooks.json"))

	created, err := store.Add(Hook{
		Kind:    KindCommand,
		Command: "echo hi",
		Enabled: true,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("Add: expected ID to be set")
	}

	listed, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("List: expected 1 hook, got %d", len(listed))
	}
	if listed[0].ID != created.ID {
		t.Fatalf("List: expected ID %s, got %s", created.ID, listed[0].ID)
	}
}

func TestStoreListEmpty(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(filepath.Join(dir, "hooks.json"))

	listed, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("List: expected empty, got %d", len(listed))
	}
}
