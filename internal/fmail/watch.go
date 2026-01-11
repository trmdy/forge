package fmail

import (
	"encoding/json"
	"fmt"
	"io"
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
	if err := store.EnsureRoot(); err != nil {
		return Exitf(ExitCodeFailure, "init store: %v", err)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	start := time.Now()
	var deadline time.Time
	if timeout > 0 {
		deadline = start.Add(timeout)
	}

	seen := make(map[string]struct{})
	remaining := count

	ticker := time.NewTicker(watchPollInterval)
	defer ticker.Stop()

	for {
		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			messages, err := scanNewMessages(store, target, seen, start)
			if err != nil {
				return Exitf(ExitCodeFailure, "watch: %v", err)
			}
			for _, message := range messages {
				if err := writeWatchMessage(cmd.OutOrStdout(), message, jsonOutput); err != nil {
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

func scanNewMessages(store *Store, target watchTarget, seen map[string]struct{}, start time.Time) ([]*Message, error) {
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
