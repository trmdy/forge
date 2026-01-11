package fmail

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// AgentRecord tracks agent presence in the project.
type AgentRecord struct {
	Name      string    `json:"name"`
	Host      string    `json:"host,omitempty"`
	Status    string    `json:"status,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// UpdateAgentRecord creates or updates the agent registry entry.
func (s *Store) UpdateAgentRecord(name, host string) (*AgentRecord, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	path, normalized, err := s.agentRecordPath(name)
	if err != nil {
		return nil, err
	}
	if err := s.EnsureRoot(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.AgentsDir(), 0o755); err != nil {
		return nil, err
	}
	record, exists, err := readAgentRecord(path)
	if err != nil {
		return nil, err
	}
	if !exists {
		record = &AgentRecord{Name: normalized}
	}
	now := s.now()
	if record.Name == "" {
		record.Name = normalized
	}
	if record.FirstSeen.IsZero() {
		record.FirstSeen = now
	}
	record.LastSeen = now

	host = strings.TrimSpace(host)
	if host != "" {
		record.Host = host
	}

	if err := writeAgentRecord(path, record); err != nil {
		return nil, err
	}
	return record, nil
}

// ReadAgentRecord loads an agent record by name.
func (s *Store) ReadAgentRecord(name string) (*AgentRecord, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	path, normalized, err := s.agentRecordPath(name)
	if err != nil {
		return nil, err
	}
	record, exists, err := readAgentRecord(path)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, os.ErrNotExist
	}
	if record.Name == "" {
		record.Name = normalized
	}
	return record, nil
}

// SetAgentStatus sets or clears the status for an agent.
func (s *Store) SetAgentStatus(name, status, host string) (*AgentRecord, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	path, normalized, err := s.agentRecordPath(name)
	if err != nil {
		return nil, err
	}
	if err := s.EnsureRoot(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.AgentsDir(), 0o755); err != nil {
		return nil, err
	}

	record, exists, err := readAgentRecord(path)
	if err != nil {
		return nil, err
	}
	if !exists {
		record = &AgentRecord{Name: normalized}
	}
	now := s.now()
	if record.Name == "" {
		record.Name = normalized
	}
	if record.FirstSeen.IsZero() {
		record.FirstSeen = now
	}
	record.LastSeen = now

	status = strings.TrimSpace(status)
	record.Status = status

	host = strings.TrimSpace(host)
	if host != "" {
		record.Host = host
	}

	if err := writeAgentRecord(path, record); err != nil {
		return nil, err
	}
	return record, nil
}

// ListAgentRecords returns all known agent records.
func (s *Store) ListAgentRecords() ([]AgentRecord, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}
	entries, err := os.ReadDir(s.AgentsDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	records := make([]AgentRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(s.AgentsDir(), entry.Name())
		record, exists, err := readAgentRecord(path)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		if record.Name == "" {
			record.Name = strings.TrimSuffix(entry.Name(), ".json")
		}
		records = append(records, *record)
	}

	sort.Slice(records, func(i, j int) bool {
		return records[i].Name < records[j].Name
	})
	return records, nil
}

func (s *Store) agentRecordPath(name string) (string, string, error) {
	normalized, err := NormalizeAgentName(name)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(s.AgentsDir(), normalized+".json"), normalized, nil
}

func readAgentRecord(path string) (*AgentRecord, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var record AgentRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, false, err
	}
	return &record, true, nil
}

func writeAgentRecord(path string, record *AgentRecord) error {
	encoded, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, encoded, 0o644)
}
