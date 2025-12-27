import { tool, $ } from "opencode/plugin";

function parseJSON(stdout: string) {
  try {
    return JSON.parse(stdout);
  } catch (err) {
    const error = err instanceof Error ? err.message : String(err);
    throw new Error(`Failed to parse swarm JSON output: ${error}\n${stdout}`);
  }
}

function buildArgs(base: string[], options: Array<[boolean, string[]]>) {
  const args = [...base];
  for (const [enabled, extra] of options) {
    if (enabled) {
      args.push(...extra);
    }
  }
  return args;
}

// Mail tools
export const swarm_mail_send = tool({
  name: "swarm_mail_send",
  description: "Send a message to another agent or mailbox",
  parameters: {
    type: "object",
    properties: {
      to: { type: "string", description: "Recipient agent name" },
      subject: { type: "string", description: "Message subject" },
      body: { type: "string", description: "Message body (markdown)" },
      priority: { type: "string", enum: ["low", "normal", "high", "urgent"] },
      ack_required: { type: "boolean", description: "Request acknowledgement" }
    },
    required: ["to", "subject", "body"]
  },
  async execute({ to, subject, body, priority, ack_required }) {
    const args = buildArgs([
      "mail",
      "send",
      "--to",
      to,
      "--subject",
      subject,
      "--json"
    ], [
      [Boolean(priority), ["--priority", priority as string]],
      [Boolean(ack_required), ["--ack-required"]]
    ]);

    const result = await $`swarm ${args} --body ${body}`;
    return parseJSON(result.stdout);
  }
});

export const swarm_mail_inbox = tool({
  name: "swarm_mail_inbox",
  description: "Check inbox for new messages",
  parameters: {
    type: "object",
    properties: {
      agent: { type: "string", description: "Agent name to check inbox for" },
      unread_only: { type: "boolean", description: "Only show unread" },
      limit: { type: "number", description: "Max messages to return" }
    },
    required: ["agent"]
  },
  async execute({ agent, unread_only, limit }) {
    const args = buildArgs([
      "mail",
      "inbox",
      "--agent",
      agent,
      "--json"
    ], [
      [Boolean(unread_only), ["--unread"]],
      [typeof limit === "number", ["--limit", String(limit)]]
    ]);

    const result = await $`swarm ${args}`;
    return parseJSON(result.stdout);
  }
});

export const swarm_mail_read = tool({
  name: "swarm_mail_read",
  description: "Read a mailbox message",
  parameters: {
    type: "object",
    properties: {
      agent: { type: "string", description: "Agent name to read inbox for" },
      message_id: { type: "string", description: "Message ID (e.g., m-001)" }
    },
    required: ["agent", "message_id"]
  },
  async execute({ agent, message_id }) {
    const args = ["mail", "read", message_id, "--agent", agent, "--json"];
    const result = await $`swarm ${args}`;
    return parseJSON(result.stdout);
  }
});

export const swarm_mail_ack = tool({
  name: "swarm_mail_ack",
  description: "Acknowledge a mailbox message",
  parameters: {
    type: "object",
    properties: {
      agent: { type: "string", description: "Agent name to acknowledge for" },
      message_id: { type: "string", description: "Message ID (e.g., m-001)" }
    },
    required: ["agent", "message_id"]
  },
  async execute({ agent, message_id }) {
    const args = ["mail", "ack", message_id, "--agent", agent, "--json"];
    const result = await $`swarm ${args}`;
    return parseJSON(result.stdout);
  }
});

// Lock tools
export const swarm_lock_claim = tool({
  name: "swarm_lock_claim",
  description: "Claim advisory file lock before editing",
  parameters: {
    type: "object",
    properties: {
      agent: { type: "string", description: "Agent name claiming lock" },
      path: { type: "string", description: "File path or glob pattern" },
      ttl: { type: "string", description: "Lock duration (e.g., 30m, 1h)" },
      reason: { type: "string", description: "Why lock is needed" },
      exclusive: { type: "boolean", description: "Request exclusive lock" }
    },
    required: ["agent", "path"]
  },
  async execute({ agent, path, ttl, reason, exclusive }) {
    const args = buildArgs([
      "lock",
      "claim",
      "--agent",
      agent,
      "--path",
      path,
      "--json"
    ], [
      [Boolean(ttl), ["--ttl", ttl as string]],
      [Boolean(reason), ["--reason", reason as string]],
      [exclusive === false, ["--exclusive=false"]]
    ]);

    const result = await $`swarm ${args}`;
    return parseJSON(result.stdout);
  }
});

export const swarm_lock_release = tool({
  name: "swarm_lock_release",
  description: "Release file locks when done editing",
  parameters: {
    type: "object",
    properties: {
      agent: { type: "string", description: "Agent name releasing lock" },
      path: { type: "string", description: "Specific path to release (optional)" }
    },
    required: ["agent"]
  },
  async execute({ agent, path }) {
    const args = buildArgs([
      "lock",
      "release",
      "--agent",
      agent,
      "--json"
    ], [
      [Boolean(path), ["--path", path as string]]
    ]);

    const result = await $`swarm ${args}`;
    return parseJSON(result.stdout);
  }
});

export const swarm_lock_status = tool({
  name: "swarm_lock_status",
  description: "Check current file lock status",
  parameters: {
    type: "object",
    properties: {
      path: { type: "string", description: "Check specific path" },
      agent: { type: "string", description: "Filter by agent name" }
    }
  },
  async execute({ path, agent }) {
    const args = buildArgs([
      "lock",
      "status",
      "--json"
    ], [
      [Boolean(path), ["--path", path as string]],
      [Boolean(agent), ["--agent", agent as string]]
    ]);

    const result = await $`swarm ${args}`;
    return parseJSON(result.stdout);
  }
});
