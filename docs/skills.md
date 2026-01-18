# Agent Skills

Forge keeps harness-agnostic skills in `.agent-skills/` following the Agent Skills
folder format. You can install the built-in skills to harnesses with the CLI.

## Bootstrap skills into a repo

```bash
forge skills bootstrap
```

This installs the built-in skill set into harness-specific locations based on
the configured Forge profiles. When a profile does not specify `auth_home`,
skills are installed into repo-local harness folders (for example
`.codex/skills/`). Use `--force` to overwrite existing files, or `--path` to
point at a custom skills source directory.

## Install to configured harnesses

```bash
scripts/install-skills.sh
```

The script reads `~/.config/forge/config.yaml` (or `--config`) and installs the
skills into the harness-specific locations based on the configured profiles.

Options:

- `--config PATH`: explicit Forge config path.
- `--source DIR`: override the skills source directory (default: `.agent-skills`).
- `--dry-run`: show the install plan without writing files.

## Harness mapping defaults

When a profile has `auth_home`, the installer writes to `<auth_home>/skills`.
If `auth_home` is empty, it uses the defaults below:

- `codex` -> `~/.codex/skills`
- `claude` / `claude_code` -> `~/.claude/skills`
- `opencode` -> `~/.config/opencode/skills`
- `pi` -> `~/.pi/skills`
