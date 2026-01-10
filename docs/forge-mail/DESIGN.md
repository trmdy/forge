# Forge Mail Design Notes

This document captures design decisions, trade-offs, and future considerations.

---

## Design Philosophy

### Core Principles

1. **Zero friction** - Works immediately, no setup required
2. **Files as truth** - Everything is inspectable with ls, cat, grep
3. **Shell-native** - Pipes work, JSON output, exit codes matter
4. **Optional enhancement** - Forged adds features but isn't required

### Unix Philosophy Applied

1. **Do one thing well** - messaging between agents
2. **Text streams** - JSON is text, files are text, pipes work
3. **Composability** - `fmail log --json | jq | ...`
4. **Transparency** - `.fmail/` is inspectable with standard tools

### Anti-Patterns Avoided

From mcp_agent_mail's complexity:
- No 80+ config options (we have 3)
- No dual storage (Git + SQLite) - just files
- No MCP protocol overhead - just JSON lines
- No complex contact policies - open pub/sub
- No JWT/RBAC/rate limiting - trust model assumes same-project agents are trusted

---

## Key Design Decisions

### 1. Files as Primary Storage

**Decision:** JSON files in `.fmail/` directories

**Rationale:**
- Zero dependencies (no SQLite, no Redis)
- Human-readable with `cat`
- Git-trackable if desired
- Debuggable with ls/grep/find
- Works in containers, CI, anywhere

**Trade-offs:**
- No full-text search (use grep)
- No complex queries (use jq)
- File count can grow (use gc)

### 2. Timestamp-Based IDs

**Decision:** Files named `YYYYMMDD-HHMMSS-NNNN.json`

**Rationale:**
- Globally sortable across all topics and hosts
- Human-readable timestamp embedded
- Collision-resistant (sequence within same second)
- No coordination needed between hosts

**Trade-offs:**
- Longer filenames than simple sequence numbers
- Requires clock synchronization for strict ordering

**Implementation:**
```go
func generateID() string {
    now := time.Now().UTC()
    seq := atomic.AddUint32(&counter, 1) % 10000
    return fmt.Sprintf("%s-%04d", now.Format("20060102-150405"), seq)
}
```

### 3. @ Prefix for Direct Messages

**Decision:** `@agent` for DMs, plain names for topics

**Rationale:**
- Intuitive (email-like)
- Visually distinct from topics
- Clear semantics in CLI: `fmail send @reviewer` vs `fmail send task`

**Storage:** Separate `dm/` directory for DMs
- `topics/task/` for topic messages
- `dm/reviewer/` for DMs to @reviewer

### 4. Optional Forged Integration

**Decision:** Works standalone, enhanced with forged

**Rationale:**
- Lower barrier to entry
- Works in environments without daemon
- Gradual adoption path

**Trade-offs:**
- Standalone mode is polling-based (100ms)
- No cross-host without forged

### 5. No Read Receipts / Ack System

**Decision:** Omit delivery confirmation mechanisms

**Rationale:**
- Simplicity
- Use request/response pattern instead
- Agents can track their own cursor with `--since`

**Alternative considered:** Full ack system like mcp_agent_mail
- Rejected: Significant complexity, marginal value

### 6. Optional Reply-To (Not Threading)

**Decision:** Simple `reply_to` field, not full threading

**Rationale:**
- Enables correlation without complexity
- Single reference, not a chain
- Agents can use it or ignore it

**Usage:**
```bash
fmail send @lead --reply-to "$ORIGINAL_MSG_ID" "task completed"
```

---

## Edge Cases

### Concurrent Writes

Two agents write to same topic in the same millisecond:

**Solution:** Timestamp + atomic counter ensures unique IDs

```go
var counter uint32

func generateID() string {
    now := time.Now().UTC()
    seq := atomic.AddUint32(&counter, 1) % 10000
    return fmt.Sprintf("%s-%04d", now.Format("20060102-150405"), seq)
}
```

Even if two processes generate IDs at the same second, the atomic counter ensures uniqueness within a process. Cross-process collisions in the same second are handled by retry:

```go
for attempt := 0; attempt < 10; attempt++ {
    id := generateID()
    path := filepath.Join(dir, id+".json")
    err := writeFileExclusive(path, data)  // O_EXCL flag
    if err == nil {
        return id, nil
    }
    if os.IsExist(err) {
        time.Sleep(time.Millisecond)
        continue
    }
    return "", err
}
```

### Topic Name Validation

Allowed: `[a-z0-9-]` (lowercase alphanumeric and hyphens)

Rejected:
- Uppercase → force lowercase
- Spaces → error
- Slashes → error (reserved)
- Starting with `.` → error (reserved for internal)
- Starting with `@` → treated as DM, not topic

### Large Messages

Default limit: 1MB

For larger data, use file references:
```bash
# Write data to file
echo "$large_data" > /tmp/output.csv

# Send reference
fmail send results '{"file":"/tmp/output.csv","size":52428800}'
```

### Agent Name Conflicts

Same agent name on different hosts (connected mode):
- Forged tracks `(name, host)` pairs
- Messages to `@agent` go to all instances of that agent
- Each host's DM inbox is separate

Same agent name on same host (unlikely):
- Later process overwrites agent registry entry
- Both share the same inbox
- Recommendation: Use unique names (e.g., include PID or loop ID)

### Project ID Generation

For cross-host coordination:

```go
func deriveProjectID(root string) string {
    // 1. Explicit override
    if id := os.Getenv("FMAIL_PROJECT"); id != "" {
        return id
    }

    // 2. From git remote (stable across clones)
    if remote := getGitRemote(root); remote != "" {
        h := sha256.Sum256([]byte(remote))
        return "proj-" + hex.EncodeToString(h[:])[:12]
    }

    // 3. From directory name (not portable but works)
    h := sha256.Sum256([]byte(filepath.Base(root)))
    return "proj-" + hex.EncodeToString(h[:])[:12]
}
```

---

## Future Considerations

### Possible Future Features

1. **Message TTL** - Auto-expire messages after N hours
2. **Encryption** - Age encryption for sensitive messages
3. **Webhooks** - HTTP callbacks on message arrival
4. **Priority queues** - High-priority messages delivered first

### Explicitly Deferred

1. **Contact Policies** - Too complex, trust is implicit
2. **File Reservations/Locking** - Convention-based coordination is simpler
3. **LLM Summarization** - Out of scope for core
4. **Git Integration** - Users can commit `.fmail/` if they want
5. **Message Editing** - Messages are immutable once sent
6. **Typing Indicators** - Not relevant for async agent communication

---

## Implementation Notes

### Directory Structure

```go
type Store struct {
    Root string  // Absolute path to .fmail/
}

func (s *Store) TopicDir(topic string) string {
    return filepath.Join(s.Root, "topics", topic)
}

func (s *Store) DMDir(agent string) string {
    return filepath.Join(s.Root, "dm", agent)
}

func (s *Store) AgentsDir() string {
    return filepath.Join(s.Root, "agents")
}

func (s *Store) ProjectFile() string {
    return filepath.Join(s.Root, "project.json")
}
```

### Message Struct

```go
type Message struct {
    ID       string    `json:"id"`
    From     string    `json:"from"`
    To       string    `json:"to"`              // Topic name or @agent
    Time     time.Time `json:"time"`
    Body     any       `json:"body"`
    ReplyTo  string    `json:"reply_to,omitempty"`
    Priority string    `json:"priority,omitempty"` // low, normal, high
    Host     string    `json:"host,omitempty"`     // In connected mode
}
```

### CLI Structure

```go
// cmd/fmail/main.go
func main() {
    app := &cli.App{
        Name:  "fmail",
        Usage: "Agent-to-agent messaging",
        Commands: []*cli.Command{
            sendCmd,
            logCmd,
            watchCmd,
            whoCmd,
            statusCmd,
            topicsCmd,
            gcCmd,
            initCmd,
        },
        Flags: []cli.Flag{
            &cli.BoolFlag{Name: "robot-help", Usage: "Machine-readable help"},
        },
    }
    app.Run(os.Args)
}
```

---

## Metrics and Observability

### What to Track (Future)

- Messages sent/received per topic
- Message latency (send to receive)
- Active agents count
- Storage size per topic

### What NOT to Track

- Message content (privacy)
- Individual agent behavior
- Cross-project metrics

---

## Security Considerations

### Threat Model

- Agents on same project are trusted
- No authentication in MVP (local trust)
- No encryption in MVP

### Future: Multi-Tenant

If multiple untrusted agents share a project:
1. Agent authentication (via forged)
2. Topic ACLs
3. Message encryption

Not in v1.0 scope.

---

## Testing Strategy

### Unit Tests

- Message ID generation
- Topic/agent name validation
- Store operations (read, write, list)
- Time parsing for `--since`

### Integration Tests

- Concurrent write handling
- Watch polling behavior
- Forged socket connection

### End-to-End Tests

- Multi-agent scenarios in same process
- Forged real-time delivery
- gc cleanup

---

## Dependencies

### Required

- Go standard library
- urfave/cli for CLI framework (simpler than Cobra)

### Optional

- None for standalone mode

### Avoided

- SQLite (files only)
- Redis (forged handles real-time)
- gRPC (JSON lines is simpler)
- External services
