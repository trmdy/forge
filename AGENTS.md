# Agent Instructions

This project uses `tk` for issue tracking. Run `tk help` when you need it.

## Quick Reference

```bash
tk ready              # List ready tickets
tk show <id>          # View ticket details
tk start <id>         # Mark in_progress
tk close <id>         # Close ticket
tk create "Title" -t bug|feature|task -p 0-4 -d "Description"
```

## Issue Tracking with tk

This project uses `tk` for issue tracking.
Tickets live in `.tickets/` and should be committed with related code changes.
Run `tk help` when you need more commands.


### Workflow Pattern

3. **Work**: Implement the task

### Key Concepts

- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (use numbers, not words)
- **Types**: task, bug, feature, epic, question, docs

### Session Protocol

**Before ending any session, run this checklist:**

```bash
git status              # Check what changed
git add <files>         # Stage code changes
git commit -m "..."     # Commit code
git push                # Push to remote
```

### Best Practices

- Update status as you work (in_progress → closed)
- Use descriptive titles and set appropriate priority/type
