---
id: swarm-j25q.3
status: closed
deps: []
links: []
created: 2025-12-27T07:10:54.176169127+01:00
type: task
priority: 2
parent: swarm-j25q
---
# Create Swarm Mail skill for Claude/OpenCode

Create a skill document that teaches agents best practices for Swarm coordination.

## File: .claude/skills/swarm-mail/SKILL.md

```markdown
---
name: swarm-mail
description: Coordinate with other agents using Swarm mail and file locking
globs:
  - "**/*"
alwaysApply: true
---

# Swarm Multi-Agent Coordination

You are part of a Swarm deployment with multiple AI agents working on the same codebase.
Use these patterns to coordinate effectively.

## When to Use Mail vs Queue vs Direct

- **Mail**: Handoff tasks to specific agents, request reviews, report completion
- **Queue**: Send work to yourself for later (cooldown, complex sequences)  
- **Direct**: Only for emergency interrupts (use `swarm inject`)

## Writing Actionable Handoff Messages

Good handoff messages require NO follow-up questions:

### Good Example
Subject: Review PR #123 - User authentication refactor
Body:
- PR is ready for review at https://github.com/...
- Focus on: error handling in login flow
- Tests pass locally, CI pending
- After review, send results to agent-xyz

### Bad Example  
Subject: Please review
Body: The PR is ready.

## Advisory File Locking

Before editing files, claim a lock:
```bash
swarm lock claim --agent $AGENT_ID --path "src/api/auth.go" --ttl 30m
```

Check for conflicts before claiming:
```bash
swarm lock check --path "src/api/auth.go"
```

Always release when done:
```bash
swarm lock release --agent $AGENT_ID
```

## Subject/Body Conventions

Subjects should be:
- Specific: "Fix null pointer in UserService.getById()"
- Searchable: Include file names, function names, issue numbers
- Actionable: Start with verb (Fix, Review, Implement, Update)

## Checking Your Inbox

Poll for new work regularly:
```bash
swarm mail inbox --agent $AGENT_ID --unread
```
```

## Additional template files

### templates/handoff.md
Standard handoff message template.

### templates/review-request.md  
Request another agent to review code.

### templates/conflict-resolution.md
Template for resolving file lock conflicts.


