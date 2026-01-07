---
id: swarm-ns0x.5
status: closed
deps: []
links: []
created: 2026-01-06T20:01:00.393562438+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Add harness abstraction + defaults (pi/opencode/codex/claude)

Implement harness abstraction with command templates and prompt delivery modes (PRD sections 3.3, 11).

Scope:
- Harness interface supports {prompt} substitution, env var, and stdin modes.
- Default commands:
  - pi: `pi -p "{prompt}"` (uses PI_CODING_AGENT_DIR).
  - claude: `claude -p "$FORGE_PROMPT_CONTENT" --dangerously-skip-permissions`.
  - codex: `codex exec --full-auto -` (stdin).
  - opencode: `opencode run --model <model> "$FORGE_PROMPT_CONTENT"`.
- Profile-specific env overrides and extra args.

Acceptance:
- Profiles can override command template, prompt delivery mode, and env vars.
- FORGE_PROMPT_CONTENT is set when using env mode.
- pi profiles inject PI_CODING_AGENT_DIR from profile auth_home.



