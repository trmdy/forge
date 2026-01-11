package fmail

import "fmt"

const (
	ExitCodeFailure = 1
	ExitCodeUsage   = 2
)

// ExitError carries an exit code and optional print state for consistent exits.
type ExitError struct {
	Code    int
	Err     error
	Printed bool
}

func (e *ExitError) Error() string {
	return e.Err.Error()
}

func Exit(code int, err error) *ExitError {
	return &ExitError{Code: code, Err: err}
}

func Exitf(code int, format string, args ...any) *ExitError {
	return &ExitError{Code: code, Err: fmt.Errorf(format, args...)}
}
