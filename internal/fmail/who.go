package fmail

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func runWho(cmd *cobra.Command, args []string) error {
	runtime, err := EnsureRuntime(cmd)
	if err != nil {
		return err
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")

	store, err := NewStore(runtime.Root)
	if err != nil {
		return Exitf(ExitCodeFailure, "init store: %v", err)
	}

	records, err := store.ListAgentRecords()
	if err != nil {
		return Exitf(ExitCodeFailure, "list agents: %v", err)
	}

	if jsonOutput {
		payload, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			return Exitf(ExitCodeFailure, "encode agents: %v", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(payload))
		return nil
	}

	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
	fmt.Fprintln(writer, "NAME\tLAST SEEN\tSTATUS")
	now := time.Now().UTC()
	for _, record := range records {
		status := strings.TrimSpace(record.Status)
		if status == "" && !isActive(now, record.LastSeen) {
			status = "offline"
		}
		if status == "" {
			status = "-"
		}
		fmt.Fprintf(writer, "%s\t%s\t%s\n", record.Name, formatLastSeen(now, record.LastSeen), status)
	}
	if err := writer.Flush(); err != nil {
		return Exitf(ExitCodeFailure, "write output: %v", err)
	}
	return nil
}
