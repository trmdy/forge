package fmail

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
	"strings"
	"sync/atomic"
	"time"
)

const (
	forgedSocketName    = "forged.sock"
	forgedTCPAddr       = "127.0.0.1:7463"
	forgedDialTimeout   = 200 * time.Millisecond
	forgedLineLimit     = MaxMessageSize + 64*1024
	forgedReconnectWait = 2 * time.Second
)

var (
	errForgedUnavailable  = errors.New("forged unavailable")
	errForgedDisconnected = errors.New("forged disconnected")
)

type mailConn struct {
	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
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

type mailEnvelope struct {
	OK    *bool    `json:"ok,omitempty"`
	Error *mailErr `json:"error,omitempty"`
	Msg   *Message `json:"msg,omitempty"`
	Event string   `json:"event,omitempty"`
	ReqID string   `json:"req_id,omitempty"`
}

type forgedServerError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *forgedServerError) Error() string {
	msg := strings.TrimSpace(e.Message)
	if msg == "" {
		msg = "forged error"
	}
	if e.Code == "" {
		return msg
	}
	return fmt.Sprintf("%s (%s)", msg, e.Code)
}

func formatForgedError(err *mailErr) string {
	if err == nil {
		return "unknown error"
	}
	msg := strings.TrimSpace(err.Message)
	if msg == "" {
		msg = err.Code
	}
	if msg == "" {
		return "unknown error"
	}
	if err.Code == "" || strings.Contains(msg, err.Code) {
		return msg
	}
	return fmt.Sprintf("%s (%s)", msg, err.Code)
}

func dialForged(root string) (*mailConn, error) {
	trimmed := strings.TrimSpace(root)
	if trimmed == "" {
		return nil, fmt.Errorf("project root required")
	}

	socketPath := filepath.Join(trimmed, ".fmail", forgedSocketName)
	if conn, err := net.DialTimeout("unix", socketPath, forgedDialTimeout); err == nil {
		return newMailConn(conn), nil
	}

	conn, err := net.DialTimeout("tcp", forgedTCPAddr, forgedDialTimeout)
	if err != nil {
		return nil, errForgedUnavailable
	}
	return newMailConn(conn), nil
}

func newMailConn(conn net.Conn) *mailConn {
	return &mailConn{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
	}
}

func (c *mailConn) Close() error {
	if c == nil || c.conn == nil {
		return nil
	}
	return c.conn.Close()
}

func (c *mailConn) writeJSON(payload any) error {
	if c == nil || c.writer == nil {
		return errors.New("mail connection is nil")
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := c.writer.Write(data); err != nil {
		return err
	}
	if _, err := c.writer.Write([]byte("\n")); err != nil {
		return err
	}
	return c.writer.Flush()
}

func (c *mailConn) readLine() ([]byte, error) {
	if c == nil || c.reader == nil {
		return nil, errors.New("mail connection is nil")
	}
	return readMailLine(c.reader)
}

func readMailLine(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(line) == 0 && errors.Is(err, io.EOF) {
		return nil, io.EOF
	}
	if len(line) > forgedLineLimit {
		return nil, errors.New("line too long")
	}
	return bytes.TrimSpace(line), nil
}

func encodeMailBody(body any) (json.RawMessage, error) {
	if body == nil {
		return nil, errors.New("missing body")
	}
	switch value := body.(type) {
	case json.RawMessage:
		trimmed := bytes.TrimSpace(value)
		if len(trimmed) == 0 {
			return nil, errors.New("missing body")
		}
		return trimmed, nil
	default:
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return data, nil
	}
}

func resolveProjectID(root string) (string, error) {
	if id, ok := readProjectID(root); ok {
		return id, nil
	}
	return DeriveProjectID(root)
}

func readProjectID(root string) (string, bool) {
	path := filepath.Join(root, ".fmail", "project.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	var payload Project
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", false
	}
	id := strings.TrimSpace(payload.ID)
	if id == "" {
		return "", false
	}
	return id, true
}

var reqCounter uint64

func nextReqID() string {
	seq := atomic.AddUint64(&reqCounter, 1)
	return fmt.Sprintf("req-%d-%d", time.Now().UTC().UnixNano(), seq)
}
