package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVault_InitializeAndUnlock(t *testing.T) {
	dir := t.TempDir()
	v := New(dir)

	// Should not be initialized
	if v.IsInitialized() {
		t.Error("vault should not be initialized")
	}

	// Initialize
	if err := v.Initialize("testpassword"); err != nil {
		t.Fatalf("failed to initialize vault: %v", err)
	}

	// Should now be initialized
	if !v.IsInitialized() {
		t.Error("vault should be initialized")
	}

	// Should be unlocked after init
	if !v.IsUnlocked() {
		t.Error("vault should be unlocked after initialization")
	}

	// Lock it
	if err := v.Lock(); err != nil {
		t.Fatalf("failed to lock vault: %v", err)
	}

	if v.IsUnlocked() {
		t.Error("vault should be locked")
	}

	// Unlock with correct password
	if err := v.Unlock("testpassword"); err != nil {
		t.Fatalf("failed to unlock vault: %v", err)
	}

	if !v.IsUnlocked() {
		t.Error("vault should be unlocked")
	}
}

func TestVault_UnlockWrongPassword(t *testing.T) {
	dir := t.TempDir()
	v := New(dir)

	if err := v.Initialize("correctpassword"); err != nil {
		t.Fatalf("failed to initialize vault: %v", err)
	}

	// Store a secret to create vault file
	if err := v.Store("test", "value", nil); err != nil {
		t.Fatalf("failed to store secret: %v", err)
	}

	if err := v.Lock(); err != nil {
		t.Fatalf("failed to lock vault: %v", err)
	}

	// Try wrong password
	if err := v.Unlock("wrongpassword"); err != ErrInvalidPassword {
		t.Errorf("expected ErrInvalidPassword, got %v", err)
	}

	if v.IsUnlocked() {
		t.Error("vault should not be unlocked with wrong password")
	}
}

func TestVault_StoreRetrieveDelete(t *testing.T) {
	dir := t.TempDir()
	v := New(dir)

	if err := v.Initialize("testpassword"); err != nil {
		t.Fatalf("failed to initialize vault: %v", err)
	}

	// Store secret
	metadata := map[string]string{"provider": "anthropic"}
	if err := v.Store("api-key", "sk-ant-test123", metadata); err != nil {
		t.Fatalf("failed to store secret: %v", err)
	}

	// Retrieve secret
	secret, err := v.Retrieve("api-key")
	if err != nil {
		t.Fatalf("failed to retrieve secret: %v", err)
	}

	if secret.Name != "api-key" {
		t.Errorf("expected name 'api-key', got %q", secret.Name)
	}
	if secret.Value != "sk-ant-test123" {
		t.Errorf("expected value 'sk-ant-test123', got %q", secret.Value)
	}
	if secret.Metadata["provider"] != "anthropic" {
		t.Errorf("expected provider 'anthropic', got %q", secret.Metadata["provider"])
	}

	// Delete secret
	if err := v.Delete("api-key"); err != nil {
		t.Fatalf("failed to delete secret: %v", err)
	}

	// Should not be retrievable
	_, err = v.Retrieve("api-key")
	if err != ErrSecretNotFound {
		t.Errorf("expected ErrSecretNotFound, got %v", err)
	}
}

func TestVault_List(t *testing.T) {
	dir := t.TempDir()
	v := New(dir)

	if err := v.Initialize("testpassword"); err != nil {
		t.Fatalf("failed to initialize vault: %v", err)
	}

	// Store multiple secrets
	secrets := []string{"key1", "key2", "key3"}
	for _, name := range secrets {
		if err := v.Store(name, "value-"+name, nil); err != nil {
			t.Fatalf("failed to store secret %s: %v", name, err)
		}
	}

	// List secrets
	names, err := v.List()
	if err != nil {
		t.Fatalf("failed to list secrets: %v", err)
	}

	if len(names) != len(secrets) {
		t.Errorf("expected %d secrets, got %d", len(secrets), len(names))
	}

	// Check all secrets are present
	nameSet := make(map[string]bool)
	for _, name := range names {
		nameSet[name] = true
	}
	for _, expected := range secrets {
		if !nameSet[expected] {
			t.Errorf("missing secret %q in list", expected)
		}
	}
}

func TestVault_Persistence(t *testing.T) {
	dir := t.TempDir()

	// Create and populate vault
	v1 := New(dir)
	if err := v1.Initialize("testpassword"); err != nil {
		t.Fatalf("failed to initialize vault: %v", err)
	}

	if err := v1.Store("persistent-key", "persistent-value", nil); err != nil {
		t.Fatalf("failed to store secret: %v", err)
	}

	if err := v1.Lock(); err != nil {
		t.Fatalf("failed to lock vault: %v", err)
	}

	// Open with new vault instance
	v2 := New(dir)
	if err := v2.Unlock("testpassword"); err != nil {
		t.Fatalf("failed to unlock vault: %v", err)
	}

	// Retrieve secret
	secret, err := v2.Retrieve("persistent-key")
	if err != nil {
		t.Fatalf("failed to retrieve secret: %v", err)
	}

	if secret.Value != "persistent-value" {
		t.Errorf("expected value 'persistent-value', got %q", secret.Value)
	}
}

func TestVault_LockedOperations(t *testing.T) {
	dir := t.TempDir()
	v := New(dir)

	if err := v.Initialize("testpassword"); err != nil {
		t.Fatalf("failed to initialize vault: %v", err)
	}

	if err := v.Lock(); err != nil {
		t.Fatalf("failed to lock vault: %v", err)
	}

	// All operations should fail when locked
	if err := v.Store("key", "value", nil); err != ErrVaultLocked {
		t.Errorf("Store: expected ErrVaultLocked, got %v", err)
	}

	if _, err := v.Retrieve("key"); err != ErrVaultLocked {
		t.Errorf("Retrieve: expected ErrVaultLocked, got %v", err)
	}

	if err := v.Delete("key"); err != ErrVaultLocked {
		t.Errorf("Delete: expected ErrVaultLocked, got %v", err)
	}

	if _, err := v.List(); err != ErrVaultLocked {
		t.Errorf("List: expected ErrVaultLocked, got %v", err)
	}
}

func TestVault_UpdateSecret(t *testing.T) {
	dir := t.TempDir()
	v := New(dir)

	if err := v.Initialize("testpassword"); err != nil {
		t.Fatalf("failed to initialize vault: %v", err)
	}

	// Store initial secret
	if err := v.Store("key", "value1", nil); err != nil {
		t.Fatalf("failed to store secret: %v", err)
	}

	secret1, _ := v.Retrieve("key")
	createdAt := secret1.CreatedAt

	// Update secret
	if err := v.Store("key", "value2", nil); err != nil {
		t.Fatalf("failed to update secret: %v", err)
	}

	secret2, _ := v.Retrieve("key")

	if secret2.Value != "value2" {
		t.Errorf("expected value 'value2', got %q", secret2.Value)
	}

	// CreatedAt should be preserved
	if !secret2.CreatedAt.Equal(createdAt) {
		t.Error("CreatedAt should be preserved on update")
	}

	// UpdatedAt should be updated
	if !secret2.UpdatedAt.After(secret2.CreatedAt) {
		t.Error("UpdatedAt should be after CreatedAt on update")
	}
}

func TestVault_ChangePassword(t *testing.T) {
	dir := t.TempDir()
	v := New(dir)

	if err := v.Initialize("oldpassword"); err != nil {
		t.Fatalf("failed to initialize vault: %v", err)
	}

	if err := v.Store("key", "value", nil); err != nil {
		t.Fatalf("failed to store secret: %v", err)
	}

	// Change password
	if err := v.ChangePassword("oldpassword", "newpassword"); err != nil {
		t.Fatalf("failed to change password: %v", err)
	}

	if err := v.Lock(); err != nil {
		t.Fatalf("failed to lock vault: %v", err)
	}

	// Old password should fail
	if err := v.Unlock("oldpassword"); err != ErrInvalidPassword {
		t.Errorf("expected ErrInvalidPassword with old password, got %v", err)
	}

	// New password should work
	v2 := New(dir)
	if err := v2.Unlock("newpassword"); err != nil {
		t.Fatalf("failed to unlock with new password: %v", err)
	}

	secret, err := v2.Retrieve("key")
	if err != nil {
		t.Fatalf("failed to retrieve secret: %v", err)
	}

	if secret.Value != "value" {
		t.Errorf("expected value 'value', got %q", secret.Value)
	}
}

func TestVault_ResolveCredential(t *testing.T) {
	dir := t.TempDir()
	v := New(dir)

	if err := v.Initialize("testpassword"); err != nil {
		t.Fatalf("failed to initialize vault: %v", err)
	}

	if err := v.Store("my-api-key", "secret-value", nil); err != nil {
		t.Fatalf("failed to store secret: %v", err)
	}

	// Set env var for testing
	os.Setenv("TEST_API_KEY", "env-value")
	defer os.Unsetenv("TEST_API_KEY")

	tests := []struct {
		ref      string
		expected string
		wantErr  bool
	}{
		{"vault:my-api-key", "secret-value", false},
		{"env:TEST_API_KEY", "env-value", false},
		{"literal-value", "literal-value", false},
		{"vault:nonexistent", "", true},
		{"env:NONEXISTENT_VAR", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			value, err := v.ResolveCredential(tt.ref)
			if tt.wantErr && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantErr && value != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, value)
			}
		})
	}
}

func TestDefaultPath(t *testing.T) {
	path := DefaultPath()
	if path == "" {
		t.Error("DefaultPath returned empty string")
	}
	if !filepath.IsAbs(path) {
		t.Error("DefaultPath should return absolute path")
	}
}

func TestGenerateSecretName(t *testing.T) {
	name1 := GenerateSecretName("api-key")
	name2 := GenerateSecretName("api-key")

	if name1 == name2 {
		t.Error("GenerateSecretName should return unique names")
	}

	if len(name1) < len("api-key-") {
		t.Errorf("generated name too short: %s", name1)
	}
}

func TestEncryptDecrypt(t *testing.T) {
	key := &[32]byte{}
	for i := range key {
		key[i] = byte(i)
	}

	plaintext := []byte("Hello, World!")

	encrypted, err := encrypt(plaintext, key)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	decrypted, err := decrypt(encrypted, key)
	if err != nil {
		t.Fatalf("failed to decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("decrypted text doesn't match: got %q, want %q", decrypted, plaintext)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	key1 := &[32]byte{}
	key2 := &[32]byte{}
	for i := range key1 {
		key1[i] = byte(i)
		key2[i] = byte(i + 1)
	}

	plaintext := []byte("Hello, World!")

	encrypted, err := encrypt(plaintext, key1)
	if err != nil {
		t.Fatalf("failed to encrypt: %v", err)
	}

	_, err = decrypt(encrypted, key2)
	if err == nil {
		t.Error("expected decryption to fail with wrong key")
	}
}
