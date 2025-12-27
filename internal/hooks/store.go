package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

type hookFile struct {
	Hooks []Hook `json:"hooks"`
}

// Store persists hooks to disk.
type Store struct {
	path string
	mu   sync.RWMutex
}

// NewStore creates a new hook store.
func NewStore(path string) *Store {
	return &Store{path: path}
}

// Path returns the store file path.
func (s *Store) Path() string {
	return s.path
}

// List returns all stored hooks.
func (s *Store) List() ([]Hook, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.loadLocked()
}

// Add stores a new hook.
func (s *Store) Add(hook Hook) (Hook, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hooks, err := s.loadLocked()
	if err != nil {
		return Hook{}, err
	}

	now := time.Now().UTC()
	if hook.ID == "" {
		hook.ID = uuid.New().String()
	}
	if hook.CreatedAt.IsZero() {
		hook.CreatedAt = now
	}
	hook.UpdatedAt = now
	if hook.Headers == nil {
		hook.Headers = map[string]string{}
	}

	hooks = append(hooks, hook)
	if err := s.saveLocked(hooks); err != nil {
		return Hook{}, err
	}

	return hook, nil
}

func (s *Store) loadLocked() ([]Hook, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []Hook{}, nil
		}
		return nil, fmt.Errorf("failed to read hook store: %w", err)
	}

	var payload hookFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse hook store: %w", err)
	}
	if payload.Hooks == nil {
		return []Hook{}, nil
	}
	return payload.Hooks, nil
}

func (s *Store) saveLocked(hooks []Hook) error {
	if s.path == "" {
		return fmt.Errorf("hook store path is required")
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create hook store directory: %w", err)
	}

	payload := hookFile{Hooks: hooks}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize hook store: %w", err)
	}

	file, err := os.CreateTemp(dir, "hooks-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create hook temp file: %w", err)
	}
	name := file.Name()
	defer func() {
		_ = os.Remove(name)
	}()

	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("failed to write hook store: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close hook store: %w", err)
	}

	if err := os.Rename(name, s.path); err != nil {
		return fmt.Errorf("failed to save hook store: %w", err)
	}

	return nil
}
