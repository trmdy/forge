package fmail

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func skipIfNoNetwork(t *testing.T) {
	t.Helper()
	if os.Getenv("FORGE_TEST_SKIP_NETWORK") != "" {
		t.Skip("skipping network test: FORGE_TEST_SKIP_NETWORK is set")
	}
}

func TestStandaloneSendLog(t *testing.T) {
	t.Setenv(EnvProject, "proj-test")
	root := t.TempDir()
	runtime := &Runtime{Root: root, Agent: "alice"}

	result, err := sendStandalone(runtime, &Message{
		From: runtime.Agent,
		To:   "task",
		Body: "hello",
	})
	require.NoError(t, err)
	require.NotEmpty(t, result.ID)

	messages := runLogJSON(t, runtime, []string{"task"}, nil)
	require.Len(t, messages, 1)
	require.Equal(t, "alice", messages[0].From)
	require.Equal(t, "task", messages[0].To)
	require.Equal(t, "hello", messages[0].Body)
}

func TestStandaloneWatchReceivesMessage(t *testing.T) {
	t.Setenv(EnvProject, "proj-test")
	root := t.TempDir()
	runtime := &Runtime{Root: root, Agent: "alice"}

	store, err := NewStore(root)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var out bytes.Buffer
	opts := watchOptions{
		count:      1,
		jsonOutput: true,
		deadline:   time.Now().Add(2 * time.Second),
	}
	target := watchTarget{mode: watchTopic, name: "task"}
	scanStart := time.Now().UTC().Add(-time.Second)

	errCh := make(chan error, 1)
	go func() {
		errCh <- watchStandalone(ctx, store, target, opts, scanStart, messageSince{}, &out)
	}()

	_, err = sendStandalone(runtime, &Message{
		From: runtime.Agent,
		To:   "task",
		Body: "ping",
	})
	require.NoError(t, err)

	require.NoError(t, <-errCh)
	messages := parseJSONMessages(t, out.String())
	require.Len(t, messages, 1)
	require.Equal(t, "task", messages[0].To)
	require.Equal(t, "ping", messages[0].Body)
}

func TestStandaloneDMInbox(t *testing.T) {
	t.Setenv(EnvProject, "proj-test")
	root := t.TempDir()
	runtime := &Runtime{Root: root, Agent: "alice"}

	_, err := sendStandalone(runtime, &Message{
		From: runtime.Agent,
		To:   "@bob",
		Body: "hi bob",
	})
	require.NoError(t, err)

	_, err = runLogCmd(t, runtime, []string{"@bob"}, map[string]string{"json": "true"})
	require.Error(t, err)

	var exitErr *ExitError
	require.True(t, errors.As(err, &exitErr))
	require.Equal(t, ExitCodeFailure, exitErr.Code)

	messages := runLogJSON(t, runtime, []string{"@bob"}, map[string]string{"allow-other-dm": "true"})
	require.Len(t, messages, 1)
	require.Equal(t, "alice", messages[0].From)
	require.Equal(t, "@bob", messages[0].To)
	require.Equal(t, "hi bob", messages[0].Body)
}

func TestWatchDMInboxRequiresOverride(t *testing.T) {
	t.Setenv(EnvProject, "proj-test")
	root := t.TempDir()
	runtime := &Runtime{Root: root, Agent: "alice"}

	watchCmd := newWatchCmd()
	watchCmd.SetOut(io.Discard)
	watchCmd.SetErr(io.Discard)
	watchCmd.SetContext(context.WithValue(context.Background(), runtimeKey{}, runtime))

	err := runWatch(watchCmd, []string{"@bob"})
	require.Error(t, err)

	var exitErr *ExitError
	require.True(t, errors.As(err, &exitErr))
	require.Equal(t, ExitCodeFailure, exitErr.Code)
}

func TestStandaloneLogSince(t *testing.T) {
	t.Setenv(EnvProject, "proj-test")
	root := t.TempDir()
	runtime := &Runtime{Root: root, Agent: "alice"}

	store, err := NewStore(root)
	require.NoError(t, err)

	older := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 1, 10, 13, 0, 0, 0, time.UTC)

	_, err = store.SaveMessage(&Message{
		From: "alice",
		To:   "task",
		Time: older,
		Body: "old",
	})
	require.NoError(t, err)
	_, err = store.SaveMessage(&Message{
		From: "alice",
		To:   "task",
		Time: newer,
		Body: "new",
	})
	require.NoError(t, err)

	flags := map[string]string{"since": "2026-01-10T12:30:00Z"}
	messages := runLogJSON(t, runtime, []string{"task"}, flags)
	require.Len(t, messages, 1)
	require.Equal(t, "new", messages[0].Body)
}

func TestStoreConcurrentSaveUniqueIDs(t *testing.T) {
	atomic.StoreUint32(&idCounter, 0)
	root := t.TempDir()
	fixed := time.Date(2026, 1, 10, 15, 30, 0, 0, time.UTC)

	store, err := NewStore(root, WithNow(func() time.Time { return fixed }))
	require.NoError(t, err)

	const total = 50
	ids := make(chan string, total)
	errs := make(chan error, total)

	var wg sync.WaitGroup
	wg.Add(total)
	for i := 0; i < total; i++ {
		i := i
		go func() {
			defer wg.Done()
			id, err := store.SaveMessage(&Message{
				From: "alice",
				To:   "task",
				Body: fmt.Sprintf("msg-%d", i),
			})
			if err != nil {
				errs <- err
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}

	seen := make(map[string]struct{}, total)
	for id := range ids {
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate id: %s", id)
		}
		seen[id] = struct{}{}
	}
	require.Len(t, seen, total)
}

func TestConnectedSendWatch(t *testing.T) {
	skipIfNoNetwork(t)

	t.Setenv(EnvProject, "proj-test")
	root := t.TempDir()
	runtime := &Runtime{Root: root, Agent: "agent1"}

	store, err := NewStore(root)
	require.NoError(t, err)
	require.NoError(t, store.EnsureRoot())

	socketPath := filepath.Join(store.Root, forgedSocketName)
	_ = os.Remove(socketPath)

	listener, err := net.Listen("unix", socketPath)
	require.NoError(t, err)
	defer listener.Close()

	server := newTestMailServer(store)
	go server.Serve(listener)

	var out bytes.Buffer
	watchCmd := newWatchCmd()
	watchCmd.SetOut(&out)
	watchCmd.SetErr(io.Discard)
	watchCmd.SetContext(context.WithValue(context.Background(), runtimeKey{}, runtime))
	require.NoError(t, watchCmd.Flags().Set("json", "true"))
	require.NoError(t, watchCmd.Flags().Set("count", "1"))
	require.NoError(t, watchCmd.Flags().Set("timeout", "2s"))

	errCh := make(chan error, 1)
	go func() {
		errCh <- runWatch(watchCmd, []string{"task"})
	}()

	select {
	case <-server.ready:
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not register in time")
	}

	sendCmd := newSendCmd()
	sendCmd.SetOut(io.Discard)
	sendCmd.SetErr(io.Discard)
	sendCmd.SetContext(context.WithValue(context.Background(), runtimeKey{}, runtime))

	require.NoError(t, runSend(sendCmd, []string{"task", "connected hello"}))
	require.NoError(t, <-errCh)

	messages := parseJSONMessages(t, out.String())
	require.Len(t, messages, 1)
	require.Equal(t, "task", messages[0].To)
	require.Equal(t, "connected hello", messages[0].Body)
}

func runLogJSON(t *testing.T, runtime *Runtime, args []string, flags map[string]string) []*Message {
	t.Helper()
	if flags == nil {
		flags = map[string]string{}
	}
	flags["json"] = "true"
	out, err := runLogCmd(t, runtime, args, flags)
	require.NoError(t, err)
	return parseJSONMessages(t, out)
}

func runLogCmd(t *testing.T, runtime *Runtime, args []string, flags map[string]string) (string, error) {
	t.Helper()
	cmd := newLogCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetContext(context.WithValue(context.Background(), runtimeKey{}, runtime))
	for name, value := range flags {
		require.NoError(t, cmd.Flags().Set(name, value))
	}
	err := runLog(cmd, args)
	return out.String(), err
}

func parseJSONMessages(t *testing.T, data string) []*Message {
	t.Helper()
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return nil
	}
	scanner := bufio.NewScanner(strings.NewReader(trimmed))
	scanner.Buffer(make([]byte, 0, 1024), MaxMessageSize+1024)

	var messages []*Message
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var message Message
		require.NoError(t, json.Unmarshal([]byte(line), &message))
		messages = append(messages, &message)
	}
	require.NoError(t, scanner.Err())
	return messages
}

type testMailServer struct {
	store     *Store
	mu        sync.Mutex
	watchers  []*testWatcher
	ready     chan struct{}
	readyOnce sync.Once
}

type testWatcher struct {
	target string
	agent  string
	conn   net.Conn
	mu     sync.Mutex
}

func newTestMailServer(store *Store) *testMailServer {
	return &testMailServer{
		store: store,
		ready: make(chan struct{}),
	}
}

func (s *testMailServer) Serve(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go s.handleConn(conn)
	}
}

func (s *testMailServer) handleConn(conn net.Conn) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	line, err := readMailLine(reader)
	if err != nil || len(line) == 0 {
		return
	}

	var base mailBaseRequest
	if err := json.Unmarshal(line, &base); err != nil {
		return
	}

	switch strings.TrimSpace(base.Cmd) {
	case "send":
		s.handleSend(conn, line)
	case "watch":
		s.handleWatch(conn, line)
	default:
		return
	}
}

func (s *testMailServer) handleSend(conn net.Conn, line []byte) {
	var req mailSendRequest
	if err := json.Unmarshal(line, &req); err != nil {
		_ = writeJSONLine(conn, mailResponse{OK: false})
		return
	}

	body, err := decodeMailBody(req.Body)
	if err != nil {
		_ = writeJSONLine(conn, mailResponse{OK: false, Error: &mailErr{Code: "invalid_request", Message: err.Error()}, ReqID: req.ReqID})
		return
	}

	message := &Message{
		From: req.Agent,
		To:   req.To,
		Body: body,
		Host: strings.TrimSpace(req.Host),
	}
	if strings.TrimSpace(req.ReplyTo) != "" {
		message.ReplyTo = strings.TrimSpace(req.ReplyTo)
	}
	if strings.TrimSpace(req.Priority) != "" {
		message.Priority = strings.TrimSpace(req.Priority)
	}

	if _, err := s.store.SaveMessage(message); err != nil {
		_ = writeJSONLine(conn, mailResponse{OK: false, Error: &mailErr{Code: "internal", Message: err.Error()}, ReqID: req.ReqID})
		return
	}

	s.broadcast(message)
	_ = writeJSONLine(conn, mailResponse{OK: true, ID: message.ID, ReqID: req.ReqID})
}

func (s *testMailServer) handleWatch(conn net.Conn, line []byte) {
	var req mailWatchRequest
	if err := json.Unmarshal(line, &req); err != nil {
		_ = writeJSONLine(conn, mailResponse{OK: false})
		return
	}

	agent, err := NormalizeAgentName(req.Agent)
	if err != nil {
		_ = writeJSONLine(conn, mailResponse{OK: false, Error: &mailErr{Code: "invalid_agent", Message: err.Error()}, ReqID: req.ReqID})
		return
	}

	target := strings.TrimSpace(req.Topic)
	if target == "" {
		target = "*"
	}

	watcher := &testWatcher{
		target: target,
		agent:  agent,
		conn:   conn,
	}
	s.addWatcher(watcher)
	defer s.removeWatcher(watcher)

	if err := writeJSONLine(conn, mailResponse{OK: true, ReqID: req.ReqID}); err != nil {
		return
	}

	s.readyOnce.Do(func() { close(s.ready) })

	_, _ = io.Copy(io.Discard, conn)
}

func (s *testMailServer) addWatcher(watcher *testWatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.watchers = append(s.watchers, watcher)
}

func (s *testMailServer) removeWatcher(watcher *testWatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, candidate := range s.watchers {
		if candidate == watcher {
			s.watchers = append(s.watchers[:i], s.watchers[i+1:]...)
			return
		}
	}
}

func (s *testMailServer) broadcast(message *Message) {
	s.mu.Lock()
	watchers := append([]*testWatcher(nil), s.watchers...)
	s.mu.Unlock()

	for _, watcher := range watchers {
		if watcher.matches(message) {
			_ = watcher.send(message)
		}
	}
}

func (w *testWatcher) matches(message *Message) bool {
	if message == nil {
		return false
	}
	target := strings.TrimSpace(w.target)
	if target == "" || target == "*" {
		if strings.HasPrefix(message.To, "@") {
			return strings.EqualFold(message.To, "@"+w.agent)
		}
		return true
	}
	if strings.HasPrefix(target, "@") {
		return strings.EqualFold(message.To, target)
	}
	return strings.EqualFold(message.To, target)
}

func (w *testWatcher) send(message *Message) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return writeJSONLine(w.conn, mailEnvelope{Msg: message})
}

func writeJSONLine(writer io.Writer, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = writer.Write(data)
	return err
}

func decodeMailBody(raw json.RawMessage) (any, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("missing body")
	}
	var body any
	if err := json.Unmarshal(trimmed, &body); err != nil {
		return nil, err
	}
	if body == nil {
		return json.RawMessage("null"), nil
	}
	return body, nil
}
