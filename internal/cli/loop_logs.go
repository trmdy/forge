package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tOgg1/forge/internal/db"
	"github.com/tOgg1/forge/internal/loop"
)

var (
	logsFollow bool
	logsLines  int
	logsSince  string
	logsAll    bool
)

func init() {
	rootCmd.AddCommand(logsCmd)

	logsCmd.Flags().BoolVarP(&logsFollow, "follow", "f", false, "follow log output")
	logsCmd.Flags().IntVarP(&logsLines, "lines", "n", 50, "number of lines to show")
	logsCmd.Flags().StringVar(&logsSince, "since", "", "show logs since duration or timestamp")
	logsCmd.Flags().BoolVar(&logsAll, "all", false, "show logs for all loops in repo")
}

var logsCmd = &cobra.Command{
	Use:     "logs [loop]",
	Aliases: []string{"log"},
	Short:   "Tail loop logs",
	Args:    cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := openDatabase()
		if err != nil {
			return err
		}
		defer database.Close()

		loopRepo := db.NewLoopRepository(database)
		poolRepo := db.NewPoolRepository(database)
		profileRepo := db.NewProfileRepository(database)

		selector := loopSelector{}
		if len(args) > 0 {
			selector.LoopRef = args[0]
		} else if !logsAll {
			return fmt.Errorf("loop name required (or use --all)")
		}

		if logsAll {
			repoPath, err := resolveRepoPath("")
			if err != nil {
				return err
			}
			selector.Repo = repoPath
		}

		loops, err := selectLoops(context.Background(), loopRepo, poolRepo, profileRepo, selector)
		if err != nil {
			return err
		}
		if len(loops) == 0 {
			return fmt.Errorf("no loops matched")
		}

		for idx, loopEntry := range loops {
			path := loopEntry.LogPath
			if path == "" {
				path = loop.LogPath(GetConfig().Global.DataDir, loopEntry.Name, loopEntry.ID)
			}

			if idx > 0 && !IsQuiet() {
				fmt.Fprintln(os.Stdout, "")
			}
			if !IsQuiet() {
				fmt.Fprintf(os.Stdout, "==> %s <==\n", loopEntry.Name)
			}

			if logsFollow {
				if err := followFile(path, logsLines); err != nil {
					return err
				}
				continue
			}

			content, err := readLog(path, logsLines, logsSince)
			if err != nil {
				return err
			}
			if content != "" {
				fmt.Fprintln(os.Stdout, content)
			}
		}

		return nil
	},
}

func readLog(path string, lines int, since string) (string, error) {
	if lines <= 0 {
		lines = 50
	}

	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	parsedSince, _ := parseSince(since)

	scanner := bufio.NewScanner(file)
	buffer := make([]string, 0, lines)
	highlighter := newLogHighlighter()
	for scanner.Scan() {
		line := scanner.Text()
		if !parsedSince.IsZero() {
			if ts, ok := parseLogTimestamp(line); ok && ts.Before(parsedSince) {
				continue
			}
		}

		buffer = append(buffer, highlighter.HighlightLine(line))
		if len(buffer) > lines {
			buffer = buffer[1:]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.Join(buffer, "\n"), nil
}

func followFile(path string, lines int) error {
	if lines > 0 {
		content, err := readLog(path, lines, "")
		if err == nil && content != "" {
			fmt.Fprintln(os.Stdout, content)
		}
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	offset, _ := file.Seek(0, io.SeekEnd)
	reader := bufio.NewReader(file)
	highlighter := newLogHighlighter()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			time.Sleep(250 * time.Millisecond)
			if _, seekErr := file.Seek(offset, io.SeekStart); seekErr != nil {
				return seekErr
			}
			continue
		}
		offset += int64(len(line))
		fmt.Print(highlighter.HighlightLine(line))
	}
}

func parseSince(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, nil
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return time.Now().UTC().Add(-duration), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, err
	}
	return parsed.UTC(), nil
}

func parseLogTimestamp(line string) (time.Time, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "[") {
		return time.Time{}, false
	}
	end := strings.Index(line, "]")
	if end == -1 {
		return time.Time{}, false
	}
	ts := line[1:end]
	parsed, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}
