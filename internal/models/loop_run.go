package models

import "time"

// LoopRunStatus represents the status of a loop run.
type LoopRunStatus string

const (
	LoopRunStatusRunning LoopRunStatus = "running"
	LoopRunStatusSuccess LoopRunStatus = "success"
	LoopRunStatusError   LoopRunStatus = "error"
	LoopRunStatusKilled  LoopRunStatus = "killed"
)

// LoopRun captures a single loop iteration.
type LoopRun struct {
	ID             string         `json:"id"`
	LoopID         string         `json:"loop_id"`
	ProfileID      string         `json:"profile_id,omitempty"`
	Status         LoopRunStatus  `json:"status"`
	PromptSource   string         `json:"prompt_source,omitempty"`
	PromptPath     string         `json:"prompt_path,omitempty"`
	PromptOverride bool           `json:"prompt_override"`
	StartedAt      time.Time      `json:"started_at"`
	FinishedAt     *time.Time     `json:"finished_at,omitempty"`
	ExitCode       *int           `json:"exit_code,omitempty"`
	OutputTail     string         `json:"output_tail,omitempty"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}
