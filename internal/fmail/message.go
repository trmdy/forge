package fmail

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

const (
	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
)

type Message struct {
	ID       string    `json:"id"`
	From     string    `json:"from"`
	To       string    `json:"to"`
	Time     time.Time `json:"time"`
	Body     any       `json:"body"`
	ReplyTo  string    `json:"reply_to,omitempty"`
	Priority string    `json:"priority,omitempty"`
	Host     string    `json:"host,omitempty"`
}

var idCounter uint32

// GenerateMessageID creates a sortable message ID using UTC time and a per-process sequence.
func GenerateMessageID(now time.Time) string {
	seq := atomic.AddUint32(&idCounter, 1) % 10000
	return fmt.Sprintf("%s-%04d", now.UTC().Format("20060102-150405"), seq)
}

// NewMessageID generates a new message ID using the current time.
func NewMessageID() string {
	return GenerateMessageID(time.Now().UTC())
}

// Validate checks required fields and basic constraints.
func (m *Message) Validate() error {
	if m == nil {
		return ErrEmptyMessage
	}
	if m.ID == "" {
		return fmt.Errorf("missing id")
	}
	if err := ValidateAgentName(m.From); err != nil {
		return fmt.Errorf("invalid from: %w", err)
	}
	if err := ValidateTarget(m.To); err != nil {
		return fmt.Errorf("invalid to: %w", err)
	}
	if m.Time.IsZero() {
		return fmt.Errorf("missing time")
	}
	if m.Body == nil {
		return fmt.Errorf("missing body")
	}
	if m.Priority != "" {
		if err := ValidatePriority(m.Priority); err != nil {
			return err
		}
	}
	return nil
}

// ValidatePriority enforces allowed message priorities.
func ValidatePriority(value string) error {
	switch value {
	case PriorityLow, PriorityNormal, PriorityHigh:
		return nil
	default:
		return fmt.Errorf("invalid priority: %s", value)
	}
}

func marshalMessage(message *Message) ([]byte, error) {
	return json.MarshalIndent(message, "", "  ")
}
