// Package cli provides status summary commands.
package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/workspace"
)

const (
	statusWatchInterval = 1 * time.Second
	statusAlertLimit    = 5
)

func init() {
	rootCmd.AddCommand(statusCmd)
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show fleet status summary",
	Long:  "Show a fleet summary: node counts, workspace counts, agent state breakdown, and top alerts.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := MustBeJSONLForWatch(); err != nil {
			return err
		}

		ctx := context.Background()
		if IsWatchMode() {
			return streamStatus(ctx)
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		summary, err := buildStatusSummary(ctx, database)
		if err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, summary)
		}

		return writeStatusHuman(summary)
	},
}

type StatusSummary struct {
	Timestamp  time.Time    `json:"timestamp"`
	Nodes      NodeSummary  `json:"nodes"`
	Workspaces int          `json:"workspaces"`
	Agents     AgentSummary `json:"agents"`
	Alerts     AlertSummary `json:"alerts"`
}

type NodeSummary struct {
	Total   int `json:"total"`
	Online  int `json:"online"`
	Offline int `json:"offline"`
	Unknown int `json:"unknown"`
}

type AgentSummary struct {
	Total   int                       `json:"total"`
	ByState map[models.AgentState]int `json:"by_state"`
}

type AlertSummary struct {
	Total int            `json:"total"`
	Items []models.Alert `json:"items,omitempty"`
}

func streamStatus(ctx context.Context) error {
	database, err := openDatabase()
	if err != nil {
		return err
	}
	defer database.Close()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigChan:
			cancel()
		case <-ctx.Done():
		}
	}()

	ticker := time.NewTicker(statusWatchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			summary, err := buildStatusSummary(ctx, database)
			if err != nil {
				return err
			}
			if err := WriteOutput(os.Stdout, summary); err != nil {
				return err
			}
		}
	}
}

func buildStatusSummary(ctx context.Context, database *db.DB) (*StatusSummary, error) {
	status, err := buildExportStatus(ctx, database)
	if err != nil {
		return nil, err
	}

	nodes := status.Nodes
	nodeSummary := NodeSummary{Total: len(nodes)}
	for _, node := range nodes {
		switch node.Status {
		case models.NodeStatusOnline:
			nodeSummary.Online++
		case models.NodeStatusOffline:
			nodeSummary.Offline++
		default:
			nodeSummary.Unknown++
		}
	}

	agentSummary := AgentSummary{
		Total:   len(status.Agents),
		ByState: make(map[models.AgentState]int),
	}
	for _, agent := range status.Agents {
		agentSummary.ByState[agent.State]++
	}

	alerts := status.Alerts
	if len(alerts) == 0 {
		alerts = workspace.BuildAlerts(status.Agents)
	}
	topAlerts := selectTopAlerts(alerts, statusAlertLimit)

	return &StatusSummary{
		Timestamp:  time.Now().UTC(),
		Nodes:      nodeSummary,
		Workspaces: len(status.Workspaces),
		Agents:     agentSummary,
		Alerts: AlertSummary{
			Total: len(alerts),
			Items: topAlerts,
		},
	}, nil
}

func selectTopAlerts(alerts []models.Alert, limit int) []models.Alert {
	if len(alerts) == 0 || limit <= 0 {
		return nil
	}

	sorted := make([]models.Alert, len(alerts))
	copy(sorted, alerts)

	sort.Slice(sorted, func(i, j int) bool {
		ai := alertSeverityRank(sorted[i].Severity)
		aj := alertSeverityRank(sorted[j].Severity)
		if ai != aj {
			return ai > aj
		}
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	if len(sorted) > limit {
		return sorted[:limit]
	}
	return sorted
}

func alertSeverityRank(sev models.AlertSeverity) int {
	switch sev {
	case models.AlertSeverityCritical:
		return 4
	case models.AlertSeverityError:
		return 3
	case models.AlertSeverityWarning:
		return 2
	case models.AlertSeverityInfo:
		return 1
	default:
		return 0
	}
}

func writeStatusHuman(summary *StatusSummary) error {
	if summary == nil {
		return fmt.Errorf("status summary is nil")
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	fmt.Fprintf(writer, "Timestamp:\t%s\n", summary.Timestamp.Format(time.RFC3339))
	fmt.Fprintf(
		writer,
		"Nodes:\t%d (online %d, offline %d, unknown %d)\n",
		summary.Nodes.Total,
		summary.Nodes.Online,
		summary.Nodes.Offline,
		summary.Nodes.Unknown,
	)
	fmt.Fprintf(writer, "Workspaces:\t%d\n", summary.Workspaces)
	fmt.Fprintf(writer, "Agents:\t%d\n", summary.Agents.Total)
	fmt.Fprintf(writer, "Agent states:\t%s\n", formatAgentStateCounts(summary.Agents.ByState))
	fmt.Fprintf(writer, "Alerts:\t%d\n", summary.Alerts.Total)
	if err := writer.Flush(); err != nil {
		return err
	}

	if len(summary.Alerts.Items) > 0 {
		fmt.Fprintln(os.Stdout, "Top alerts:")
		for _, alert := range summary.Alerts.Items {
			fmt.Fprintf(os.Stdout, "- [%s] %s", alert.Severity, alert.Message)
			if alert.AgentID != "" {
				fmt.Fprintf(os.Stdout, " (agent %s)", alert.AgentID)
			}
			fmt.Fprintln(os.Stdout)
		}
	}

	return nil
}

func formatAgentStateCounts(counts map[models.AgentState]int) string {
	order := []models.AgentState{
		models.AgentStateWorking,
		models.AgentStateIdle,
		models.AgentStateAwaitingApproval,
		models.AgentStateRateLimited,
		models.AgentStateError,
		models.AgentStatePaused,
		models.AgentStateStarting,
		models.AgentStateStopped,
	}

	parts := make([]string, 0, len(order))
	for _, state := range order {
		parts = append(parts, fmt.Sprintf("%s=%d", state, counts[state]))
	}
	return strings.Join(parts, " ")
}
