---
id: swarm-j25q
status: closed
deps: []
links: []
created: 2025-12-27T07:09:47.821604134+01:00
type: epic
priority: 2
---
# EPIC: OpenCode Plugin and Skills Integration

Create OpenCode plugins and Claude-compatible skills to enable agents to use Swarm features directly.

## Strategic Value
OpenCode hackability is a competitive advantage. By exposing swarm_mail_* and swarm_lock_* tools via plugins, agents can coordinate without MCP, using the CLI as the universal interface.

## Components

### 1. Swarm Mail Skill (.claude/skills/swarm-mail/SKILL.md)
Teaches agents:
- When to use mail vs queue vs direct injection
- How to write actionable handoff messages
- Advisory file locking best practices
- Subject/body conventions for searchability

### 2. OpenCode Plugin (.opencode/plugin/swarm-mail.ts)
Exposes tools:
- swarm_mail_send
- swarm_mail_inbox
- swarm_mail_read
- swarm_mail_ack
- swarm_lock_claim
- swarm_lock_release
- swarm_lock_status

### 3. CLI Commands (for plugin to call)
- swarm mail send|inbox|read|ack --json
- swarm lock claim|release|status --json

## Why Not Just MCP?
- MCP requires server setup and configuration
- OpenCode plugins work immediately
- CLI is the universal interface
- Agents can use Swarm features outside the codebase

## Templates for Skills
Next to SKILL.md:
- templates/handoff.md
- templates/review-request.md
- templates/conflict-resolution.md

These templates are usable by TUI message palette.

## Success Criteria
- Agents can send/receive mail without user intervention
- File locking prevents conflicts in multi-agent repos
- Skills teach best practices automatically
- Plugin works on fresh clone without setup


