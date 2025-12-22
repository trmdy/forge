# ADR 0001: OpenCode Deployment Model

## Status

Accepted

## Context

Swarm needs a native integration tier for OpenCode to support reliable agent
control and telemetry. There are two viable deployment patterns:

- One OpenCode server per agent/profile (isolated, per-account)
- One OpenCode server per workspace (shared)

The system must support multiple accounts and frequent profile rotation.

## Decision

Use one OpenCode server per agent/profile by default.

## Consequences

- Pros: strong isolation of credentials, straightforward account rotation,
  reduced cross-agent interference.
- Cons: higher process count and resource usage per agent.

## Alternatives considered

- One OpenCode server per workspace: simpler topology and fewer processes, but
  makes per-agent account isolation and rotation harder.
