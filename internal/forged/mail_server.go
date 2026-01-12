package forged

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
	"github.com/tOgg1/forge/internal/fmail"
)

const (
	mailLineLimit       = fmail.MaxMessageSize + 64*1024
	mailSubscriberBuf   = 128
	mailSubscriberQueue = 512
)

var (
	errBackpressure      = errors.New("backpressure")
	mailPresenceInterval = 5 * time.Second
)

type mailServer struct {
	logger zerolog.Logger
	host   string

	mu   sync.Mutex
	hubs map[string]*mailHub
}

func newMailServer(logger zerolog.Logger) *mailServer {
	host, _ := os.Hostname()
	return &mailServer{
		logger: logger,
		host:   host,
		hubs:   make(map[string]*mailHub),
	}
}

func (s *mailServer) Serve(listener net.Listener, resolver mailProjectResolver, requireProjectID bool) error {
	if listener == nil {
		return errors.New("listener is nil")
	}
	if resolver == nil {
		return errors.New("resolver is nil")
	}
	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			s.logger.Warn().Err(err).Msg("mail server accept failed")
			continue
		}
		go s.handleConn(conn, resolver, requireProjectID)
	}
}

func (s *mailServer) handleConn(conn net.Conn, resolver mailProjectResolver, requireProjectID bool) {
	defer conn.Close()

	reader := bufio.NewReader(conn)
	line, err := readMailLine(reader)
	if err != nil {
		return
	}

	var base mailBaseRequest
	if err := json.Unmarshal(line, &base); err != nil {
		_ = writeMailError(conn, base.ReqID, "invalid_request", "invalid json")
		return
	}

	cmd := strings.TrimSpace(base.Cmd)
	if cmd == "" {
		_ = writeMailError(conn, base.ReqID, "invalid_request", "missing cmd")
		return
	}

	projectID := strings.TrimSpace(base.ProjectID)
	if requireProjectID && projectID == "" {
		_ = writeMailError(conn, base.ReqID, "invalid_request", "missing project_id")
		return
	}

	agent, err := fmail.NormalizeAgentName(base.Agent)
	if err != nil {
		_ = writeMailError(conn, base.ReqID, "invalid_agent", "invalid agent")
		return
	}

	host := strings.TrimSpace(base.Host)
	if host == "" {
		host = s.host
	}

	project, err := resolver.Resolve(projectID)
	if err != nil {
		code := "invalid_project"
		if errors.Is(err, errMissingProjectID) {
			code = "invalid_request"
		}
		_ = writeMailError(conn, base.ReqID, code, err.Error())
		return
	}

	hub, err := s.getHub(project)
	if err != nil {
		_ = writeMailError(conn, base.ReqID, "internal", err.Error())
		return
	}

	base.Agent = agent
	base.Host = host
	base.ProjectID = hub.project.ID

	switch cmd {
	case "send":
		s.handleSend(conn, line, base, hub)
	case "watch":
		s.handleWatch(conn, line, base, hub)
	case "relay":
		s.handleRelay(conn, line, base, hub)
	default:
		_ = writeMailError(conn, base.ReqID, "invalid_request", "unknown cmd")
	}
}

func (s *mailServer) handleSend(conn net.Conn, line []byte, base mailBaseRequest, hub *mailHub) {
	var req mailSendRequest
	if err := json.Unmarshal(line, &req); err != nil {
		_ = writeMailError(conn, base.ReqID, "invalid_request", "invalid send request")
		return
	}

	body, err := parseMailBody(req.Body)
	if err != nil {
		_ = writeMailError(conn, base.ReqID, "invalid_request", err.Error())
		return
	}

	priority := strings.ToLower(strings.TrimSpace(req.Priority))
	if priority != "" {
		if err := fmail.ValidatePriority(priority); err != nil {
			_ = writeMailError(conn, base.ReqID, "invalid_request", "invalid priority")
			return
		}
	}

	tags, err := fmail.NormalizeTags(req.Tags)
	if err != nil {
		_ = writeMailError(conn, base.ReqID, "invalid_request", err.Error())
		return
	}

	message := &fmail.Message{
		From: base.Agent,
		To:   req.To,
		Body: body,
		Host: base.Host,
		Tags: tags,
	}
	if strings.TrimSpace(req.ReplyTo) != "" {
		message.ReplyTo = strings.TrimSpace(req.ReplyTo)
	}
	if priority != "" {
		message.Priority = priority
	}

	if _, err := hub.store.UpdateAgentRecord(base.Agent, base.Host); err != nil {
		_ = writeMailError(conn, base.ReqID, "internal", "update agent registry failed")
		return
	}

	if _, err := hub.store.SaveMessage(message); err != nil {
		s.handleStoreError(conn, base.ReqID, err)
		return
	}

	hub.broadcast(message)
	_ = writeMailResponse(conn, mailResponse{OK: true, ID: message.ID, ReqID: base.ReqID})
}

func (s *mailServer) handleWatch(conn net.Conn, line []byte, base mailBaseRequest, hub *mailHub) {
	var req mailWatchRequest
	if err := json.Unmarshal(line, &req); err != nil {
		_ = writeMailError(conn, base.ReqID, "invalid_request", "invalid watch request")
		return
	}

	if _, err := hub.store.UpdateAgentRecord(base.Agent, base.Host); err != nil {
		_ = writeMailError(conn, base.ReqID, "internal", "update agent registry failed")
		return
	}
	stopPresence := hub.trackPresence(base.Agent, base.Host)
	defer stopPresence()

	if req.Agent != "" && strings.ToLower(strings.TrimSpace(req.Agent)) != base.Agent {
		_ = writeMailError(conn, base.ReqID, "invalid_request", "agent mismatch")
		return
	}

	target, err := parseMailWatchTarget(req.Topic, base.Agent)
	if err != nil {
		code := "invalid_request"
		if errors.Is(err, fmail.ErrInvalidAgent) {
			code = "invalid_agent"
		} else if errors.Is(err, fmail.ErrInvalidTopic) {
			code = "invalid_topic"
		}
		_ = writeMailError(conn, base.ReqID, code, err.Error())
		return
	}

	since, err := parseMailSince(req.Since)
	if err != nil {
		_ = writeMailError(conn, base.ReqID, "invalid_request", "invalid since")
		return
	}

	subscriber := hub.subscribe(target, since)
	defer hub.unsubscribe(subscriber)

	if err := writeMailResponse(conn, mailResponse{OK: true, ReqID: base.ReqID}); err != nil {
		return
	}

	backlog, err := loadMailBacklog(hub.store, target, since)
	if err != nil {
		_ = writeMailError(conn, base.ReqID, "internal", err.Error())
		return
	}

	sentIDs := make(map[string]struct{}, len(backlog))
	for _, message := range backlog {
		if err := writeMailMessage(conn, message); err != nil {
			return
		}
		sentIDs[message.ID] = struct{}{}
	}

	pending := subscriber.resume()
	if len(pending) > 0 {
		pending = filterMessages(pending, since)
		sortMailMessages(pending)
		for _, message := range pending {
			if _, ok := sentIDs[message.ID]; ok {
				continue
			}
			if err := writeMailMessage(conn, message); err != nil {
				return
			}
		}
	}

	for message := range subscriber.ch {
		if err := writeMailMessage(conn, message); err != nil {
			return
		}
	}

	if err := subscriber.error(); err != nil {
		_ = writeMailError(conn, base.ReqID, "backpressure", err.Error())
	}
}

func (s *mailServer) handleRelay(conn net.Conn, line []byte, base mailBaseRequest, hub *mailHub) {
	var req mailRelayRequest
	if err := json.Unmarshal(line, &req); err != nil {
		_ = writeMailError(conn, base.ReqID, "invalid_request", "invalid relay request")
		return
	}

	if _, err := hub.store.UpdateAgentRecord(base.Agent, base.Host); err != nil {
		_ = writeMailError(conn, base.ReqID, "internal", "update agent registry failed")
		return
	}

	since, err := parseMailSince(req.Since)
	if err != nil {
		_ = writeMailError(conn, base.ReqID, "invalid_request", "invalid since")
		return
	}

	target := mailWatchTarget{mode: watchRelay}
	subscriber := hub.subscribe(target, since)
	defer hub.unsubscribe(subscriber)

	if err := writeMailResponse(conn, mailResponse{OK: true, ReqID: base.ReqID}); err != nil {
		return
	}

	backlog, err := loadMailBacklog(hub.store, target, since)
	if err != nil {
		_ = writeMailError(conn, base.ReqID, "internal", err.Error())
		return
	}

	sentIDs := make(map[string]struct{}, len(backlog))
	for _, message := range backlog {
		if err := writeMailMessage(conn, message); err != nil {
			return
		}
		sentIDs[message.ID] = struct{}{}
	}

	pending := subscriber.resume()
	if len(pending) > 0 {
		pending = filterMessages(pending, since)
		sortMailMessages(pending)
		for _, message := range pending {
			if _, ok := sentIDs[message.ID]; ok {
				continue
			}
			if err := writeMailMessage(conn, message); err != nil {
				return
			}
		}
	}

	for message := range subscriber.ch {
		if err := writeMailMessage(conn, message); err != nil {
			return
		}
	}

	if err := subscriber.error(); err != nil {
		_ = writeMailError(conn, base.ReqID, "backpressure", err.Error())
	}
}

func (s *mailServer) handleStoreError(conn net.Conn, reqID string, err error) {
	switch {
	case errors.Is(err, fmail.ErrInvalidTopic), errors.Is(err, fmail.ErrInvalidTarget):
		_ = writeMailError(conn, reqID, "invalid_topic", err.Error())
	case errors.Is(err, fmail.ErrInvalidAgent):
		_ = writeMailError(conn, reqID, "invalid_agent", err.Error())
	case errors.Is(err, fmail.ErrMessageTooLarge):
		_ = writeMailError(conn, reqID, "too_large", "message exceeds size limit")
	default:
		_ = writeMailError(conn, reqID, "internal", err.Error())
	}
}

func (s *mailServer) getHub(project mailProject) (*mailHub, error) {
	id := strings.TrimSpace(project.ID)
	root := strings.TrimSpace(project.Root)
	if root == "" {
		return nil, errors.New("project root required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if id == "" {
		derived, err := fmail.DeriveProjectID(absRoot)
		if err != nil {
			return nil, err
		}
		id = derived
	}

	s.mu.Lock()
	if hub, ok := s.hubs[absRoot]; ok {
		s.mu.Unlock()
		return hub, nil
	}
	s.mu.Unlock()

	store, err := fmail.NewStore(absRoot)
	if err != nil {
		return nil, err
	}
	if err := store.EnsureRoot(); err != nil {
		return nil, err
	}
	projectRecord, err := store.EnsureProject(id)
	if err != nil {
		return nil, err
	}
	if projectRecord.ID != "" && projectRecord.ID != id {
		return nil, fmt.Errorf("project id mismatch: %s", projectRecord.ID)
	}

	hub := &mailHub{
		project:  mailProject{ID: id, Root: absRoot},
		store:    store,
		host:     s.host,
		logger:   s.logger,
		subs:     make(map[string]*mailSubscriber),
		presence: make(map[mailPresenceKey]*mailPresenceTracker),
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.hubs[absRoot]; ok {
		return existing, nil
	}
	s.hubs[absRoot] = hub
	return hub, nil
}

type mailHub struct {
	project mailProject
	store   *fmail.Store
	host    string
	logger  zerolog.Logger

	mu   sync.RWMutex
	subs map[string]*mailSubscriber
	seq  uint64

	presenceMu sync.Mutex
	presence   map[mailPresenceKey]*mailPresenceTracker
}

type mailPresenceKey struct {
	agent string
	host  string
}

type mailPresenceTracker struct {
	agent string
	host  string
	refs  int
	stop  chan struct{}
}

func (h *mailHub) trackPresence(agent, host string) func() {
	key := mailPresenceKey{agent: agent, host: host}

	h.presenceMu.Lock()
	tracker, ok := h.presence[key]
	if ok {
		tracker.refs++
		h.presenceMu.Unlock()
		return func() { h.untrackPresence(key) }
	}
	tracker = &mailPresenceTracker{
		agent: agent,
		host:  host,
		refs:  1,
		stop:  make(chan struct{}),
	}
	h.presence[key] = tracker
	h.presenceMu.Unlock()

	go h.runPresence(tracker)
	return func() { h.untrackPresence(key) }
}

func (h *mailHub) untrackPresence(key mailPresenceKey) {
	h.presenceMu.Lock()
	tracker, ok := h.presence[key]
	if !ok {
		h.presenceMu.Unlock()
		return
	}
	tracker.refs--
	if tracker.refs <= 0 {
		delete(h.presence, key)
		close(tracker.stop)
	}
	h.presenceMu.Unlock()
}

func (h *mailHub) runPresence(tracker *mailPresenceTracker) {
	ticker := time.NewTicker(mailPresenceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tracker.stop:
			return
		case <-ticker.C:
			if _, err := h.store.UpdateAgentRecord(tracker.agent, tracker.host); err != nil {
				h.logger.Warn().Err(err).Str("agent", tracker.agent).Msg("mail presence update failed")
			}
		}
	}
}

func (h *mailHub) subscribe(target mailWatchTarget, since sinceFilter) *mailSubscriber {
	subscriber := &mailSubscriber{
		id:      fmt.Sprintf("sub-%d", atomic.AddUint64(&h.seq, 1)),
		target:  target,
		since:   since,
		ch:      make(chan *fmail.Message, mailSubscriberBuf),
		paused:  true,
		pending: make([]*fmail.Message, 0, 8),
	}

	h.mu.Lock()
	h.subs[subscriber.id] = subscriber
	h.mu.Unlock()
	return subscriber
}

func (h *mailHub) unsubscribe(subscriber *mailSubscriber) {
	if subscriber == nil {
		return
	}
	h.mu.Lock()
	delete(h.subs, subscriber.id)
	h.mu.Unlock()
	subscriber.close(nil)
}

func (h *mailHub) broadcast(message *fmail.Message) {
	if message == nil {
		return
	}
	h.mu.RLock()
	subscribers := make([]*mailSubscriber, 0, len(h.subs))
	for _, subscriber := range h.subs {
		subscribers = append(subscribers, subscriber)
	}
	h.mu.RUnlock()

	for _, subscriber := range subscribers {
		if !subscriber.matches(message) {
			continue
		}
		subscriber.enqueue(message)
	}
}

func (h *mailHub) ingestMessage(message *fmail.Message) (bool, error) {
	if message == nil {
		return false, errors.New("message is nil")
	}
	saved, err := h.store.SaveMessageExact(message)
	if err != nil {
		return false, err
	}
	if _, err := h.store.UpdateAgentRecord(message.From, message.Host); err != nil {
		h.logger.Warn().Err(err).Str("agent", message.From).Msg("mail relay agent registry update failed")
	}
	if saved {
		h.broadcast(message)
	}
	return saved, nil
}

type mailSubscriber struct {
	id     string
	target mailWatchTarget
	since  sinceFilter

	ch chan *fmail.Message

	mu      sync.Mutex
	paused  bool
	pending []*fmail.Message
	closed  bool
	err     error
}

func (s *mailSubscriber) matches(message *fmail.Message) bool {
	if message == nil {
		return false
	}
	if !s.since.allows(message) {
		return false
	}
	return s.target.matches(message)
}

func (s *mailSubscriber) enqueue(message *fmail.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if s.paused {
		if len(s.pending) >= mailSubscriberQueue {
			s.closeLocked(errBackpressure)
			return
		}
		s.pending = append(s.pending, message)
		return
	}
	select {
	case s.ch <- message:
	default:
		s.closeLocked(errBackpressure)
	}
}

func (s *mailSubscriber) resume() []*fmail.Message {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.paused = false
	pending := make([]*fmail.Message, len(s.pending))
	copy(pending, s.pending)
	s.pending = nil
	return pending
}

func (s *mailSubscriber) error() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

func (s *mailSubscriber) close(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked(err)
}

func (s *mailSubscriber) closeLocked(err error) {
	if s.closed {
		return
	}
	s.closed = true
	if err != nil {
		s.err = err
	}
	close(s.ch)
}

type mailWatchMode int

const (
	watchAll mailWatchMode = iota
	watchTopic
	watchDM
	watchRelay
)

type mailWatchTarget struct {
	mode  mailWatchMode
	name  string
	agent string
}

func (t mailWatchTarget) matches(message *fmail.Message) bool {
	if message == nil {
		return false
	}
	to := message.To
	switch t.mode {
	case watchTopic:
		return to == t.name
	case watchDM:
		return to == "@"+t.name
	case watchAll:
		if strings.HasPrefix(to, "@") {
			return to == "@"+t.agent
		}
		return true
	case watchRelay:
		return true
	default:
		return false
	}
}

type sinceFilter struct {
	id   string
	time *time.Time
}

func (f sinceFilter) allows(message *fmail.Message) bool {
	if message == nil {
		return false
	}
	if f.id != "" {
		return message.ID > f.id
	}
	if f.time != nil {
		return message.Time.After(*f.time)
	}
	return true
}

type mailBaseRequest struct {
	Cmd       string `json:"cmd"`
	ProjectID string `json:"project_id,omitempty"`
	Agent     string `json:"agent"`
	Host      string `json:"host,omitempty"`
	ReqID     string `json:"req_id,omitempty"`
}

type mailSendRequest struct {
	mailBaseRequest
	To       string          `json:"to"`
	Body     json.RawMessage `json:"body"`
	ReplyTo  string          `json:"reply_to,omitempty"`
	Priority string          `json:"priority,omitempty"`
	Tags     []string        `json:"tags,omitempty"`
}

type mailWatchRequest struct {
	mailBaseRequest
	Topic string `json:"topic,omitempty"`
	Since string `json:"since,omitempty"`
}

type mailRelayRequest struct {
	mailBaseRequest
	Since string `json:"since,omitempty"`
}

type mailResponse struct {
	OK    bool     `json:"ok"`
	ID    string   `json:"id,omitempty"`
	Error *mailErr `json:"error,omitempty"`
	ReqID string   `json:"req_id,omitempty"`
}

type mailErr struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

func parseMailWatchTarget(topic, agent string) (mailWatchTarget, error) {
	trimmed := strings.TrimSpace(topic)
	if trimmed == "" || trimmed == "*" {
		return mailWatchTarget{mode: watchAll, agent: agent}, nil
	}
	if strings.HasPrefix(trimmed, "@") {
		name, err := fmail.NormalizeAgentName(strings.TrimPrefix(trimmed, "@"))
		if err != nil {
			return mailWatchTarget{}, err
		}
		if name != agent {
			return mailWatchTarget{}, errors.New("dm watch must match agent")
		}
		return mailWatchTarget{mode: watchDM, name: name}, nil
	}
	topicName, err := fmail.NormalizeTopic(trimmed)
	if err != nil {
		return mailWatchTarget{}, err
	}
	return mailWatchTarget{mode: watchTopic, name: topicName}, nil
}

var mailIDPattern = regexp.MustCompile(`^\d{8}-\d{6}-\d{4}$`)

func parseMailSince(raw string) (sinceFilter, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return sinceFilter{}, nil
	}
	if mailIDPattern.MatchString(trimmed) {
		return sinceFilter{id: trimmed}, nil
	}
	if ts, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		utc := ts.UTC()
		return sinceFilter{time: &utc}, nil
	}
	if ts, err := time.Parse(time.RFC3339, trimmed); err == nil {
		utc := ts.UTC()
		return sinceFilter{time: &utc}, nil
	}
	return sinceFilter{}, fmt.Errorf("invalid since")
}

func parseMailBody(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, errors.New("missing body")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, errors.New("missing body")
	}
	return json.RawMessage(trimmed), nil
}

func loadMailBacklog(store *fmail.Store, target mailWatchTarget, since sinceFilter) ([]*fmail.Message, error) {
	if store == nil {
		return nil, errors.New("store is nil")
	}
	var messages []*fmail.Message
	switch target.mode {
	case watchTopic:
		list, err := store.ListTopicMessages(target.name)
		if err != nil {
			return nil, err
		}
		messages = appendMessages(messages, list, since)
	case watchDM:
		list, err := store.ListDMMessages(target.name)
		if err != nil {
			return nil, err
		}
		messages = appendMessages(messages, list, since)
	case watchAll:
		topics, err := store.ListTopics()
		if err != nil {
			return nil, err
		}
		for _, topic := range topics {
			list, err := store.ListTopicMessages(topic.Name)
			if err != nil {
				return nil, err
			}
			messages = appendMessages(messages, list, since)
		}
		if target.agent != "" {
			list, err := store.ListDMMessages(target.agent)
			if err != nil {
				return nil, err
			}
			messages = appendMessages(messages, list, since)
		}
	case watchRelay:
		topics, err := store.ListTopics()
		if err != nil {
			return nil, err
		}
		for _, topic := range topics {
			list, err := store.ListTopicMessages(topic.Name)
			if err != nil {
				return nil, err
			}
			messages = appendMessages(messages, list, since)
		}

		agents, err := listDMMailboxes(store)
		if err != nil {
			return nil, err
		}
		for _, agent := range agents {
			list, err := store.ListDMMessages(agent)
			if err != nil {
				return nil, err
			}
			messages = appendMessages(messages, list, since)
		}
	default:
		return nil, errors.New("unknown watch target")
	}
	sortMailMessages(messages)
	return messages, nil
}

func listDMMailboxes(store *fmail.Store) ([]string, error) {
	if store == nil {
		return nil, errors.New("store is nil")
	}
	root := filepath.Join(store.Root, "dm")
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if err := fmail.ValidateAgentName(name); err != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func appendMessages(messages []*fmail.Message, list []fmail.Message, since sinceFilter) []*fmail.Message {
	for i := range list {
		msg := list[i]
		if !since.allows(&msg) {
			continue
		}
		messages = append(messages, &msg)
	}
	return messages
}

func filterMessages(messages []*fmail.Message, since sinceFilter) []*fmail.Message {
	if since.id == "" && since.time == nil {
		return messages
	}
	filtered := messages[:0]
	for _, msg := range messages {
		if since.allows(msg) {
			filtered = append(filtered, msg)
		}
	}
	copy(messages, filtered)
	for i := len(filtered); i < len(messages); i++ {
		messages[i] = nil
	}
	return filtered
}

func sortMailMessages(messages []*fmail.Message) {
	sort.Slice(messages, func(i, j int) bool {
		a := messages[i]
		b := messages[j]
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		if !a.Time.Equal(b.Time) {
			return a.Time.Before(b.Time)
		}
		if a.From != b.From {
			return a.From < b.From
		}
		return a.To < b.To
	})
}

func readMailLine(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(line) == 0 && errors.Is(err, io.EOF) {
		return nil, io.EOF
	}
	if len(line) > mailLineLimit {
		return nil, errors.New("line too long")
	}
	return bytes.TrimSpace(line), nil
}

func writeMailResponse(writer io.Writer, response mailResponse) error {
	return writeMailJSON(writer, response)
}

func writeMailError(writer io.Writer, reqID, code, message string) error {
	resp := mailResponse{
		OK:    false,
		Error: &mailErr{Code: code, Message: message},
		ReqID: reqID,
	}
	return writeMailJSON(writer, resp)
}

func writeMailMessage(writer io.Writer, message *fmail.Message) error {
	payload := struct {
		Msg *fmail.Message `json:"msg"`
	}{
		Msg: message,
	}
	return writeMailJSON(writer, payload)
}

func writeMailJSON(writer io.Writer, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := writer.Write(data); err != nil {
		return err
	}
	_, err = writer.Write([]byte("\n"))
	return err
}
