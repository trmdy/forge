package fmail

import (
	"encoding/json"
	"io"
	"strings"
)

const robotHelpSpecVersion = "2.1.0"

type robotHelpCommand struct {
	Usage       string   `json:"usage"`
	Flags       []string `json:"flags,omitempty"`
	Examples    []string `json:"examples,omitempty"`
	Description string   `json:"description,omitempty"`
}

type robotHelpPatterns struct {
	RequestResponse []string `json:"request_response"`
	Broadcast       string   `json:"broadcast"`
	Coordinate      []string `json:"coordinate"`
}

type robotHelpPayload struct {
	Name          string                      `json:"name"`
	Version       string                      `json:"version"`
	Description   string                      `json:"description"`
	Setup         string                      `json:"setup"`
	Commands      map[string]robotHelpCommand `json:"commands"`
	Patterns      robotHelpPatterns           `json:"patterns"`
	Env           map[string]string           `json:"env"`
	MessageFormat map[string]string           `json:"message_format"`
	Storage       string                      `json:"storage"`
}

func robotHelp(version string) robotHelpPayload {
	return robotHelpPayload{
		Name:        "fmail",
		Version:     normalizeRobotHelpVersion(version),
		Description: "Agent-to-agent messaging via .fmail/ files",
		Setup:       "export FMAIL_AGENT=<your-name>",
		Commands: map[string]robotHelpCommand{
			"send": {
				Usage: "fmail send <topic|@agent> <message>",
				Flags: []string{"-f FILE", "--reply-to ID", "--priority low|normal|high"},
				Examples: []string{
					"fmail send task 'implement auth'",
					"fmail send @reviewer 'check PR #42'",
				},
			},
			"log": {
				Usage: "fmail log [topic|@agent] [-n N] [--since TIME]",
				Flags: []string{"-n LIMIT", "--since TIME", "--from AGENT", "--json", "-f/--follow", "--allow-other-dm"},
				Examples: []string{
					"fmail log task -n 5",
					"fmail log @$FMAIL_AGENT --since 1h",
				},
			},
			"watch": {
				Usage: "fmail watch [topic|@agent] [--timeout T] [--count N]",
				Flags: []string{"--timeout DURATION", "--count N", "--json", "--allow-other-dm"},
				Examples: []string{
					"fmail watch task",
					"fmail watch @$FMAIL_AGENT --count 1 --timeout 2m",
				},
			},
			"who": {
				Usage:       "fmail who [--json]",
				Description: "List agents in project",
			},
			"status": {
				Usage: "fmail status [message] [--clear]",
				Examples: []string{
					"fmail status 'working on auth'",
					"fmail status --clear",
				},
			},
			"register": {
				Usage: "fmail register [name]",
				Flags: []string{"--json"},
				Examples: []string{
					"fmail register",
					"fmail register agent-42",
				},
			},
			"topics": {
				Usage:       "fmail topics [--json]",
				Description: "List topics with activity",
			},
			"gc": {
				Usage: "fmail gc [--days N] [--dry-run]",
			},
		},
		Patterns: robotHelpPatterns{
			RequestResponse: []string{
				"fmail send @worker 'analyze src/auth.go'",
				"response=$(fmail watch @$FMAIL_AGENT --count 1 --timeout 2m)",
			},
			Broadcast: "fmail send status 'starting work'",
			Coordinate: []string{
				"fmail send editing 'src/auth.go'",
				"fmail log editing --since 5m --json | grep -q 'auth.go'",
			},
		},
		Env: map[string]string{
			"FMAIL_AGENT":   "Your agent name (strongly recommended)",
			"FMAIL_ROOT":    "Project directory (auto-detected)",
			"FMAIL_PROJECT": "Project ID for cross-host sync",
		},
		MessageFormat: map[string]string{
			"id":   "YYYYMMDD-HHMMSS-NNNN",
			"from": "sender agent name",
			"to":   "topic or @agent",
			"time": "ISO 8601 timestamp",
			"body": "string or JSON object",
		},
		Storage: ".fmail/topics/<topic>/<id>.json and .fmail/dm/<agent>/<id>.json",
	}
}

func normalizeRobotHelpVersion(version string) string {
	trimmed := strings.TrimSpace(version)
	if trimmed == "" || strings.EqualFold(trimmed, "dev") {
		return robotHelpSpecVersion
	}
	return strings.TrimPrefix(trimmed, "v")
}

func writeRobotHelp(w io.Writer, version string) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(robotHelp(version)); err != nil {
		return Exitf(ExitCodeFailure, "write robot help: %v", err)
	}
	return nil
}

func hasRobotHelpFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--robot-help" || strings.HasPrefix(arg, "--robot-help=") {
			return true
		}
	}
	return false
}
