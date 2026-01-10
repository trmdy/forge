package fmail

import "errors"

const (
	EnvAgent   = "FMAIL_AGENT"
	EnvRoot    = "FMAIL_ROOT"
	EnvProject = "FMAIL_PROJECT"

	MaxMessageSize = 1 << 20 // 1MB
)

var (
	ErrInvalidTopic    = errors.New("invalid topic name")
	ErrInvalidAgent    = errors.New("invalid agent name")
	ErrInvalidTarget   = errors.New("invalid target")
	ErrMessageTooLarge = errors.New("message exceeds 1MB limit")
	ErrEmptyMessage    = errors.New("message is nil")
	ErrIDCollision     = errors.New("message id collision")
)
