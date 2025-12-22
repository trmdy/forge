# ADR 0002: Control Plane Topology

## Status

Accepted

## Context

Swarm must operate both locally and across remote nodes. The product spec
outlines two modes:

- SSH-only control plane (no per-node daemon)
- swarmd per-node daemon with a structured event stream

The MVP needs to ship quickly while keeping interfaces compatible with future
swarmd adoption.

## Decision

Start with SSH-only control plane for MVP, and keep the interfaces daemon-ready
for a future swarmd integration.

## Consequences

- Pros: faster delivery, minimal footprint on nodes, easier setup.
- Cons: polling-based state detection and lower real-time fidelity.

## Alternatives considered

- Deploy swarmd from day one: better performance and telemetry, but higher
  implementation and operational complexity for MVP.
