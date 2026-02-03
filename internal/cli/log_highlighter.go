package cli

import "strings"

type logHighlighter struct {
	inCodeFence        bool
	codeFenceLang      string
	section            string
	execCommandPending bool
}

func newLogHighlighter() *logHighlighter {
	return &logHighlighter{}
}

func (h *logHighlighter) HighlightLine(line string) string {
	if !colorEnabled() {
		return line
	}

	newline := ""
	if strings.HasSuffix(line, "\n") {
		newline = "\n"
		line = strings.TrimSuffix(line, "\n")
	}

	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return line + newline
	}

	switch trimmed {
	case "thinking", "exec", "user", "assistant", "system":
		h.section = trimmed
		if trimmed == "exec" {
			h.execCommandPending = true
		}
		return colorize(line, sectionColor(trimmed)) + newline
	}

	if strings.HasPrefix(trimmed, "```") {
		if h.inCodeFence {
			h.inCodeFence = false
			h.codeFenceLang = ""
		} else {
			h.inCodeFence = true
			h.codeFenceLang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
		}
		return colorize(line, colorBlue) + newline
	}

	if h.inCodeFence {
		if h.codeFenceLang == "diff" {
			if color := diffLineColor(line, true); color != "" {
				return colorize(line, color) + newline
			}
		}
		return colorize(line, colorBlue) + newline
	}

	if h.execCommandPending {
		if trimmed != "" {
			h.execCommandPending = false
			return colorize(line, colorCyan) + newline
		}
	}

	if ts, ok := parseLogTimestamp(line); ok {
		_ = ts
		leading := len(line) - len(strings.TrimLeft(line, " "))
		rest := line[leading:]
		end := strings.Index(rest, "]")
		if end != -1 {
			prefix := rest[:end+1]
			remainder := rest[end+1:]
			message := strings.TrimLeft(remainder, " ")
			color := pickLogMessageColor(message)
			if color == "" {
				return strings.Repeat(" ", leading) + colorize(prefix, colorCyan) + remainder + newline
			}
			return strings.Repeat(" ", leading) + colorize(prefix, colorCyan) + colorize(remainder, color) + newline
		}
	}

	if strings.Contains(trimmed, "succeeded in") {
		return colorize(line, colorGreen) + newline
	}
	if strings.Contains(trimmed, "exited ") || strings.Contains(trimmed, "failed") {
		return colorize(line, colorRed) + newline
	}

	if color := diffLineColor(line, false); color != "" {
		return colorize(line, color) + newline
	}

	if strings.HasPrefix(trimmed, "#") {
		return colorize(line, colorBlue) + newline
	}

	if trimmed == "--------" || trimmed == "---" {
		return colorize(line, colorYellow) + newline
	}

	if h.section == "thinking" {
		return colorize(line, colorMagenta) + newline
	}

	if h.section == "user" {
		return colorize(line, colorGreen) + newline
	}

	if color := pickLogMessageColor(line); color != "" {
		return colorize(line, color) + newline
	}

	return line + newline
}

func sectionColor(section string) string {
	switch section {
	case "thinking":
		return colorMagenta
	case "exec":
		return colorCyan
	case "user":
		return colorGreen
	case "assistant":
		return colorBlue
	case "system":
		return colorYellow
	default:
		return ""
	}
}

func diffLineColor(line string, inFence bool) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(trimmed, "diff --git"),
		strings.HasPrefix(trimmed, "index "):
		return colorMagenta
	case strings.HasPrefix(trimmed, "@@"):
		return colorYellow
	case strings.HasPrefix(trimmed, "+++"):
		return colorGreen
	case strings.HasPrefix(trimmed, "---"):
		return colorRed
	case inFence && strings.HasPrefix(trimmed, "+"):
		return colorGreen
	case inFence && strings.HasPrefix(trimmed, "-"):
		return colorRed
	default:
		return ""
	}
}

func pickLogMessageColor(message string) string {
	if message == "" {
		return ""
	}
	needle := strings.ToLower(message)
	switch {
	case strings.Contains(needle, "panic"),
		strings.Contains(needle, "fatal"),
		strings.Contains(needle, "error"),
		strings.Contains(needle, "failed"),
		strings.Contains(needle, "failure"):
		return colorRed
	case strings.Contains(needle, "warn"),
		strings.Contains(needle, "warning"):
		return colorYellow
	case strings.Contains(needle, "kill"),
		strings.Contains(needle, "terminate"),
		strings.Contains(needle, "interrupt"),
		strings.Contains(needle, "stopped"),
		strings.Contains(needle, "stop requested"):
		return colorMagenta
	case strings.Contains(needle, "pause"),
		strings.Contains(needle, "sleep"),
		strings.Contains(needle, "waiting"):
		return colorCyan
	case strings.Contains(needle, "started"),
		strings.Contains(needle, "start "),
		strings.Contains(needle, "ready"),
		strings.Contains(needle, "running"):
		return colorGreen
	default:
		return ""
	}
}
