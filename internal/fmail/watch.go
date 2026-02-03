package fmail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const watchPollInterval = 100 * time.Millisecond

type watchTargetMode int

const (
	watchAllTopics watchTargetMode = iota
	watchTopic
	watchDM
)

type watchTarget struct {
	mode watchTargetMode
	name string
}

type watchOptions struct {
	count      int
	jsonOutput bool
	deadline   time.Time
}

type watchFallback struct {
	scanStart time.Time
	since     messageSince
}

type messageSince struct {
	id   string
	time *time.Time
}

type messageFile struct {
	path    string
	modTime time.Time
}

type messageSort struct {
	message *Message
	path    string
}

func runWatch(cmd *cobra.Command, args []string) error {
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
	if err := ensureDMReadAccess(runtime, target, allowOtherDM, "watch"); err != nil {
		return err
	}

	count, _ := cmd.Flags().GetInt("count")
	if count < 0 {
		return usageError(cmd, "count must be >= 0")
	}
	timeout, _ := cmd.Flags().GetDuration("timeout")
	if timeout < 0 {
		return usageError(cmd, "timeout must be >= 0")
	}
	jsonOutput, _ := cmd.Flags().GetBool("json")

	store, err := NewStore(runtime.Root)
	if err != nil {
		return Exitf(ExitCodeFailure, "init store: %v", err)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	start := time.Now().UTC()
	var deadline time.Time
	if timeout > 0 {
		deadline = start.Add(timeout)
	}
	opts := watchOptions{
		count:      count,
		jsonOutput: jsonOutput,
		deadline:   deadline,
	}

	fallback, err := watchConnected(ctx, runtime, target, opts, start, cmd.OutOrStdout())
	if err != nil {
		return err
	}
	if fallback != nil {
		return watchStandalone(ctx, store, target, opts, fallback.scanStart, fallback.since, cmd.OutOrStdout())
	}
	return nil
}

func watchConnected(ctx context.Context, runtime *Runtime, target watchTarget, opts watchOptions, start time.Time, out io.Writer) (*watchFallback, error) {
	if runtime == nil {
		return nil, Exitf(ExitCodeFailure, "runtime unavailable")
	}
	projectID, err := resolveProjectID(runtime.Root)
	if err != nil {
		return nil, Exitf(ExitCodeFailure, "resolve project id: %v", err)
	}

	topic := watchTopicRequest(target)
	allowDM := target.mode != watchAllTopics

	remaining := opts.count
	lastSeenID := ""
	sinceStart := start
	everConnected := false

	backoff := []time.Duration{0, 200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond}
	var reconnectUntil time.Time
	reconnectAttempts := 0

	for {
		if ctx.Err() != nil {
			return nil, nil
		}
		if !opts.deadline.IsZero() && time.Now().After(opts.deadline) {
			return nil, nil
		}

		if !reconnectUntil.IsZero() {
			if time.Now().After(reconnectUntil) {
				return &watchFallback{scanStart: time.Time{}, since: fallbackSince(lastSeenID, start)}, nil
			}
			delay := backoff[len(backoff)-1]
			if reconnectAttempts < len(backoff) {
				delay = backoff[reconnectAttempts]
			}
			reconnectAttempts++
			if !opts.deadline.IsZero() {
				remaining := time.Until(opts.deadline)
				if remaining <= 0 {
					return nil, nil
				}
				if delay > remaining {
					delay = remaining
				}
			}
			if delay > 0 {
				timer := time.NewTimer(delay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return nil, nil
				case <-timer.C:
				}
			}
		}

		conn, err := dialForged(runtime.Root)
		if err != nil {
			if everConnected {
				if reconnectUntil.IsZero() {
					reconnectUntil = time.Now().Add(forgedReconnectWait)
					reconnectAttempts = 0
				}
				continue
			}
			return &watchFallback{scanStart: start}, nil
		}
		reconnectUntil = time.Time{} //nolint:staticcheck // reset reconnect window after success
		reconnectAttempts = 0

		if !opts.deadline.IsZero() {
			_ = conn.conn.SetReadDeadline(opts.deadline)
		}

		host, _ := os.Hostname()
		req := mailWatchRequest{
			mailBaseRequest: mailBaseRequest{
				Cmd:       "watch",
				ProjectID: projectID,
				Agent:     runtime.Agent,
				Host:      host,
				ReqID:     nextReqID(),
			},
			Topic: topic,
			Since: watchSinceValue(lastSeenID, sinceStart),
		}

		if err := conn.writeJSON(req); err != nil {
			conn.Close()
			if everConnected {
				reconnectUntil = time.Now().Add(forgedReconnectWait)
				reconnectAttempts = 0
				continue
			}
			return &watchFallback{scanStart: start}, nil
		}

		line, err := conn.readLine()
		if err != nil {
			conn.Close()
			if everConnected {
				reconnectUntil = time.Now().Add(forgedReconnectWait)
				reconnectAttempts = 0
				continue
			}
			return &watchFallback{scanStart: start}, nil
		}

		var ack mailResponse
		if err := json.Unmarshal(line, &ack); err != nil {
			conn.Close()
			return nil, Exitf(ExitCodeFailure, "invalid forged response: %v", err)
		}
		if !ack.OK {
			conn.Close()
			return nil, Exitf(ExitCodeFailure, "forged: %s", formatForgedError(ack.Error))
		}
		everConnected = true
		stopWatch := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				_ = conn.Close()
			case <-stopWatch:
			}
		}()

		for {
			if remaining == 0 && opts.count > 0 {
				close(stopWatch)
				conn.Close()
				return nil, nil
			}
			if ctx.Err() != nil {
				close(stopWatch)
				conn.Close()
				return nil, nil
			}

			line, err := conn.readLine()
			if err != nil {
				close(stopWatch)
				conn.Close()
				if isTimeout(err) {
					return nil, nil
				}
				reconnectUntil = time.Now().Add(forgedReconnectWait)
				reconnectAttempts = 0
				break
			}
			if len(line) == 0 {
				continue
			}

			var env mailEnvelope
			if err := json.Unmarshal(line, &env); err != nil {
				close(stopWatch)
				conn.Close()
				return nil, Exitf(ExitCodeFailure, "invalid forged stream data: %v", err)
			}
			if env.Msg != nil {
				if env.Msg.ID != "" {
					lastSeenID = env.Msg.ID
				}
				if allowDM || !strings.HasPrefix(env.Msg.To, "@") {
					if err := writeWatchMessage(out, env.Msg, opts.jsonOutput); err != nil {
						close(stopWatch)
						conn.Close()
						return nil, Exitf(ExitCodeFailure, "output: %v", err)
					}
					if remaining > 0 {
						remaining--
					}
				}
				continue
			}
			if env.OK != nil && !*env.OK {
				close(stopWatch)
				conn.Close()
				if shouldRetryWatch(env.Error) {
					reconnectUntil = time.Now().Add(forgedReconnectWait)
					reconnectAttempts = 0
					break
				}
				return nil, Exitf(ExitCodeFailure, "forged: %s", formatForgedError(env.Error))
			}
		}
	}
}

func watchStandalone(ctx context.Context, store *Store, target watchTarget, opts watchOptions, scanStart time.Time, since messageSince, out io.Writer) error {
	if err := store.EnsureRoot(); err != nil {
		return Exitf(ExitCodeFailure, "init store: %v", err)
	}

	seen := make(map[string]struct{})
	remaining := opts.count

	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()

	for {
		if !opts.deadline.IsZero() && time.Now().After(opts.deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			messages, err := scanNewMessages(store, target, seen, scanStart, since)
			if err != nil {
				return Exitf(ExitCodeFailure, "watch: %v", err)
			}
			for _, message := range messages {
				if err := writeWatchMessage(out, message, opts.jsonOutput); err != nil {
					return Exitf(ExitCodeFailure, "output: %v", err)
				}
				if remaining > 0 {
					remaining--
					if remaining == 0 {
						return nil
					}
				}
			}
		}
	}
}

func parseWatchTarget(arg string) (watchTarget, error) {
	trimmed := strings.TrimSpace(arg)
	if trimmed == "" {
		return watchTarget{mode: watchAllTopics}, nil
	}
	if strings.HasPrefix(trimmed, "@") {
		agent, err := NormalizeAgentName(strings.TrimPrefix(trimmed, "@"))
		if err != nil {
			return watchTarget{}, err
		}
		return watchTarget{mode: watchDM, name: agent}, nil
	}
	topic, err := NormalizeTopic(trimmed)
	if err != nil {
		return watchTarget{}, err
	}
	return watchTarget{mode: watchTopic, name: topic}, nil
}

func scanNewMessages(store *Store, target watchTarget, seen map[string]struct{}, start time.Time, since messageSince) ([]*Message, error) {
	files, err := listMessageFiles(store, target)
	if err != nil {
		return nil, err
	}

	updates := make([]messageSort, 0, len(files))
	for _, file := range files {
		if _, ok := seen[file.path]; ok {
			continue
		}
		if !start.IsZero() && file.modTime.Before(start) {
			seen[file.path] = struct{}{}
			continue
		}
		message, err := store.ReadMessage(file.path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !since.allows(message) {
			seen[file.path] = struct{}{}
			continue
		}
		seen[file.path] = struct{}{}
		updates = append(updates, messageSort{message: message, path: file.path})
	}

	sort.Slice(updates, func(i, j int) bool {
		a := updates[i].message
		b := updates[j].message
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if !a.Time.Equal(b.Time) {
			return a.Time.Before(b.Time)
		}
		return updates[i].path < updates[j].path
	})

	messages := make([]*Message, 0, len(updates))
	for _, update := range updates {
		messages = append(messages, update.message)
	}
	return messages, nil
}

func (s messageSince) allows(message *Message) bool {
	if message == nil {
		return false
	}
	if s.id != "" {
		return message.ID > s.id
	}
	if s.time != nil {
		if message.Time.IsZero() {
			return false
		}
		return message.Time.After(*s.time)
	}
	return true
}

func watchTopicRequest(target watchTarget) string {
	switch target.mode {
	case watchAllTopics:
		return "*"
	case watchDM:
		return "@" + target.name
	case watchTopic:
		return target.name
	default:
		return ""
	}
}

func watchSinceValue(lastSeenID string, start time.Time) string {
	if strings.TrimSpace(lastSeenID) != "" {
		return lastSeenID
	}
	if start.IsZero() {
		return ""
	}
	return start.UTC().Format(time.RFC3339Nano)
}

func fallbackSince(lastSeenID string, start time.Time) messageSince {
	if strings.TrimSpace(lastSeenID) != "" {
		return messageSince{id: lastSeenID}
	}
	if start.IsZero() {
		return messageSince{}
	}
	return messageSince{time: &start}
}

func shouldRetryWatch(err *mailErr) bool {
	if err == nil {
		return false
	}
	if err.Retryable {
		return true
	}
	return strings.EqualFold(err.Code, "backpressure")
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

func listMessageFiles(store *Store, target watchTarget) ([]messageFile, error) {
	switch target.mode {
	case watchAllTopics:
		return listAllTopicFiles(store)
	case watchTopic:
		return listFilesInDir(store.TopicDir(target.name))
	case watchDM:
		return listFilesInDir(store.DMDir(target.name))
	default:
		return nil, fmt.Errorf("unknown watch target")
	}
}

func listAllTopicFiles(store *Store) ([]messageFile, error) {
	root := filepath.Join(store.Root, "topics")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	dirs := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(root, entry.Name()))
		}
	}
	sort.Strings(dirs)

	files := make([]messageFile, 0)
	for _, dir := range dirs {
		list, err := listFilesInDir(dir)
		if err != nil {
			return nil, err
		}
		files = append(files, list...)
	}
	return files, nil
}

func listFilesInDir(dir string) ([]messageFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	names := make([]string, 0, len(entries))
	entryMap := make(map[string]os.DirEntry, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		names = append(names, entry.Name())
		entryMap[entry.Name()] = entry
	}
	sort.Strings(names)

	files := make([]messageFile, 0, len(names))
	for _, name := range names {
		entry := entryMap[name]
		info, err := entry.Info()
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		files = append(files, messageFile{
			path:    filepath.Join(dir, name),
			modTime: info.ModTime(),
		})
	}
	return files, nil
}

func writeWatchMessage(out io.Writer, message *Message, jsonOutput bool) error {
	if jsonOutput {
		data, err := json.Marshal(message)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, string(data))
		return err
	}
	body, err := formatMessageBody(message.Body)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s %s -> %s: %s\n", message.ID, message.From, message.To, body)
	return err
}

func formatMessageBody(body any) (string, error) {
	switch value := body.(type) {
	case string:
		return value, nil
	case json.RawMessage:
		return string(value), nil
	default:
		data, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
}
