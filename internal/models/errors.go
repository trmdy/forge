package models

import "errors"

// Validation errors for models
var (
	// Node errors
	ErrInvalidNodeName  = errors.New("node name is required")
	ErrInvalidSSHTarget = errors.New("SSH target is required for remote nodes")

	// Workspace errors
	ErrInvalidWorkspaceNode = errors.New("workspace must be associated with a node")
	ErrInvalidRepoPath      = errors.New("repository path is required")
	ErrInvalidTmuxSession   = errors.New("tmux session name is required")

	// Agent errors
	ErrInvalidAgentWorkspace = errors.New("agent must be associated with a workspace")
	ErrInvalidAgentType      = errors.New("agent type is required")
	ErrInvalidTmuxPane       = errors.New("tmux pane target is required")

	// Queue errors
	ErrInvalidQueueItem = errors.New("queue item payload is required")
	ErrEmptyQueue       = errors.New("queue is empty")

	// Account errors
	ErrInvalidProvider    = errors.New("provider is required")
	ErrInvalidProfileName = errors.New("profile name is required")

	// Loop errors
	ErrInvalidLoopName     = errors.New("loop name is required")
	ErrInvalidLoopRepoPath = errors.New("loop repo path is required")
	ErrInvalidLoopShortID  = errors.New("loop short ID must be 6-9 alphanumeric characters")

	// Profile errors
	ErrInvalidProfileHarness  = errors.New("profile harness is required")
	ErrInvalidCommandTemplate = errors.New("command template is required")

	// Pool errors
	ErrInvalidPoolName = errors.New("pool name is required")
)
