# Swarm Terminology and Naming

This document defines canonical terms, acceptable abbreviations, and naming
rules used across the CLI, TUI, docs, and configuration.

## Principles

- Prefer clear, unambiguous nouns over synonyms.
- Use the same term across UI, logs, and docs.
- Do not invent new terms for the same concept.
- Abbreviations are allowed only when consistent and explicit.

## Canonical terms

- **Node**: A machine Swarm can control (local or remote).
- **Workspace**: A repo + node + tmux session managed by Swarm.
- **Agent**: A running CLI instance in a tmux pane.
- **Queue item**: A queued message/pause/conditional for an agent.
- **Account profile**: Provider credentials or account context for an agent.
- **Session**: A tmux session associated with a workspace.
- **Pane**: A tmux pane where an agent runs.
- **Approval**: A user-confirmed permission required by the agent.
- **State**: Agent state (Idle, Working, Error, etc.).
- **Cooldown**: Temporary pause due to rate limiting or policy.
- **Alert**: A prominent issue requiring attention (approval, error, cooldown).

## Acceptable abbreviations

- `ws` = workspace (CLI only; avoid in prose).
- `id` = identifier.
- `acct` = account (CLI only; avoid in prose).
- `cfg` = config (CLI only; avoid in prose).

Avoid: "host", "box", "worker", "task" (use node, agent, queue item).

## Naming and display rules

- **CLI list views**: show Name (if present) + short ID in parentheses.
- **Short IDs**: first 8 chars; keep full ID in JSON output.
- **Profiles**: display as "account profile" or "profile" (consistent in a view).
- **States**: use Title Case in human output, raw enum in JSON.
- **Commands**: use full nouns (`agent terminate`, not `agent kill`), but allow
  aliases for expert users.

## State names (canonical)

- Starting
- Working
- Idle
- AwaitingApproval
- RateLimited
- Paused
- Error
- Stopped

## Synonyms to avoid or map

| Avoid         | Use instead          | Notes |
|--------------|----------------------|-------|
| host         | node                 | "host" implies SSH only |
| project      | workspace            | workspace is core unit |
| worker       | agent                | agent is canonical |
| task         | queue item           | queue item is precise |
| permission   | approval             | "approval" is user-facing |
| acct         | account profile      | in docs; CLI can use acct |
| session pane | pane                 | keep it short |

## Voice and tone

- Prefer short, direct sentences.
- Include a one-line "next step" in error messages.
- Avoid jargon when a common word is available.
