---
id: swarm-5ipa.4
status: closed
deps: []
links: []
created: 2025-12-27T09:32:49.489877992+01:00
type: task
priority: 2
parent: swarm-5ipa
---
# Add Codex subscription auth via opencode-openai-codex-auth

Support Codex subscription-style auth instead of raw OpenAI API keys.

## From UX_FEEDBACK_2.md - Phase 3

## Background
The `opencode-openai-codex-auth` plugin enables OpenCode to use the Codex backend
using ChatGPT Plus/Pro OAuth authentication, consuming subscription instead of API credits.

## Deliverables

### 1. Plugin installation command
```bash
swarm opencode plugin install codex-auth
```
- Writes/patches OpenCode config to include the plugin
- Pins version (doesnt auto-update)

### 2. Account integration
```bash
swarm account add --provider openai-codex --profile my-codex
```
- Integrates with existing vault/profile machinery
- Copies auth blobs appropriately

### 3. Agent spawn integration
```bash
swarm agent spawn --type opencode --account my-codex
```
- Ensures correct profile is active before spawn
- Validates auth is valid

### 4. Doctor check
Add to `swarm doctor`:
- Port 1455 conflict warning (Codex CLI uses this port)
- Plugin version check
- Auth validity check

## Important Note
That plugin README warns about port 1455 conflict with official Codex CLI.
Must be in doctor output.


