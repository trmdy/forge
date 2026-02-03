// Package vault provides encrypted credential storage.
package vault

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/nacl/secretbox"
)

// Errors returned by the vault.
var (
	ErrVaultLocked     = errors.New("vault is locked")
	ErrVaultUnlocked   = errors.New("vault is already unlocked")
	ErrSecretNotFound  = errors.New("secret not found")
	ErrInvalidPassword = errors.New("invalid password")
	ErrVaultCorrupted  = errors.New("vault data corrupted")
)

const (
	// Argon2 parameters
	argon2Time    = 1
	argon2Memory  = 64 * 1024 // 64 MB
	argon2Threads = 4
	keyLength     = 32

	// NaCl constants
	nonceSize = 24
	saltSize  = 16

	// File names
	vaultFileName = "vault.enc"
	saltFileName  = "vault.salt"
)

// Secret represents a stored credential.
type Secret struct {
	// Name is the unique identifier for the secret.
	Name string `json:"name"`

	// Value is the secret value (e.g., API key).
	Value string `json:"value"`

	// Metadata contains optional metadata.
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt is when the secret was stored.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the secret was last updated.
	UpdatedAt time.Time `json:"updated_at"`
}

// vaultData is the internal structure stored encrypted.
type vaultData struct {
	Version int                `json:"version"`
	Secrets map[string]*Secret `json:"secrets"`
}

// Vault provides encrypted credential storage.
type Vault struct {
	mu       sync.RWMutex
	path     string
	key      *[32]byte
	salt     []byte
	data     *vaultData
	unlocked bool
}

// New creates a new vault at the specified path.
func New(path string) *Vault {
	return &Vault{
		path: path,
	}
}

// DefaultPath returns the default vault path.
func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".forge", "vault")
}

// IsInitialized checks if the vault has been initialized.
func (v *Vault) IsInitialized() bool {
	saltPath := filepath.Join(v.path, saltFileName)
	_, err := os.Stat(saltPath)
	return err == nil
}

// IsUnlocked returns whether the vault is currently unlocked.
func (v *Vault) IsUnlocked() bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.unlocked
}

// Initialize creates a new vault with the given password.
func (v *Vault) Initialize(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.IsInitialized() {
		return errors.New("vault already initialized")
	}

	// Create vault directory
	if err := os.MkdirAll(v.path, 0700); err != nil {
		return fmt.Errorf("failed to create vault directory: %w", err)
	}

	// Generate salt
	salt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Write salt file
	saltPath := filepath.Join(v.path, saltFileName)
	if err := os.WriteFile(saltPath, salt, 0600); err != nil {
		return fmt.Errorf("failed to write salt: %w", err)
	}

	// Derive key
	key := deriveKey(password, salt)

	// Initialize empty vault data
	v.salt = salt
	v.key = key
	v.data = &vaultData{
		Version: 1,
		Secrets: make(map[string]*Secret),
	}
	v.unlocked = true

	// Save empty vault
	if err := v.saveLocked(); err != nil {
		return fmt.Errorf("failed to save vault: %w", err)
	}

	return nil
}

// Unlock opens the vault with the given password.
func (v *Vault) Unlock(password string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if v.unlocked {
		return ErrVaultUnlocked
	}

	if !v.IsInitialized() {
		return errors.New("vault not initialized")
	}

	// Read salt
	saltPath := filepath.Join(v.path, saltFileName)
	salt, err := os.ReadFile(saltPath)
	if err != nil {
		return fmt.Errorf("failed to read salt: %w", err)
	}

	// Derive key
	key := deriveKey(password, salt)

	// Read and decrypt vault
	vaultPath := filepath.Join(v.path, vaultFileName)
	encrypted, err := os.ReadFile(vaultPath)
	if err != nil {
		if os.IsNotExist(err) {
			// New vault with no secrets yet
			v.salt = salt
			v.key = key
			v.data = &vaultData{
				Version: 1,
				Secrets: make(map[string]*Secret),
			}
			v.unlocked = true
			return nil
		}
		return fmt.Errorf("failed to read vault: %w", err)
	}

	// Decrypt
	data, err := decrypt(encrypted, key)
	if err != nil {
		return ErrInvalidPassword
	}

	// Parse vault data
	var vd vaultData
	if err := json.Unmarshal(data, &vd); err != nil {
		return ErrVaultCorrupted
	}

	v.salt = salt
	v.key = key
	v.data = &vd
	v.unlocked = true

	return nil
}

// Lock closes the vault and clears the key from memory.
func (v *Vault) Lock() error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.unlocked {
		return ErrVaultLocked
	}

	// Clear sensitive data
	if v.key != nil {
		for i := range v.key {
			v.key[i] = 0
		}
	}
	v.key = nil
	v.data = nil
	v.unlocked = false

	return nil
}

// Store adds or updates a secret in the vault.
func (v *Vault) Store(name, value string, metadata map[string]string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.unlocked {
		return ErrVaultLocked
	}

	now := time.Now().UTC()
	existing, exists := v.data.Secrets[name]

	secret := &Secret{
		Name:      name,
		Value:     value,
		Metadata:  metadata,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if exists {
		secret.CreatedAt = existing.CreatedAt
	}

	v.data.Secrets[name] = secret

	return v.saveLocked()
}

// Retrieve gets a secret from the vault.
func (v *Vault) Retrieve(name string) (*Secret, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if !v.unlocked {
		return nil, ErrVaultLocked
	}

	secret, exists := v.data.Secrets[name]
	if !exists {
		return nil, ErrSecretNotFound
	}

	// Return a copy to prevent mutation
	copy := *secret
	if secret.Metadata != nil {
		copy.Metadata = make(map[string]string, len(secret.Metadata))
		for k, val := range secret.Metadata {
			copy.Metadata[k] = val
		}
	}

	return &copy, nil
}

// Delete removes a secret from the vault.
func (v *Vault) Delete(name string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.unlocked {
		return ErrVaultLocked
	}

	if _, exists := v.data.Secrets[name]; !exists {
		return ErrSecretNotFound
	}

	delete(v.data.Secrets, name)

	return v.saveLocked()
}

// List returns all secret names in the vault.
func (v *Vault) List() ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if !v.unlocked {
		return nil, ErrVaultLocked
	}

	names := make([]string, 0, len(v.data.Secrets))
	for name := range v.data.Secrets {
		names = append(names, name)
	}

	return names, nil
}

// ChangePassword changes the vault password.
func (v *Vault) ChangePassword(oldPassword, newPassword string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	if !v.unlocked {
		return ErrVaultLocked
	}

	// Verify old password
	oldKey := deriveKey(oldPassword, v.salt)
	if *oldKey != *v.key {
		return ErrInvalidPassword
	}

	// Generate new salt
	newSalt := make([]byte, saltSize)
	if _, err := io.ReadFull(rand.Reader, newSalt); err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	// Derive new key
	newKey := deriveKey(newPassword, newSalt)

	// Write new salt
	saltPath := filepath.Join(v.path, saltFileName)
	if err := os.WriteFile(saltPath, newSalt, 0600); err != nil {
		return fmt.Errorf("failed to write salt: %w", err)
	}

	// Update vault with new key
	v.salt = newSalt
	v.key = newKey

	return v.saveLocked()
}

// saveLocked saves the vault data (must be called with lock held).
func (v *Vault) saveLocked() error {
	// Serialize vault data
	data, err := json.Marshal(v.data)
	if err != nil {
		return fmt.Errorf("failed to serialize vault: %w", err)
	}

	// Encrypt
	encrypted, err := encrypt(data, v.key)
	if err != nil {
		return fmt.Errorf("failed to encrypt vault: %w", err)
	}

	// Write to file
	vaultPath := filepath.Join(v.path, vaultFileName)
	if err := os.WriteFile(vaultPath, encrypted, 0600); err != nil {
		return fmt.Errorf("failed to write vault: %w", err)
	}

	return nil
}

// deriveKey derives an encryption key from a password using Argon2.
func deriveKey(password string, salt []byte) *[32]byte {
	keyBytes := argon2.IDKey(
		[]byte(password),
		salt,
		argon2Time,
		argon2Memory,
		argon2Threads,
		keyLength,
	)

	var key [32]byte
	copy(key[:], keyBytes)
	return &key
}

// encrypt encrypts data using NaCl secretbox.
func encrypt(plaintext []byte, key *[32]byte) ([]byte, error) {
	// Generate random nonce
	var nonce [nonceSize]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	encrypted := secretbox.Seal(nonce[:], plaintext, &nonce, key)
	return encrypted, nil
}

// decrypt decrypts data using NaCl secretbox.
func decrypt(ciphertext []byte, key *[32]byte) ([]byte, error) {
	if len(ciphertext) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	// Extract nonce
	var nonce [nonceSize]byte
	copy(nonce[:], ciphertext[:nonceSize])

	// Decrypt
	plaintext, ok := secretbox.Open(nil, ciphertext[nonceSize:], &nonce, key)
	if !ok {
		return nil, errors.New("decryption failed")
	}

	return plaintext, nil
}

// ResolveCredential resolves a credential reference to its value.
// Supports:
//   - "vault:secret-name" - retrieves from vault
//   - "env:VAR_NAME" - retrieves from environment
//   - literal value - returns as-is
func (v *Vault) ResolveCredential(ref string) (string, error) {
	if len(ref) > 6 && ref[:6] == "vault:" {
		name := ref[6:]
		secret, err := v.Retrieve(name)
		if err != nil {
			return "", fmt.Errorf("vault secret %q: %w", name, err)
		}
		return secret.Value, nil
	}

	if len(ref) > 4 && ref[:4] == "env:" {
		varName := ref[4:]
		value, ok := os.LookupEnv(varName)
		if !ok {
			return "", fmt.Errorf("environment variable %q not set", varName)
		}
		return value, nil
	}

	// Literal value
	return ref, nil
}

// ExportSecretNames returns a hex-encoded list of secret names for display.
// This is useful for debugging without exposing secret values.
func (v *Vault) ExportSecretNames() ([]string, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if !v.unlocked {
		return nil, ErrVaultLocked
	}

	names := make([]string, 0, len(v.data.Secrets))
	for name, secret := range v.data.Secrets {
		info := fmt.Sprintf("%s (created: %s)", name, secret.CreatedAt.Format(time.RFC3339))
		names = append(names, info)
	}

	return names, nil
}

// GenerateSecretName generates a unique secret name with the given prefix.
func GenerateSecretName(prefix string) string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b)
}
