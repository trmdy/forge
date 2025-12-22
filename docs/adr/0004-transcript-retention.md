# ADR 0004: Transcript Retention

## Status

Accepted

## Context

Swarm captures agent transcripts for state detection, debugging, and audit.
Retention affects disk usage and privacy. The MVP needs a pragmatic default.

## Decision

Store transcripts locally and keep a rolling buffer per agent, with optional
persistence in the database. The initial default is a finite buffer size (see
`agent_defaults.transcript_buffer_size`), and long-term retention is left to
future configuration.

## Consequences

- Pros: bounded disk usage, easy debugging within recent history.
- Cons: older transcripts may be dropped; long-term audit requires exports.

## Alternatives considered

- Full retention: useful for audit but can grow quickly and requires cleanup.
- No retention: minimal storage but makes debugging and state reasoning harder.
