# Forge

Forge runs looped AI coding agents per repository.

## Overview

Forge focuses on a simple loop runtime instead of tmux- or node-managed agents. Each loop:

- resolves a base prompt
- applies queued messages/overrides
- runs a harness command (pi, opencode, codex, claude)
- appends logs + ledger entries
- sleeps and repeats

Key features:

- **Loops**: background processes per repo
- **Profiles + Pools**: harness + auth homes with concurrency caps
- **Queue**: message, pause, stop, kill, next-prompt override
- **Logs + Ledgers**: logs centralized in the data dir, ledgers committed per repo
- **TUI**: 2x2 grid of active loops with log tails

## Core Concepts

| Concept | Description |
|---------|-------------|
| **Loop** | Background process that repeatedly runs a harness against a prompt |
| **Profile** | Harness + auth config (pi/opencode/codex/claude) |
| **Pool** | Ordered list of profiles used for selection |
| **Prompt** | Base prompt content or file used each iteration |
| **Queue** | Per-loop queue of messages and control items |
| **Ledger** | Markdown log of loop iterations stored in the repo |

## Install

Linux (x86_64/arm64):

```bash
curl -fsSL https://raw.githubusercontent.com/tOgg1/forge/main/scripts/install-linux.sh | bash
```

Optional overrides:

```bash
FORGE_VERSION=v0.0.0 FORGE_INSTALL_DIR="$HOME/.local/bin" FORGE_BINARIES="forge" \
  bash -c 'curl -fsSL https://raw.githubusercontent.com/tOgg1/forge/main/scripts/install-linux.sh | bash'
```

Homebrew (macOS) can be wired once a tap repo is chosen.

## Quick Start

```bash
# 1) Initialize repo scaffolding
forge init

# 2) Import aliases (or add a profile manually)
# Scans common shell alias files and detects installed harnesses (claude/codex/opencode/pi/droid).
forge profile init
# forge profile add pi --name local

# 3) Create a pool and add profiles
forge pool create default
forge pool add default oc1
forge pool set-default default

# 4) Start a loop
forge up --count 1

# 5) Send a message and watch logs
forge msg <loop-name> "Review the PRD and summarize next steps"
forge logs <loop-name> -f

# 6) Launch the TUI
forge
```

## Repo Layout

Forge keeps committed repo state in `.forge/`:

```
.forge/
  forge.yaml
  prompts/
  templates/
  sequences/
  ledgers/
```

Runtime data (sqlite, logs, pids) stays in the machine-local data dir.

## Configuration

Global config lives at `~/.config/forge/config.yaml`. Repo config lives at `.forge/forge.yaml`.
See `docs/config.md` for details.

## Agent Skills

Install repo skills for configured harnesses with:

```bash
forge skills bootstrap
```

If a profile has no `auth_home`, skills are installed into repo-local harness
folders (for example `.codex/skills/`). When `.agent-skills/` is missing, the
CLI falls back to the embedded skills.

Install skills into harness-specific locations with:

```bash
scripts/install-skills.sh
```

See `docs/skills.md` for details.

## CLI Reference

See `docs/cli.md` for the full CLI surface.
