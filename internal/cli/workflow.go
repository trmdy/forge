// Package cli provides workflow management commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/workflows"
)

func init() {
	rootCmd.AddCommand(workflowCmd)

	workflowCmd.AddCommand(workflowListCmd)
	workflowCmd.AddCommand(workflowShowCmd)
	workflowCmd.AddCommand(workflowValidateCmd)
}

var workflowCmd = &cobra.Command{
	Use:     "workflow",
	Aliases: []string{"wf"},
	Short:   "Manage workflows",
	Long:    "List, inspect, and validate workflow definitions.",
}

var workflowListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List workflows",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		wsRepo := db.NewWorkspaceRepository(database)
		projectDir := resolveTemplateProjectDir(ctx, wsRepo)
		items, err := workflows.LoadWorkflowsFromSearchPaths(projectDir)
		if err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, items)
		}

		if len(items) == 0 {
			fmt.Println("No workflows found")
			return nil
		}

		rows := make([][]string, 0, len(items))
		for _, wf := range items {
			rows = append(rows, []string{
				wf.Name,
				fmt.Sprintf("%d", len(wf.Steps)),
				wf.Description,
				workflowSourcePath(wf.Source, projectDir),
			})
		}

		return writeTable(os.Stdout, []string{"NAME", "STEPS", "DESCRIPTION", "PATH"}, rows)
	},
}

var workflowShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show workflow details",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		wsRepo := db.NewWorkspaceRepository(database)
		projectDir := resolveTemplateProjectDir(ctx, wsRepo)
		wf, err := loadWorkflowByName(projectDir, args[0])
		if err != nil {
			return err
		}

		validated, err := workflows.ValidateWorkflow(wf)
		if err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, validated)
		}

		printWorkflow(validated, projectDir)
		return nil
	},
}

var workflowValidateCmd = &cobra.Command{
	Use:   "validate <name>",
	Short: "Validate a workflow",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		wsRepo := db.NewWorkspaceRepository(database)
		projectDir := resolveTemplateProjectDir(ctx, wsRepo)
		wf, err := loadWorkflowByName(projectDir, args[0])
		if err != nil {
			return err
		}

		validated, err := workflows.ValidateWorkflow(wf)
		if err != nil {
			return outputWorkflowValidation(validated, err)
		}

		return outputWorkflowValidation(validated, nil)
	},
}

type workflowValidationResult struct {
	Name   string                    `json:"name,omitempty"`
	Path   string                    `json:"path,omitempty"`
	Valid  bool                      `json:"valid"`
	Errors []workflows.WorkflowError `json:"errors,omitempty"`
}

func outputWorkflowValidation(wf *workflows.Workflow, err error) error {
	result := workflowValidationResult{}
	if wf != nil {
		result.Name = wf.Name
		result.Path = wf.Source
	}

	var list *workflows.ErrorList
	if err != nil && errors.As(err, &list) {
		result.Errors = list.Errors
	}

	result.Valid = err == nil

	if IsJSONOutput() || IsJSONLOutput() {
		if writeErr := WriteOutput(os.Stdout, result); writeErr != nil {
			return writeErr
		}
	} else {
		if result.Valid {
			fmt.Printf("Workflow valid: %s\n", result.Name)
		} else {
			fmt.Printf("Workflow invalid: %s\n", result.Name)
			if len(result.Errors) == 0 && err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
			} else {
				for _, item := range result.Errors {
					fmt.Fprintf(os.Stderr, "- %s\n", item.HumanString())
				}
			}
		}
	}

	if err != nil {
		return &ExitError{Code: 1, Err: err, Printed: true}
	}
	return nil
}

func loadWorkflowByName(projectDir, name string) (*workflows.Workflow, error) {
	clean, err := normalizeWorkflowName(name)
	if err != nil {
		return nil, err
	}

	if projectDir != "" {
		candidate := filepath.Join(projectDir, ".forge", "workflows", clean+".toml")
		if _, statErr := os.Stat(candidate); statErr == nil {
			return workflows.LoadWorkflow(candidate)
		}
	}

	items, err := workflows.LoadWorkflowsFromSearchPaths(projectDir)
	if err != nil {
		return nil, err
	}

	wf := findWorkflowByName(items, clean)
	if wf == nil {
		return nil, fmt.Errorf("workflow %q not found", clean)
	}

	return wf, nil
}

func normalizeWorkflowName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("workflow name is required")
	}
	trimmed = strings.TrimSuffix(trimmed, ".toml")
	if strings.Contains(trimmed, string(filepath.Separator)) || strings.Contains(trimmed, "..") {
		return "", fmt.Errorf("invalid workflow name %q", trimmed)
	}
	return trimmed, nil
}

func findWorkflowByName(items []*workflows.Workflow, name string) *workflows.Workflow {
	if name == "" {
		return nil
	}
	for _, wf := range items {
		if strings.EqualFold(wf.Name, name) {
			return wf
		}
	}
	return nil
}

func workflowSourcePath(source, projectDir string) string {
	if source == "" {
		return ""
	}
	if projectDir != "" {
		if rel, err := filepath.Rel(projectDir, source); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return source
}

func printWorkflow(wf *workflows.Workflow, projectDir string) {
	fmt.Printf("Workflow: %s\n", wf.Name)
	fmt.Printf("Source: %s\n", workflowSourcePath(wf.Source, projectDir))
	if wf.Version != "" {
		fmt.Printf("Version: %s\n", wf.Version)
	}
	if wf.Description != "" {
		fmt.Printf("Description: %s\n", wf.Description)
	}

	if len(wf.Inputs) > 0 {
		fmt.Printf("Inputs: %s\n", formatWorkflowMap(wf.Inputs))
	}
	if len(wf.Outputs) > 0 {
		fmt.Printf("Outputs: %s\n", formatWorkflowMap(wf.Outputs))
	}

	fmt.Println("\nSteps:")
	for i, step := range wf.Steps {
		fmt.Printf("  %d. %s\n", i+1, formatWorkflowStep(step))
		details := formatWorkflowStepDetails(wf, step)
		for _, line := range details {
			fmt.Printf("     %s\n", line)
		}
	}

	lines := workflowFlowchartLines(wf)
	if len(lines) > 0 {
		fmt.Println("\nFlow:")
		for _, line := range lines {
			fmt.Printf("  %s\n", line)
		}
	}
}

func formatWorkflowMap(values map[string]any) string {
	if len(values) == 0 {
		return "(none)"
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%v", key, values[key]))
	}
	return strings.Join(parts, ", ")
}

func formatWorkflowStep(step workflows.WorkflowStep) string {
	label := step.ID
	if label == "" {
		label = "(unnamed)"
	}
	label = fmt.Sprintf("%s [%s]", label, step.Type)
	if step.Name != "" {
		label = fmt.Sprintf("%s - %s", label, step.Name)
	}
	if len(step.DependsOn) > 0 {
		label = fmt.Sprintf("%s (depends_on: %s)", label, strings.Join(step.DependsOn, ", "))
	}
	return label
}

func formatWorkflowStepDetails(wf *workflows.Workflow, step workflows.WorkflowStep) []string {
	lines := make([]string, 0, 2)

	switch step.Type {
	case workflows.StepTypeAgent, workflows.StepTypeLoop, workflows.StepTypeHuman:
		prompt := workflows.ResolveStepPrompt(wf, step)
		if prompt.Inline != "" {
			lines = append(lines, "prompt: [inline]")
		} else if prompt.Path != "" {
			lines = append(lines, fmt.Sprintf("prompt: %s", prompt.Path))
		}
	case workflows.StepTypeBash:
		if step.Cmd != "" {
			lines = append(lines, fmt.Sprintf("cmd: %s", step.Cmd))
		}
	}

	return lines
}

func workflowFlowchartLines(wf *workflows.Workflow) []string {
	if wf == nil || len(wf.Steps) == 0 {
		return nil
	}

	order := make(map[string]int, len(wf.Steps))
	for i, step := range wf.Steps {
		if step.ID == "" {
			continue
		}
		order[step.ID] = i
	}

	outgoing := make(map[string][]string)
	incoming := make(map[string]int)

	for _, step := range wf.Steps {
		if step.ID == "" {
			continue
		}
		if _, ok := incoming[step.ID]; !ok {
			incoming[step.ID] = 0
		}
		for _, dep := range step.DependsOn {
			if dep == "" {
				continue
			}
			outgoing[dep] = append(outgoing[dep], step.ID)
			incoming[step.ID]++
		}
	}

	lines := make([]string, 0)
	for _, step := range wf.Steps {
		id := step.ID
		if id == "" {
			continue
		}
		targets := outgoing[id]
		if len(targets) == 0 {
			continue
		}
		sort.Slice(targets, func(i, j int) bool {
			return order[targets[i]] < order[targets[j]]
		})
		lines = append(lines, fmt.Sprintf("%s -> %s", id, strings.Join(targets, ", ")))
	}

	for _, step := range wf.Steps {
		id := step.ID
		if id == "" {
			continue
		}
		if incoming[id] == 0 && len(outgoing[id]) == 0 {
			lines = append(lines, id)
		}
	}

	return lines
}
