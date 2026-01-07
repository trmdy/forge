---
id: swarm-j25q.1
status: closed
deps: []
links: []
created: 2025-12-27T07:10:08.760063817+01:00
type: task
priority: 1
parent: swarm-j25q
---
# Implement swarm mail CLI commands

Add mail subcommand for agent-to-agent and user-to-agent communication.

## Commands

### swarm mail send
Send a message to an agent or mailbox.
```bash
swarm mail send --to agent-a1 --subject "Task handoff" --body "Please review PR #123"
swarm mail send --to agent-a1 --file message.md
swarm mail send --to agent-a1 --stdin
```

Flags:
- --to: Recipient agent ID or name (required)
- --subject, -s: Message subject (required)
- --body, -b: Message body (or use --file/--stdin)
- --file, -f: Read body from file
- --stdin: Read body from stdin
- --priority: low|normal|high|urgent
- --ack-required: Request acknowledgement
- --json: Output JSON

### swarm mail inbox
List messages for an agent.
```bash
swarm mail inbox --agent agent-a1
swarm mail inbox --agent agent-a1 --unread
swarm mail inbox --agent agent-a1 --since 1h
```

Output:
```
ID      FROM        SUBJECT                     TIME     STATUS
m-001   user        Task handoff                5m ago   unread
m-002   agent-b2    Review complete             1h ago   read
```

### swarm mail read
Read a specific message.
```bash
swarm mail read m-001 --agent agent-a1
```

### swarm mail ack
Acknowledge a message.
```bash
swarm mail ack m-001 --agent agent-a1
```

## Storage
- Use existing Agent Mail MCP if available
- Fallback to local SQLite storage
- Store in ~/.config/swarm/mail.db


