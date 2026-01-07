package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(promptCmd)

	promptCmd.AddCommand(promptListCmd)
	promptCmd.AddCommand(promptAddCmd)
	promptCmd.AddCommand(promptEditCmd)
	promptCmd.AddCommand(promptSetDefaultCmd)
}

var promptCmd = &cobra.Command{
	Use:   "prompt",
	Short: "Manage loop prompts",
}

var promptListCmd = &cobra.Command{
	Use:     "ls",
	Aliases: []string{"list"},
	Short:   "List prompts",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		repoPath, err := resolveRepoPath("")
		if err != nil {
			return err
		}

		promptsDir := filepath.Join(repoPath, ".forge", "prompts")
		entries, err := os.ReadDir(promptsDir)
		if err != nil {
			if os.IsNotExist(err) {
				if IsJSONOutput() || IsJSONLOutput() {
					return WriteOutput(os.Stdout, []string{})
				}
				fmt.Fprintln(os.Stdout, "No prompts found")
				return nil
			}
			return err
		}

		prompts := make([]string, 0)
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			if strings.ToLower(filepath.Ext(entry.Name())) != ".md" {
				continue
			}
			name := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			prompts = append(prompts, name)
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, prompts)
		}

		if len(prompts) == 0 {
			fmt.Fprintln(os.Stdout, "No prompts found")
			return nil
		}

		for _, name := range prompts {
			fmt.Fprintln(os.Stdout, name)
		}
		return nil
	},
}

var promptAddCmd = &cobra.Command{
	Use:   "add <name> <path>",
	Short: "Add a prompt",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoPath, err := resolveRepoPath("")
		if err != nil {
			return err
		}

		name := args[0]
		source := args[1]
		destDir := filepath.Join(repoPath, ".forge", "prompts")
		if err := os.MkdirAll(destDir, 0o755); err != nil {
			return err
		}

		dest := filepath.Join(destDir, name+".md")
		if err := copyFile(source, dest); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]string{"prompt": name, "path": dest})
		}

		fmt.Fprintf(os.Stdout, "Prompt %q added\n", name)
		return nil
	},
}

var promptEditCmd = &cobra.Command{
	Use:   "edit <name>",
	Short: "Edit a prompt",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoPath, err := resolveRepoPath("")
		if err != nil {
			return err
		}

		path := filepath.Join(repoPath, ".forge", "prompts", args[0]+".md")
		if _, err := os.Stat(path); err != nil {
			return fmt.Errorf("prompt not found: %s", args[0])
		}

		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vi"
		}

		cmdEdit := exec.Command(editor, path)
		cmdEdit.Stdin = os.Stdin
		cmdEdit.Stdout = os.Stdout
		cmdEdit.Stderr = os.Stderr
		if err := cmdEdit.Run(); err != nil {
			return err
		}

		if IsQuiet() {
			return nil
		}

		fmt.Fprintf(os.Stdout, "Prompt %q updated\n", args[0])
		return nil
	},
}

var promptSetDefaultCmd = &cobra.Command{
	Use:   "set-default <name>",
	Short: "Set the default prompt",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoPath, err := resolveRepoPath("")
		if err != nil {
			return err
		}

		source := filepath.Join(repoPath, ".forge", "prompts", args[0]+".md")
		if _, err := os.Stat(source); err != nil {
			return fmt.Errorf("prompt not found: %s", args[0])
		}
		dest := filepath.Join(repoPath, ".forge", "prompts", "default.md")

		if err := copyFile(source, dest); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]string{"default_prompt": args[0]})
		}

		fmt.Fprintf(os.Stdout, "Default prompt set to %q\n", args[0])
		return nil
	},
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
