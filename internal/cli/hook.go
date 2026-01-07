// Package cli provides hook management commands.
package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/hooks"
	"github.com/tOgg1/forge/internal/models"
)

var (
	hookCommand  string
	hookURL      string
	hookHeaders  []string
	hookTypes    string
	hookEntity   string
	hookEntityID string
	hookTimeout  string
	hookDisabled bool
)

func init() {
	rootCmd.AddCommand(hookCmd)
	hookCmd.AddCommand(hookOnEventCmd)

	hookOnEventCmd.Flags().StringVar(&hookCommand, "cmd", "", "command to execute for matching events")
	hookOnEventCmd.Flags().StringVar(&hookURL, "url", "", "webhook URL to POST matching events")
	hookOnEventCmd.Flags().StringSliceVar(&hookHeaders, "header", nil, "webhook header (key=value)")
	hookOnEventCmd.Flags().StringVar(&hookTypes, "type", "", "filter by event type (comma-separated)")
	hookOnEventCmd.Flags().StringVar(&hookEntity, "entity-type", "", "filter by entity type (node, workspace, agent, queue, account, system)")
	hookOnEventCmd.Flags().StringVar(&hookEntityID, "entity-id", "", "filter by entity ID")
	hookOnEventCmd.Flags().StringVar(&hookTimeout, "timeout", hooks.DefaultTimeout.String(), "hook execution timeout (0 to disable)")
	hookOnEventCmd.Flags().BoolVar(&hookDisabled, "disabled", false, "register hook as disabled")
}

var hookCmd = &cobra.Command{
	Use:   "hook",
	Short: "Manage event hooks",
	Long:  "Register scripts or webhooks that run when Forge emits events.",
}

var hookOnEventCmd = &cobra.Command{
	Use:   "on-event",
	Short: "Register a hook for events",
	Long: `Register a command or webhook to run when events match the filter.

The hook is stored on disk and will be loaded whenever Forge executes commands
that publish events.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		command := strings.TrimSpace(hookCommand)
		url := strings.TrimSpace(hookURL)

		if (command == "") == (url == "") {
			return fmt.Errorf("exactly one of --cmd or --url is required")
		}

		if hookTimeout != "" {
			if hookTimeout != "0" {
				if _, err := time.ParseDuration(hookTimeout); err != nil {
					return fmt.Errorf("invalid --timeout value: %w", err)
				}
			}
		}

		eventTypes, err := parseEventTypes(hookTypes)
		if err != nil {
			return err
		}

		entityTypes, err := parseEntityTypes(hookEntity)
		if err != nil {
			return err
		}

		headers, err := parseHeaders(hookHeaders)
		if err != nil {
			return err
		}

		hook := hooks.Hook{
			Kind:        hooks.KindCommand,
			Command:     command,
			URL:         url,
			Headers:     headers,
			EventTypes:  eventTypes,
			EntityTypes: entityTypes,
			EntityID:    strings.TrimSpace(hookEntityID),
			Enabled:     !hookDisabled,
			Timeout:     strings.TrimSpace(hookTimeout),
		}
		if url != "" {
			hook.Kind = hooks.KindWebhook
		}

		store := hooks.NewStore(hookStorePath())
		stored, err := store.Add(hook)
		if err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, stored)
		}

		fmt.Printf("Hook registered: %s\n", stored.ID)
		fmt.Printf("Store: %s\n", store.Path())
		return nil
	},
}

func parseEntityTypes(raw string) ([]models.EntityType, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}

	entity := models.EntityType(trimmed)
	switch entity {
	case models.EntityTypeNode, models.EntityTypeWorkspace, models.EntityTypeAgent,
		models.EntityTypeQueue, models.EntityTypeAccount, models.EntityTypeSystem:
		return []models.EntityType{entity}, nil
	default:
		return nil, fmt.Errorf("invalid entity type: %s", trimmed)
	}
}

func parseHeaders(values []string) (map[string]string, error) {
	if len(values) == 0 {
		return map[string]string{}, nil
	}

	headers := make(map[string]string, len(values))
	for _, raw := range values {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid header %q (expected key=value)", raw)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid header %q (empty key)", raw)
		}
		headers[key] = value
	}
	return headers, nil
}

func hookStorePath() string {
	if cfg := GetConfig(); cfg != nil {
		if strings.TrimSpace(cfg.Global.ConfigDir) != "" {
			return filepath.Join(cfg.Global.ConfigDir, "hooks.json")
		}
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return "hooks.json"
	}

	return filepath.Join(homeDir, ".config", "forge", "hooks.json")
}
