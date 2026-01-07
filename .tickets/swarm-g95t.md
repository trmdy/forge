---
id: swarm-g95t
status: closed
deps: []
links: []
created: 2025-12-27T08:52:32.044964121+01:00
type: epic
priority: 1
---
# EPIC: Remote Node Architecture Fixes

Wire up the existing node/daemon abstraction for true remote execution.

## Problem (from UX_FEEDBACK_1.md)
Your workspace and agent services currently use a local tmux client, so they
dont leverage the node/daemon abstraction youve already built. Remote is
"designed" but not truly "end-to-end."

## Current State
- `internal/node/client.go` is good: supports daemon mode (SSH-tunneled gRPC) and SSH fallback
- `workspace.Service` creates tmux sessions using a factory that cant vary per node
- `agent.Service` is instantiated with `tmux.NewLocalClient()` in CLI commands

## Net Effect
You can register nodes, but actual agent/workspace lifecycle is still "local-first."

## Solution
1. Add node-aware tmux client factory
2. Update workspace.Service to use per-node tmux clients
3. Update agent.Service to use per-node execution
4. Route commands through swarmd when in daemon mode

## Success Criteria
- `swarm agent spawn --node remote-1` actually runs on remote-1
- `swarm ws create --path /remote/path --node remote-1` works
- Daemon mode provides better performance than SSH fallback


