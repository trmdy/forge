package forged

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/fmail"
)

func skipNetworkTest(t *testing.T) {
	t.Helper()
	if os.Getenv("FORGE_TEST_SKIP_NETWORK") != "" {
		t.Skip("skipping network test: FORGE_TEST_SKIP_NETWORK is set")
	}
}

func TestMailServerSendWatch(t *testing.T) {
	skipNetworkTest(t)

	originalInterval := mailPresenceInterval
	mailPresenceInterval = 50 * time.Millisecond
	t.Cleanup(func() {
		mailPresenceInterval = originalInterval
	})

	root := t.TempDir()
	projectID, err := fmail.DeriveProjectID(root)
	if err != nil {
		t.Fatalf("derive project id: %v", err)
	}

	server := newMailServer(zerolog.Nop())
	resolver, err := newStaticProjectResolver(root)
	if err != nil {
		t.Fatalf("static resolver: %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	go func() {
		_ = server.Serve(listener, resolver, true)
	}()

	addr := listener.Addr().String()

	watchConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial watch: %v", err)
	}
	defer watchConn.Close()
	watchReader := bufio.NewReader(watchConn)
	if err := watchConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("watch deadline: %v", err)
	}

	watchReq := mailWatchRequest{
		mailBaseRequest: mailBaseRequest{
			Cmd:       "watch",
			ProjectID: projectID,
			Agent:     "watcher",
			ReqID:     "w1",
		},
		Topic: "task",
	}
	writeLine(t, watchConn, watchReq)

	var watchAck mailResponse
	readJSONLine(t, watchReader, &watchAck)
	if !watchAck.OK {
		t.Fatalf("watch ack failed: %+v", watchAck)
	}

	store, err := fmail.NewStore(root)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	record, err := store.ReadAgentRecord("watcher")
	if err != nil {
		t.Fatalf("read agent record: %v", err)
	}
	if record.LastSeen.IsZero() {
		t.Fatalf("expected last_seen to be set")
	}
	initialSeen := record.LastSeen

	time.Sleep(2*mailPresenceInterval + 20*time.Millisecond)
	record, err = store.ReadAgentRecord("watcher")
	if err != nil {
		t.Fatalf("read agent record after heartbeat: %v", err)
	}
	if !record.LastSeen.After(initialSeen) {
		t.Fatalf("expected last_seen to advance, got %v <= %v", record.LastSeen, initialSeen)
	}

	sendConn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial send: %v", err)
	}
	defer sendConn.Close()
	sendReader := bufio.NewReader(sendConn)
	if err := sendConn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("send deadline: %v", err)
	}

	sendReq := mailSendRequest{
		mailBaseRequest: mailBaseRequest{
			Cmd:       "send",
			ProjectID: projectID,
			Agent:     "sender",
			ReqID:     "s1",
		},
		To:   "task",
		Body: json.RawMessage(`"hello"`),
	}
	writeLine(t, sendConn, sendReq)

	var sendResp mailResponse
	readJSONLine(t, sendReader, &sendResp)
	if !sendResp.OK || sendResp.ID == "" {
		t.Fatalf("send response invalid: %+v", sendResp)
	}

	var payload struct {
		Msg fmail.Message `json:"msg"`
	}
	readJSONLine(t, watchReader, &payload)
	if payload.Msg.ID == "" {
		t.Fatalf("expected message id")
	}
	if payload.Msg.From != "sender" {
		t.Fatalf("expected from sender, got %q", payload.Msg.From)
	}
	if payload.Msg.To != "task" {
		t.Fatalf("expected to task, got %q", payload.Msg.To)
	}
	if body, ok := payload.Msg.Body.(string); !ok || body != "hello" {
		t.Fatalf("expected body hello, got %#v", payload.Msg.Body)
	}
	if payload.Msg.Host == "" {
		t.Fatalf("expected host to be set")
	}

	messages, err := store.ListTopicMessages("task")
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(messages))
	}
}

func writeLine(t *testing.T, conn net.Conn, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	data = append(data, '\n')
	if _, err := conn.Write(data); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readJSONLine(t *testing.T, reader *bufio.Reader, out any) {
	line, err := readMailLine(reader)
	if err != nil {
		t.Fatalf("read line: %v", err)
	}
	if err := json.Unmarshal(line, out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}
