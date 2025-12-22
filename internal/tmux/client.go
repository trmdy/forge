// Package tmux provides a small wrapper for tmux command execution.
package tmux

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Executor runs tmux commands.
type Executor interface {
	Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error)
}

const (
	historyChunkLines = 2000
	historyMaxLines   = 20000
)

// LocalExecutor executes commands locally via os/exec.
type LocalExecutor struct{}

// Exec runs a command locally and returns stdout and stderr.
func (e *LocalExecutor) Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error) {
	c := exec.CommandContext(ctx, "sh", "-c", cmd)
	var stdoutBuf, stderrBuf bytes.Buffer
	c.Stdout = &stdoutBuf
	c.Stderr = &stderrBuf
	err = c.Run()
	return stdoutBuf.Bytes(), stderrBuf.Bytes(), err
}

// Client wraps tmux command helpers.
type Client struct {
	exec Executor
}

// AgentWindowName is the default window name used for agent panes.
const AgentWindowName = "agents"

// NewClient creates a new tmux client.
func NewClient(exec Executor) *Client {
	return &Client{exec: exec}
}

// NewTmuxClient is an alias for NewClient for backward compatibility.
func NewTmuxClient(exec Executor) *Client {
	return NewClient(exec)
}

// NewLocalClient creates a new tmux client that executes commands locally.
func NewLocalClient() *Client {
	return &Client{exec: &LocalExecutor{}}
}

// Session describes a tmux session.
type Session struct {
	Name        string
	WindowCount int
}

// ListSessions returns all known tmux sessions.
func (c *Client) ListSessions(ctx context.Context) ([]Session, error) {
	stdout, stderr, err := c.exec.Exec(ctx, "tmux list-sessions -F '#{session_name}|#{session_windows}'")
	if err != nil {
		if isNoServerRunning(stderr) {
			return []Session{}, nil
		}
		return nil, fmt.Errorf("tmux list-sessions failed: %w", err)
	}

	output := strings.TrimSpace(string(stdout))
	if output == "" {
		return []Session{}, nil
	}

	lines := strings.Split(output, "\n")
	sessions := make([]Session, 0, len(lines))

	for _, line := range lines {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("unexpected tmux output line: %q", line)
		}

		count, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid window count in tmux output: %q", line)
		}

		sessions = append(sessions, Session{
			Name:        strings.TrimSpace(parts[0]),
			WindowCount: count,
		})
	}

	return sessions, nil
}

// ListPanePaths returns the unique working directories for panes in a session.
func (c *Client) ListPanePaths(ctx context.Context, session string) ([]string, error) {
	if strings.TrimSpace(session) == "" {
		return nil, fmt.Errorf("session name is required")
	}

	cmd := fmt.Sprintf("tmux list-panes -t %s -F '#{pane_id}|#{pane_current_path}'", session)
	stdout, stderr, err := c.exec.Exec(ctx, cmd)
	if err != nil {
		if isNoServerRunning(stderr) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("tmux list-panes failed: %w", err)
	}

	output := strings.TrimSpace(string(stdout))
	if output == "" {
		return []string{}, nil
	}

	seen := make(map[string]struct{})
	paths := []string{}

	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("unexpected tmux output line: %q", line)
		}

		path := strings.TrimSpace(parts[1])
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	return paths, nil
}

// HasSession checks if a session with the given name exists.
func (c *Client) HasSession(ctx context.Context, session string) (bool, error) {
	if strings.TrimSpace(session) == "" {
		return false, fmt.Errorf("session name is required")
	}

	cmd := fmt.Sprintf("tmux has-session -t %s", escapeSessionName(session))
	_, stderr, err := c.exec.Exec(ctx, cmd)
	if err != nil {
		// "no server running" or "session not found" both mean session doesn't exist
		if isNoServerRunning(stderr) || isSessionNotFound(stderr) {
			return false, nil
		}
		return false, fmt.Errorf("tmux has-session failed: %w", err)
	}

	return true, nil
}

// NewSession creates a new tmux session with the given name and working directory.
func (c *Client) NewSession(ctx context.Context, session, workDir string) error {
	if strings.TrimSpace(session) == "" {
		return fmt.Errorf("session name is required")
	}

	cmd := fmt.Sprintf("tmux new-session -d -s %s", escapeSessionName(session))
	if workDir != "" {
		cmd = fmt.Sprintf("%s -c %s", cmd, escapeArg(workDir))
	}

	_, stderr, err := c.exec.Exec(ctx, cmd)
	if err != nil {
		if isDuplicateSession(stderr) {
			return ErrSessionExists
		}
		return fmt.Errorf("tmux new-session failed: %w", err)
	}

	return nil
}

// NewWindow creates a new tmux window in the given session.
func (c *Client) NewWindow(ctx context.Context, session, name, workDir string) error {
	if strings.TrimSpace(session) == "" {
		return fmt.Errorf("session name is required")
	}

	cmd := fmt.Sprintf("tmux new-window -t %s", escapeSessionName(session))
	if strings.TrimSpace(name) != "" {
		cmd = fmt.Sprintf("%s -n %s", cmd, escapeArg(name))
	}
	if strings.TrimSpace(workDir) != "" {
		cmd = fmt.Sprintf("%s -c %s", cmd, escapeArg(workDir))
	}

	if _, _, err := c.exec.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("tmux new-window failed: %w", err)
	}

	return nil
}

// SelectWindow focuses the specified tmux window.
func (c *Client) SelectWindow(ctx context.Context, target string) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("target is required")
	}

	cmd := fmt.Sprintf("tmux select-window -t %s", escapeArg(target))
	if _, _, err := c.exec.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("tmux select-window failed: %w", err)
	}

	return nil
}

// SelectLayout applies a tmux layout preset to a target.
func (c *Client) SelectLayout(ctx context.Context, target string, layout string) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("target is required")
	}
	if strings.TrimSpace(layout) == "" {
		return fmt.Errorf("layout is required")
	}

	cmd := fmt.Sprintf("tmux select-layout -t %s %s", escapeArg(target), escapeArg(layout))
	if _, _, err := c.exec.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("tmux select-layout failed: %w", err)
	}

	return nil
}

// KillSession terminates a tmux session.
func (c *Client) KillSession(ctx context.Context, session string) error {
	if strings.TrimSpace(session) == "" {
		return fmt.Errorf("session name is required")
	}

	cmd := fmt.Sprintf("tmux kill-session -t %s", escapeSessionName(session))
	_, stderr, err := c.exec.Exec(ctx, cmd)
	if err != nil {
		if isNoServerRunning(stderr) || isSessionNotFound(stderr) {
			return ErrSessionNotFound
		}
		return fmt.Errorf("tmux kill-session failed: %w", err)
	}

	return nil
}

// Pane describes a tmux pane.
type Pane struct {
	ID          string // e.g., "%1"
	WindowIndex int    // window index within session
	Index       int    // pane index within window
	CurrentDir  string
	Command     string
	Active      bool
}

// ListPanes returns all panes in a session.
func (c *Client) ListPanes(ctx context.Context, session string) ([]Pane, error) {
	if strings.TrimSpace(session) == "" {
		return nil, fmt.Errorf("session name is required")
	}

	cmd := fmt.Sprintf("tmux list-panes -t %s -F '#{pane_id}|#{window_index}|#{pane_index}|#{pane_current_path}|#{pane_active}|#{pane_current_command}'", escapeSessionName(session))
	stdout, stderr, err := c.exec.Exec(ctx, cmd)
	if err != nil {
		if isNoServerRunning(stderr) {
			return []Pane{}, nil
		}
		return nil, fmt.Errorf("tmux list-panes failed: %w", err)
	}

	output := strings.TrimSpace(string(stdout))
	if output == "" {
		return []Pane{}, nil
	}

	var panes []Pane
	for _, line := range strings.Split(output, "\n") {
		parts := strings.SplitN(line, "|", 6)
		if len(parts) != 6 {
			return nil, fmt.Errorf("unexpected tmux output line: %q", line)
		}

		windowIndex, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
		index, _ := strconv.Atoi(strings.TrimSpace(parts[2]))
		panes = append(panes, Pane{
			ID:          strings.TrimSpace(parts[0]),
			WindowIndex: windowIndex,
			Index:       index,
			CurrentDir:  strings.TrimSpace(parts[3]),
			Active:      strings.TrimSpace(parts[4]) == "1",
			Command:     strings.TrimSpace(parts[5]),
		})
	}

	return panes, nil
}

// SplitWindow creates a new pane by splitting the current window.
// If horizontal is true, splits left-right; otherwise splits top-bottom.
// Returns the new pane ID.
func (c *Client) SplitWindow(ctx context.Context, target string, horizontal bool, workDir string) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", fmt.Errorf("target is required")
	}

	splitFlag := "-v" // vertical split (top-bottom)
	if horizontal {
		splitFlag = "-h" // horizontal split (left-right)
	}

	// Use -P -F to print the new pane ID
	cmd := fmt.Sprintf("tmux split-window %s -t %s -P -F '#{pane_id}'", splitFlag, escapeArg(target))
	if workDir != "" {
		cmd = fmt.Sprintf("%s -c %s", cmd, escapeArg(workDir))
	}

	stdout, _, err := c.exec.Exec(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("tmux split-window failed: %w", err)
	}

	return strings.TrimSpace(string(stdout)), nil
}

// SendKeys sends keys to a tmux pane.
// If literal is true, sends keys literally (no translation).
// If enter is true, appends an Enter keypress.
func (c *Client) SendKeys(ctx context.Context, target, keys string, literal, enter bool) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("target is required")
	}

	literalFlag := ""
	if literal {
		literalFlag = "-l"
	}

	// Escape the keys for shell
	escapedKeys := escapeArg(keys)
	cmd := fmt.Sprintf("tmux send-keys -t %s %s %s", escapeArg(target), literalFlag, escapedKeys)

	if _, _, err := c.exec.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("tmux send-keys failed: %w", err)
	}

	if enter {
		enterCmd := fmt.Sprintf("tmux send-keys -t %s Enter", escapeArg(target))
		if _, _, err := c.exec.Exec(ctx, enterCmd); err != nil {
			return fmt.Errorf("tmux send-keys Enter failed: %w", err)
		}
	}

	return nil
}

// SendInterrupt sends Ctrl+C to a tmux pane.
func (c *Client) SendInterrupt(ctx context.Context, target string) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("target is required")
	}

	cmd := fmt.Sprintf("tmux send-keys -t %s C-c", escapeArg(target))
	if _, _, err := c.exec.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("tmux send-keys C-c failed: %w", err)
	}

	return nil
}

// SendAndWait sends keys to a tmux pane and waits for output to stabilize.
// It polls capture-pane output until the hash is unchanged for stableRounds.
func (c *Client) SendAndWait(ctx context.Context, target, keys string, literal, enter bool, stableRounds int) (string, error) {
	if stableRounds <= 0 {
		stableRounds = 2
	}

	if err := c.SendKeys(ctx, target, keys, literal, enter); err != nil {
		return "", err
	}

	var lastHash string
	stable := 0

	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		content, err := c.CapturePane(ctx, target, false)
		if err != nil {
			return "", err
		}

		hash := HashSnapshot(content)
		if hash == lastHash {
			stable++
			if stable >= stableRounds {
				return content, nil
			}
		} else {
			stable = 0
			lastHash = hash
		}
	}
}

// CapturePane captures the visible content of a pane.
// If history is true, includes scrollback history.
func (c *Client) CapturePane(ctx context.Context, target string, history bool) (string, error) {
	if strings.TrimSpace(target) == "" {
		return "", fmt.Errorf("target is required")
	}

	if !history {
		return c.capturePaneRange(ctx, target, "", "")
	}

	return c.capturePaneHistory(ctx, target)
}

// HistorySize reports the number of history lines for a pane.
func (c *Client) HistorySize(ctx context.Context, target string) (int, error) {
	if strings.TrimSpace(target) == "" {
		return 0, fmt.Errorf("target is required")
	}

	cmd := fmt.Sprintf("tmux display-message -p -t %s '#{history_size}'", escapeArg(target))
	stdout, _, err := c.exec.Exec(ctx, cmd)
	if err != nil {
		return 0, fmt.Errorf("tmux display-message failed: %w", err)
	}

	raw := strings.TrimSpace(string(stdout))
	if raw == "" {
		return 0, fmt.Errorf("history size unavailable")
	}
	size, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid history size %q: %w", raw, err)
	}
	if size < 0 {
		size = 0
	}
	return size, nil
}

func (c *Client) capturePaneHistory(ctx context.Context, target string) (string, error) {
	historySize, err := c.HistorySize(ctx, target)
	if err != nil {
		if historyMaxLines > 0 {
			return c.capturePaneRange(ctx, target, fmt.Sprintf("-%d", historyMaxLines), "")
		}
		return c.capturePaneAll(ctx, target)
	}

	if historySize <= historyChunkLines {
		return c.capturePaneAll(ctx, target)
	}

	if historySize > historyMaxLines {
		historySize = historyMaxLines
	}

	var builder strings.Builder
	lastEndedWithNewline := false
	appendChunk := func(chunk string) {
		if chunk == "" {
			return
		}
		if builder.Len() > 0 && !lastEndedWithNewline && !strings.HasPrefix(chunk, "\n") {
			builder.WriteByte('\n')
		}
		builder.WriteString(chunk)
		lastEndedWithNewline = strings.HasSuffix(chunk, "\n")
	}

	for start := -historySize; start < 0; start += historyChunkLines {
		end := start + historyChunkLines - 1
		if end >= 0 {
			end = -1
		}
		chunk, err := c.capturePaneRange(ctx, target, strconv.Itoa(start), strconv.Itoa(end))
		if err != nil {
			return "", err
		}
		appendChunk(chunk)
	}

	visible, err := c.capturePaneRange(ctx, target, "0", "-")
	if err != nil {
		return "", err
	}
	appendChunk(visible)

	return builder.String(), nil
}

func (c *Client) capturePaneAll(ctx context.Context, target string) (string, error) {
	return c.capturePaneRange(ctx, target, "-", "")
}

func (c *Client) capturePaneRange(ctx context.Context, target, start, end string) (string, error) {
	cmd := fmt.Sprintf("tmux capture-pane -t %s -p", escapeArg(target))
	if strings.TrimSpace(start) != "" {
		cmd = fmt.Sprintf("%s -S %s", cmd, start)
	}
	if strings.TrimSpace(end) != "" {
		cmd = fmt.Sprintf("%s -E %s", cmd, end)
	}

	stdout, _, err := c.exec.Exec(ctx, cmd)
	if err != nil {
		return "", fmt.Errorf("tmux capture-pane failed: %w", err)
	}

	return string(stdout), nil
}

// KillPane kills a specific pane.
func (c *Client) KillPane(ctx context.Context, target string) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("target is required")
	}

	cmd := fmt.Sprintf("tmux kill-pane -t %s", escapeArg(target))
	if _, _, err := c.exec.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("tmux kill-pane failed: %w", err)
	}

	return nil
}

// SelectPane selects (focuses) a pane.
func (c *Client) SelectPane(ctx context.Context, target string) error {
	if strings.TrimSpace(target) == "" {
		return fmt.Errorf("target is required")
	}

	cmd := fmt.Sprintf("tmux select-pane -t %s", escapeArg(target))
	if _, _, err := c.exec.Exec(ctx, cmd); err != nil {
		return fmt.Errorf("tmux select-pane failed: %w", err)
	}

	return nil
}

// Common errors
var (
	ErrSessionExists   = fmt.Errorf("session already exists")
	ErrSessionNotFound = fmt.Errorf("session not found")
)

func isNoServerRunning(stderr []byte) bool {
	return strings.Contains(strings.ToLower(string(stderr)), "no server running")
}

func isSessionNotFound(stderr []byte) bool {
	s := strings.ToLower(string(stderr))
	return strings.Contains(s, "session not found") ||
		strings.Contains(s, "can't find session")
}

func isDuplicateSession(stderr []byte) bool {
	return strings.Contains(strings.ToLower(string(stderr)), "duplicate session")
}

// escapeSessionName escapes a session name for use in tmux commands.
func escapeSessionName(name string) string {
	// For session names, we just need to quote if there are special chars
	if strings.ContainsAny(name, " \t\n'\"\\$`!") {
		return fmt.Sprintf("'%s'", strings.ReplaceAll(name, "'", "'\\''"))
	}
	return name
}

// escapeArg escapes an argument for shell use.
func escapeArg(arg string) string {
	// Use single quotes and escape any internal single quotes
	return fmt.Sprintf("'%s'", strings.ReplaceAll(arg, "'", "'\\''"))
}
