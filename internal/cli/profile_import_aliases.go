package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/harness"
	"github.com/tOgg1/forge/internal/models"
)

var profileInitCmd = &cobra.Command{
	Use:     "init [alias...]",
	Aliases: []string{"import-aliases"},
	Short:   "Initialize profiles from shell aliases",
	Args:    cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		aliasNames := args
		aliasFiles := resolveAliasFiles()
		usingDefaultAliasFiles := strings.TrimSpace(os.Getenv("FORGE_ALIAS_FILE")) == ""
		shellPath := resolveAliasShell()
		aliasEntries := collectAliasEntries(aliasFiles)
		aliasOutputs := make(map[string]string, len(aliasEntries))
		for _, entry := range aliasEntries {
			aliasOutputs[entry.Name] = entry.Output
		}

		if len(aliasNames) == 0 {
			aliasNames = appendAliasNames(aliasNames, filterHarnessAliases(aliasEntries))
			if usingDefaultAliasFiles {
				defaultEntries := detectDefaultHarnessAliases(aliasOutputs)
				for _, entry := range defaultEntries {
					aliasOutputs[entry.Name] = entry.Output
				}
				aliasNames = appendAliasNames(aliasNames, defaultEntries)
			}
		}

		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		repo := db.NewProfileRepository(database)
		poolRepo := db.NewPoolRepository(database)
		result := importAliasResult{}

		ctx := context.Background()
		for _, aliasName := range aliasNames {
			aliasName = strings.TrimSpace(aliasName)
			if aliasName == "" {
				continue
			}

			aliasOutput := aliasOutputs[aliasName]
			if aliasOutput == "" {
				var err error
				aliasOutput, err = getAliasOutput(aliasName, aliasFiles, shellPath)
				if err != nil {
					result.Missing = append(result.Missing, aliasName)
					continue
				}
			}

			aliasCmd := parseAliasCommand(aliasOutput, aliasName)
			if aliasCmd == "" {
				result.Missing = append(result.Missing, aliasName)
				continue
			}

			profile, err := buildAliasProfile(aliasName, aliasCmd)
			if err != nil {
				return err
			}

			if _, err := repo.GetByName(ctx, profile.Name); err == nil {
				result.Skipped = append(result.Skipped, profile.Name)
				continue
			} else if !errors.Is(err, db.ErrProfileNotFound) {
				return err
			}

			if err := repo.Create(ctx, profile); err != nil {
				if errors.Is(err, db.ErrProfileAlreadyExists) {
					result.Skipped = append(result.Skipped, profile.Name)
					continue
				}
				return err
			}

			result.Created = append(result.Created, profile.Name)
		}

		if err := maybeSelectDefaultPool(ctx, poolRepo, repo, result); err != nil {
			return err
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, result)
		}

		printAliasResult(result)
		return nil
	},
}

type importAliasResult struct {
	Created []string `json:"created"`
	Skipped []string `json:"skipped"`
	Missing []string `json:"missing"`
}

func printAliasResult(result importAliasResult) {
	if len(result.Created) == 0 && len(result.Skipped) == 0 && len(result.Missing) == 0 {
		fmt.Fprintln(os.Stdout, "No aliases processed")
		return
	}

	if len(result.Created) > 0 {
		fmt.Fprintf(os.Stdout, "Created: %s\n", strings.Join(result.Created, ", "))
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(os.Stdout, "Skipped: %s\n", strings.Join(result.Skipped, ", "))
	}
	if len(result.Missing) > 0 {
		fmt.Fprintf(os.Stdout, "Missing: %s\n", strings.Join(result.Missing, ", "))
	}
}

func maybeSelectDefaultPool(ctx context.Context, poolRepo *db.PoolRepository, profileRepo *db.ProfileRepository, result importAliasResult) error {
	if SkipConfirmation() || !IsInteractive() {
		return nil
	}
	if _, err := poolRepo.GetDefault(ctx); err == nil {
		return nil
	} else if !errors.Is(err, db.ErrPoolNotFound) {
		return err
	}

	choices, err := defaultProfileChoices(ctx, profileRepo, result)
	if err != nil {
		return err
	}
	if len(choices) == 0 {
		return nil
	}

	selection, ok, err := selectChoice("Select default profile", choices)
	if err != nil || !ok {
		return err
	}

	return ensureDefaultPoolForProfile(ctx, poolRepo, profileRepo, selection)
}

func defaultProfileChoices(ctx context.Context, repo *db.ProfileRepository, result importAliasResult) ([]string, error) {
	seen := make(map[string]struct{})
	choices := make([]string, 0, len(result.Created)+len(result.Skipped))
	for _, name := range append(result.Created, result.Skipped...) {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		choices = append(choices, name)
	}
	if len(choices) > 0 {
		return choices, nil
	}

	profiles, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		name := strings.TrimSpace(profile.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		choices = append(choices, name)
	}

	return choices, nil
}

func ensureDefaultPoolForProfile(ctx context.Context, poolRepo *db.PoolRepository, profileRepo *db.ProfileRepository, profileName string) error {
	profile, err := profileRepo.GetByName(ctx, profileName)
	if err != nil {
		return err
	}

	poolName := "default"
	if cfg := GetConfig(); cfg != nil && strings.TrimSpace(cfg.DefaultPool) != "" {
		poolName = cfg.DefaultPool
	}

	pool, err := poolRepo.GetByName(ctx, poolName)
	if err != nil {
		if !errors.Is(err, db.ErrPoolNotFound) {
			return err
		}
		pool = &models.Pool{
			Name:      poolName,
			Strategy:  models.PoolStrategyRoundRobin,
			IsDefault: true,
		}
		if err := poolRepo.Create(ctx, pool); err != nil {
			return err
		}
	}

	members, err := poolRepo.ListMembers(ctx, pool.ID)
	if err != nil {
		return err
	}
	position := 0
	for _, member := range members {
		if member.ProfileID == profile.ID {
			return poolRepo.SetDefault(ctx, pool.ID)
		}
		if member.Position > position {
			position = member.Position
		}
	}

	member := &models.PoolMember{
		PoolID:    pool.ID,
		ProfileID: profile.ID,
		Position:  position + 1,
	}
	if err := poolRepo.AddMember(ctx, member); err != nil {
		return err
	}
	return poolRepo.SetDefault(ctx, pool.ID)
}

type aliasEntry struct {
	Name   string
	Output string
}

func resolveAliasFiles() []string {
	aliasFiles := splitAliasFiles(strings.TrimSpace(os.Getenv("FORGE_ALIAS_FILE")))
	if len(aliasFiles) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil
		}
		aliasFiles = []string{
			filepath.Join(home, ".zsh_aliases"),
			filepath.Join(home, ".bash_aliases"),
			filepath.Join(home, ".bashrc"),
			filepath.Join(home, ".zshrc"),
			filepath.Join(home, ".config", "fish", "config.fish"),
		}
	}

	return uniqueExistingPaths(expandHomePaths(aliasFiles))
}

func splitAliasFiles(value string) []string {
	if value == "" {
		return nil
	}
	parts := filepath.SplitList(value)
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		files = append(files, part)
	}
	return files
}

func expandHomePaths(paths []string) []string {
	expanded := make([]string, 0, len(paths))
	for _, path := range paths {
		expanded = append(expanded, expandHome(path))
	}
	return expanded
}

func uniqueExistingPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}
	return unique
}

func resolveAliasShell() string {
	shellPath := strings.TrimSpace(os.Getenv("FORGE_ALIAS_SHELL"))
	if shellPath == "" {
		shellPath = strings.TrimSpace(os.Getenv("SHELL"))
	}
	if shellPath == "" {
		shellPath = "/bin/zsh"
	}
	return shellPath
}

var errAliasNotFound = errors.New("alias not found")

func getAliasOutput(aliasName string, aliasFiles []string, shellPath string) (string, error) {
	aliasName = strings.TrimSpace(aliasName)
	if aliasName == "" {
		return "", errAliasNotFound
	}

	if shellPath != "" {
		sourceCmd := strings.Builder{}
		for _, aliasFile := range aliasFiles {
			if aliasFile == "" {
				continue
			}
			fmt.Fprintf(&sourceCmd, "source %q >/dev/null 2>&1; ", aliasFile)
		}
		sourceCmd.WriteString("alias ")
		sourceCmd.WriteString(aliasName)
		cmd := exec.Command(shellPath, "-lc", sourceCmd.String())
		if output, err := cmd.Output(); err == nil {
			text := strings.TrimSpace(string(output))
			if text != "" {
				return text, nil
			}
		}
	}

	for _, aliasFile := range aliasFiles {
		if aliasFile == "" {
			continue
		}
		if output := findAliasInFile(aliasFile, aliasName); output != "" {
			return output, nil
		}
	}

	return "", errAliasNotFound
}

func collectAliasEntries(aliasFiles []string) []aliasEntry {
	if len(aliasFiles) == 0 {
		return nil
	}
	entries := make([]aliasEntry, 0, 16)
	seen := make(map[string]struct{}, 16)
	for _, aliasFile := range aliasFiles {
		data, err := os.ReadFile(aliasFile)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			name, output := parseAliasLine(line)
			if name == "" || output == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			entries = append(entries, aliasEntry{
				Name:   name,
				Output: output,
			})
		}
	}
	return entries
}

func filterHarnessAliases(entries []aliasEntry) []aliasEntry {
	if len(entries) == 0 {
		return nil
	}
	filtered := make([]aliasEntry, 0, len(entries))
	for _, entry := range entries {
		aliasCmd := parseAliasCommand(entry.Output, entry.Name)
		if aliasCmd == "" {
			continue
		}
		if isHarnessAlias(aliasCmd) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func appendAliasNames(names []string, entries []aliasEntry) []string {
	if len(entries) == 0 {
		return names
	}
	seen := make(map[string]struct{}, len(names)+len(entries))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names
}

type harnessCandidate struct {
	Name     string
	Commands []string
}

func detectDefaultHarnessAliases(existing map[string]string) []aliasEntry {
	candidates := []harnessCandidate{
		{Name: "codex", Commands: []string{"codex"}},
		{Name: "claude", Commands: []string{"claude", "claude-code"}},
		{Name: "opencode", Commands: []string{"opencode"}},
		{Name: "pi", Commands: []string{"pi"}},
		{Name: "droid", Commands: []string{"droid"}},
	}

	entries := make([]aliasEntry, 0, len(candidates))
	for _, candidate := range candidates {
		if _, ok := existing[candidate.Name]; ok {
			continue
		}
		output := ""
		for _, cmd := range candidate.Commands {
			if cmd == "" {
				continue
			}
			if _, err := exec.LookPath(cmd); err == nil {
				output = cmd
				break
			}
		}
		if output == "" {
			continue
		}
		entries = append(entries, aliasEntry{
			Name:   candidate.Name,
			Output: output,
		})
	}
	return entries
}

func findAliasInFile(aliasFile, aliasName string) string {
	data, err := os.ReadFile(aliasFile)
	if err != nil {
		return ""
	}
	re := regexp.MustCompile("^alias\\s+" + regexp.QuoteMeta(aliasName) + "(\\s+|=)")
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := stripInlineComment(strings.TrimSpace(line))
		if trimmed == "" {
			continue
		}
		if re.MatchString(trimmed) {
			return trimmed
		}
	}
	return ""
}

func parseAliasLine(line string) (string, string) {
	trimmed := stripInlineComment(strings.TrimSpace(line))
	if trimmed == "" || !strings.HasPrefix(trimmed, "alias ") {
		return "", ""
	}
	rest := strings.TrimSpace(strings.TrimPrefix(trimmed, "alias "))
	if rest == "" {
		return "", ""
	}

	name, command, eqStyle := splitAliasDefinition(rest)
	if name == "" || command == "" {
		return "", ""
	}
	if eqStyle {
		return name, "alias " + name + "=" + command
	}
	return name, "alias " + name + " " + command
}

func splitAliasDefinition(rest string) (string, string, bool) {
	for i, r := range rest {
		switch r {
		case '=':
			name := strings.TrimSpace(rest[:i])
			command := strings.TrimSpace(rest[i+1:])
			return name, command, true
		case ' ', '\t':
			name := strings.TrimSpace(rest[:i])
			command := strings.TrimSpace(rest[i:])
			return name, command, false
		}
	}
	return strings.TrimSpace(rest), "", false
}

func stripInlineComment(line string) string {
	inSingle := false
	inDouble := false
	for i, r := range line {
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if r == '#' && !inSingle && !inDouble {
			if i == 0 {
				return ""
			}
			return strings.TrimSpace(line[:i])
		}
	}
	return strings.TrimSpace(line)
}

func parseAliasCommand(aliasOutput, aliasName string) string {
	line := strings.TrimSpace(aliasOutput)
	if line == "" {
		return ""
	}
	if idx := strings.Index(line, "\n"); idx != -1 {
		line = line[:idx]
	}
	line = strings.TrimPrefix(line, "alias ")
	if strings.HasPrefix(line, aliasName+"=") {
		line = strings.TrimPrefix(line, aliasName+"=")
	} else if strings.HasPrefix(line, aliasName+" ") {
		line = strings.TrimSpace(strings.TrimPrefix(line, aliasName))
	} else if idx := strings.Index(line, "="); idx != -1 {
		line = line[idx+1:]
	}
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "\"")
	line = strings.TrimSuffix(line, "\"")
	line = strings.TrimPrefix(line, "'")
	line = strings.TrimSuffix(line, "'")
	return strings.TrimSpace(line)
}

func isHarnessAlias(aliasCmd string) bool {
	token := firstCommandToken(aliasCmd)
	if token == "" {
		return false
	}
	token = strings.ToLower(filepath.Base(token))
	return isHarnessToken(token)
}

func isHarnessToken(token string) bool {
	switch token {
	case "opencode", "codex", "claude", "claude-code", "pi", "droid":
		return true
	default:
		return false
	}
}

func firstCommandToken(command string) string {
	fields := shellFields(command)
	if len(fields) == 0 {
		return ""
	}
	for _, field := range fields {
		if field == "" {
			continue
		}
		lower := strings.ToLower(field)
		if isCommandWrapper(lower) {
			continue
		}
		if isEnvAssignment(field) {
			continue
		}
		return field
	}
	return ""
}

func isCommandWrapper(field string) bool {
	switch field {
	case "env", "command", "sudo", "exec", "time":
		return true
	default:
		return false
	}
}

func isEnvAssignment(field string) bool {
	idx := strings.Index(field, "=")
	if idx <= 0 {
		return false
	}
	key := field[:idx]
	for i, r := range key {
		if i == 0 {
			if !(r == '_' || unicode.IsLetter(r)) {
				return false
			}
			continue
		}
		if !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}

func shellFields(command string) []string {
	fields := make([]string, 0, 8)
	var buf strings.Builder
	inSingle := false
	inDouble := false
	escaped := false

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		fields = append(fields, buf.String())
		buf.Reset()
	}

	for _, r := range command {
		if escaped {
			buf.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && !inSingle {
			escaped = true
			continue
		}
		if r == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if r == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if !inSingle && !inDouble && unicode.IsSpace(r) {
			flush()
			continue
		}
		buf.WriteRune(r)
	}
	flush()

	return fields
}

func buildAliasProfile(aliasName, aliasCmd string) (*models.Profile, error) {
	aliasCmd = strings.TrimSpace(aliasCmd)
	if aliasCmd == "" {
		return nil, fmt.Errorf("alias %q resolved to empty command", aliasName)
	}

	env := parseLeadingEnv(aliasCmd)
	authHome := resolveAuthHome(aliasName, env)

	harnessValue, promptMode, commandTemplate := resolveAliasCommand(aliasName, aliasCmd)
	if promptMode == "" {
		promptMode = harness.DefaultPromptMode(harnessValue)
	}

	profile := &models.Profile{
		Name:            aliasName,
		Harness:         harnessValue,
		AuthHome:        authHome,
		PromptMode:      promptMode,
		CommandTemplate: commandTemplate,
		MaxConcurrency:  1,
	}

	return profile, nil
}

func resolveAliasCommand(aliasName, aliasCmd string) (models.Harness, models.PromptMode, string) {
	switch strings.ToLower(aliasName) {
	case "oc1", "oc2", "oc3":
		return buildOpenCode(aliasCmd)
	case "codex1", "codex2":
		return buildCodex(aliasCmd)
	case "cc1", "cc2", "cc3":
		return buildClaude(aliasCmd)
	case "droid1", "droid2":
		return buildDroid(aliasCmd)
	case "pi":
		return buildPi(aliasCmd)
	}

	harnessValue := inferHarness(aliasName, aliasCmd)
	switch harnessValue {
	case models.HarnessOpenCode:
		return buildOpenCode(aliasCmd)
	case models.HarnessCodex:
		return buildCodex(aliasCmd)
	case models.HarnessClaude:
		return buildClaude(aliasCmd)
	case models.HarnessDroid:
		return buildDroid(aliasCmd)
	case models.HarnessPi:
		return buildPi(aliasCmd)
	default:
		return buildPi(aliasCmd)
	}
}

func buildOpenCode(aliasCmd string) (models.Harness, models.PromptMode, string) {
	model := strings.TrimSpace(os.Getenv("FORGE_OPENCODE_MODEL"))
	if model == "" {
		model = "anthropic/claude-opus-4-5"
	}
	command := fmt.Sprintf("%s run --model %s \"$FORGE_PROMPT_CONTENT\"", aliasCmd, model)
	return models.HarnessOpenCode, models.PromptModeEnv, command
}

func buildCodex(aliasCmd string) (models.Harness, models.PromptMode, string) {
	command := fmt.Sprintf("%s exec -", aliasCmd)
	return models.HarnessCodex, models.PromptModeStdin, command
}

func buildClaude(aliasCmd string) (models.Harness, models.PromptMode, string) {
	command := fmt.Sprintf("%s -p \"$FORGE_PROMPT_CONTENT\"", aliasCmd)
	if !strings.Contains(command, "--dangerously-skip-permissions") {
		command = command + " --dangerously-skip-permissions"
	}
	// Wrap with script to create a PTY so Claude streams output in real-time
	command = fmt.Sprintf("script -q -c '%s' /dev/null", command)
	return models.HarnessClaude, models.PromptModeEnv, command
}

func buildPi(aliasCmd string) (models.Harness, models.PromptMode, string) {
	command := fmt.Sprintf("%s -p \"$FORGE_PROMPT_CONTENT\"", aliasCmd)
	return models.HarnessPi, models.PromptModeEnv, command
}

func buildDroid(aliasCmd string) (models.Harness, models.PromptMode, string) {
	// Droid reads from stdin when no prompt argument is provided
	command := fmt.Sprintf("%s exec --skip-permissions-unsafe", aliasCmd)
	return models.HarnessDroid, models.PromptModeStdin, command
}

func inferHarness(aliasName, aliasCmd string) models.Harness {
	candidate := strings.ToLower(aliasName + " " + aliasCmd)
	switch {
	case strings.Contains(candidate, "opencode"):
		return models.HarnessOpenCode
	case strings.Contains(candidate, "codex"):
		return models.HarnessCodex
	case strings.Contains(candidate, "claude"):
		return models.HarnessClaude
	case strings.Contains(candidate, "droid"):
		return models.HarnessDroid
	case strings.Contains(candidate, "pi"):
		return models.HarnessPi
	default:
		return models.HarnessPi
	}
}

func parseLeadingEnv(command string) map[string]string {
	fields := strings.Fields(command)
	env := make(map[string]string)
	for _, field := range fields {
		if !strings.Contains(field, "=") || strings.HasPrefix(field, "=") {
			break
		}
		parts := strings.SplitN(field, "=", 2)
		key := strings.TrimSpace(parts[0])
		if key == "" {
			break
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"'")
		env[key] = value
	}
	return env
}

func resolveAuthHome(aliasName string, env map[string]string) string {
	if value := env["PI_CODING_AGENT_DIR"]; value != "" {
		return expandHome(value)
	}
	if value := env["CODEX_HOME"]; value != "" {
		return expandHome(value)
	}
	if value := env["OPENCODE_CONFIG_DIR"]; value != "" {
		return expandHome(value)
	}
	if value := env["OPENCODE_HOME"]; value != "" {
		return expandHome(value)
	}
	if value := env["XDG_DATA_HOME"]; value != "" {
		return expandHome(value)
	}
	if value := env["CLAUDE_CONFIG_DIR"]; value != "" {
		return expandHome(value)
	}
	if value := env["CLAUDE_HOME"]; value != "" {
		return expandHome(value)
	}
	if value := env["HOME"]; value != "" {
		return expandHome(value)
	}

	aliasLower := strings.ToLower(aliasName)
	if strings.HasPrefix(aliasLower, "codex") {
		if suffix := strings.TrimPrefix(aliasLower, "codex"); suffix != "" {
			return expandHome("~/.codex-" + suffix)
		}
	}
	if strings.HasPrefix(aliasLower, "oc") {
		if suffix := strings.TrimPrefix(aliasLower, "oc"); suffix != "" {
			return expandHome("~/.opencode-" + suffix)
		}
	}
	if strings.HasPrefix(aliasLower, "cc") {
		if suffix := strings.TrimPrefix(aliasLower, "cc"); suffix != "" {
			return expandHome("~/.claude-" + suffix)
		}
	}

	return ""
}

func expandHome(path string) string {
	path = os.ExpandEnv(path)
	if strings.HasPrefix(path, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	return path
}
