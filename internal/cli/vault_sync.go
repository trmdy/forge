// Package cli provides vault sync commands.
package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/models"
	"github.com/tOgg1/forge/internal/node"
	"github.com/tOgg1/forge/internal/ssh"
	"github.com/tOgg1/forge/internal/vault"
)

var (
	vaultSyncProfile string
	vaultSyncAll     bool
	vaultSyncForce   bool
	vaultSyncDryRun  bool
)

type vaultSyncDirection string

const (
	vaultSyncPush vaultSyncDirection = "push"
	vaultSyncPull vaultSyncDirection = "pull"
)

func init() {
	vaultPushCmd.Flags().StringVar(&vaultSyncProfile, "profile", "", "profile to sync (adapter/name or provider/name)")
	vaultPushCmd.Flags().BoolVar(&vaultSyncAll, "all", false, "sync all profiles")
	vaultPushCmd.Flags().BoolVar(&vaultSyncForce, "force", false, "overwrite without confirmation")
	vaultPushCmd.Flags().BoolVar(&vaultSyncDryRun, "dry-run", false, "show what would be transferred")

	vaultPullCmd.Flags().StringVar(&vaultSyncProfile, "profile", "", "profile to sync (adapter/name or provider/name)")
	vaultPullCmd.Flags().BoolVar(&vaultSyncAll, "all", false, "sync all profiles")
	vaultPullCmd.Flags().BoolVar(&vaultSyncForce, "force", false, "overwrite without confirmation")
	vaultPullCmd.Flags().BoolVar(&vaultSyncDryRun, "dry-run", false, "show what would be transferred")
}

var vaultPushCmd = &cobra.Command{
	Use:   "push <node>",
	Short: "Push vault profiles to a node",
	Long: `Sync vault profiles to a node using SSH transfer.

Examples:
  forge vault push node-1 --profile claude/work
  forge vault push node-1 --all`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runVaultSync(args[0], vaultSyncPush)
	},
}

var vaultPullCmd = &cobra.Command{
	Use:   "pull <node>",
	Short: "Pull vault profiles from a node",
	Long: `Sync vault profiles from a node using SSH transfer.

Examples:
  forge vault pull node-1 --profile claude/work
  forge vault pull node-1 --all`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runVaultSync(args[0], vaultSyncPull)
	},
}

type vaultProfileRef struct {
	Adapter  vault.Adapter
	Provider string
	Name     string
}

func (r vaultProfileRef) ArchivePath() string {
	return path.Join(r.Provider, r.Name)
}

func (r vaultProfileRef) Display() string {
	return fmt.Sprintf("%s/%s", r.Adapter, r.Name)
}

type vaultSyncOutput struct {
	Status    string   `json:"status"`
	Direction string   `json:"direction"`
	Node      string   `json:"node"`
	Profiles  []string `json:"profiles"`
	Conflicts []string `json:"conflicts,omitempty"`
	DryRun    bool     `json:"dry_run,omitempty"`
}

func runVaultSync(nodeName string, direction vaultSyncDirection) error {
	if vaultSyncProfile != "" && vaultSyncAll {
		return errors.New("use either --profile or --all, not both")
	}

	ctx := context.Background()

	database, err := openDatabase()
	if err != nil {
		return err
	}
	defer database.Close()

	repo := db.NewNodeRepository(database)
	service := node.NewService(repo, node.WithPublisher(newEventPublisher(database)))

	n, err := findNode(ctx, service, nodeName)
	if err != nil {
		return err
	}

	executor, closeExecutor, err := vaultSyncExecutor(ctx, service, n)
	if err != nil {
		return err
	}
	defer closeExecutor()

	if !n.IsLocal && IsInteractive() && !vaultSyncDryRun {
		fmt.Fprintf(os.Stderr, "Warning: syncing vault profiles to node %q will transfer sensitive auth files.\n", n.Name)
	}

	localVaultPath := getVaultPath()
	remoteVaultPath := getRemoteVaultPath()
	localProfilesPath := vault.ProfilesPath(localVaultPath)
	remoteProfilesPath := filepath.Join(remoteVaultPath, "profiles")

	switch direction {
	case vaultSyncPush:
		profiles, err := selectPushProfiles(localVaultPath)
		if err != nil {
			return err
		}
		return pushVaultProfiles(ctx, n, executor, localProfilesPath, remoteProfilesPath, profiles)
	case vaultSyncPull:
		profiles, err := selectPullProfiles(ctx, executor, remoteProfilesPath)
		if err != nil {
			return err
		}
		return pullVaultProfiles(ctx, n, executor, localProfilesPath, remoteProfilesPath, profiles)
	default:
		return fmt.Errorf("unknown vault sync direction: %s", direction)
	}
}

func getRemoteVaultPath() string {
	if vaultPath != "" {
		return vaultPath
	}
	return "~/.config/forge/vault"
}

func vaultSyncExecutor(ctx context.Context, service *node.Service, n *models.Node) (ssh.Executor, func(), error) {
	if n.IsLocal {
		executor := ssh.NewLocalExecutor()
		return executor, func() { _ = executor.Close() }, nil
	}

	nodeExec, err := service.NewNodeExecutor(ctx, n, node.WithFallbackPolicy(node.FallbackPolicySSHOnly))
	if err != nil {
		return nil, nil, err
	}

	executor := nodeExec.SSHExecutor()
	if executor == nil {
		_ = nodeExec.Close()
		return nil, nil, errors.New("no SSH executor available for node")
	}

	closeFn := func() {
		_ = executor.Close()
		_ = nodeExec.Close()
	}

	return executor, closeFn, nil
}

func selectPushProfiles(vaultPath string) ([]vaultProfileRef, error) {
	profiles, err := collectLocalProfiles(vaultPath)
	if err != nil {
		return nil, err
	}

	if vaultSyncProfile != "" {
		ref, err := parseVaultProfileRef(vaultSyncProfile)
		if err != nil {
			return nil, err
		}
		if _, err := vault.Get(vaultPath, ref.Adapter, ref.Name); err != nil {
			return nil, fmt.Errorf("profile %s not found in local vault: %w", ref.Display(), err)
		}
		return []vaultProfileRef{ref}, nil
	}

	if vaultSyncAll {
		if len(profiles) == 0 {
			return nil, errors.New("no profiles found in local vault")
		}
		return profiles, nil
	}

	if !canPromptVaultSelection() {
		return nil, errors.New("profile selection required (use --profile or --all)")
	}

	return promptVaultProfileSelection(profiles)
}

func selectPullProfiles(ctx context.Context, executor ssh.Executor, remoteProfilesPath string) ([]vaultProfileRef, error) {
	if vaultSyncProfile != "" {
		ref, err := parseVaultProfileRef(vaultSyncProfile)
		if err != nil {
			return nil, err
		}
		exists, err := remoteDirExists(ctx, executor, path.Join(remoteProfilesPath, ref.ArchivePath()))
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("profile %s not found on node", ref.Display())
		}
		return []vaultProfileRef{ref}, nil
	}

	profiles, err := listRemoteProfiles(ctx, executor, remoteProfilesPath)
	if err != nil {
		return nil, err
	}

	if vaultSyncAll {
		if len(profiles) == 0 {
			return nil, errors.New("no profiles found on node")
		}
		return profiles, nil
	}

	if !canPromptVaultSelection() {
		return nil, errors.New("profile selection required (use --profile or --all)")
	}

	return promptVaultProfileSelection(profiles)
}

func collectLocalProfiles(vaultPath string) ([]vaultProfileRef, error) {
	profiles, err := vault.List(vaultPath, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list local profiles: %w", err)
	}

	refs := make([]vaultProfileRef, 0, len(profiles))
	for _, profile := range profiles {
		refs = append(refs, vaultProfileRef{
			Adapter:  profile.Adapter,
			Provider: profile.Adapter.Provider(),
			Name:     profile.Name,
		})
	}

	sortVaultProfiles(refs)
	return refs, nil
}

func listRemoteProfiles(ctx context.Context, executor ssh.Executor, remoteProfilesPath string) ([]vaultProfileRef, error) {
	exists, err := remoteDirExists(ctx, executor, remoteProfilesPath)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}

	cmd := fmt.Sprintf("cd %s && find . -mindepth 2 -maxdepth 2 -type d -print", shellEscapePath(remoteProfilesPath))
	stdout, stderr, err := executor.Exec(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to list remote profiles: %w (stderr: %s)", err, strings.TrimSpace(string(stderr)))
	}

	lines := strings.Split(string(stdout), "\n")
	refs := make([]vaultProfileRef, 0, len(lines))
	for _, line := range lines {
		entry := strings.TrimSpace(line)
		if entry == "" {
			continue
		}
		entry = strings.TrimPrefix(entry, "./")

		parts := strings.SplitN(entry, "/", 2)
		if len(parts) != 2 {
			continue
		}
		adapter := vault.ParseAdapter(parts[0])
		if adapter == "" {
			continue
		}
		refs = append(refs, vaultProfileRef{
			Adapter:  adapter,
			Provider: parts[0],
			Name:     parts[1],
		})
	}

	sortVaultProfiles(refs)
	return refs, nil
}

func pushVaultProfiles(ctx context.Context, n *models.Node, executor ssh.Executor, localProfilesPath, remoteProfilesPath string, profiles []vaultProfileRef) error {
	if len(profiles) == 0 {
		return errors.New("no profiles selected for push")
	}

	conflicts, err := remoteProfileConflicts(ctx, executor, remoteProfilesPath, profiles)
	if err != nil {
		return err
	}

	if vaultSyncDryRun {
		return writeVaultSyncOutput(vaultSyncPush, n.Name, profiles, conflicts, true)
	}

	overwrite, err := confirmVaultOverwrite("node", n.Name, conflicts)
	if err != nil {
		return err
	}
	if !overwrite {
		fmt.Fprintln(os.Stdout, "Vault push cancelled.")
		return nil
	}

	if overwrite && len(conflicts) > 0 {
		if err := removeRemoteProfiles(ctx, executor, remoteProfilesPath, conflicts); err != nil {
			return err
		}
	}

	archivePath, err := createVaultArchive(ctx, localProfilesPath, profiles)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open vault archive: %w", err)
	}
	defer file.Close()

	remoteCmd := fmt.Sprintf("mkdir -p %s && tar -xzf - --no-same-owner -C %s", shellEscapePath(remoteProfilesPath), shellEscapePath(remoteProfilesPath))
	if err := executor.ExecInteractive(ctx, remoteCmd, file); err != nil {
		return fmt.Errorf("failed to extract archive on node: %w", err)
	}

	return writeVaultSyncOutput(vaultSyncPush, n.Name, profiles, conflicts, false)
}

func pullVaultProfiles(ctx context.Context, n *models.Node, executor ssh.Executor, localProfilesPath, remoteProfilesPath string, profiles []vaultProfileRef) error {
	if len(profiles) == 0 {
		return errors.New("no profiles selected for pull")
	}

	conflicts, err := localProfileConflicts(localProfilesPath, profiles)
	if err != nil {
		return err
	}

	if vaultSyncDryRun {
		return writeVaultSyncOutput(vaultSyncPull, n.Name, profiles, conflicts, true)
	}

	overwrite, err := confirmVaultOverwrite("local vault", "", conflicts)
	if err != nil {
		return err
	}
	if !overwrite {
		fmt.Fprintln(os.Stdout, "Vault pull cancelled.")
		return nil
	}

	if overwrite && len(conflicts) > 0 {
		if err := removeLocalProfiles(localProfilesPath, conflicts); err != nil {
			return err
		}
	}

	archiveData, err := fetchRemoteArchive(ctx, executor, remoteProfilesPath, profiles)
	if err != nil {
		return err
	}

	archivePath, err := writeTempArchive(archiveData)
	if err != nil {
		return err
	}
	defer os.Remove(archivePath)

	if err := os.MkdirAll(localProfilesPath, 0700); err != nil {
		return fmt.Errorf("failed to create vault profiles directory: %w", err)
	}

	if err := extractVaultArchive(ctx, archivePath, localProfilesPath); err != nil {
		return err
	}

	return writeVaultSyncOutput(vaultSyncPull, n.Name, profiles, conflicts, false)
}

func fetchRemoteArchive(ctx context.Context, executor ssh.Executor, remoteProfilesPath string, profiles []vaultProfileRef) ([]byte, error) {
	args := archivePaths(profiles)
	remoteArgs := make([]string, 0, len(args))
	for _, arg := range args {
		remoteArgs = append(remoteArgs, shellEscape(arg))
	}
	cmd := fmt.Sprintf("tar -czf - -C %s -- %s", shellEscapePath(remoteProfilesPath), strings.Join(remoteArgs, " "))
	stdout, stderr, err := executor.Exec(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("failed to create archive on node: %w (stderr: %s)", err, strings.TrimSpace(string(stderr)))
	}
	return stdout, nil
}

func createVaultArchive(ctx context.Context, localProfilesPath string, profiles []vaultProfileRef) (string, error) {
	if len(profiles) == 0 {
		return "", errors.New("no profiles to archive")
	}

	tempFile, err := os.CreateTemp("", "forge-vault-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("failed to create temp archive: %w", err)
	}
	if err := tempFile.Chmod(0600); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to set archive permissions: %w", err)
	}
	defer tempFile.Close()

	args := []string{"-czf", "-", "-C", localProfilesPath, "--"}
	args = append(args, archivePaths(profiles)...)

	cmd := exec.CommandContext(ctx, "tar", args...)
	cmd.Stdout = tempFile
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to create vault archive: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	return tempFile.Name(), nil
}

func writeTempArchive(data []byte) (string, error) {
	tempFile, err := os.CreateTemp("", "forge-vault-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("failed to create temp archive: %w", err)
	}
	if err := tempFile.Chmod(0600); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to set archive permissions: %w", err)
	}
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to write archive data: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFile.Name())
		return "", fmt.Errorf("failed to close archive file: %w", err)
	}
	return tempFile.Name(), nil
}

func extractVaultArchive(ctx context.Context, archivePath, destPath string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open archive: %w", err)
	}
	defer file.Close()

	cmd := exec.CommandContext(ctx, "tar", "-xzf", "-", "--no-same-owner", "-C", destPath)
	cmd.Stdin = file
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract archive: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func archivePaths(profiles []vaultProfileRef) []string {
	paths := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		paths = append(paths, profile.ArchivePath())
	}
	if len(paths) == 0 {
		return []string{"."}
	}
	return paths
}

func writeVaultSyncOutput(direction vaultSyncDirection, nodeName string, profiles, conflicts []vaultProfileRef, dryRun bool) error {
	profileNames := formatProfileList(profiles)
	conflictNames := formatProfileList(conflicts)

	if IsJSONOutput() || IsJSONLOutput() {
		status := "synced"
		if dryRun {
			status = "dry_run"
		}
		return WriteOutput(os.Stdout, vaultSyncOutput{
			Status:    status,
			Direction: string(direction),
			Node:      nodeName,
			Profiles:  profileNames,
			Conflicts: conflictNames,
			DryRun:    dryRun,
		})
	}

	action := "Synced"
	if dryRun {
		action = "Would sync"
	}

	preposition := "to"
	if direction == vaultSyncPull {
		preposition = "from"
	}

	fmt.Fprintf(os.Stdout, "%s %d profile(s) %s node %q:\n", action, len(profileNames), preposition, nodeName)
	for _, name := range profileNames {
		fmt.Fprintf(os.Stdout, "  - %s\n", name)
	}
	if len(conflictNames) > 0 {
		fmt.Fprintln(os.Stdout, "Conflicts:")
		for _, name := range conflictNames {
			fmt.Fprintf(os.Stdout, "  - %s\n", name)
		}
	}

	return nil
}

func parseVaultProfileRef(value string) (vaultProfileRef, error) {
	parts := strings.SplitN(strings.TrimSpace(value), "/", 2)
	if len(parts) != 2 {
		return vaultProfileRef{}, fmt.Errorf("invalid profile reference %q (expected adapter/name)", value)
	}

	adapter := vault.ParseAdapter(strings.ToLower(parts[0]))
	if adapter == "" {
		return vaultProfileRef{}, fmt.Errorf("unknown adapter or provider %q", parts[0])
	}

	name := strings.TrimSpace(parts[1])
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, "..") {
		return vaultProfileRef{}, fmt.Errorf("invalid profile name %q", name)
	}

	return vaultProfileRef{
		Adapter:  adapter,
		Provider: adapter.Provider(),
		Name:     name,
	}, nil
}

func promptVaultProfileSelection(profiles []vaultProfileRef) ([]vaultProfileRef, error) {
	if len(profiles) == 0 {
		return nil, errors.New("no profiles available to select")
	}

	sortVaultProfiles(profiles)

	fmt.Fprintln(os.Stderr, "Available vault profiles:")
	for i, profile := range profiles {
		fmt.Fprintf(os.Stderr, "  %d) %s\n", i+1, profile.Display())
	}

	reader := bufio.NewReader(os.Stdin)
	choice, err := promptLine(reader, fmt.Sprintf("Select profile [1-%d] or 'all': ", len(profiles)))
	if err != nil {
		return nil, err
	}
	choice = strings.TrimSpace(choice)

	if strings.EqualFold(choice, "all") {
		return profiles, nil
	}

	index, err := strconv.Atoi(choice)
	if err != nil || index < 1 || index > len(profiles) {
		return nil, fmt.Errorf("invalid selection %q", choice)
	}
	return []vaultProfileRef{profiles[index-1]}, nil
}

func confirmVaultOverwrite(location string, nodeName string, conflicts []vaultProfileRef) (bool, error) {
	if len(conflicts) == 0 || shouldForceOverwrite() {
		return true, nil
	}
	if !canPromptVaultSelection() {
		return false, fmt.Errorf("profiles already exist (%s) - use --force to overwrite", strings.Join(formatProfileList(conflicts), ", "))
	}

	target := location
	if nodeName != "" {
		target = fmt.Sprintf("%s %q", location, nodeName)
	}

	fmt.Fprintf(os.Stderr, "The following profiles already exist in %s:\n", target)
	for _, profile := range conflicts {
		fmt.Fprintf(os.Stderr, "  - %s\n", profile.Display())
	}

	reader := bufio.NewReader(os.Stdin)
	choice, err := promptLine(reader, "Overwrite these profiles? [y/N]: ")
	if err != nil {
		return false, err
	}

	choice = strings.ToLower(strings.TrimSpace(choice))
	return choice == "y" || choice == "yes", nil
}

func shouldForceOverwrite() bool {
	return vaultSyncForce || yesFlag
}

func canPromptVaultSelection() bool {
	if IsJSONOutput() || IsJSONLOutput() {
		return false
	}
	return IsInteractive()
}

func remoteProfileConflicts(ctx context.Context, executor ssh.Executor, remoteProfilesPath string, profiles []vaultProfileRef) ([]vaultProfileRef, error) {
	conflicts := make([]vaultProfileRef, 0)
	for _, profile := range profiles {
		remotePath := path.Join(remoteProfilesPath, profile.ArchivePath())
		exists, err := remoteDirExists(ctx, executor, remotePath)
		if err != nil {
			return nil, err
		}
		if exists {
			conflicts = append(conflicts, profile)
		}
	}
	return conflicts, nil
}

func localProfileConflicts(localProfilesPath string, profiles []vaultProfileRef) ([]vaultProfileRef, error) {
	conflicts := make([]vaultProfileRef, 0)
	for _, profile := range profiles {
		localPath := filepath.Join(localProfilesPath, profile.Provider, profile.Name)
		if _, err := os.Stat(localPath); err == nil {
			conflicts = append(conflicts, profile)
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to check profile %s: %w", profile.Display(), err)
		}
	}
	return conflicts, nil
}

func removeRemoteProfiles(ctx context.Context, executor ssh.Executor, remoteProfilesPath string, profiles []vaultProfileRef) error {
	if len(profiles) == 0 {
		return nil
	}

	paths := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		remotePath := path.Join(remoteProfilesPath, profile.ArchivePath())
		paths = append(paths, shellEscapePath(remotePath))
	}

	cmd := fmt.Sprintf("rm -rf %s", strings.Join(paths, " "))
	_, stderr, err := executor.Exec(ctx, cmd)
	if err != nil {
		return fmt.Errorf("failed to remove existing profiles on node: %w (stderr: %s)", err, strings.TrimSpace(string(stderr)))
	}
	return nil
}

func removeLocalProfiles(localProfilesPath string, profiles []vaultProfileRef) error {
	for _, profile := range profiles {
		localPath := filepath.Join(localProfilesPath, profile.Provider, profile.Name)
		if err := os.RemoveAll(localPath); err != nil {
			return fmt.Errorf("failed to remove local profile %s: %w", profile.Display(), err)
		}
	}
	return nil
}

func remoteDirExists(ctx context.Context, executor ssh.Executor, remotePath string) (bool, error) {
	cmd := fmt.Sprintf("test -d %s", shellEscapePath(remotePath))
	_, stderr, err := executor.Exec(ctx, cmd)
	if err == nil {
		return true, nil
	}

	var execErr *ssh.ExecError
	if errors.As(err, &execErr) {
		if execErr.ExitCode == 1 || execErr.ExitCode == 2 {
			return false, nil
		}
	}

	return false, fmt.Errorf("failed to check remote path: %w (stderr: %s)", err, strings.TrimSpace(string(stderr)))
}

func shellEscape(value string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(value, "'", "'\\''"))
}

func shellEscapePath(value string) string {
	if value == "" {
		return "''"
	}
	if strings.HasPrefix(value, "~") && isSafePath(value) {
		return value
	}
	if strings.HasPrefix(value, "$HOME") && isSafePath(value) {
		return value
	}
	return shellEscape(value)
}

func isSafePath(value string) bool {
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '/', r == '.', r == '-', r == '_', r == '~', r == '$':
		default:
			return false
		}
	}
	return true
}

func formatProfileList(profiles []vaultProfileRef) []string {
	names := make([]string, 0, len(profiles))
	for _, profile := range profiles {
		names = append(names, profile.Display())
	}
	return names
}

func sortVaultProfiles(profiles []vaultProfileRef) {
	sort.Slice(profiles, func(i, j int) bool {
		if profiles[i].Adapter == profiles[j].Adapter {
			return profiles[i].Name < profiles[j].Name
		}
		return profiles[i].Adapter < profiles[j].Adapter
	})
}
