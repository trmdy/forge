# Agent Instructions

Forge is a control plane for running and supervising AI coding agents across
repos and servers, using SSH + tmux with an optional per-node daemon and a
SQLite-backed event log.

This repo uses Agent Skills for common workflows. The canonical instructions
live in `.agent-skills/` and are installed to harnesses via
`scripts/install-skills.sh` (see `docs/skills.md`).

## Skills Index

- `issue-tracking`: `tk` workflow and ticket expectations.
- `agent-communication`: `fmail` usage and naming conventions.
- `session-protocol`: end-of-session git checklist.
- `workflow-pattern`: status updates, priorities, and workflow guidance.

If you need a specific detail, open the relevant skill in `.agent-skills/`.
