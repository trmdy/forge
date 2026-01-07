---
id: swarm-ns0x.11
status: closed
deps: []
links: []
created: 2026-01-06T20:02:09.160035983+01:00
type: task
priority: 2
parent: swarm-ns0x
---
# Implement prompt registry + defaults

Implement prompt registry management per PRD section 6.3 and clarified precedence.

Scope:
- `.forge/prompts/` name resolution for `forge prompt` and `--prompt` by name.
- Default prompt selection: --prompt > PROMPT.md > .forge/prompts/default.md.
- `--prompt-msg` base prompt text for loop.

Acceptance:
- Prompt name resolution works with add/edit/ls/set-default.
- Precedence rules are tested and documented.



