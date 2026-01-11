package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// LoopQueueItemType specifies the type of loop queue item.
type LoopQueueItemType string

const (
	LoopQueueItemMessageAppend      LoopQueueItemType = "message_append"
	LoopQueueItemNextPromptOverride LoopQueueItemType = "next_prompt_override"
	LoopQueueItemPause              LoopQueueItemType = "pause"
	LoopQueueItemStopGraceful       LoopQueueItemType = "stop_graceful"
	LoopQueueItemKillNow            LoopQueueItemType = "kill_now"
	LoopQueueItemSteerMessage       LoopQueueItemType = "steer_message"
)

// LoopQueueItemStatus represents the status of a loop queue item.
type LoopQueueItemStatus string

const (
	LoopQueueStatusPending    LoopQueueItemStatus = "pending"
	LoopQueueStatusDispatched LoopQueueItemStatus = "dispatched"
	LoopQueueStatusCompleted  LoopQueueItemStatus = "completed"
	LoopQueueStatusFailed     LoopQueueItemStatus = "failed"
	LoopQueueStatusSkipped    LoopQueueItemStatus = "skipped"
)

// LoopQueueItem represents an item in a loop's queue.
type LoopQueueItem struct {
	ID           string              `json:"id"`
	LoopID       string              `json:"loop_id"`
	Type         LoopQueueItemType   `json:"type"`
	Position     int                 `json:"position"`
	Status       LoopQueueItemStatus `json:"status"`
	Attempts     int                 `json:"attempts"`
	Payload      json.RawMessage     `json:"payload"`
	CreatedAt    time.Time           `json:"created_at"`
	DispatchedAt *time.Time          `json:"dispatched_at,omitempty"`
	CompletedAt  *time.Time          `json:"completed_at,omitempty"`
	Error        string              `json:"error,omitempty"`
}

// MessageAppendPayload appends a message to the prompt.
type MessageAppendPayload struct {
	Text string `json:"text"`
}

// NextPromptOverridePayload overrides the base prompt for one iteration.
type NextPromptOverridePayload struct {
	Prompt string `json:"prompt"`
	IsPath bool   `json:"is_path"`
}

// LoopPausePayload pauses the loop for a duration.
type LoopPausePayload struct {
	DurationSeconds int    `json:"duration_seconds"`
	Reason          string `json:"reason,omitempty"`
}

// StopPayload requests a graceful stop.
type StopPayload struct {
	Reason string `json:"reason,omitempty"`
}

// KillPayload requests an immediate kill.
type KillPayload struct {
	Reason string `json:"reason,omitempty"`
}

// SteerPayload requests an interrupt + message.
type SteerPayload struct {
	Message string `json:"message"`
}

// Validate checks if the queue item is valid.
func (q *LoopQueueItem) Validate() error {
	validation := &ValidationErrors{}
	if q.Type == "" {
		validation.AddMessage("type", "queue item type is required")
	}
	if q.Attempts < 0 {
		validation.AddMessage("attempts", "attempts must be >= 0")
	}
	if len(q.Payload) == 0 {
		validation.Add("payload", ErrInvalidQueueItem)
	}
	if validation.Err() != nil {
		return validation.Err()
	}

	switch q.Type {
	case LoopQueueItemMessageAppend:
		var payload MessageAppendPayload
		if err := json.Unmarshal(q.Payload, &payload); err != nil {
			return fmt.Errorf("invalid message_append payload: %w", err)
		}
		if strings.TrimSpace(payload.Text) == "" {
			return errors.New("message_append payload text is required")
		}
	case LoopQueueItemNextPromptOverride:
		var payload NextPromptOverridePayload
		if err := json.Unmarshal(q.Payload, &payload); err != nil {
			return fmt.Errorf("invalid next_prompt_override payload: %w", err)
		}
		if strings.TrimSpace(payload.Prompt) == "" {
			return errors.New("next_prompt_override payload prompt is required")
		}
	case LoopQueueItemPause:
		var payload LoopPausePayload
		if err := json.Unmarshal(q.Payload, &payload); err != nil {
			return fmt.Errorf("invalid pause payload: %w", err)
		}
		if payload.DurationSeconds <= 0 {
			return errors.New("pause payload duration_seconds must be > 0")
		}
	case LoopQueueItemStopGraceful:
		var payload StopPayload
		if err := json.Unmarshal(q.Payload, &payload); err != nil {
			return fmt.Errorf("invalid stop_graceful payload: %w", err)
		}
	case LoopQueueItemKillNow:
		var payload KillPayload
		if err := json.Unmarshal(q.Payload, &payload); err != nil {
			return fmt.Errorf("invalid kill_now payload: %w", err)
		}
	case LoopQueueItemSteerMessage:
		var payload SteerPayload
		if err := json.Unmarshal(q.Payload, &payload); err != nil {
			return fmt.Errorf("invalid steer_message payload: %w", err)
		}
		if strings.TrimSpace(payload.Message) == "" {
			return errors.New("steer_message payload message is required")
		}
	default:
		return fmt.Errorf("unknown loop queue item type %q", q.Type)
	}

	return nil
}
