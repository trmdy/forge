// Package hooks provides event hook configuration and execution.
package hooks

import (
	"time"

	"github.com/opencode-ai/swarm/internal/models"
)

// Kind describes how a hook is executed.
type Kind string

const (
	// KindCommand executes a local command.
	KindCommand Kind = "command"
	// KindWebhook sends an HTTP request.
	KindWebhook Kind = "webhook"
)

// Hook defines a registered event hook.
type Hook struct {
	ID string `json:"id"`

	Kind Kind `json:"kind"`

	Command string            `json:"command,omitempty"`
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`

	EventTypes  []models.EventType  `json:"event_types,omitempty"`
	EntityTypes []models.EntityType `json:"entity_types,omitempty"`
	EntityID    string              `json:"entity_id,omitempty"`

	Enabled bool `json:"enabled"`

	Timeout string `json:"timeout,omitempty"`

	CreatedAt time.Time `json:"created_at,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}
