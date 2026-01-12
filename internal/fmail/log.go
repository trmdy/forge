package fmail

import (
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type logFilter struct {
	since *time.Time
	from  string
}

func runLog(cmd *cobra.Command, args []string) error {
	runtime, err := EnsureRuntime(cmd)
	if err != nil {
		return err
	}

	targetArg := ""
	if len(args) > 0 {
		targetArg = args[0]
	}
	target, err := parseWatchTarget(targetArg)
	if err != nil {
		return Exitf(ExitCodeFailure, "invalid target %q: %v", targetArg, err)
	}
	allowOtherDM, _ := cmd.Flags().GetBool("allow-other-dm")
	if err := ensureDMReadAccess(runtime, target, allowOtherDM, "read"); err != nil {
		return err
	}

	limit, _ := cmd.Flags().GetInt("limit")
	if limit < 0 {
		return usageError(cmd, "limit must be >= 0")
	}
	sinceFlag, _ := cmd.Flags().GetString("since")
	since, err := parseSince(sinceFlag, time.Now().UTC())
	if err != nil {
		return usageError(cmd, "invalid --since value: %v", err)
	}
	fromFlag, _ := cmd.Flags().GetString("from")
	from, err := normalizeFromFilter(fromFlag)
	if err != nil {
		return usageError(cmd, "invalid --from value: %v", err)
	}
	filter := logFilter{
		since: since,
		from:  from,
	}
	follow, _ := cmd.Flags().GetBool("follow")
	jsonOutput, _ := cmd.Flags().GetBool("json")

	store, err := NewStore(runtime.Root)
	if err != nil {
		return Exitf(ExitCodeFailure, "init store: %v", err)
	}

	followStart := time.Now().UTC()
	files, err := listMessageFiles(store, target)
	if err != nil {
		return Exitf(ExitCodeFailure, "log: %v", err)
	}

	seen := make(map[string]struct{}, len(files))
	for _, file := range files {
		seen[file.path] = struct{}{}
	}

	messages, err := loadMessageSorts(store, files)
	if err != nil {
		return Exitf(ExitCodeFailure, "log: %v", err)
	}
	messages = filterMessageSorts(messages, filter)
	sortMessageSorts(messages)
	if limit > 0 && len(messages) > limit {
		messages = messages[len(messages)-limit:]
	}

	for _, entry := range messages {
		if err := writeWatchMessage(cmd.OutOrStdout(), entry.message, jsonOutput); err != nil {
			return Exitf(ExitCodeFailure, "output: %v", err)
		}
	}

	if !follow {
		return nil
	}
	return followLog(cmd, store, target, seen, followStart, filter, jsonOutput)
}

func followLog(cmd *cobra.Command, store *Store, target watchTarget, seen map[string]struct{}, start time.Time, filter logFilter, jsonOutput bool) error {
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			messages, err := scanNewMessages(store, target, seen, start, messageSince{})
			if err != nil {
				return Exitf(ExitCodeFailure, "follow: %v", err)
			}
			for _, message := range filterMessages(messages, filter) {
				if err := writeWatchMessage(cmd.OutOrStdout(), message, jsonOutput); err != nil {
					return Exitf(ExitCodeFailure, "output: %v", err)
				}
			}
		}
	}
}

func filterMessages(messages []*Message, filter logFilter) []*Message {
	filtered := make([]*Message, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		if !filter.match(message) {
			continue
		}
		filtered = append(filtered, message)
	}
	return filtered
}

func filterMessageSorts(messages []messageSort, filter logFilter) []messageSort {
	filtered := make([]messageSort, 0, len(messages))
	for _, message := range messages {
		if message.message == nil {
			continue
		}
		if !filter.match(message.message) {
			continue
		}
		filtered = append(filtered, message)
	}
	return filtered
}

func (f logFilter) match(message *Message) bool {
	if message == nil {
		return false
	}
	if f.from != "" && !strings.EqualFold(message.From, f.from) {
		return false
	}
	if f.since != nil {
		if message.Time.IsZero() || message.Time.Before(*f.since) {
			return false
		}
	}
	return true
}

func loadMessageSorts(store *Store, files []messageFile) ([]messageSort, error) {
	messages := make([]messageSort, 0, len(files))
	for _, file := range files {
		message, err := store.ReadMessage(file.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		messages = append(messages, messageSort{message: message, path: file.path})
	}
	return messages, nil
}

func sortMessageSorts(messages []messageSort) {
	sort.Slice(messages, func(i, j int) bool {
		a := messages[i].message
		b := messages[j].message
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if !a.Time.Equal(b.Time) {
			return a.Time.Before(b.Time)
		}
		return messages[i].path < messages[j].path
	})
}

func normalizeFromFilter(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	trimmed = strings.TrimPrefix(trimmed, "@")
	return NormalizeAgentName(trimmed)
}

func parseSince(value string, now time.Time) (*time.Time, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil, nil
	}
	if strings.EqualFold(trimmed, "now") {
		t := now.UTC()
		return &t, nil
	}
	if dur, err := parseDurationWithDays(trimmed); err == nil {
		t := now.UTC().Add(-dur)
		return &t, nil
	}
	if t, err := time.Parse(time.RFC3339, trimmed); err == nil {
		utc := t.UTC()
		return &utc, nil
	}
	if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		utc := t.UTC()
		return &utc, nil
	}
	if t, err := time.Parse("2006-01-02", trimmed); err == nil {
		utc := t.UTC()
		return &utc, nil
	}
	if t, err := time.Parse("2006-01-02T15:04:05", trimmed); err == nil {
		utc := t.UTC()
		return &utc, nil
	}
	return nil, fmt.Errorf("use duration like '1h' or timestamp like '2024-01-15T10:30:00Z'")
}

func parseDurationWithDays(value string) (time.Duration, error) {
	if strings.HasSuffix(value, "d") {
		dayStr := strings.TrimSuffix(value, "d")
		var days float64
		if _, err := fmt.Sscanf(dayStr, "%f", &days); err != nil {
			return 0, err
		}
		return time.Duration(days * 24 * float64(time.Hour)), nil
	}
	return time.ParseDuration(value)
}
