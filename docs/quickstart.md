# Forge Quickstart

This guide walks through the loop-first workflow.

## Prerequisites

- Go 1.25+ (see `go.mod`)
- Git
- A supported harness: `pi`, `opencode`, `codex`, or `claude`

## Build

```bash
make build
```

Binaries are written to `./build/forge`.

## Initialize a repo

From the repo you want to run loops in:

```bash
./build/forge init
```

This creates `.forge/` scaffolding and a `PROMPT.md` if missing.

## Configure profiles

Import aliases from `~/.zsh_aliases` (or `FORGE_ALIAS_FILE`):

```bash
./build/forge profile import-aliases
```

Or add one manually:

```bash
./build/forge profile add pi --name local
```

If you want separate Pi config directories per profile:

```bash
PI_CODING_AGENT_DIR="$HOME/.pi/agent-work" pi
```

Forge sets `PI_CODING_AGENT_DIR` from `profile.auth_home` automatically.

## Create a pool

```bash
./build/forge pool create default
./build/forge pool add default oc1
./build/forge pool set-default default
```

## Start loops

```bash
./build/forge up --count 1
./build/forge ps
```

## Send messages and watch logs

```bash
./build/forge msg <loop-name> "Summarize the open tasks"
./build/forge logs <loop-name> -f
```

## Launch the TUI

```bash
./build/forge
# or
./build/forge tui
```

## Troubleshooting

See `docs/troubleshooting.md` for common fixes.
