# ADR 0003: Approval Policy Defaults

## Status

Accepted

## Context

Agents may request approvals for actions with risk (file writes, command exec,
network operations). The system must choose a default approval policy that is
safe for new users while allowing future configuration.

## Decision

Default approval policy is "strict". All approval requests are surfaced and
require explicit user approval. Configuration can override this per workspace
or agent in the future.

## Consequences

- Pros: safer defaults, less risk of unintended actions.
- Cons: more user prompts, lower automation throughput.

## Alternatives considered

- Permissive by default: faster automation but higher risk of unintended
  changes, especially for new users.
