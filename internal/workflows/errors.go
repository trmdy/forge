package workflows

import (
	"fmt"
	"strings"
)

const (
	ErrCodeParse         = "ERR_PARSE"
	ErrCodeMissingField  = "ERR_MISSING_FIELD"
	ErrCodeInvalidField  = "ERR_INVALID_FIELD"
	ErrCodeUnknownType   = "ERR_UNKNOWN_TYPE"
	ErrCodeDuplicateStep = "ERR_DUPLICATE_STEP"
	ErrCodeMissingStep   = "ERR_MISSING_STEP"
	ErrCodeCycle         = "ERR_CYCLE"
)

// WorkflowError carries structured validation information.
type WorkflowError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	StepID  string `json:"step_id,omitempty"`
	Field   string `json:"field,omitempty"`
	Line    int    `json:"line,omitempty"`
	Column  int    `json:"column,omitempty"`
	Index   int    `json:"index,omitempty"`
}

func (e WorkflowError) Error() string {
	return e.HumanString()
}

// HumanString renders a human-friendly message with context.
func (e WorkflowError) HumanString() string {
	parts := make([]string, 0, 3)
	if e.Path != "" {
		parts = append(parts, e.Path)
	}
	if e.StepID != "" {
		parts = append(parts, fmt.Sprintf("step %s", e.StepID))
	} else if e.Index > 0 {
		parts = append(parts, fmt.Sprintf("step #%d", e.Index))
	}
	if e.Field != "" {
		parts = append(parts, e.Field)
	}

	prefix := "workflow"
	if len(parts) > 0 {
		prefix = strings.Join(parts, ": ")
	}

	message := e.Message
	if message == "" {
		message = e.Code
	}

	if e.Line > 0 {
		location := fmt.Sprintf("line %d", e.Line)
		if e.Column > 0 {
			location = fmt.Sprintf("%s:%d", location, e.Column)
		}
		message = fmt.Sprintf("%s (%s)", message, location)
	}

	return fmt.Sprintf("%s: %s", prefix, message)
}

// ErrorList groups workflow errors.
type ErrorList struct {
	Errors []WorkflowError `json:"errors"`
}

func (e *ErrorList) Error() string {
	if e == nil || len(e.Errors) == 0 {
		return ""
	}
	lines := make([]string, 0, len(e.Errors))
	for _, err := range e.Errors {
		lines = append(lines, err.HumanString())
	}
	return strings.Join(lines, "\n")
}

func (e *ErrorList) Add(err WorkflowError) {
	e.Errors = append(e.Errors, err)
}

func (e *ErrorList) Empty() bool {
	return e == nil || len(e.Errors) == 0
}
