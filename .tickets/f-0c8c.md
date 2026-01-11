---
id: f-0c8c
status: open
deps: []
links: []
created: 2026-01-11T17:51:08Z
type: feature
priority: 3
assignee: Tormod Haugland
parent: f-4c08
---
# Multi-recipient DM fan-out with broadcast_id

## Problem

Currently, sending the same message to multiple agents requires separate sends:

```bash
fmail send @alice "please review PR #42"
fmail send @bob "please review PR #42"
fmail send @charlie "please review PR #42"
```

Issues:
1. **Tedious**: Three commands for one logical message
2. **No correlation**: Recipients can't tell this was a broadcast
3. **No reply-all**: Can't easily respond to everyone

## Solution

Allow comma-separated recipients with automatic fan-out:

```bash
fmail send @alice,@bob,@charlie "please review PR #42"
```

This creates three separate DM files (one per recipient), but with shared metadata for correlation:

```json
// .fmail/dm/alice/20260111-153000-0001.json
{
  "id": "20260111-153000-0001",
  "from": "architect",
  "to": "@alice",
  "body": "please review PR #42",
  "broadcast_id": "20260111-153000-0001",
  "broadcast_to": ["alice", "bob", "charlie"]
}
```

### Why Separate Files?

- **Privacy**: Each recipient only sees their inbox
- **Permissions**: DM directories have restrictive permissions
- **Simplicity**: No shared inbox complexity
- **Existing behavior**: `fmail log @alice` just works

### Correlation via broadcast_id

The `broadcast_id` is the message ID of the first recipient (lexicographically). All copies share this ID for correlation.

## Implementation Notes

### Schema Changes (internal/mail/message.go)

```go
type Message struct {
    // ... existing fields ...
    BroadcastID string   `json:"broadcast_id,omitempty"`
    BroadcastTo []string `json:"broadcast_to,omitempty"`
}
```

### Send Command (cmd/fmail/send.go)

```go
func sendMessage(target, body string) error {
    targets := parseTargets(target)  // Split on comma
    
    if len(targets) == 1 {
        // Single recipient - existing behavior
        return store.Send(targets[0], msg)
    }
    
    // Multi-recipient broadcast
    broadcastTo := make([]string, len(targets))
    for i, t := range targets {
        broadcastTo[i] = strings.TrimPrefix(t, "@")
    }
    
    // Generate broadcast ID (use first recipient's message ID)
    broadcastID := generateID()
    
    for i, t := range targets {
        msg := Message{
            ID:          generateID(),
            From:        agentName,
            To:          t,
            Body:        body,
            BroadcastID: broadcastID,
            BroadcastTo: broadcastTo,
        }
        store.SendDM(strings.TrimPrefix(t, "@"), msg)
    }
    
    return nil
}
```

### Display in fmail log

Show broadcast indicator:

```bash
$ fmail log @alice
20260111-153000-0001  architect  [broadcast to 3]  please review PR #42
20260111-152000-0001  bob        (reply)           on it
```

### Reply-All Pattern

With broadcast metadata, recipients can reply to all:

```bash
# Alice sees the broadcast
$ fmail log @alice --json | jq '.broadcast_to'
["alice", "bob", "charlie"]

# Alice replies to all
$ fmail send @architect,@bob,@charlie "I'll take this one"
```

Future: Add `fmail reply-all <msg-id> "text"` shorthand.

### CLI Output

```bash
$ fmail send @alice,@bob,@charlie "please review"
Sent to 3 recipients (broadcast_id: 20260111-153000-0001)
```

## Acceptance Criteria

- [ ] `fmail send @a,@b "msg"` creates separate DM files for each recipient
- [ ] All copies share `broadcast_id` field
- [ ] All copies include `broadcast_to` array
- [ ] `fmail log --json` includes broadcast fields
- [ ] Human output shows broadcast indicator
- [ ] Single-recipient sends unchanged (no broadcast fields)
- [ ] SPEC.md documents broadcast_id and broadcast_to fields

## Edge Cases

### Empty Recipients

```bash
fmail send @, "msg"  # Error: empty recipient
```

### Duplicate Recipients

```bash
fmail send @alice,@alice "msg"  # Dedupe: send once
```

### Self-Send

```bash
export FMAIL_AGENT=alice
fmail send @alice,@bob "msg"  # Include self in broadcast? Yes.
```

## Why This Matters

Enables:
- **Team communication**: Send to multiple agents at once
- **Coordination**: "All reviewers, please look at this"
- **Traceability**: Recipients know who else received the message
- **Future reply-all**: Foundation for group conversations

## Related

- f-1240: Reply shorthand (could extend to reply-all)
- f-1e51: Bridge forge msg + fmail (broadcasts could target multiple loops)
