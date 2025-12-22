package adapters

import (
	"testing"

	"github.com/opencode-ai/swarm/internal/models"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("expected non-nil registry")
	}

	if len(r.List()) != 0 {
		t.Error("expected empty registry")
	}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	adapter := NewGenericAdapter("test", "test-cmd")
	err := r.Register(adapter)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(r.List()) != 1 {
		t.Errorf("expected 1 adapter, got %d", len(r.List()))
	}
}

func TestRegistry_RegisterDuplicate(t *testing.T) {
	r := NewRegistry()

	adapter1 := NewGenericAdapter("test", "test-cmd")
	adapter2 := NewGenericAdapter("test", "test-cmd-2")

	if err := r.Register(adapter1); err != nil {
		t.Fatalf("first register failed: %v", err)
	}

	err := r.Register(adapter2)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistry_MustRegister(t *testing.T) {
	r := NewRegistry()

	adapter := NewGenericAdapter("test", "test-cmd")

	// Should not panic
	r.MustRegister(adapter)

	if len(r.List()) != 1 {
		t.Errorf("expected 1 adapter, got %d", len(r.List()))
	}
}

func TestRegistry_MustRegisterPanic(t *testing.T) {
	r := NewRegistry()

	adapter := NewGenericAdapter("test", "test-cmd")
	r.MustRegister(adapter)

	// Should panic on duplicate
	defer func() {
		if recover() == nil {
			t.Error("expected panic for duplicate MustRegister")
		}
	}()

	r.MustRegister(adapter)
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()

	adapter := NewGenericAdapter("test", "test-cmd")
	r.MustRegister(adapter)

	got := r.Get("test")
	if got == nil {
		t.Fatal("expected adapter, got nil")
	}

	if got.Name() != "test" {
		t.Errorf("expected name 'test', got %q", got.Name())
	}
}

func TestRegistry_GetNotFound(t *testing.T) {
	r := NewRegistry()

	got := r.Get("nonexistent")
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestRegistry_GetByAgentType(t *testing.T) {
	r := NewRegistry()

	adapter := NewGenericAdapter(string(models.AgentTypeOpenCode), "opencode")
	r.MustRegister(adapter)

	got := r.GetByAgentType(models.AgentTypeOpenCode)
	if got == nil {
		t.Fatal("expected adapter, got nil")
	}

	if got.Name() != string(models.AgentTypeOpenCode) {
		t.Errorf("expected name %q, got %q", models.AgentTypeOpenCode, got.Name())
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	r.MustRegister(NewGenericAdapter("a", "a-cmd"))
	r.MustRegister(NewGenericAdapter("b", "b-cmd"))
	r.MustRegister(NewGenericAdapter("c", "c-cmd"))

	list := r.List()
	if len(list) != 3 {
		t.Errorf("expected 3 adapters, got %d", len(list))
	}
}

func TestRegistry_Names(t *testing.T) {
	r := NewRegistry()

	r.MustRegister(NewGenericAdapter("alpha", "alpha-cmd"))
	r.MustRegister(NewGenericAdapter("beta", "beta-cmd"))

	names := r.Names()
	if len(names) != 2 {
		t.Errorf("expected 2 names, got %d", len(names))
	}

	// Check both names are present
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}

	if !found["alpha"] {
		t.Error("expected 'alpha' in names")
	}
	if !found["beta"] {
		t.Error("expected 'beta' in names")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()

	r.MustRegister(NewGenericAdapter("test", "test-cmd"))

	removed := r.Unregister("test")
	if !removed {
		t.Error("expected true for successful unregister")
	}

	if len(r.List()) != 0 {
		t.Error("expected empty registry after unregister")
	}
}

func TestRegistry_UnregisterNotFound(t *testing.T) {
	r := NewRegistry()

	removed := r.Unregister("nonexistent")
	if removed {
		t.Error("expected false for nonexistent unregister")
	}
}

func TestDefaultRegistry_BuiltinAdapters(t *testing.T) {
	// The default registry should have built-in adapters from init()
	names := DefaultRegistry.Names()

	expectedAdapters := []string{
		string(models.AgentTypeOpenCode),
		string(models.AgentTypeClaudeCode),
		string(models.AgentTypeCodex),
		string(models.AgentTypeGemini),
		string(models.AgentTypeGeneric),
	}

	for _, expected := range expectedAdapters {
		adapter := DefaultRegistry.Get(expected)
		if adapter == nil {
			t.Errorf("expected built-in adapter %q, but not found", expected)
		}
	}

	if len(names) < len(expectedAdapters) {
		t.Errorf("expected at least %d adapters, got %d", len(expectedAdapters), len(names))
	}
}

func TestGlobalFunctions(t *testing.T) {
	// Test that global functions work with DefaultRegistry
	adapter := Get(string(models.AgentTypeOpenCode))
	if adapter == nil {
		t.Error("Get() should return opencode adapter")
	}

	adapter = GetByAgentType(models.AgentTypeClaudeCode)
	if adapter == nil {
		t.Error("GetByAgentType() should return claude-code adapter")
	}

	names := Names()
	if len(names) == 0 {
		t.Error("Names() should return non-empty list")
	}

	list := List()
	if len(list) == 0 {
		t.Error("List() should return non-empty list")
	}
}
