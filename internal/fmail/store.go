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

const (
	maxIDRetries = 10
)

type Store struct {
	Root        string
	now         func() time.Time
	idGenerator func(time.Time) string
}

type StoreOption func(*Store)

func WithNow(now func() time.Time) StoreOption {
	return func(store *Store) {
		if now != nil {
			store.now = now
		}
	}
}

func WithIDGenerator(gen func(time.Time) string) StoreOption {
	return func(store *Store) {
		if gen != nil {
			store.idGenerator = gen
		}
	}
}

// NewStore initializes a store rooted at <projectRoot>/.fmail.
func NewStore(projectRoot string, opts ...StoreOption) (*Store, error) {
	if strings.TrimSpace(projectRoot) == "" {
		return nil, fmt.Errorf("project root required")
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}
	store := &Store{
		Root:        filepath.Join(abs, ".fmail"),
		now:         func() time.Time { return time.Now().UTC() },
		idGenerator: GenerateMessageID,
	}
	for _, opt := range opts {
		opt(store)
	}
	return store, nil
}

func (s *Store) EnsureRoot() error {
	return os.MkdirAll(s.Root, 0o755)
}

func (s *Store) TopicDir(topic string) string {
	return filepath.Join(s.Root, "topics", topic)
}

func (s *Store) DMDir(agent string) string {
	return filepath.Join(s.Root, "dm", agent)
}

func (s *Store) AgentsDir() string {
	return filepath.Join(s.Root, "agents")
}

func (s *Store) ProjectFile() string {
	return filepath.Join(s.Root, "project.json")
}

func (s *Store) TopicMessagePath(topic, id string) string {
	return filepath.Join(s.TopicDir(topic), id+".json")
}

func (s *Store) DMMessagePath(agent, id string) string {
	return filepath.Join(s.DMDir(agent), id+".json")
}

func (s *Store) SaveMessage(message *Message) (string, error) {
	if message == nil {
		return "", ErrEmptyMessage
	}

	normalizedFrom, err := NormalizeAgentName(message.From)
	if err != nil {
		return "", err
	}
	message.From = normalizedFrom

	normalizedTarget, isDM, err := NormalizeTarget(message.To)
	if err != nil {
		return "", err
	}
	message.To = normalizedTarget

	if message.Time.IsZero() {
		message.Time = s.now()
	}

	if message.ID == "" {
		message.ID = s.idGenerator(message.Time)
	}

	if err := message.Validate(); err != nil {
		return "", err
	}

	if err := s.EnsureRoot(); err != nil {
		return "", err
	}

	var dir string
	if isDM {
		dir = s.DMDir(strings.TrimPrefix(normalizedTarget, "@"))
	} else {
		dir = s.TopicDir(normalizedTarget)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}

	for attempt := 0; attempt < maxIDRetries; attempt++ {
		data, err := marshalMessage(message)
		if err != nil {
			return "", err
		}
		if len(data) > MaxMessageSize {
			return "", ErrMessageTooLarge
		}

		path := filepath.Join(dir, message.ID+".json")
		err = writeFileExclusive(path, data)
		if err == nil {
			return message.ID, nil
		}
		if errors.Is(err, os.ErrExist) {
			message.ID = s.idGenerator(s.now())
			continue
		}
		return "", err
	}
	return "", ErrIDCollision
}

func (s *Store) ReadMessage(path string) (*Message, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

func (s *Store) ListTopicMessages(topic string) ([]Message, error) {
	normalized, err := NormalizeTopic(topic)
	if err != nil {
		return nil, err
	}
	return s.listMessages(s.TopicDir(normalized))
}

func (s *Store) ListDMMessages(agent string) ([]Message, error) {
	normalized, err := NormalizeAgentName(agent)
	if err != nil {
		return nil, err
	}
	return s.listMessages(s.DMDir(normalized))
}

func (s *Store) EnsureProject(id string) (*Project, error) {
	if err := s.EnsureRoot(); err != nil {
		return nil, err
	}
	path := s.ProjectFile()
	if _, err := os.Stat(path); err == nil {
		return readProject(path)
	}
	project := Project{ID: id, Created: s.now()}
	data, err := json.MarshalIndent(project, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := writeFileExclusive(path, data); err != nil {
		if errors.Is(err, os.ErrExist) {
			return readProject(path)
		}
		return nil, err
	}
	return &project, nil
}

func readProject(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var project Project
	if err := json.Unmarshal(data, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *Store) listMessages(dir string) ([]Message, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	messages := make([]Message, 0, len(names))
	for _, name := range names {
		msg, err := s.ReadMessage(filepath.Join(dir, name))
		if err != nil {
			return nil, err
		}
		messages = append(messages, *msg)
	}
	return messages, nil
}

func writeFileExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()

	if _, err := file.Write(data); err != nil {
		return err
	}
	return file.Close()
}
