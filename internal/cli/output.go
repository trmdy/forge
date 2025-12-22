// Package cli provides output formatting helpers for CLI commands.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"reflect"
)

// Formatter renders command output as human-readable, JSON, or JSONL.
type Formatter struct {
	out   io.Writer
	json  bool
	jsonl bool
}

// NewFormatter builds a formatter using the current CLI flags.
func NewFormatter(out io.Writer) *Formatter {
	return &Formatter{
		out:   out,
		json:  IsJSONOutput(),
		jsonl: IsJSONLOutput(),
	}
}

// Write formats and writes output based on CLI flags.
func (f *Formatter) Write(value any) error {
	if f.jsonl {
		return writeJSONL(f.out, value)
	}
	if f.json {
		return writeJSON(f.out, value)
	}
	return writeHuman(f.out, value)
}

// WriteOutput is a convenience wrapper around NewFormatter.
func WriteOutput(out io.Writer, value any) error {
	return NewFormatter(out).Write(value)
}

func writeJSON(out io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal JSON: %w", err)
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

func writeJSONL(out io.Writer, value any) error {
	val := reflect.ValueOf(value)
	if val.IsValid() && (val.Kind() == reflect.Slice || val.Kind() == reflect.Array) {
		for i := 0; i < val.Len(); i++ {
			if err := writeJSONLine(out, val.Index(i).Interface()); err != nil {
				return err
			}
		}
		return nil
	}
	return writeJSONLine(out, value)
}

func writeJSONLine(out io.Writer, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal JSONL: %w", err)
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}

func writeHuman(out io.Writer, value any) error {
	_, err := fmt.Fprintln(out, value)
	return err
}
