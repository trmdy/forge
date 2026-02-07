# sv <-> forge integration (proposal)

Goal
- sv owns tasks
- forge loop just needs "current task pointer" + status for prompt injection + dashboards
- forge stays task-tech-agnostic

Forge primitive (implemented)
- `forge work set <task-id> --status <status> --detail "<txt>" [--loop <loop-ref>] [--agent <agent-id>]`
  - stores: `agent_id`, `task_id`, `status`, `updated_at`, `loop_iteration`
  - loop ref default: `FORGE_LOOP_ID` (when called inside a loop run)
  - agent id default: `FMAIL_AGENT` or `SV_ACTOR` or `FORGE_LOOP_NAME`
- `forge work current|ls|clear`

Env injection (implemented in forge loop runs)
- `SV_REPO=<repoPath>`
- `SV_ACTOR=<loop-name>` (defaults to `FMAIL_AGENT`)

sv hook shape (desired)
- sv emits task lifecycle events already (`--events` JSONL)
- add a "hook runner" feature: on task events, run commands

Minimal config idea (`.sv.toml`)
```toml
[integrations.forge]
enabled = true

# how to select forge loop for this repo/actor
loop_ref = "{actor}"       # common: forge loop name == sv actor
# or: loop_ref = "review-loop"

[integrations.forge.on_task_start]
cmd = "forge work set {task_id} --status in_progress --loop {loop_ref} --agent {actor}"

[integrations.forge.on_task_block]
cmd = "forge work set {task_id} --status blocked --loop {loop_ref} --agent {actor}"

[integrations.forge.on_task_close]
cmd = "forge work clear --loop {loop_ref}"
```

sv UX command idea
- `sv forge hooks install`
  - writes the above `[integrations.forge]` block to `.sv.toml` (or `.sv/overrides/...`)
  - defaults:
    - `loop_ref="{actor}"`
    - `--agent` = `{actor}`
  - optional flags:
    - `--loop <loop-ref>`
    - `--status-map open=in_progress,blocked=blocked,closed=done`

Notes
- keep forge agnostic: task ids are opaque strings; no sv-specific parsing
- hook should be best-effort (don’t block sv task ops if forge missing)

