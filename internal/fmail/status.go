package fmail

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func runStatus(cmd *cobra.Command, args []string) error {
	runtime, err := EnsureRuntime(cmd)
	if err != nil {
		return err
	}

	clear, _ := cmd.Flags().GetBool("clear")
	if clear && len(args) > 0 {
		return usageError(cmd, "status does not take a message with --clear")
	}

	store, err := NewStore(runtime.Root)
	if err != nil {
		return Exitf(ExitCodeFailure, "init store: %v", err)
	}

	if len(args) == 0 && !clear {
		record, err := store.ReadAgentRecord(runtime.Agent)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return Exitf(ExitCodeFailure, "load status: %v", err)
		}
		status := strings.TrimSpace(record.Status)
		if status != "" {
			fmt.Fprintln(cmd.OutOrStdout(), status)
		}
		return nil
	}

	status := ""
	if !clear {
		status = strings.TrimSpace(args[0])
		if status == "" {
			return usageError(cmd, "status message is required")
		}
	}

	host, _ := os.Hostname()
	if _, err := store.SetAgentStatus(runtime.Agent, status, host); err != nil {
		return Exitf(ExitCodeFailure, "update status: %v", err)
	}
	return nil
}
