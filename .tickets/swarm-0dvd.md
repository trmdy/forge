---
id: swarm-0dvd
status: closed
deps: []
links: []
created: 2025-12-27T08:53:42.184338047+01:00
type: task
priority: 2
---
# Copy pointer files into codebase

Import the reference implementations from forge-ux-pointers.zip into the codebase.

## Files to Add

### 1. docs/ux/forge-cli-v2.md (rename to swarm-cli-v2.md)
Full CLI v2 specification with:
- Command tree
- Help text templates
- Design principles
- Global flags

### 2. .claude/skills/swarm-mail/SKILL.md
Skill document for Claude/OpenCode agents:
- When to use mail vs queue vs direct
- Messaging conventions
- Advisory file locking conventions
- Examples

### 3. .opencode/plugin/swarm-mail.ts
OpenCode plugin skeleton exposing:
- swarm_mail_send
- swarm_mail_inbox
- swarm_mail_read
- swarm_mail_ack
- swarm_lock_claim
- swarm_lock_release
- swarm_lock_status

## Renaming
Change "forge" to "swarm" throughout (or to final chosen name).

## Location
Files are in /tmp/forge-ux-pointers/ after extraction.


