---
id: swarm-r493
status: closed
deps: []
links: []
created: 2025-12-27T09:34:35.983807644+01:00
type: task
priority: 1
---
# Auto-apply DB migrations on startup (remove manual step)

Automatically run DB migrations during init so users dont need to run migrate up.

## From UX_FEEDBACK_2.md - Phase 1

## Current Problem
Users must manually run `swarm migrate up` which:
- Adds friction to getting started
- Is easy to forget
- Causes confusing errors when forgotten

## Solution
Auto-migrate on startup:

```go
func initConfig() {
    // ... load config ...
    
    // Auto-migrate (skip if already current)
    if err := db.AutoMigrate(dbPath); err != nil {
        // Only fatal if truly broken, not if just "already migrated"
        log.Fatal(err)
    }
}
```

## Behavior
1. Check current migration version
2. If behind, apply pending migrations
3. If current, do nothing (fast path)
4. Log what was migrated (if anything)
5. Continue with normal startup

## UX Output
```
✓ Database migrated (v5 → v7)
```

Or silently continue if already current.

## Migration Command
Keep `swarm migrate` for:
- `swarm migrate status` - show current version
- `swarm migrate down` - rollback (admin use)
- `swarm migrate up` - still works (now no-op if current)


