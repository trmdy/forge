package fmail

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func newSendCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send <topic|@agent> [message]",
		Short: "Send a message to a topic or agent",
		Args:  argsRange(1, 2),
		RunE:  runSend,
	}
	cmd.Flags().StringP("file", "f", "", "Read message from file")
	cmd.Flags().StringP("reply-to", "r", "", "Reference a previous message ID")
	cmd.Flags().StringP("priority", "p", "normal", "Set priority: low, normal, high")
	cmd.Flags().StringSliceP("tag", "t", nil, "Add tags (repeatable or comma-separated)")
	cmd.Flags().Bool("json", false, "Output message as JSON")
	return cmd
}

func newLogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "log [topic|@agent]",
		Short: "View recent messages",
		Args:  argsMax(1),
		RunE:  runLog,
	}
	cmd.Flags().IntP("limit", "n", 20, "Max messages to show")
	cmd.Flags().String("since", "", "Filter by time window")
	cmd.Flags().String("from", "", "Filter by sender")
	cmd.Flags().BoolP("follow", "f", false, "Stream new messages")
	cmd.Flags().Bool("allow-other-dm", false, "Allow reading another agent's DM inbox")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func newWatchCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "watch [topic|@agent]",
		Short: "Stream messages as they arrive",
		Args:  argsMax(1),
		RunE:  runWatch,
	}
	cmd.Flags().Duration("timeout", 0, "Maximum wait time before exiting")
	cmd.Flags().IntP("count", "c", 0, "Exit after receiving N messages")
	cmd.Flags().Bool("allow-other-dm", false, "Allow watching another agent's DM inbox")
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func newWhoCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "who",
		Short: "List known agents",
		Args:  argsMax(0),
		RunE:  runWho,
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func newStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status [message]",
		Short: "Show or set your status",
		Args:  argsMax(1),
		RunE:  runStatus,
	}
	cmd.Flags().Bool("clear", false, "Clear your status")
	return cmd
}

func newTopicsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "topics",
		Short: "List topics with activity",
		Args:  argsMax(0),
		RunE:  runTopics,
	}
	cmd.Flags().Bool("json", false, "Output as JSON")
	return cmd
}

func newGCCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "gc",
		Short: "Remove old messages",
		Args:  argsMax(0),
		RunE:  runGC,
	}
	cmd.Flags().Int("days", 7, "Remove messages older than N days")
	cmd.Flags().Bool("dry-run", false, "Show what would be removed")
	return cmd
}

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize a project mailbox",
		Args:  argsMax(0),
		RunE:  runInit,
	}
	cmd.Flags().String("project", "", "Explicit project ID")
	return cmd
}

func runNotImplemented(cmd *cobra.Command, args []string) error {
	_, err := EnsureRuntime(cmd)
	if err != nil {
		return err
	}
	return Exitf(ExitCodeFailure, "%s not implemented", cmd.Name())
}

var _ = runNotImplemented

func argsRange(min, max int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) < min || len(args) > max {
			return usageError(cmd, "expected %d-%d args, got %d", min, max, len(args))
		}
		return nil
	}
}

func argsMax(max int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > max {
			return usageError(cmd, "expected at most %d args, got %d", max, len(args))
		}
		return nil
	}
}

func usageError(cmd *cobra.Command, format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(cmd.ErrOrStderr(), "Error: %s\n\n", msg)
	_ = cmd.Usage()
	return &ExitError{Code: ExitCodeUsage, Err: errors.New(msg), Printed: true}
}
