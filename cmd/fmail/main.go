// Package main is the entry point for the fmail CLI.
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/tOgg1/forge/internal/fmail"
)

func main() {
	if err := fmail.Execute(); err != nil {
		var exitErr *fmail.ExitError
		if errors.As(err, &exitErr) {
			if !exitErr.Printed {
				fmt.Fprintf(os.Stderr, "Error: %v\n", exitErr.Err)
			}
			os.Exit(exitErr.Code)
		}

		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
