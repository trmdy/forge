# Forge Mail: Forged Transport Protocol (JSON Lines)

**Version:** 1.0.0-draft
**Status:** Draft
**Last Updated:** 2026-01-10

This document defines the on-the-wire contract between `fmail` and `forged` in
connected mode. It is intentionally small: newline-delimited JSON over a local
socket, with only `send` and `watch` required.

---

## Transport

- **Unix socket (preferred):** `<project-root>/.fmail/forged.sock`
- **TCP fallback:** `127.0.0.1:7463`

Protocol framing is one JSON object per line. Each line must be a single JSON
object with no leading/trailing content. UTF-8 is allowed but all fields and
examples here are ASCII.

No gRPC. No protobuf. Just JSON lines.

---

## Connection Discovery (fmail)

1. Resolve the project root (same rules as standalone mode).
2. Attempt `unix://<project-root>/.fmail/forged.sock`.
3. If the socket is missing or connection fails quickly, attempt
   `tcp://127.0.0.1:7463`.
4. If both fail, fall back to standalone file mode.

Discovery should be fast (sub-second) so CLI commands stay responsive.

---

## Project Scoping

- **Unix socket:** treated as **one project** scoped to its project root.
- **TCP fallback:** can serve **multiple projects concurrently**.

Clients **must** send `project_id` on TCP connections. For unix sockets,
`project_id` is optional but recommended for validation and logs.

If a unix-socket server receives a `project_id` that does not match its bound
project, it should return `invalid_project`.

---

## Common Fields

These fields are shared across commands.

| Field | Required | Description |
|-------|----------|-------------|
| `cmd` | yes | Command name (`send`, `watch`) |
| `project_id` | required for TCP | Project ID (see SPEC.md) |
| `agent` | yes | Sender/identity (`FMAIL_AGENT` or generated) |
| `host` | no | Hostname of the sender (optional but recommended) |
| `req_id` | no | Client-generated request ID for tracing |

---

## Message Schema

Messages delivered over the socket match the on-disk JSON format:

```json
{
  "id": "20260110-153000-0001",
  "from": "architect",
  "to": "task",
  "time": "2026-01-10T15:30:00Z",
  "body": "implement user auth",
  "reply_to": "20260110-152500-0003",
  "priority": "high",
  "host": "build-server",
  "tags": ["urgent", "auth"]
}
```

`reply_to`, `priority`, `host`, and `tags` are optional.

---

## Command: send

Send a message to a topic or direct message target.

### Request

```json
{
  "cmd": "send",
  "project_id": "proj-abc123",
  "agent": "architect",
  "host": "build-server",
  "to": "task",
  "body": "implement auth",
  "reply_to": "20260110-152500-0003",
  "priority": "normal",
  "tags": ["urgent", "auth"],
  "req_id": "c1"
}
```

Notes:
- `to` accepts topic names or `@agent` for DMs.
- `body` can be a JSON string, object, array, or number.
- `priority` is `low`, `normal`, or `high` (default: `normal`).
- `tags` is an optional array of lowercase alphanumeric tags (max 10, each max 50 chars).

### Response (success)

```json
{ "ok": true, "id": "20260110-153000-0001", "req_id": "c1" }
```

### Response (error)

```json
{
  "ok": false,
  "error": { "code": "invalid_topic", "message": "topics must match [a-z0-9-]" },
  "req_id": "c1"
}
```

The server must only return `ok: true` after the message is persisted.

---

## Command: watch

Subscribe to a stream of new messages.

### Request

```json
{
  "cmd": "watch",
  "project_id": "proj-abc123",
  "agent": "architect",
  "host": "build-server",
  "topic": "task",
  "since": "2026-01-10T15:00:00Z",
  "req_id": "w1"
}
```

Rules:
- `topic` uses the same syntax as the CLI:
  - `task` watches that topic
  - `@agent` watches DMs for that agent (must match `agent`)
  - `*` or empty watches all topics plus DMs to `agent`
- `since` is optional. If present, the server should send any messages newer
  than `since` before entering live stream mode. `since` may be a message ID
  (`YYYYMMDD-HHMMSS-NNNN`) or RFC3339 timestamp.

### Response (ack)

```json
{ "ok": true, "req_id": "w1" }
```

### Streamed messages

```json
{ "msg": { "id": "...", "from": "architect", "to": "task", "time": "...", "body": "..." } }
{ "msg": { "id": "...", "from": "coder", "to": "task", "time": "...", "body": "on it" } }
```

Optional keepalive:

```json
{ "event": "ping" }
```

Errors during a watch stream should be sent as an error response and then the
connection closed.

---

## Command: relay (for forged-to-forged sync)

Stream all messages (topics + direct messages) for a project. Intended for
trusted forged peers only.

### Request

```json
{
  "cmd": "relay",
  "project_id": "proj-abc123",
  "agent": "relay-host-a",
  "host": "host-a",
  "since": "2026-01-10T15:00:00Z",
  "req_id": "r1"
}
```

Notes:
- `since` follows the same rules as `watch`.
- The server streams all topics and all DM inboxes for the project.

### Response (ack)

```json
{ "ok": true, "req_id": "r1" }
```

### Streamed messages

```json
{ "msg": { "id": "...", "from": "architect", "to": "task", "time": "...", "body": "..." } }
```

---

## Errors

Error responses follow this shape:

```json
{
  "ok": false,
  "error": {
    "code": "invalid_request",
    "message": "missing project_id",
    "retryable": false
  },
  "req_id": "w1"
}
```

Recommended error codes:
- `invalid_request`
- `invalid_project`
- `invalid_topic`
- `invalid_agent`
- `too_large`
- `backpressure`
- `internal`

Unknown error codes should be treated as non-retryable unless `retryable` is
explicitly true.

---

## Ordering, Backpressure, and Reconnect

- **Ordering:** The server should deliver messages in ascending ID order per
  topic/DM. Across multiple topics, ordering is best-effort by ID.
- **Backpressure:** If a client cannot keep up, the server may terminate the
  stream with `backpressure`. Clients should reconnect with `since` equal to the
  last seen message ID.
- **At-least-once:** Reconnects may result in duplicate messages. Clients should
  deduplicate by message ID.

---

## Fallback Rules (fmail)

1. If the socket connection fails, `fmail` falls back to standalone mode.
2. If `send` receives a server **error response**, `fmail` surfaces the error
   and does **not** fall back to avoid double-send.
3. If `send` loses the connection **before** a response, `fmail` may fall back
   to standalone mode and should warn about potential duplicates.
4. If `watch` loses the connection, `fmail` should retry with backoff; if it
   cannot reconnect quickly, it falls back to polling with `since` set to the
   last seen message ID.

---

## Notes

- The TCP fallback is **optional** for forged; if unused, fmail simply falls
  back to standalone mode.
- Forged may support multiple projects concurrently over TCP by routing on
  `project_id`.
