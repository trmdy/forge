---
id: f-f1e6
status: open
deps: []
links: []
created: 2026-01-11T17:50:48Z
type: feature
priority: 2
assignee: Tormod Haugland
parent: f-3a50
---
# Scoped gc (per-topic, --before)

## Problem

Current `fmail gc` is a blunt instrument:

```bash
fmail gc              # Remove messages older than 7 days
fmail gc --days 1     # Remove messages older than 1 day
```

This has issues:

1. **All-or-nothing**: Can't clean up noisy topics while preserving important ones
2. **Age-only**: No way to clean up to a specific point (e.g., "delete everything before this checkpoint")
3. **No DM scoping**: Can't target specific DM inboxes

Real scenarios:
- `build` topic has 1000s of ephemeral messages - want aggressive cleanup
- `task` topic has important history - want to preserve
- After processing messages, want to delete up to the cursor

## Solution

Add scoping options to `fmail gc`:

```bash
# Per-topic cleanup
fmail gc --topic build --days 1
fmail gc --topic build,status --days 1  # Multiple topics

# Per-DM cleanup
fmail gc --dm myname --days 7

# ID-based cutoff (delete up to and including this ID)
fmail gc --topic build --before 20260111-120000-0000

# Combine: delete old messages from specific topic
fmail gc --topic ephemeral --days 0  # Delete all from topic
```

## Implementation Notes

### CLI Changes (cmd/fmail/gc.go)

```go
var (
    topics   []string
    dms      []string
    beforeID string
    days     int
    dryRun   bool
)

cmd.Flags().StringSliceVar(&topics, "topic", nil, "Limit to specific topics")
cmd.Flags().StringSliceVar(&dms, "dm", nil, "Limit to specific DM inboxes")
cmd.Flags().StringVar(&beforeID, "before", "", "Delete messages with ID <= this value")
cmd.Flags().IntVar(&days, "days", 7, "Delete messages older than N days")
cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted")
```

### Store Changes (internal/mail/store.go)

```go
type GCOptions struct {
    Topics   []string      // Empty = all topics
    DMs      []string      // Empty = all DMs
    BeforeID string        // Empty = use age cutoff
    MaxAge   time.Duration // Used if BeforeID empty
    DryRun   bool
}

func (s *Store) GC(opts GCOptions) (GCResult, error) {
    var result GCResult
    
    // Determine topics to process
    topics := opts.Topics
    if len(topics) == 0 {
        topics = s.ListTopics()
    }
    
    for _, topic := range topics {
        deleted, _ := s.gcTopic(topic, opts)
        result.TopicMessages += deleted
    }
    
    // Similar for DMs
    dms := opts.DMs
    if len(dms) == 0 {
        dms = s.ListDMInboxes()
    }
    
    for _, dm := range dms {
        deleted, _ := s.gcDM(dm, opts)
        result.DMMessages += deleted
    }
    
    return result, nil
}

func (s *Store) gcTopic(topic string, opts GCOptions) (int, error) {
    dir := s.TopicDir(topic)
    entries, _ := os.ReadDir(dir)
    
    var deleted int
    cutoff := time.Now().Add(-opts.MaxAge)
    
    for _, e := range entries {
        id := strings.TrimSuffix(e.Name(), ".json")
        
        // Check ID-based cutoff
        if opts.BeforeID != "" && id > opts.BeforeID {
            continue  // Keep messages after BeforeID
        }
        
        // Check age-based cutoff (parse timestamp from ID)
        if opts.BeforeID == "" {
            msgTime := parseIDTime(id)
            if msgTime.After(cutoff) {
                continue  // Keep recent messages
            }
        }
        
        if opts.DryRun {
            fmt.Printf("would delete: %s/%s\n", topic, id)
        } else {
            os.Remove(filepath.Join(dir, e.Name()))
        }
        deleted++
    }
    return deleted, nil
}
```

### Output

```bash
$ fmail gc --topic build --days 1 --dry-run
Would delete 847 messages from topic 'build'

$ fmail gc --topic build --days 1
Deleted 847 messages from topic 'build'

$ fmail gc --before 20260111-120000-0000
Deleted 1,234 messages across 5 topics
Deleted 42 direct messages across 3 inboxes
```

## Acceptance Criteria

- [ ] `fmail gc --topic X` limits cleanup to specified topic(s)
- [ ] `fmail gc --dm X` limits cleanup to specified DM inbox(es)
- [ ] `fmail gc --before <id>` deletes messages with ID <= specified
- [ ] `fmail gc --topic X --days 0` deletes all messages from topic
- [ ] `--dry-run` shows what would be deleted without deleting
- [ ] Default behavior unchanged (all topics/DMs, 7 days)
- [ ] Output shows count of deleted messages per scope

## Why This Matters

Enables:
- **Ephemeral topics**: High-volume topics like `build` or `heartbeat` can be cleaned aggressively
- **Cursor-based cleanup**: After processing, delete everything up to cursor
- **Selective preservation**: Keep `task` history while cleaning `status`

## Related

- f-de22: Cursor resume provides the `--before` ID for cleanup
- Forge loops could auto-gc their event topics
