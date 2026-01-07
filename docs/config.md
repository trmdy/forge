# Forge Configuration Reference

Forge loads configuration in this precedence order:

1. Defaults (built into the binary)
2. Config file
3. Environment variables
4. CLI flags

## Config file locations

Forge looks for `config.yaml` in:

- `$XDG_CONFIG_HOME/forge/config.yaml`
- `~/.config/forge/config.yaml`
- `./config.yaml` (current directory)

You can also pass an explicit path with `--config`.

## Environment variable overrides

Environment variables use the prefix `FORGE_` and replace dots with underscores.
Legacy `SWARM_` variables are still accepted as deprecated aliases.

Examples:

- `logging.level` -> `FORGE_LOGGING_LEVEL`
- `database.max_connections` -> `FORGE_DATABASE_MAX_CONNECTIONS`

## Duration format

Duration fields use Go duration strings, for example: `250ms`, `2s`, `5m`, `1h`.

## Global config (machine-local)

### global

- `global.data_dir` (string): Data directory (database + logs). Default: `~/.local/share/forge`.
- `global.config_dir` (string): Config directory. Default: `~/.config/forge`.

### database

- `database.path` (string): SQLite database file path. Default: empty (uses `{data_dir}/forge.db`).
- `database.max_connections` (int): Maximum DB connections. Default: `10`.
- `database.busy_timeout_ms` (int): SQLite busy timeout in milliseconds. Default: `5000`.

### logging

- `logging.level` (string): `debug`, `info`, `warn`, `error`. Default: `info`.
- `logging.format` (string): `json` or `console`. Default: `console`.
- `logging.file` (string): Optional log file path. Default: empty.
- `logging.enable_caller` (bool): Include caller info in logs. Default: `false`.

### profiles

Profiles define harness + auth homes (machine-local).

- `profiles[].name` (string): Profile name (unique).
- `profiles[].harness` (string): `pi`, `opencode`, `codex`, `claude`.
- `profiles[].auth_kind` (string): Optional auth kind label (e.g., `claude`, `codex`).
- `profiles[].auth_home` (string): Harness-specific auth/config directory (e.g., `~/.pi/agent-work`).
- `profiles[].prompt_mode` (string): `env`, `stdin`, or `path`.
- `profiles[].command_template` (string): Command template (supports `{prompt}` substitution).
- `profiles[].model` (string): Optional model default.
- `profiles[].extra_args` (list): Extra CLI args.
- `profiles[].env` (map): Environment overrides.
- `profiles[].max_concurrency` (int): Max concurrent runs for this profile.

### pools

Pools are ordered lists of profile references.

- `pools[].name` (string): Pool name (unique).
- `pools[].strategy` (string): `round_robin` (default).
- `pools[].profiles` (list): Profile names in this pool.
- `pools[].weights` (map): Optional weight per profile name.
- `pools[].is_default` (bool): Mark as default pool.

### default_pool

- `default_pool` (string): Default pool name to use when loops are not pinned.

### loop_defaults

- `loop_defaults.interval` (duration): Sleep between iterations. Default: `30s`.
- `loop_defaults.prompt` (string): Default prompt path or name (optional).
- `loop_defaults.prompt_msg` (string): Default base prompt message (optional).

### tui

- `tui.refresh_interval` (duration): UI refresh rate. Default: `2s`.

## Repo config (`.forge/forge.yaml`)

Repo config is committed and describes loop defaults and shared assets.

```yaml
# Forge loop config
# This file is committed with the repo.

default_prompt: PROMPT.md

ledger:
  git_status: false
  git_diff_stat: false
```

Fields:

- `default_prompt`: Default prompt path for this repo.
- `ledger.git_status`: Include `git status --porcelain` summary per ledger entry.
- `ledger.git_diff_stat`: Include `git diff --stat` summary per ledger entry.
