package ssh

import (
	"errors"
	"fmt"
)

var (
	ErrPassphraseRequired       = errors.New("passphrase required for private key")
	ErrSSHAgentUnavailable      = errors.New("ssh agent not available")
	ErrHostKeyRejected          = errors.New("host key rejected")
	ErrHostKeyPromptUnavailable = errors.New("host key prompt unavailable")
	ErrPortForwardUnsupported   = errors.New("port forwarding not supported")
	ErrPortForwardLocalRequired = errors.New("local port is required for port forwarding")
)

// ExitError represents a non-zero exit code from a remote command.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit status %d", e.Code)
}

// NewExitError creates a new ExitError with the given exit code.
func NewExitError(code int) *ExitError {
	return &ExitError{Code: code}
}
