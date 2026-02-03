package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/config"
	"github.com/tOgg1/forge/internal/skills"
)

var (
	skillsBootstrapForce bool
	skillsBootstrapPath  string
	skillsBootstrapAll   bool
)

func init() {
	rootCmd.AddCommand(skillsCmd)
	skillsCmd.AddCommand(skillsBootstrapCmd)

	skillsBootstrapCmd.Flags().BoolVarP(&skillsBootstrapForce, "force", "f", false, "overwrite existing skill files")
	skillsBootstrapCmd.Flags().StringVar(&skillsBootstrapPath, "path", "", "skills source directory (default: .agent-skills in repo if present)")
	skillsBootstrapCmd.Flags().BoolVar(&skillsBootstrapAll, "all-profiles", false, "install for all profiles (not just default pool)")
}

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "Manage workspace skills",
}

var skillsBootstrapCmd = &cobra.Command{
	Use:   "bootstrap",
	Short: "Bootstrap repo skills and install to configured harnesses",
	Long: `Install skills into harness-specific locations based on configured profiles.

By default, this installs the repo .agent-skills if present, falling back to
the embedded skill set. Use --path to install from a custom skill source
directory.`,
	RunE: runSkillsBootstrap,
}

type skillsBootstrapResult struct {
	Source    string                 `json:"source"`
	Installed []skills.InstallResult `json:"installed"`
}

func runSkillsBootstrap(cmd *cobra.Command, args []string) error {
	repoPath, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to resolve working directory: %w", err)
	}

	config := GetConfig()
	if config == nil {
		return fmt.Errorf("config not loaded")
	}

	profiles := selectProfilesForSkills(config, skillsBootstrapAll)
	if len(profiles) == 0 {
		return fmt.Errorf("no profiles configured for skills install")
	}

	source := strings.TrimSpace(skillsBootstrapPath)
	var installed []skills.InstallResult
	if source != "" {
		if !filepath.IsAbs(source) {
			source = filepath.Join(repoPath, source)
		}
		installed, err = skills.InstallToHarnesses(repoPath, source, profiles, skillsBootstrapForce)
	} else {
		repoSkills := filepath.Join(repoPath, ".agent-skills")
		if info, statErr := os.Stat(repoSkills); statErr == nil && info.IsDir() {
			source = repoSkills
			installed, err = skills.InstallToHarnesses(repoPath, repoSkills, profiles, skillsBootstrapForce)
		} else {
			source = "builtin"
			installed, err = skills.InstallBuiltinToHarnesses(repoPath, profiles, skillsBootstrapForce)
		}
	}
	if err != nil {
		return err
	}

	output := skillsBootstrapResult{
		Source:    source,
		Installed: installed,
	}

	if IsJSONOutput() || IsJSONLOutput() {
		data, err := json.Marshal(output)
		if err != nil {
			return fmt.Errorf("failed to marshal output: %w", err)
		}
		_, _ = os.Stdout.Write(data)
		_, _ = io.WriteString(os.Stdout, "\n")
		return nil
	}

	fmt.Printf("Skills source: %s\n", output.Source)

	fmt.Println("Installed:")
	for _, item := range output.Installed {
		fmt.Printf("  - %s (harnesses: %s)\n", item.Dest, strings.Join(item.Harnesses, ", "))
		if len(item.Created) > 0 {
			fmt.Printf("    created: %d files\n", len(item.Created))
		}
		if len(item.Skipped) > 0 {
			fmt.Printf("    skipped: %d files\n", len(item.Skipped))
		}
	}

	return nil
}

func selectProfilesForSkills(cfg *config.Config, allProfiles bool) []config.ProfileConfig {
	if allProfiles {
		return cfg.Profiles
	}

	if cfg.DefaultPool == "" || len(cfg.Pools) == 0 {
		return cfg.Profiles
	}

	var poolProfiles []string
	for _, pool := range cfg.Pools {
		if pool.Name == cfg.DefaultPool {
			poolProfiles = append(poolProfiles, pool.Profiles...)
			break
		}
	}
	if len(poolProfiles) == 0 {
		return cfg.Profiles
	}

	lookup := map[string]config.ProfileConfig{}
	for _, profile := range cfg.Profiles {
		lookup[profile.Name] = profile
	}

	selected := make([]config.ProfileConfig, 0, len(poolProfiles))
	for _, name := range poolProfiles {
		if profile, ok := lookup[name]; ok {
			selected = append(selected, profile)
		}
	}

	if len(selected) == 0 {
		return cfg.Profiles
	}

	return selected
}
