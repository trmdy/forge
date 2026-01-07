---
id: swarm-j25q.4
status: closed
deps: []
links: []
created: 2025-12-27T07:11:21.20719851+01:00
type: task
priority: 2
parent: swarm-j25q
---
# Create OpenCode plugin for Swarm tools

Create an OpenCode plugin that exposes Swarm mail and lock tools to agents.

## File: .opencode/plugin/swarm-mail.ts

```typescript
import { tool, $ } from "opencode/plugin";

// Mail tools
export const swarm_mail_send = tool({
  name: "swarm_mail_send",
  description: "Send a message to another agent or mailbox",
  parameters: {
    type: "object",
    properties: {
      to: { type: "string", description: "Recipient agent ID" },
      subject: { type: "string", description: "Message subject" },
      body: { type: "string", description: "Message body (markdown)" },
      priority: { type: "string", enum: ["low", "normal", "high", "urgent"] },
      ack_required: { type: "boolean", description: "Request acknowledgement" }
    },
    required: ["to", "subject", "body"]
  },
  async execute({ to, subject, body, priority, ack_required }) {
    const args = ["mail", "send", "--to", to, "--subject", subject, "--json"];
    if (priority) args.push("--priority", priority);
    if (ack_required) args.push("--ack-required");
    
    const result = await $`swarm ${args} --body ${body}`;
    return JSON.parse(result.stdout);
  }
});

export const swarm_mail_inbox = tool({
  name: "swarm_mail_inbox",
  description: "Check inbox for new messages",
  parameters: {
    type: "object",
    properties: {
      agent: { type: "string", description: "Agent ID to check inbox for" },
      unread_only: { type: "boolean", description: "Only show unread" },
      limit: { type: "number", description: "Max messages to return" }
    },
    required: ["agent"]
  },
  async execute({ agent, unread_only, limit }) {
    const args = ["mail", "inbox", "--agent", agent, "--json"];
    if (unread_only) args.push("--unread");
    if (limit) args.push("--limit", String(limit));
    
    const result = await $`swarm ${args}`;
    return JSON.parse(result.stdout);
  }
});

// Lock tools
export const swarm_lock_claim = tool({
  name: "swarm_lock_claim",
  description: "Claim advisory file lock before editing",
  parameters: {
    type: "object", 
    properties: {
      agent: { type: "string", description: "Agent ID claiming lock" },
      path: { type: "string", description: "File path or glob pattern" },
      ttl: { type: "string", description: "Lock duration (e.g., 30m, 1h)" },
      reason: { type: "string", description: "Why lock is needed" }
    },
    required: ["agent", "path"]
  },
  async execute({ agent, path, ttl, reason }) {
    const args = ["lock", "claim", "--agent", agent, "--path", path, "--json"];
    if (ttl) args.push("--ttl", ttl);
    if (reason) args.push("--reason", reason);
    
    const result = await $`swarm ${args}`;
    return JSON.parse(result.stdout);
  }
});

export const swarm_lock_release = tool({
  name: "swarm_lock_release",
  description: "Release file locks when done editing",
  parameters: {
    type: "object",
    properties: {
      agent: { type: "string", description: "Agent ID releasing lock" },
      path: { type: "string", description: "Specific path to release (optional)" }
    },
    required: ["agent"]
  },
  async execute({ agent, path }) {
    const args = ["lock", "release", "--agent", agent, "--json"];
    if (path) args.push("--path", path);
    
    const result = await $`swarm ${args}`;
    return JSON.parse(result.stdout);
  }
});

export const swarm_lock_status = tool({
  name: "swarm_lock_status", 
  description: "Check current file lock status",
  parameters: {
    type: "object",
    properties: {
      path: { type: "string", description: "Check specific path" },
      agent: { type: "string", description: "Filter by agent" }
    }
  },
  async execute({ path, agent }) {
    const args = ["lock", "status", "--json"];
    if (path) args.push("--path", path);
    if (agent) args.push("--agent", agent);
    
    const result = await $`swarm ${args}`;
    return JSON.parse(result.stdout);
  }
});
```

## Installation
Plugin auto-loads from .opencode/plugin/ directory.
No additional configuration needed.


