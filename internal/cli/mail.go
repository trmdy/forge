// Package cli provides Forge Mail CLI commands.
package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/agentmail"
)

const (
	defaultMailURL     = "http://127.0.0.1:8765/mcp/"
	defaultMailLimit   = 50
	defaultMailTimeout = 5 * time.Second
)

type mailBackendKind string

const (
	mailBackendMCP   mailBackendKind = "mcp"
	mailBackendLocal mailBackendKind = "local"
)

type mailConfig struct {
	URL     string
	Project string
	Agent   string
	Limit   int
	Timeout time.Duration
}

var (
	mailURL     string
	mailProject string
	mailAgent   string
	mailLimit   int
	mailTimeout time.Duration

	mailTo          []string
	mailSubject     string
	mailBody        string
	mailFile        string
	mailStdin       bool
	mailPriority    string
	mailAckRequired bool
	mailFrom        string

	mailUnread bool
)

func init() {
	rootCmd.AddCommand(mailCmd)
	mailCmd.AddCommand(mailSendCmd)
	mailCmd.AddCommand(mailInboxCmd)
	mailCmd.AddCommand(mailReadCmd)
	mailCmd.AddCommand(mailAckCmd)

	mailCmd.PersistentFlags().StringVar(&mailURL, "url", "", "Agent Mail MCP URL (default FORGE_AGENT_MAIL_URL)")
	mailCmd.PersistentFlags().StringVar(&mailProject, "project", "", "Agent Mail project key (default FORGE_AGENT_MAIL_PROJECT or repo root)")
	mailCmd.PersistentFlags().StringVar(&mailAgent, "agent", "", "Agent Mail agent name (default FORGE_AGENT_MAIL_AGENT)")
	mailCmd.PersistentFlags().IntVar(&mailLimit, "limit", 0, "max inbox messages to fetch (default FORGE_AGENT_MAIL_LIMIT or 50)")
	mailCmd.PersistentFlags().DurationVar(&mailTimeout, "timeout", 0, "Agent Mail request timeout (default FORGE_AGENT_MAIL_TIMEOUT or 5s)")

	mailSendCmd.Flags().StringSliceVar(&mailTo, "to", nil, "recipient agent name (required)")
	mailSendCmd.Flags().StringVarP(&mailSubject, "subject", "s", "", "message subject (required)")
	mailSendCmd.Flags().StringVarP(&mailBody, "body", "b", "", "message body")
	mailSendCmd.Flags().StringVarP(&mailFile, "file", "f", "", "read message body from file")
	mailSendCmd.Flags().BoolVar(&mailStdin, "stdin", false, "read message body from stdin")
	mailSendCmd.Flags().StringVar(&mailPriority, "priority", "normal", "message priority (low, normal, high, urgent)")
	mailSendCmd.Flags().BoolVar(&mailAckRequired, "ack-required", false, "request acknowledgement")
	mailSendCmd.Flags().StringVar(&mailFrom, "from", "", "sender agent name (default --agent or FORGE_AGENT_MAIL_AGENT)")

	mailInboxCmd.Flags().BoolVar(&mailUnread, "unread", false, "show only unread messages")
}

var mailCmd = &cobra.Command{
	Use:   "mail",
	Short: "Forge Mail messaging",
	Long: `Forge Mail provides lightweight agent-to-agent messaging.

If Agent Mail MCP is configured, messages are sent through the MCP server.
Otherwise, Forge falls back to a local mail store in ~/.config/forge/mail.db.
Legacy SWARM_AGENT_MAIL_* environment variables are still accepted.`,
}

var mailSendCmd = &cobra.Command{
	Use:   "send",
	Short: "Send a message to an agent mailbox",
	Long: `Send a message to another agent mailbox.

Use --to to specify the recipient(s), and --subject/--body to provide content.
You can also provide the body via --file or --stdin.`,
	Example: `  forge mail send --to agent-a1 --subject "Task handoff" --body "Please review PR #123"
  forge mail send --to agent-a1 --subject "Task handoff" --file message.md
  cat message.md | forge mail send --to agent-a1 --subject "Task handoff" --stdin`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, backend, err := resolveMailConfig()
		if err != nil {
			return err
		}

		from := strings.TrimSpace(mailFrom)
		if from == "" {
			from = cfg.Agent
		}
		if from == "" {
			return errors.New("sender required (set --from or FORGE_AGENT_MAIL_AGENT)")
		}

		recipients := normalizeMailRecipients(mailTo)
		if len(recipients) == 0 {
			return errors.New("--to is required")
		}

		subject := strings.TrimSpace(mailSubject)
		if subject == "" {
			return errors.New("--subject is required")
		}

		body, err := resolveMailBody()
		if err != nil {
			return err
		}

		priority, err := normalizeMailPriority(mailPriority)
		if err != nil {
			return err
		}

		req := mailSendRequest{
			Project:     cfg.Project,
			From:        from,
			To:          recipients,
			Subject:     subject,
			Body:        body,
			Priority:    priority,
			AckRequired: mailAckRequired,
		}

		switch backend {
		case mailBackendMCP:
			client := newMailMCPClient(cfg)
			if err := client.SendMessage(context.Background(), req); err != nil {
				return err
			}

			result := mailSendResult{
				Backend: backend,
				Project: cfg.Project,
				From:    from,
				To:      recipients,
				Subject: subject,
			}

			if IsJSONOutput() || IsJSONLOutput() {
				return WriteOutput(os.Stdout, result)
			}

			fmt.Printf("Sent message to %d recipient(s)\n", len(recipients))
			return nil
		case mailBackendLocal:
			store, err := openMailStore()
			if err != nil {
				return err
			}
			defer store.Close()

			ids, err := store.SendLocal(context.Background(), req)
			if err != nil {
				return err
			}

			result := mailSendResult{
				Backend:    backend,
				Project:    cfg.Project,
				From:       from,
				To:         recipients,
				Subject:    subject,
				MessageIDs: ids,
			}

			if IsJSONOutput() || IsJSONLOutput() {
				return WriteOutput(os.Stdout, result)
			}

			fmt.Printf("Saved message to local mailbox for %d recipient(s)\n", len(ids))
			return nil
		default:
			return fmt.Errorf("unknown mail backend: %s", backend)
		}
	},
}

var mailInboxCmd = &cobra.Command{
	Use:   "inbox",
	Short: "List mailbox messages",
	Long: `List messages for an agent mailbox.

By default, reads the inbox for --agent or FORGE_AGENT_MAIL_AGENT.`,
	Example: `  forge mail inbox --agent agent-a1
  forge mail inbox --agent agent-a1 --unread
  forge mail inbox --agent agent-a1 --since 1h`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, backend, err := resolveMailConfig()
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Agent) == "" {
			return errors.New("--agent is required (or set FORGE_AGENT_MAIL_AGENT)")
		}

		since, err := GetSinceTime()
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}

		var messages []mailMessage
		switch backend {
		case mailBackendMCP:
			store, err := openMailStore()
			if err != nil {
				return err
			}
			defer store.Close()

			client := newMailMCPClient(cfg)
			messages, err = client.FetchInbox(context.Background(), mailInboxRequest{
				Project: cfg.Project,
				Agent:   cfg.Agent,
				Limit:   cfg.Limit,
				Since:   since,
			})
			if err != nil {
				return err
			}

			if len(messages) > 0 {
				statuses, err := store.LoadStatus(context.Background(), cfg.Project, cfg.Agent, collectMessageIDs(messages))
				if err != nil {
					return err
				}
				applyMailStatuses(messages, statuses)
			}
		case mailBackendLocal:
			store, err := openMailStore()
			if err != nil {
				return err
			}
			defer store.Close()

			messages, err = store.ListLocal(context.Background(), cfg.Project, cfg.Agent, since, mailUnread, cfg.Limit)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown mail backend: %s", backend)
		}

		if mailUnread {
			messages = filterUnread(messages)
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, messages)
		}

		if len(messages) == 0 {
			fmt.Println("No messages found")
			return nil
		}

		rows := make([][]string, 0, len(messages))
		for _, msg := range messages {
			rows = append(rows, []string{
				formatMailID(msg.ID),
				msg.From,
				msg.Subject,
				formatRelativeTime(msg.CreatedAt),
				formatMailStatus(msg),
			})
		}

		return writeTable(os.Stdout, []string{"ID", "FROM", "SUBJECT", "TIME", "STATUS"}, rows)
	},
}

var mailReadCmd = &cobra.Command{
	Use:   "read <message-id>",
	Short: "Read a mailbox message",
	Long: `Read a specific message by ID.

Use --agent to specify which mailbox to read from.`,
	Example: `  forge mail read m-001 --agent agent-a1`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, backend, err := resolveMailConfig()
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Agent) == "" {
			return errors.New("--agent is required (or set FORGE_AGENT_MAIL_AGENT)")
		}

		messageID, err := parseMailID(args[0])
		if err != nil {
			return err
		}

		since, err := GetSinceTime()
		if err != nil {
			return fmt.Errorf("invalid --since value: %w", err)
		}

		var message mailMessage
		switch backend {
		case mailBackendMCP:
			store, err := openMailStore()
			if err != nil {
				return err
			}
			defer store.Close()

			client := newMailMCPClient(cfg)
			message, err = client.ReadMessage(context.Background(), mailReadRequest{
				Project:   cfg.Project,
				Agent:     cfg.Agent,
				MessageID: messageID,
				Limit:     cfg.Limit,
				Since:     since,
			})
			if err != nil {
				return err
			}

			now := time.Now().UTC()
			if err := client.MarkRead(context.Background(), mailStatusRequest{
				Project:   cfg.Project,
				Agent:     cfg.Agent,
				MessageID: messageID,
			}); err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to mark read in MCP: %v\n", err)
			}
			if err := store.MarkRead(context.Background(), cfg.Project, cfg.Agent, messageID, now); err != nil {
				return err
			}
			message.ReadAt = &now
		case mailBackendLocal:
			store, err := openMailStore()
			if err != nil {
				return err
			}
			defer store.Close()

			message, err = store.GetLocal(context.Background(), cfg.Project, cfg.Agent, messageID)
			if err != nil {
				return err
			}
			now := time.Now().UTC()
			if err := store.MarkRead(context.Background(), cfg.Project, cfg.Agent, messageID, now); err != nil {
				return err
			}
			message.ReadAt = &now
		default:
			return fmt.Errorf("unknown mail backend: %s", backend)
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, message)
		}

		fmt.Printf("ID:      %s\n", formatMailID(message.ID))
		fmt.Printf("From:    %s\n", message.From)
		fmt.Printf("Subject: %s\n", message.Subject)
		fmt.Printf("Date:    %s\n", message.CreatedAt.Format(time.RFC3339))
		if message.ThreadID != "" {
			fmt.Printf("Thread:  %s\n", message.ThreadID)
		}
		if message.Importance != "" {
			fmt.Printf("Priority: %s\n", formatMailPriorityLabel(message.Importance))
		}
		if message.AckRequired {
			fmt.Printf("Ack:     required\n")
		}
		fmt.Println()
		fmt.Println(message.Body)
		return nil
	},
}

var mailAckCmd = &cobra.Command{
	Use:     "ack <message-id>",
	Short:   "Acknowledge a mailbox message",
	Long:    "Send an acknowledgement for a specific message.",
	Example: `  forge mail ack m-001 --agent agent-a1`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, backend, err := resolveMailConfig()
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.Agent) == "" {
			return errors.New("--agent is required (or set FORGE_AGENT_MAIL_AGENT)")
		}

		messageID, err := parseMailID(args[0])
		if err != nil {
			return err
		}

		now := time.Now().UTC()

		switch backend {
		case mailBackendMCP:
			store, err := openMailStore()
			if err != nil {
				return err
			}
			defer store.Close()

			client := newMailMCPClient(cfg)
			if err := client.Acknowledge(context.Background(), mailStatusRequest{
				Project:   cfg.Project,
				Agent:     cfg.Agent,
				MessageID: messageID,
			}); err != nil {
				return err
			}
			if err := store.MarkAck(context.Background(), cfg.Project, cfg.Agent, messageID, now); err != nil {
				return err
			}
		case mailBackendLocal:
			store, err := openMailStore()
			if err != nil {
				return err
			}
			defer store.Close()

			if err := store.MarkAck(context.Background(), cfg.Project, cfg.Agent, messageID, now); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown mail backend: %s", backend)
		}

		if IsJSONOutput() || IsJSONLOutput() {
			return WriteOutput(os.Stdout, map[string]any{
				"id":           messageID,
				"agent":        cfg.Agent,
				"project":      cfg.Project,
				"acked_at":     now.Format(time.RFC3339),
				"backend":      backend,
				"acknowledged": true,
			})
		}

		fmt.Printf("Acknowledged message %s\n", formatMailID(messageID))
		return nil
	},
}

type mailSendRequest struct {
	Project     string
	From        string
	To          []string
	Subject     string
	Body        string
	Priority    string
	AckRequired bool
}

type mailSendResult struct {
	Backend    mailBackendKind `json:"backend"`
	Project    string          `json:"project"`
	From       string          `json:"from"`
	To         []string        `json:"to"`
	Subject    string          `json:"subject"`
	MessageIDs []int64         `json:"message_ids,omitempty"`
}

type mailInboxRequest struct {
	Project string
	Agent   string
	Limit   int
	Since   *time.Time
}

type mailReadRequest struct {
	Project   string
	Agent     string
	MessageID int64
	Limit     int
	Since     *time.Time
}

type mailStatusRequest struct {
	Project   string
	Agent     string
	MessageID int64
}

type mailMessage struct {
	ID          int64      `json:"id"`
	ThreadID    string     `json:"thread_id,omitempty"`
	From        string     `json:"from"`
	Subject     string     `json:"subject"`
	Body        string     `json:"body,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	Importance  string     `json:"importance,omitempty"`
	AckRequired bool       `json:"ack_required,omitempty"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	AckedAt     *time.Time `json:"acked_at,omitempty"`
	Backend     string     `json:"backend,omitempty"`
}

type mailStatus struct {
	ReadAt  *time.Time
	AckedAt *time.Time
}

func resolveMailConfig() (mailConfig, mailBackendKind, error) {
	cfg := mailConfigFromEnv()

	if strings.TrimSpace(mailURL) != "" {
		cfg.URL = mailURL
	}
	if strings.TrimSpace(mailProject) != "" {
		cfg.Project = mailProject
	}
	if strings.TrimSpace(mailAgent) != "" {
		cfg.Agent = mailAgent
	}
	if mailLimit > 0 {
		cfg.Limit = mailLimit
	}
	if mailTimeout > 0 {
		cfg.Timeout = mailTimeout
	}

	if strings.TrimSpace(cfg.URL) == "" {
		cfg.URL = defaultMailURL
	}
	if cfg.Limit <= 0 {
		cfg.Limit = defaultMailLimit
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultMailTimeout
	}

	if strings.TrimSpace(cfg.Project) == "" {
		project, err := detectProjectKey()
		if err != nil {
			return cfg, mailBackendLocal, err
		}
		cfg.Project = project
	}

	useMCP := false
	if strings.TrimSpace(mailURL) != "" || strings.TrimSpace(mailProject) != "" || strings.TrimSpace(mailAgent) != "" {
		useMCP = true
	}
	if !useMCP {
		if getEnvWithFallback("FORGE_AGENT_MAIL_URL", "SWARM_AGENT_MAIL_URL") != "" ||
			getEnvWithFallback("FORGE_AGENT_MAIL_PROJECT", "SWARM_AGENT_MAIL_PROJECT") != "" ||
			getEnvWithFallback("FORGE_AGENT_MAIL_AGENT", "SWARM_AGENT_MAIL_AGENT") != "" {
			useMCP = true
		}
	}
	if !useMCP && cfg.Project != "" {
		detected, err := agentmail.HasAgentMailConfig(cfg.Project)
		if err == nil && detected {
			useMCP = true
		}
	}

	if useMCP {
		return cfg, mailBackendMCP, nil
	}
	return cfg, mailBackendLocal, nil
}

func mailConfigFromEnv() mailConfig {
	cfg := mailConfig{
		URL:     getEnvWithFallback("FORGE_AGENT_MAIL_URL", "SWARM_AGENT_MAIL_URL"),
		Project: getEnvWithFallback("FORGE_AGENT_MAIL_PROJECT", "SWARM_AGENT_MAIL_PROJECT"),
		Agent:   getEnvWithFallback("FORGE_AGENT_MAIL_AGENT", "SWARM_AGENT_MAIL_AGENT"),
	}

	if value := getEnvWithFallback("FORGE_AGENT_MAIL_LIMIT", "SWARM_AGENT_MAIL_LIMIT"); value != "" {
		if limit, err := strconv.Atoi(value); err == nil && limit > 0 {
			cfg.Limit = limit
		}
	}

	if value := getEnvWithFallback("FORGE_AGENT_MAIL_TIMEOUT", "SWARM_AGENT_MAIL_TIMEOUT"); value != "" {
		if parsed, ok := parseEnvDuration(value); ok {
			cfg.Timeout = parsed
		}
	}

	return cfg
}

func detectProjectKey() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to determine working directory: %w", err)
	}
	root, ok := findGitRoot(cwd)
	if !ok {
		return filepath.Abs(cwd)
	}
	return filepath.Abs(root)
}

func findGitRoot(path string) (string, bool) {
	current := filepath.Clean(path)
	for {
		if isGitDir(current) {
			return current, true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", false
		}
		current = parent
	}
}

func isGitDir(path string) bool {
	info, err := os.Stat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir()
}

func resolveMailBody() (string, error) {
	sourceCount := 0
	if strings.TrimSpace(mailBody) != "" {
		sourceCount++
	}
	if mailFile != "" {
		sourceCount++
	}
	if mailStdin {
		sourceCount++
	}

	if sourceCount == 0 {
		return "", errors.New("message body required (--body, --file, or --stdin)")
	}
	if sourceCount > 1 {
		return "", errors.New("choose only one body source: --body, --file, or --stdin")
	}

	switch {
	case mailFile != "":
		return readMessageFromFile(mailFile)
	case mailStdin:
		return readMessageFromStdin()
	default:
		if strings.TrimSpace(mailBody) == "" {
			return "", errors.New("message body is empty")
		}
		return mailBody, nil
	}
}

func normalizeMailRecipients(values []string) []string {
	recipients := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, raw := range values {
		for _, item := range strings.Split(raw, ",") {
			value := strings.TrimSpace(item)
			if value == "" {
				continue
			}
			key := strings.ToLower(value)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			recipients = append(recipients, value)
		}
	}
	return recipients
}

func normalizeMailPriority(value string) (string, error) {
	priority := strings.TrimSpace(strings.ToLower(value))
	if priority == "" {
		return "normal", nil
	}
	switch priority {
	case "low", "normal", "high", "urgent":
		return priority, nil
	default:
		return "", errors.New("invalid priority (use low, normal, high, or urgent)")
	}
}

func parseMailID(value string) (int64, error) {
	trimmed := strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(trimmed), "m-") {
		trimmed = trimmed[2:]
	}
	if trimmed == "" {
		return 0, errors.New("message id required")
	}
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid message id: %s", value)
	}
	return id, nil
}

func formatMailID(id int64) string {
	if id <= 0 {
		return "-"
	}
	return fmt.Sprintf("m-%d", id)
}

func formatMailStatus(msg mailMessage) string {
	if msg.AckRequired && msg.AckedAt != nil {
		return "acked"
	}
	if msg.ReadAt != nil {
		return "read"
	}
	return "unread"
}

func formatMailPriorityLabel(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return strings.TrimSpace(value)
}

func collectMessageIDs(messages []mailMessage) []int64 {
	ids := make([]int64, 0, len(messages))
	for _, msg := range messages {
		if msg.ID > 0 {
			ids = append(ids, msg.ID)
		}
	}
	return ids
}

func applyMailStatuses(messages []mailMessage, statuses map[int64]mailStatus) {
	for i := range messages {
		status, ok := statuses[messages[i].ID]
		if !ok {
			continue
		}
		messages[i].ReadAt = status.ReadAt
		messages[i].AckedAt = status.AckedAt
	}
}

func filterUnread(messages []mailMessage) []mailMessage {
	if len(messages) == 0 {
		return messages
	}
	filtered := make([]mailMessage, 0, len(messages))
	for _, msg := range messages {
		if msg.ReadAt == nil {
			filtered = append(filtered, msg)
		}
	}
	return filtered
}
