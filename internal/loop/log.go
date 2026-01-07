package loop

import (
	"bufio"
	"os"
	"strings"
	"sync"
	"time"
)

type loopLogger struct {
	file *os.File
	mu   sync.Mutex
	w    *bufio.Writer
}

func newLoopLogger(path string) (*loopLogger, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return &loopLogger{file: file, w: bufio.NewWriter(file)}, nil
}

func (l *loopLogger) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, err := l.w.Write(p)
	if err != nil {
		return n, err
	}
	return n, l.w.Flush()
}

func (l *loopLogger) WriteLine(message string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	stamp := time.Now().UTC().Format(time.RFC3339)
	_, _ = l.w.WriteString("[" + stamp + "] " + message + "\n")
	_ = l.w.Flush()
}

func (l *loopLogger) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	_ = l.w.Flush()
	_ = l.file.Close()
}

type tailWriter struct {
	mu       sync.Mutex
	maxLines int
	lines    []string
	buffer   string
}

func newTailWriter(maxLines int) *tailWriter {
	if maxLines <= 0 {
		maxLines = defaultOutputTailLines
	}
	return &tailWriter{maxLines: maxLines}
}

func (t *tailWriter) Write(p []byte) (int, error) {
	text := t.buffer + string(p)
	parts := strings.Split(text, "\n")
	if len(parts) == 0 {
		return len(p), nil
	}

	t.buffer = parts[len(parts)-1]
	lines := parts[:len(parts)-1]

	t.mu.Lock()
	defer t.mu.Unlock()
	for _, line := range lines {
		if len(t.lines) >= t.maxLines {
			t.lines = t.lines[1:]
		}
		t.lines = append(t.lines, line)
	}
	return len(p), nil
}

func (t *tailWriter) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := append([]string{}, t.lines...)
	if strings.TrimSpace(t.buffer) != "" {
		lines = append(lines, t.buffer)
	}
	return strings.Join(lines, "\n")
}
