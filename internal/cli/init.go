// Package cli provides the init command for repo scaffolding.
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var (
	initForce          bool
	initPromptsFrom    string
	initNoCreatePrompt bool
)

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "overwrite existing scaffold files")
	initCmd.Flags().StringVar(&initPromptsFrom, "prompts-from", "", "import prompt files from directory")
	initCmd.Flags().BoolVar(&initNoCreatePrompt, "no-create-prompt", false, "skip creating PROMPT.md")
}

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a repo for Forge loops",
	Long: `Initialize a repository with the .forge/ scaffolding and prompts.

This creates:
  - .forge/forge.yaml
  - .forge/prompts/
  - .forge/templates/
  - .forge/sequences/
  - .forge/workflows/
  - .forge/ledgers/

Also ensures .fmail/ is in .gitignore (runtime messaging state).
Optionally creates PROMPT.md if missing.`,
	Example: `  forge init
  forge init --prompts-from ./prompts
  forge init --no-create-prompt`,
	RunE: runInit,
}

type initResult struct {
	RepoPath string   `json:"repo_path"`
	Created  []string `json:"created"`
	Skipped  []string `json:"skipped"`
}

func runInit(cmd *cobra.Command, args []string) error {
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}

	created := make([]string, 0)
	skipped := make([]string, 0)

	forgeDir := filepath.Join(repoPath, ".forge")
	paths := []string{
		filepath.Join(forgeDir, "prompts"),
		filepath.Join(forgeDir, "templates"),
		filepath.Join(forgeDir, "sequences"),
		filepath.Join(forgeDir, "workflows"),
		filepath.Join(forgeDir, "ledgers"),
	}

	for _, path := range paths {
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create %s: %w", path, err)
		}
		created = append(created, path)
	}

	forgeConfigPath := filepath.Join(forgeDir, "forge.yaml")
	createdConfig, err := writeFileIfMissing(forgeConfigPath, []byte(defaultForgeConfig), initForce)
	if err != nil {
		return err
	}
	if createdConfig {
		created = append(created, forgeConfigPath)
	} else {
		skipped = append(skipped, forgeConfigPath)
	}

	if initPromptsFrom != "" {
		copied, err := copyPromptDir(initPromptsFrom, filepath.Join(forgeDir, "prompts"))
		if err != nil {
			return err
		}
		created = append(created, copied...)
	}

	// Ensure .fmail/ is gitignored (runtime messaging state)
	gitignorePath := filepath.Join(repoPath, ".gitignore")
	if addedGitignore, err := ensureGitignoreEntry(gitignorePath, ".fmail/"); err != nil {
		return err
	} else if addedGitignore {
		created = append(created, gitignorePath+" (.fmail/ entry)")
	}

	promptPath := filepath.Join(repoPath, "PROMPT.md")
	if !initNoCreatePrompt {
		createdPrompt, err := writeFileIfMissing(promptPath, []byte(defaultPrompt), initForce)
		if err != nil {
			return err
		}
		if createdPrompt {
			created = append(created, promptPath)
		} else {
			skipped = append(skipped, promptPath)
		}
	}

	result := initResult{
		RepoPath: repoPath,
		Created:  created,
		Skipped:  skipped,
	}

	if IsJSONOutput() || IsJSONLOutput() {
		data, err := json.Marshal(result)
		if err != nil {
			return fmt.Errorf("failed to marshal init output: %w", err)
		}
		if _, err := os.Stdout.Write(data); err != nil {
			return fmt.Errorf("failed to write init output: %w", err)
		}
		if _, err := os.Stdout.Write([]byte("\n")); err != nil {
			return fmt.Errorf("failed to write init output: %w", err)
		}
		return nil
	}

	fmt.Printf("Initialized Forge scaffolding in %s\n", repoPath)
	if len(created) > 0 {
		fmt.Println("Created:")
		for _, path := range created {
			fmt.Printf("  - %s\n", path)
		}
	}
	if len(skipped) > 0 {
		fmt.Println("Skipped:")
		for _, path := range skipped {
			fmt.Printf("  - %s\n", path)
		}
	}

	return nil
}

func writeFileIfMissing(path string, data []byte, force bool) (bool, error) {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return false, nil
		}
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return false, fmt.Errorf("failed to write %s: %w", path, err)
	}
	return true, nil
}

func copyPromptDir(srcDir, destDir string) ([]string, error) {
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read prompts directory: %w", err)
	}

	created := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		srcPath := filepath.Join(srcDir, entry.Name())
		destPath := filepath.Join(destDir, entry.Name())

		srcFile, err := os.Open(srcPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open prompt %s: %w", srcPath, err)
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return nil, fmt.Errorf("failed to create prompt %s: %w", destPath, err)
		}
		if _, err := io.Copy(destFile, srcFile); err != nil {
			_ = destFile.Close()
			return nil, fmt.Errorf("failed to copy prompt %s: %w", srcPath, err)
		}
		if err := destFile.Close(); err != nil {
			return nil, fmt.Errorf("failed to close prompt %s: %w", destPath, err)
		}

		created = append(created, destPath)
	}

	return created, nil
}

// ensureGitignoreEntry appends an entry to .gitignore if not already present.
// Returns true if the entry was added, false if already present or file doesn't exist.
func ensureGitignoreEntry(gitignorePath, entry string) (bool, error) {
	content, err := os.ReadFile(gitignorePath)
	if err != nil {
		if os.IsNotExist(err) {
			// No .gitignore exists, create one with the entry
			if err := os.WriteFile(gitignorePath, []byte(entry+"\n"), 0644); err != nil {
				return false, fmt.Errorf("failed to create .gitignore: %w", err)
			}
			return true, nil
		}
		return false, fmt.Errorf("failed to read .gitignore: %w", err)
	}

	// Check if entry already exists (exact line match)
	lines := splitLines(string(content))
	for _, line := range lines {
		if line == entry {
			return false, nil
		}
	}

	// Append entry with proper newline handling
	newContent := string(content)
	if len(newContent) > 0 && newContent[len(newContent)-1] != '\n' {
		newContent += "\n"
	}
	newContent += entry + "\n"

	if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
		return false, fmt.Errorf("failed to update .gitignore: %w", err)
	}
	return true, nil
}

// splitLines splits content into lines, handling both \n and \r\n.
func splitLines(content string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			line := content[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

const defaultForgeConfig = `# Forge loop config
# This file is committed with the repo.

default_prompt: PROMPT.md

# Optional ledger settings.
ledger:
  git_status: false
  git_diff_stat: false
`

const defaultPrompt = `# Prompt

Describe the task you want the loop to perform.
`
