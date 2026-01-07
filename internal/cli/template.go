// Package cli provides template management commands.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/queue"
	"github.com/tOgg1/forge/internal/templates"
)

var (
	templateTags  []string
	templateAgent string
	templateVars  []string
)

func init() {
	rootCmd.AddCommand(templateCmd)

	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateShowCmd)
	templateCmd.AddCommand(templateAddCmd)
	templateCmd.AddCommand(templateEditCmd)
	templateCmd.AddCommand(templateRunCmd)
	templateCmd.AddCommand(templateDeleteCmd)

	templateListCmd.Flags().StringSliceVar(&templateTags, "tags", nil, "filter by tags (comma-separated or repeatable)")

	templateRunCmd.Flags().StringVarP(&templateAgent, "agent", "a", "", "agent ID or prefix")
	templateRunCmd.Flags().StringSliceVar(&templateVars, "var", nil, "template variable (key=value)")
}

var templateCmd = &cobra.Command{
	Use:     "template",
	Aliases: []string{"tmpl"},
	Short:   "Manage message templates",
	Long:    "Create, edit, and run reusable message templates.",
}

var templateListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List templates",
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
		items, err := templates.LoadTemplatesFromSearchPaths(projectDir)
		if err != nil {
			return err
		}

		filtered := filterTemplates(items, templateTags)

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, filtered)
		}

		if len(filtered) == 0 {
			fmt.Println("No templates found")
			return nil
		}

		userDir := userTemplateDir()
		projectTemplatesDir := ""
		if projectDir != "" {
			projectTemplatesDir = filepath.Join(projectDir, ".forge", "templates")
		}

		rows := make([][]string, 0, len(filtered))
		for _, tmpl := range filtered {
			rows = append(rows, []string{
				tmpl.Name,
				tmpl.Description,
				templateSourceLabel(tmpl.Source, userDir, projectTemplatesDir),
				strings.Join(tmpl.Tags, ","),
			})
		}

		return writeTable(os.Stdout, []string{"NAME", "DESCRIPTION", "SOURCE", "TAGS"}, rows)
	},
}

var templateShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show template details",
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
		items, err := templates.LoadTemplatesFromSearchPaths(projectDir)
		if err != nil {
			return err
		}

		tmpl := findTemplateByName(items, args[0])
		if tmpl == nil {
			return fmt.Errorf("template %q not found", args[0])
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, tmpl)
		}

		fmt.Printf("Template: %s\n", tmpl.Name)
		fmt.Printf("Source: %s\n", tmpl.Source)
		if tmpl.Description != "" {
			fmt.Printf("Description: %s\n", tmpl.Description)
		}
		if len(tmpl.Tags) > 0 {
			fmt.Printf("Tags: %s\n", strings.Join(tmpl.Tags, ","))
		}
		fmt.Println()
		fmt.Println("Message:")
		fmt.Println(indentBlock(tmpl.Message, "  "))

		if len(tmpl.Variables) == 0 {
			fmt.Println("\nVariables: (none)")
			return nil
		}

		fmt.Println("\nVariables:")
		for _, variable := range tmpl.Variables {
			line := fmt.Sprintf("- %s", variable.Name)
			if variable.Description != "" {
				line += ": " + variable.Description
			}
			if variable.Required {
				line += " (required)"
			}
			if variable.Default != "" {
				line += fmt.Sprintf(" [default: %s]", variable.Default)
			}
			fmt.Println(line)
		}

		return nil
	},
}

var templateAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new template",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := normalizeTemplateName(args[0])
		if err != nil {
			return err
		}

		path := templateFilePath(name)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("template %q already exists at %s", name, path)
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("failed to create templates directory: %w", err)
		}

		if err := os.WriteFile(path, []byte(templateSkeleton(name)), 0644); err != nil {
			return fmt.Errorf("failed to write template file: %w", err)
		}

		if err := openEditor(path); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]string{"path": path})
		}

		fmt.Printf("Template created: %s\n", path)
		return nil
	},
}

var templateEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit an existing template",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := normalizeTemplateName(args[0])
		if err != nil {
			return err
		}

		path := templateFilePath(name)
		if _, err := os.Stat(path); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("template %q not found in user templates", name)
			}
			return fmt.Errorf("failed to stat template file: %w", err)
		}

		if err := openEditor(path); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]string{"path": path})
		}

		fmt.Printf("Template updated: %s\n", path)
		return nil
	},
}

var templateRunCmd = &cobra.Command{
	Use:   "run <name>",
	Short: "Queue a template message",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		wsRepo := db.NewWorkspaceRepository(database)
		agentRepo := db.NewAgentRepository(database)
		queueRepo := db.NewQueueRepository(database)
		queueService := queue.NewService(queueRepo)

		projectDir := resolveTemplateProjectDir(ctx, wsRepo)
		templatesList, err := templates.LoadTemplatesFromSearchPaths(projectDir)
		if err != nil {
			return err
		}
		tmpl := findTemplateByName(templatesList, args[0])
		if tmpl == nil {
			return fmt.Errorf("template %q not found", args[0])
		}

		vars, err := parseTemplateVars(templateVars)
		if err != nil {
			return err
		}

		message, err := templates.RenderTemplate(tmpl, vars)
		if err != nil {
			return err
		}

		agent, err := resolveTemplateAgent(ctx, agentRepo, templateAgent)
		if err != nil {
			return err
		}

		payload, err := json.Marshal(models.MessagePayload{Text: message})
		if err != nil {
			return fmt.Errorf("failed to encode message: %w", err)
		}

		item := &models.QueueItem{
			AgentID: agent.ID,
			Type:    models.QueueItemTypeMessage,
			Status:  models.QueueItemStatusPending,
			Payload: payload,
		}

		if err := queueService.Enqueue(ctx, agent.ID, item); err != nil {
			return fmt.Errorf("failed to enqueue template: %w", err)
		}

		result := templateRunResult{
			Template: tmpl.Name,
			AgentID:  agent.ID,
			ItemID:   item.ID,
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, result)
		}

		fmt.Printf("Queued template %q for agent %s (item %s)\n", tmpl.Name, shortID(agent.ID), shortID(item.ID))
		return nil
	},
}

var templateDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a user template",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := normalizeTemplateName(args[0])
		if err != nil {
			return err
		}

		path := templateFilePath(name)
		if err := os.Remove(path); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("template %q not found in user templates", name)
			}
			return fmt.Errorf("failed to delete template: %w", err)
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]string{"deleted": name})
		}

		fmt.Printf("Deleted template %q\n", name)
		return nil
	},
}

type templateRunResult struct {
	Template string `json:"template"`
	AgentID  string `json:"agent_id"`
	ItemID   string `json:"item_id"`
}

func resolveTemplateProjectDir(ctx context.Context, wsRepo *db.WorkspaceRepository) string {
	if wsRepo == nil {
		return ""
	}

	resolved, err := ResolveWorkspaceContext(ctx, wsRepo, "")
	if err == nil && resolved != nil && resolved.WorkspaceID != "" {
		if ws, err := wsRepo.Get(ctx, resolved.WorkspaceID); err == nil {
			return ws.RepoPath
		}
	}

	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return getGitRoot(cwd)
}

func resolveTemplateAgent(ctx context.Context, agentRepo *db.AgentRepository, agentFlag string) (*models.Agent, error) {
	if agentFlag != "" {
		return findAgent(ctx, agentRepo, agentFlag)
	}

	resolved, err := RequireAgentContext(ctx, agentRepo, "", "")
	if err != nil {
		return nil, err
	}

	return agentRepo.Get(ctx, resolved.AgentID)
}

func normalizeTemplateName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("template name is required")
	}
	if strings.Contains(trimmed, string(filepath.Separator)) || strings.Contains(trimmed, "..") {
		return "", fmt.Errorf("invalid template name %q", trimmed)
	}
	return trimmed, nil
}

func templateFilePath(name string) string {
	return filepath.Join(userTemplateDir(), name+".yaml")
}

func userTemplateDir() string {
	return filepath.Join(getConfigDir(), "templates")
}

func templateSkeleton(name string) string {
	return fmt.Sprintf("name: %s\ndescription: Describe this template\nmessage: |\n  Write the instruction here.\n", name)
}

func openEditor(path string) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = os.Getenv("VISUAL")
	}
	if editor == "" {
		return errors.New("EDITOR is not set (set $EDITOR or use --file/--stdin)")
	}

	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return errors.New("EDITOR is empty")
	}

	editorCmd := exec.Command(parts[0], append(parts[1:], path)...)
	editorCmd.Stdin = os.Stdin
	editorCmd.Stdout = os.Stdout
	editorCmd.Stderr = os.Stderr

	if err := editorCmd.Run(); err != nil {
		return fmt.Errorf("failed to run editor %q: %w", parts[0], err)
	}

	return nil
}

func parseTemplateVars(values []string) (map[string]string, error) {
	vars := make(map[string]string)
	for _, entry := range values {
		for _, part := range splitCommaList(entry) {
			if part == "" {
				continue
			}
			key, value, ok := strings.Cut(part, "=")
			if !ok {
				return nil, fmt.Errorf("invalid variable %q (expected key=value)", part)
			}
			key = strings.TrimSpace(key)
			if key == "" {
				return nil, fmt.Errorf("invalid variable %q (empty key)", part)
			}
			vars[key] = value
		}
	}
	return vars, nil
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	trimmed := make([]string, 0, len(parts))
	for _, part := range parts {
		entry := strings.TrimSpace(part)
		if entry != "" {
			trimmed = append(trimmed, entry)
		}
	}
	return trimmed
}

func filterTemplates(items []*templates.Template, tags []string) []*templates.Template {
	if len(tags) == 0 {
		return items
	}

	wanted := make(map[string]struct{})
	for _, entry := range tags {
		for _, tag := range splitCommaList(entry) {
			wanted[strings.ToLower(tag)] = struct{}{}
		}
	}

	filtered := make([]*templates.Template, 0, len(items))
	for _, tmpl := range items {
		if tmpl == nil {
			continue
		}
		if len(tmpl.Tags) == 0 {
			continue
		}
		for _, tag := range tmpl.Tags {
			if _, ok := wanted[strings.ToLower(tag)]; ok {
				filtered = append(filtered, tmpl)
				break
			}
		}
	}

	return filtered
}

func findTemplateByName(items []*templates.Template, name string) *templates.Template {
	for _, tmpl := range items {
		if tmpl == nil {
			continue
		}
		if strings.EqualFold(tmpl.Name, name) {
			return tmpl
		}
	}
	return nil
}

func templateSourceLabel(source, userDir, projectDir string) string {
	if source == "builtin" {
		return "builtin"
	}
	if userDir != "" && isWithinDir(source, userDir) {
		return "user"
	}
	if projectDir != "" && isWithinDir(source, projectDir) {
		return "project"
	}
	return "file"
}

func isWithinDir(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func indentBlock(text, prefix string) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}
