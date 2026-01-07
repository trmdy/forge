// Package cli provides the Forge command-line interface.
package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
)

func init() {
	rootCmd.AddCommand(auditCmd)

	auditCmd.Flags().StringVar(&auditEventTypes, "type", "", "filter by event type (comma-separated)")
	auditCmd.Flags().StringVar(&auditActionTypes, "action", "", "alias for --type")
	auditCmd.Flags().StringVar(&auditEntityType, "entity-type", "", "filter by entity type (node, workspace, agent, queue, account, system)")
	auditCmd.Flags().StringVar(&auditEntityID, "entity-id", "", "filter by entity ID")
	auditCmd.Flags().StringVar(&auditUntil, "until", "", "filter events before a time (same format as --since)")
	auditCmd.Flags().StringVar(&auditCursor, "cursor", "", "start after this event ID")
	auditCmd.Flags().IntVar(&auditLimit, "limit", 100, "max number of events to return")
}

var (
	auditEventTypes  string
	auditActionTypes string
	auditEntityType  string
	auditEntityID    string
	auditUntil       string
	auditCursor      string
	auditLimit       int
)

var auditCmd = &cobra.Command{
	Use:   "audit",
	Short: "View the Forge audit log",
	Long: `View the audit log with filters for time range, entity, and action.

Examples:
  forge audit --since 1h
  forge audit --type agent.state_changed --entity-type agent
  forge audit --action message.dispatched --limit 200`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		eventRepo := db.NewEventRepository(database)

		if strings.TrimSpace(auditEventTypes) != "" && strings.TrimSpace(auditActionTypes) != "" {
			return fmt.Errorf("use either --type or --action, not both")
		}

		rawTypes := strings.TrimSpace(auditEventTypes)
		if rawTypes == "" {
			rawTypes = strings.TrimSpace(auditActionTypes)
		}

		eventTypes, err := parseEventTypes(rawTypes)
		if err != nil {
			return err
		}

		since, err := GetSinceTime()
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}
		until, err := ParseSince(auditUntil)
		if err != nil {
			return fmt.Errorf("invalid --until value: %w", err)
		}
		if since != nil && until != nil && since.After(*until) {
			return fmt.Errorf("--since must be before --until")
		}

		if auditLimit <= 0 {
			auditLimit = 100
		}

		query := db.EventQuery{
			Cursor: auditCursor,
			Since:  since,
			Until:  until,
			Limit:  auditLimit,
		}

		if strings.TrimSpace(auditEntityType) != "" {
			entityType := models.EntityType(strings.TrimSpace(auditEntityType))
			query.EntityType = &entityType
		}
		if strings.TrimSpace(auditEntityID) != "" {
			entityID := strings.TrimSpace(auditEntityID)
			query.EntityID = &entityID
		}
		if len(eventTypes) == 1 {
			eventType := eventTypes[0]
			query.Type = &eventType
		}

		page, err := eventRepo.Query(ctx, query)
		if err != nil {
			return fmt.Errorf("failed to query audit log: %w", err)
		}

		events := filterEventsByType(page.Events, eventTypes)
		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, events)
		}

		writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
		fmt.Fprintln(writer, "TIME\tTYPE\tENTITY\tID")
		for _, event := range events {
			if event == nil {
				continue
			}
			fmt.Fprintf(
				writer,
				"%s\t%s\t%s\t%s\n",
				event.Timestamp.UTC().Format("2006-01-02 15:04:05"),
				event.Type,
				event.EntityType,
				event.EntityID,
			)
		}
		if err := writer.Flush(); err != nil {
			return err
		}

		if page.NextCursor != "" {
			fmt.Fprintf(os.Stdout, "\nNext cursor: %s\n", page.NextCursor)
		}
		if len(events) == 0 {
			fmt.Fprintln(os.Stdout, "No events matched the current filters.")
		}
		return nil
	},
}
