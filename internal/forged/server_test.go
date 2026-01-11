package forged

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	forgedv1 "github.com/tOgg1/forge/gen/forged/v1"
	"github.com/tOgg1/forge/internal/tmux"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/durationpb"
)

func TestServerPing(t *testing.T) {
	server := NewServer(zerolog.Nop(), WithVersion("test-version"))

	resp, err := server.Ping(context.Background(), &forgedv1.PingRequest{})
	if err != nil {
		t.Fatalf("Ping() error = %v", err)
	}

	if resp.Version != "test-version" {
		t.Errorf("Version = %q, want %q", resp.Version, "test-version")
	}
	if resp.Timestamp == nil {
		t.Error("Timestamp should not be nil")
	}
}

func TestServerGetStatus(t *testing.T) {
	server := NewServer(zerolog.Nop(), WithVersion("test-version"))

	resp, err := server.GetStatus(context.Background(), &forgedv1.GetStatusRequest{})
	if err != nil {
		t.Fatalf("GetStatus() error = %v", err)
	}

	if resp.Status == nil {
		t.Fatal("Status should not be nil")
	}
	if resp.Status.Version != "test-version" {
		t.Errorf("Version = %q, want %q", resp.Status.Version, "test-version")
	}
	if resp.Status.AgentCount != 0 {
		t.Errorf("AgentCount = %d, want 0", resp.Status.AgentCount)
	}
}

func TestServerListAgentsEmpty(t *testing.T) {
	server := NewServer(zerolog.Nop())

	resp, err := server.ListAgents(context.Background(), &forgedv1.ListAgentsRequest{})
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}

	if len(resp.Agents) != 0 {
		t.Errorf("Agents count = %d, want 0", len(resp.Agents))
	}
}

func TestServerGetAgentNotFound(t *testing.T) {
	server := NewServer(zerolog.Nop())

	_, err := server.GetAgent(context.Background(), &forgedv1.GetAgentRequest{
		AgentId: "nonexistent",
	})
	if err == nil {
		t.Error("GetAgent() should return error for nonexistent agent")
	}
}

func TestServerSpawnAgentValidation(t *testing.T) {
	server := NewServer(zerolog.Nop())

	tests := []struct {
		name    string
		req     *forgedv1.SpawnAgentRequest
		wantErr bool
	}{
		{
			name:    "empty agent_id",
			req:     &forgedv1.SpawnAgentRequest{Command: "echo"},
			wantErr: true,
		},
		{
			name:    "empty command",
			req:     &forgedv1.SpawnAgentRequest{AgentId: "test-agent"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := server.SpawnAgent(context.Background(), tt.req)
			if (err != nil) != tt.wantErr {
				t.Errorf("SpawnAgent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServerKillAgentNotFound(t *testing.T) {
	server := NewServer(zerolog.Nop())

	_, err := server.KillAgent(context.Background(), &forgedv1.KillAgentRequest{
		AgentId: "nonexistent",
	})
	if err == nil {
		t.Error("KillAgent() should return error for nonexistent agent")
	}
}

func TestServerSendInputNotFound(t *testing.T) {
	server := NewServer(zerolog.Nop())

	_, err := server.SendInput(context.Background(), &forgedv1.SendInputRequest{
		AgentId: "nonexistent",
		Text:    "hello",
	})
	if err == nil {
		t.Error("SendInput() should return error for nonexistent agent")
	}
}

func TestServerCapturePaneNotFound(t *testing.T) {
	server := NewServer(zerolog.Nop())

	_, err := server.CapturePane(context.Background(), &forgedv1.CapturePaneRequest{
		AgentId: "nonexistent",
	})
	if err == nil {
		t.Error("CapturePane() should return error for nonexistent agent")
	}
}

func TestDetectAgentState(t *testing.T) {
	server := NewServer(zerolog.Nop())

	tests := []struct {
		name    string
		content string
		want    forgedv1.AgentState
	}{
		{
			name:    "waiting for approval with y/n",
			content: "Do you want to proceed? [y/n]",
			want:    forgedv1.AgentState_AGENT_STATE_WAITING_APPROVAL,
		},
		{
			name:    "waiting for approval with confirm",
			content: "Please confirm this action",
			want:    forgedv1.AgentState_AGENT_STATE_WAITING_APPROVAL,
		},
		{
			name:    "idle with prompt",
			content: "some output\n$",
			want:    forgedv1.AgentState_AGENT_STATE_IDLE,
		},
		{
			name:    "idle with arrow prompt",
			content: "done\n❯",
			want:    forgedv1.AgentState_AGENT_STATE_IDLE,
		},
		{
			name:    "running with spinner",
			content: "⠋ Processing...",
			want:    forgedv1.AgentState_AGENT_STATE_RUNNING,
		},
		{
			name:    "running with thinking",
			content: "Thinking...",
			want:    forgedv1.AgentState_AGENT_STATE_RUNNING,
		},
		{
			name:    "failed with error",
			content: "error: something went wrong",
			want:    forgedv1.AgentState_AGENT_STATE_FAILED,
		},
		{
			name:    "failed with panic",
			content: "panic: runtime error",
			want:    forgedv1.AgentState_AGENT_STATE_FAILED,
		},
		{
			name:    "default to running",
			content: "some random output",
			want:    forgedv1.AgentState_AGENT_STATE_RUNNING,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := server.detectAgentState(tt.content, "")
			if got != tt.want {
				t.Errorf("detectAgentState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name    string
		s       string
		substrs []string
		want    bool
	}{
		{
			name:    "contains first",
			s:       "hello world",
			substrs: []string{"hello", "foo"},
			want:    true,
		},
		{
			name:    "contains second",
			s:       "hello world",
			substrs: []string{"foo", "world"},
			want:    true,
		},
		{
			name:    "contains none",
			s:       "hello world",
			substrs: []string{"foo", "bar"},
			want:    false,
		},
		{
			name:    "empty string",
			s:       "",
			substrs: []string{"foo"},
			want:    false,
		},
		{
			name:    "empty substrs",
			s:       "hello",
			substrs: []string{},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsAny(tt.s, tt.substrs...)
			if got != tt.want {
				t.Errorf("containsAny() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSplitLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "single line",
			content: "hello",
			want:    []string{"hello"},
		},
		{
			name:    "multiple lines",
			content: "line1\nline2\nline3",
			want:    []string{"line1", "line2", "line3"},
		},
		{
			name:    "trailing newline",
			content: "line1\nline2\n",
			want:    []string{"line1", "line2"},
		},
		{
			name:    "empty string",
			content: "",
			want:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitLines(tt.content)
			if len(got) != len(tt.want) {
				t.Errorf("splitLines() len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitLines()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// =============================================================================
// Transcript Tests
// =============================================================================

func TestServerGetTranscriptNotFound(t *testing.T) {
	server := NewServer(zerolog.Nop())

	_, err := server.GetTranscript(context.Background(), &forgedv1.GetTranscriptRequest{
		AgentId: "nonexistent",
	})
	if err == nil {
		t.Error("GetTranscript() should return error for nonexistent agent")
	}
}

func TestServerGetTranscriptValidation(t *testing.T) {
	server := NewServer(zerolog.Nop())

	_, err := server.GetTranscript(context.Background(), &forgedv1.GetTranscriptRequest{})
	if err == nil {
		t.Error("GetTranscript() should return error for empty agent_id")
	}
}

func TestAddTranscriptEntry(t *testing.T) {
	server := NewServer(zerolog.Nop())

	// Manually add an agent (bypass tmux)
	server.mu.Lock()
	server.agents["test-agent"] = &agentInfo{
		id:         "test-agent",
		transcript: make([]transcriptEntry, 0),
	}
	server.mu.Unlock()

	// Add a transcript entry
	server.addTranscriptEntry("test-agent", forgedv1.TranscriptEntryType_TRANSCRIPT_ENTRY_TYPE_COMMAND, "echo hello", map[string]string{"key": "value"})

	// Verify the entry was added
	resp, err := server.GetTranscript(context.Background(), &forgedv1.GetTranscriptRequest{
		AgentId: "test-agent",
	})
	if err != nil {
		t.Fatalf("GetTranscript() error = %v", err)
	}

	if len(resp.Entries) != 1 {
		t.Fatalf("Expected 1 entry, got %d", len(resp.Entries))
	}

	entry := resp.Entries[0]
	if entry.Content != "echo hello" {
		t.Errorf("Content = %q, want %q", entry.Content, "echo hello")
	}
	if entry.Type != forgedv1.TranscriptEntryType_TRANSCRIPT_ENTRY_TYPE_COMMAND {
		t.Errorf("Type = %v, want %v", entry.Type, forgedv1.TranscriptEntryType_TRANSCRIPT_ENTRY_TYPE_COMMAND)
	}
	if entry.Metadata["key"] != "value" {
		t.Errorf("Metadata[key] = %q, want %q", entry.Metadata["key"], "value")
	}
}

func TestGetTranscriptWithLimit(t *testing.T) {
	server := NewServer(zerolog.Nop())

	// Manually add an agent with multiple transcript entries
	server.mu.Lock()
	server.agents["test-agent"] = &agentInfo{
		id:         "test-agent",
		transcript: make([]transcriptEntry, 0),
	}
	server.mu.Unlock()

	// Add 10 entries
	for i := 0; i < 10; i++ {
		server.addTranscriptEntry("test-agent", forgedv1.TranscriptEntryType_TRANSCRIPT_ENTRY_TYPE_OUTPUT, "output", nil)
	}

	// Get with limit of 5
	resp, err := server.GetTranscript(context.Background(), &forgedv1.GetTranscriptRequest{
		AgentId: "test-agent",
		Limit:   5,
	})
	if err != nil {
		t.Fatalf("GetTranscript() error = %v", err)
	}

	if len(resp.Entries) != 5 {
		t.Errorf("Expected 5 entries, got %d", len(resp.Entries))
	}
	if !resp.HasMore {
		t.Error("Expected HasMore to be true")
	}
	if resp.NextCursor == "" {
		t.Error("Expected NextCursor to be set")
	}
}

func TestParseInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{
			name:    "valid number",
			input:   "123",
			want:    123,
			wantErr: false,
		},
		{
			name:    "zero",
			input:   "0",
			want:    0,
			wantErr: false,
		},
		{
			name:    "large number",
			input:   "9999999999",
			want:    9999999999,
			wantErr: false,
		},
		{
			name:    "invalid character",
			input:   "123abc",
			want:    0,
			wantErr: true,
		},
		{
			name:    "negative sign",
			input:   "-123",
			want:    0,
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			want:    0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseInt64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseInt64() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("parseInt64() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTranscriptEntryToProto(t *testing.T) {
	server := NewServer(zerolog.Nop())

	entry := &transcriptEntry{
		id:        42,
		entryType: forgedv1.TranscriptEntryType_TRANSCRIPT_ENTRY_TYPE_USER_INPUT,
		content:   "user input text",
		metadata:  map[string]string{"source": "keyboard"},
	}

	proto := server.transcriptEntryToProto(entry)

	if proto.Content != "user input text" {
		t.Errorf("Content = %q, want %q", proto.Content, "user input text")
	}
	if proto.Type != forgedv1.TranscriptEntryType_TRANSCRIPT_ENTRY_TYPE_USER_INPUT {
		t.Errorf("Type = %v, want %v", proto.Type, forgedv1.TranscriptEntryType_TRANSCRIPT_ENTRY_TYPE_USER_INPUT)
	}
	if proto.Metadata["source"] != "keyboard" {
		t.Errorf("Metadata[source] = %q, want %q", proto.Metadata["source"], "keyboard")
	}
	if proto.Timestamp == nil {
		t.Error("Timestamp should not be nil")
	}
}

// =============================================================================
// Event Streaming Tests
// =============================================================================

type eventStreamRecorder struct {
	ctx       context.Context
	cancel    context.CancelFunc
	responses []*forgedv1.StreamEventsResponse
	want      int
}

func newEventStreamRecorder(want int) *eventStreamRecorder {
	ctx, cancel := context.WithCancel(context.Background())
	return &eventStreamRecorder{
		ctx:    ctx,
		cancel: cancel,
		want:   want,
	}
}

func (s *eventStreamRecorder) Send(resp *forgedv1.StreamEventsResponse) error {
	s.responses = append(s.responses, resp)
	if s.want > 0 && len(s.responses) >= s.want {
		s.cancel()
	}
	return nil
}

func (s *eventStreamRecorder) SetHeader(metadata.MD) error  { return nil }
func (s *eventStreamRecorder) SendHeader(metadata.MD) error { return nil }
func (s *eventStreamRecorder) SetTrailer(metadata.MD)       {}
func (s *eventStreamRecorder) Context() context.Context     { return s.ctx }
func (s *eventStreamRecorder) SendMsg(interface{}) error    { return nil }
func (s *eventStreamRecorder) RecvMsg(interface{}) error    { return nil }

// =============================================================================
// Pane Update Tests
// =============================================================================

type staticExecutor struct {
	stdout []byte
	err    error
}

func (e *staticExecutor) Exec(ctx context.Context, cmd string) ([]byte, []byte, error) {
	return e.stdout, nil, e.err
}

type paneUpdateRecorder struct {
	ctx       context.Context
	cancel    context.CancelFunc
	responses []*forgedv1.StreamPaneUpdatesResponse
}

func newPaneUpdateRecorder(timeout time.Duration) *paneUpdateRecorder {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return &paneUpdateRecorder{ctx: ctx, cancel: cancel}
}

func (s *paneUpdateRecorder) Send(resp *forgedv1.StreamPaneUpdatesResponse) error {
	s.responses = append(s.responses, resp)
	return nil
}

func (s *paneUpdateRecorder) SetHeader(metadata.MD) error  { return nil }
func (s *paneUpdateRecorder) SendHeader(metadata.MD) error { return nil }
func (s *paneUpdateRecorder) SetTrailer(metadata.MD)       {}
func (s *paneUpdateRecorder) Context() context.Context     { return s.ctx }
func (s *paneUpdateRecorder) SendMsg(interface{}) error    { return nil }
func (s *paneUpdateRecorder) RecvMsg(interface{}) error    { return nil }

func TestStreamPaneUpdatesSkipsUnchangedContent(t *testing.T) {
	server := NewServer(zerolog.Nop())

	exec := &staticExecutor{stdout: []byte("steady output")}
	server.tmux = tmux.NewClient(exec)

	server.mu.Lock()
	server.agents["agent-1"] = &agentInfo{
		id:     "agent-1",
		paneID: "%1",
	}
	server.mu.Unlock()

	stream := newPaneUpdateRecorder(60 * time.Millisecond)
	req := &forgedv1.StreamPaneUpdatesRequest{
		AgentId:     "agent-1",
		MinInterval: durationpb.New(5 * time.Millisecond),
	}

	err := server.StreamPaneUpdates(req, stream)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StreamPaneUpdates() error = %v", err)
	}

	if len(stream.responses) != 1 {
		t.Fatalf("Expected 1 pane update, got %d", len(stream.responses))
	}

	resp := stream.responses[0]
	if !resp.Changed {
		t.Error("Expected first pane update to be marked changed")
	}
	wantHash := tmux.HashSnapshot("steady output")
	if resp.ContentHash != wantHash {
		t.Errorf("ContentHash = %q, want %q", resp.ContentHash, wantHash)
	}
}

func TestStreamPaneUpdatesRespectsLastKnownHash(t *testing.T) {
	server := NewServer(zerolog.Nop())

	exec := &staticExecutor{stdout: []byte("no change")}
	server.tmux = tmux.NewClient(exec)

	server.mu.Lock()
	server.agents["agent-1"] = &agentInfo{
		id:     "agent-1",
		paneID: "%1",
	}
	server.mu.Unlock()

	lastHash := tmux.HashSnapshot("no change")
	stream := newPaneUpdateRecorder(60 * time.Millisecond)
	req := &forgedv1.StreamPaneUpdatesRequest{
		AgentId:       "agent-1",
		LastKnownHash: lastHash,
		MinInterval:   durationpb.New(5 * time.Millisecond),
	}

	err := server.StreamPaneUpdates(req, stream)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("StreamPaneUpdates() error = %v", err)
	}

	if len(stream.responses) != 0 {
		t.Fatalf("Expected 0 pane updates, got %d", len(stream.responses))
	}
}

func TestPublishEvent(t *testing.T) {
	server := NewServer(zerolog.Nop())

	// Publish an event
	server.publishAgentStateChanged(
		"agent-1",
		"workspace-1",
		forgedv1.AgentState_AGENT_STATE_STARTING,
		forgedv1.AgentState_AGENT_STATE_RUNNING,
		"test reason",
	)

	// Verify event was stored
	server.eventsMu.RLock()
	defer server.eventsMu.RUnlock()

	if len(server.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(server.events))
	}

	event := server.events[0].event
	if event.AgentId != "agent-1" {
		t.Errorf("AgentId = %q, want %q", event.AgentId, "agent-1")
	}
	if event.WorkspaceId != "workspace-1" {
		t.Errorf("WorkspaceId = %q, want %q", event.WorkspaceId, "workspace-1")
	}
	if event.Type != forgedv1.EventType_EVENT_TYPE_AGENT_STATE_CHANGED {
		t.Errorf("Type = %v, want %v", event.Type, forgedv1.EventType_EVENT_TYPE_AGENT_STATE_CHANGED)
	}

	stateChanged := event.GetAgentStateChanged()
	if stateChanged == nil {
		t.Fatal("Expected AgentStateChanged payload")
	}
	if stateChanged.PreviousState != forgedv1.AgentState_AGENT_STATE_STARTING {
		t.Errorf("PreviousState = %v, want %v", stateChanged.PreviousState, forgedv1.AgentState_AGENT_STATE_STARTING)
	}
	if stateChanged.NewState != forgedv1.AgentState_AGENT_STATE_RUNNING {
		t.Errorf("NewState = %v, want %v", stateChanged.NewState, forgedv1.AgentState_AGENT_STATE_RUNNING)
	}
	if stateChanged.Reason != "test reason" {
		t.Errorf("Reason = %q, want %q", stateChanged.Reason, "test reason")
	}
}

func TestPublishErrorEvent(t *testing.T) {
	server := NewServer(zerolog.Nop())

	server.publishError("agent-1", "workspace-1", "ERR_TEST", "test error message", true)

	server.eventsMu.RLock()
	defer server.eventsMu.RUnlock()

	if len(server.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(server.events))
	}

	event := server.events[0].event
	if event.Type != forgedv1.EventType_EVENT_TYPE_ERROR {
		t.Errorf("Type = %v, want %v", event.Type, forgedv1.EventType_EVENT_TYPE_ERROR)
	}

	errEvent := event.GetError()
	if errEvent == nil {
		t.Fatal("Expected Error payload")
	}
	if errEvent.Code != "ERR_TEST" {
		t.Errorf("Code = %q, want %q", errEvent.Code, "ERR_TEST")
	}
	if errEvent.Message != "test error message" {
		t.Errorf("Message = %q, want %q", errEvent.Message, "test error message")
	}
	if !errEvent.Recoverable {
		t.Error("Expected Recoverable to be true")
	}
}

func TestPublishPaneContentChangedEvent(t *testing.T) {
	server := NewServer(zerolog.Nop())

	server.publishPaneContentChanged("agent-1", "workspace-1", "abc123", 42)

	server.eventsMu.RLock()
	defer server.eventsMu.RUnlock()

	if len(server.events) != 1 {
		t.Fatalf("Expected 1 event, got %d", len(server.events))
	}

	event := server.events[0].event
	if event.Type != forgedv1.EventType_EVENT_TYPE_PANE_CONTENT_CHANGED {
		t.Errorf("Type = %v, want %v", event.Type, forgedv1.EventType_EVENT_TYPE_PANE_CONTENT_CHANGED)
	}

	paneEvent := event.GetPaneContentChanged()
	if paneEvent == nil {
		t.Fatal("Expected PaneContentChanged payload")
	}
	if paneEvent.ContentHash != "abc123" {
		t.Errorf("ContentHash = %q, want %q", paneEvent.ContentHash, "abc123")
	}
	if paneEvent.LinesChanged != 42 {
		t.Errorf("LinesChanged = %d, want %d", paneEvent.LinesChanged, 42)
	}
}

func TestStreamEventsReplayFromCursor(t *testing.T) {
	server := NewServer(zerolog.Nop())

	server.publishAgentStateChanged("agent-1", "workspace-1", forgedv1.AgentState_AGENT_STATE_STARTING, forgedv1.AgentState_AGENT_STATE_RUNNING, "first")
	server.publishAgentStateChanged("agent-2", "workspace-1", forgedv1.AgentState_AGENT_STATE_STARTING, forgedv1.AgentState_AGENT_STATE_RUNNING, "second")
	server.publishAgentStateChanged("agent-3", "workspace-1", forgedv1.AgentState_AGENT_STATE_STARTING, forgedv1.AgentState_AGENT_STATE_RUNNING, "third")

	stream := newEventStreamRecorder(2)
	err := server.StreamEvents(&forgedv1.StreamEventsRequest{Cursor: "1"}, stream)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("StreamEvents() error = %v", err)
	}

	if len(stream.responses) != 2 {
		t.Fatalf("Expected 2 replayed events, got %d", len(stream.responses))
	}

	first := stream.responses[0].GetEvent()
	second := stream.responses[1].GetEvent()
	if first == nil || second == nil {
		t.Fatal("Expected replayed events in responses")
	}

	if first.Id != "1" {
		t.Errorf("First replayed event ID = %q, want %q", first.Id, "1")
	}
	if second.Id != "2" {
		t.Errorf("Second replayed event ID = %q, want %q", second.Id, "2")
	}
}

func TestEventMatchesFilter(t *testing.T) {
	server := NewServer(zerolog.Nop())

	event := &forgedv1.Event{
		Type:        forgedv1.EventType_EVENT_TYPE_AGENT_STATE_CHANGED,
		AgentId:     "agent-1",
		WorkspaceId: "workspace-1",
	}

	tests := []struct {
		name string
		sub  *eventSubscriber
		want bool
	}{
		{
			name: "no filters matches all",
			sub:  &eventSubscriber{},
			want: true,
		},
		{
			name: "matching event type",
			sub: &eventSubscriber{
				eventTypes: map[forgedv1.EventType]bool{
					forgedv1.EventType_EVENT_TYPE_AGENT_STATE_CHANGED: true,
				},
			},
			want: true,
		},
		{
			name: "non-matching event type",
			sub: &eventSubscriber{
				eventTypes: map[forgedv1.EventType]bool{
					forgedv1.EventType_EVENT_TYPE_ERROR: true,
				},
			},
			want: false,
		},
		{
			name: "matching agent ID",
			sub: &eventSubscriber{
				agentIDs: map[string]bool{"agent-1": true},
			},
			want: true,
		},
		{
			name: "non-matching agent ID",
			sub: &eventSubscriber{
				agentIDs: map[string]bool{"agent-2": true},
			},
			want: false,
		},
		{
			name: "matching workspace ID",
			sub: &eventSubscriber{
				workspaceIDs: map[string]bool{"workspace-1": true},
			},
			want: true,
		},
		{
			name: "non-matching workspace ID",
			sub: &eventSubscriber{
				workspaceIDs: map[string]bool{"workspace-2": true},
			},
			want: false,
		},
		{
			name: "all filters matching",
			sub: &eventSubscriber{
				eventTypes:   map[forgedv1.EventType]bool{forgedv1.EventType_EVENT_TYPE_AGENT_STATE_CHANGED: true},
				agentIDs:     map[string]bool{"agent-1": true},
				workspaceIDs: map[string]bool{"workspace-1": true},
			},
			want: true,
		},
		{
			name: "one filter not matching",
			sub: &eventSubscriber{
				eventTypes:   map[forgedv1.EventType]bool{forgedv1.EventType_EVENT_TYPE_AGENT_STATE_CHANGED: true},
				agentIDs:     map[string]bool{"agent-2": true}, // not matching
				workspaceIDs: map[string]bool{"workspace-1": true},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := server.eventMatchesFilter(event, tt.sub)
			if got != tt.want {
				t.Errorf("eventMatchesFilter() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEventCircularBuffer(t *testing.T) {
	server := NewServer(zerolog.Nop())

	// Publish more events than maxStoredEvents
	for i := 0; i < maxStoredEvents+100; i++ {
		server.publishAgentStateChanged(
			"agent-1",
			"workspace-1",
			forgedv1.AgentState_AGENT_STATE_RUNNING,
			forgedv1.AgentState_AGENT_STATE_IDLE,
			"test",
		)
	}

	server.eventsMu.RLock()
	defer server.eventsMu.RUnlock()

	// Should only have maxStoredEvents
	if len(server.events) != maxStoredEvents {
		t.Errorf("Expected %d events, got %d", maxStoredEvents, len(server.events))
	}

	// First event should have ID >= 100 (the first 100 were evicted)
	firstEventID, _ := parseInt64(server.events[0].event.Id)
	if firstEventID < 100 {
		t.Errorf("First event ID = %d, expected >= 100", firstEventID)
	}
}

func TestEventSubscriberBroadcast(t *testing.T) {
	server := NewServer(zerolog.Nop())

	// Create a subscriber
	sub := &eventSubscriber{
		id: "test-sub",
		ch: make(chan *forgedv1.Event, 10),
	}

	server.eventsMu.Lock()
	server.eventSubs[sub.id] = sub
	server.eventsMu.Unlock()

	// Publish an event
	server.publishAgentStateChanged(
		"agent-1",
		"workspace-1",
		forgedv1.AgentState_AGENT_STATE_STARTING,
		forgedv1.AgentState_AGENT_STATE_RUNNING,
		"test",
	)

	// Check that subscriber received the event
	select {
	case event := <-sub.ch:
		if event.AgentId != "agent-1" {
			t.Errorf("AgentId = %q, want %q", event.AgentId, "agent-1")
		}
	default:
		t.Error("Expected subscriber to receive event")
	}

	// Cleanup
	server.eventsMu.Lock()
	delete(server.eventSubs, sub.id)
	server.eventsMu.Unlock()
}

func TestEventSubscriberFiltering(t *testing.T) {
	server := NewServer(zerolog.Nop())

	// Create a subscriber with agent filter
	sub := &eventSubscriber{
		id:       "test-sub",
		agentIDs: map[string]bool{"agent-1": true},
		ch:       make(chan *forgedv1.Event, 10),
	}

	server.eventsMu.Lock()
	server.eventSubs[sub.id] = sub
	server.eventsMu.Unlock()

	// Publish event for agent-1 (should be received)
	server.publishAgentStateChanged("agent-1", "workspace-1", forgedv1.AgentState_AGENT_STATE_STARTING, forgedv1.AgentState_AGENT_STATE_RUNNING, "test")

	// Publish event for agent-2 (should NOT be received)
	server.publishAgentStateChanged("agent-2", "workspace-1", forgedv1.AgentState_AGENT_STATE_STARTING, forgedv1.AgentState_AGENT_STATE_RUNNING, "test")

	// Check that subscriber received only agent-1 event
	receivedCount := 0
	for {
		select {
		case event := <-sub.ch:
			receivedCount++
			if event.AgentId != "agent-1" {
				t.Errorf("Received event for wrong agent: %q", event.AgentId)
			}
		default:
			goto done
		}
	}
done:

	if receivedCount != 1 {
		t.Errorf("Expected 1 event, received %d", receivedCount)
	}

	// Cleanup
	server.eventsMu.Lock()
	delete(server.eventSubs, sub.id)
	server.eventsMu.Unlock()
}
