package fmail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

type TopicSummary struct {
	Name         string    `json:"name"`
	Messages     int       `json:"messages"`
	LastActivity time.Time `json:"last_activity,omitempty"`
}

func runTopics(cmd *cobra.Command, args []string) error {
	runtime, err := EnsureRuntime(cmd)
	if err != nil {
		return err
	}

	jsonOutput, _ := cmd.Flags().GetBool("json")

	store, err := NewStore(runtime.Root)
	if err != nil {
		return Exitf(ExitCodeFailure, "init store: %v", err)
	}

	topics, err := store.ListTopics()
	if err != nil {
		return Exitf(ExitCodeFailure, "list topics: %v", err)
	}

	if jsonOutput {
		payload, err := json.MarshalIndent(topics, "", "  ")
		if err != nil {
			return Exitf(ExitCodeFailure, "encode topics: %v", err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(payload))
		return nil
	}

	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 8, 2, ' ', 0)
	fmt.Fprintln(writer, "TOPIC\tMESSAGES\tLAST ACTIVITY")
	now := time.Now().UTC()
	for _, topic := range topics {
		last := formatRelative(now, topic.LastActivity)
		fmt.Fprintf(writer, "%s\t%d\t%s\n", topic.Name, topic.Messages, last)
	}
	if err := writer.Flush(); err != nil {
		return Exitf(ExitCodeFailure, "write output: %v", err)
	}
	return nil
}

func (s *Store) ListTopics() ([]TopicSummary, error) {
	if s == nil {
		return nil, fmt.Errorf("store is nil")
	}

	root := filepath.Join(s.Root, "topics")
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	summaries := make([]TopicSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		topicName := entry.Name()
		if err := ValidateTopic(topicName); err != nil {
			continue
		}
		summary, err := s.scanTopic(topicName, filepath.Join(root, topicName))
		if err != nil {
			return nil, err
		}
		if summary.Messages == 0 {
			continue
		}
		summaries = append(summaries, summary)
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})
	return summaries, nil
}

func (s *Store) scanTopic(name, dir string) (TopicSummary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return TopicSummary{Name: name}, nil
		}
		return TopicSummary{}, err
	}

	summary := TopicSummary{Name: name}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		summary.Messages++
		if ts, ok := parseMessageTime(entry.Name()); ok {
			if ts.After(summary.LastActivity) {
				summary.LastActivity = ts
			}
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		mod := info.ModTime().UTC()
		if mod.After(summary.LastActivity) {
			summary.LastActivity = mod
		}
	}
	return summary, nil
}

func parseMessageTime(filename string) (time.Time, bool) {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	if len(base) < len("20060102-150405") {
		return time.Time{}, false
	}
	prefix := base[:len("20060102-150405")]
	ts, err := time.Parse("20060102-150405", prefix)
	if err != nil {
		return time.Time{}, false
	}
	return ts.UTC(), true
}
