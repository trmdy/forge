# Forge CLI Reference

This document describes the loop-centric CLI surface and the emerging workflow/job layout.

## Global usage

```bash
forge [flags] [command]
```

### Global flags

- `--config <path>`: Path to config file (default: `~/.config/forge/config.yaml`).
- `--json`: Emit JSON output (where supported).
- `--jsonl`: Emit JSON Lines output.
- `--quiet`: Suppress non-essential output.
- `-C, --chdir <path>`: Run in a specific repo directory.
- `--non-interactive`: Disable prompts and use defaults.
- `-v, --verbose`: Enable debug logging.
- `--log-level <level>`: Override logging level (`debug`, `info`, `warn`, `error`).
- `--log-format <format>`: Override logging format (`json`, `console`).

## CLI structure (proposed, incremental)

Keep backward compatibility. Existing top-level loop commands stay, but map to `forge loop ...`.

Canonical groups:

- `forge loop ...` (aliases: `forge up/ps/msg/stop/...`)
- `forge workflow ...`
- `forge job ...`
- `forge trigger ...`
- `forge node ...`
- `forge mesh ...`

Alias mapping:

- `forge up` -> `forge loop up`
- `forge ps` -> `forge loop ps`
- `forge msg` -> `forge loop msg`
- `forge logs` -> `forge loop logs`
- `forge stop` -> `forge loop stop`
- `forge kill` -> `forge loop kill`
- `forge resume` -> `forge loop resume`
- `forge rm` -> `forge loop rm`
- `forge prune` -> `forge loop prune`
- `forge scale` -> `forge loop scale`
- `forge queue` -> `forge loop queue`
- `forge run` -> `forge loop run`

## Core commands

### `forge` / `forge tui`

Launch the loop TUI.

```bash
forge
forge tui
```

### `forge init`

Initialize `.forge/` scaffolding and optional `PROMPT.md`.

```bash
forge init
forge init --prompts-from ./prompts
forge init --no-create-prompt
```

### `forge config`

Manage global configuration at `~/.config/forge/config.yaml`.

```bash
forge config init          # Create default config with comments
forge config init --force  # Overwrite existing config
forge config path          # Print config file path
```

### `forge loop up` (alias: `forge up`)

Start loop(s) in the current repo.

```bash
forge up --count 1
forge up --name review-loop --prompt review
forge up --pool default --interval 30s --tags review
forge up --max-iterations 10 --max-runtime 2h
```

### `forge loop ps` (alias: `forge ps`)

List loops.

```bash
forge ps
forge ps --state running
forge ps --pool default
```

### `forge loop logs` (alias: `forge logs`)

Tail loop logs.

```bash
forge logs review-loop
forge logs review-loop -f
forge logs --all
```

### `forge loop msg` (alias: `forge msg`)

Queue a message or override for a loop.

```bash
forge msg review-loop "Focus on the PRD changes"
forge msg review-loop --next-prompt ./prompts/review.md
forge msg --pool default --now "Interrupt and refocus"
forge msg review-loop --template stop-and-refocus --var reason=scope
forge msg review-loop --seq review-seq --var mode=fast
```

### `forge loop stop` / `forge loop kill` (aliases: `forge stop` / `forge kill`)

Stop or kill loops.

```bash
forge stop review-loop
forge kill review-loop
forge stop --pool default
```

### `forge loop resume` (alias: `forge resume`)

Resume a stopped or errored loop.

```bash
forge resume review-loop
```

### `forge loop rm` (alias: `forge rm`)

Remove loop records (DB only). Logs and ledgers remain on disk. Use `--force` for selectors or running loops.

```bash
forge rm review-loop
forge rm --state stopped --force
forge rm --all --force
```

### `forge loop prune` (alias: `forge prune`)

Remove inactive loop records (stopped or errored). Logs and ledgers remain on disk.

```bash
forge prune
forge prune --repo .
forge prune --pool default
```

### `forge loop scale` (alias: `forge scale`)

Scale loops to a target count.

```bash
forge scale --count 3 --pool default
forge scale --count 0 --kill
forge scale --count 2 --max-iterations 5 --max-runtime 1h
```

### `forge loop queue` (alias: `forge queue`)

Inspect or reorder the loop queue.

```bash
forge queue ls review-loop
forge queue clear review-loop
forge queue rm review-loop <item-id>
forge queue move review-loop <item-id> --to front
```

### `forge loop run` (alias: `forge run`)

Run a single iteration for a loop.

```bash
forge run review-loop
```

## Prompt and template helpers

### `forge prompt`

Manage `.forge/prompts/`.

```bash
forge prompt ls
forge prompt add review ./prompts/review.md
forge prompt edit review
forge prompt set-default review
```

### `forge template`

Manage `.forge/templates/`.

```bash
forge template ls
forge template add review ./templates/review.md
forge template edit review
```

### `forge seq`

Manage `.forge/sequences/`.

```bash
forge seq ls
forge seq show review-seq
forge seq add review-seq ./sequences/review.seq.yaml
```

## Profiles and pools

### `forge profile`

Manage harness profiles.

```bash
forge profile ls
forge profile init
forge profile add pi --name local
forge profile edit local --max-concurrency 2
forge profile cooldown set local --until 30m
forge profile rm local
```

### `forge pool`

Manage profile pools.

```bash
forge pool ls
forge pool create default
forge pool add default oc1 oc2
forge pool set-default default
forge pool show default
```

## Workflow and job commands (planned)

### `forge workflow`

Run and inspect workflows (DAG of steps).

```bash
forge workflow ls
forge workflow show <name>
forge workflow validate <name>
forge workflow run <name> --input repo=.
forge workflow graph <name> --format dot
```

### `forge job`

Run higher-level jobs that can start workflows or dispatch work.

```bash
forge job ls
forge job show <name>
forge job run <name> --input repo=.
forge job logs <job-id>
forge job cancel <job-id>
```

### `forge trigger`

Attach triggers to jobs (cron or webhook).

```bash
forge trigger ls
forge trigger add cron:0 2 * * * --job nightly-qa
forge trigger add webhook:/hooks/ship --job spec-to-ship
forge trigger rm <trigger-id>
```

### `forge node`

Manage nodes in the mesh.

```bash
forge node ls
forge node add --ssh user@host --name <node>
forge node bootstrap --ssh root@host
forge node exec <node> -- <cmd>
forge node doctor <node>
```

### `forge mesh`

Inspect or change mesh master.

```bash
forge mesh status
forge mesh promote <node>
forge mesh demote <node>
forge mesh join <mesh-id>
forge mesh leave
```
