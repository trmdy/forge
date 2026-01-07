---
id: swarm-ns0x.7
status: closed
deps: []
links: []
created: 2026-01-06T20:01:23.404684466+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Implement loop CLI surface (up/ps/logs/stop/kill/msg/scale/run/queue/prompt)

Implement the simplified loop-centric CLI per PRD section 6 and clarified behaviors.

Scope:
- Commands: init, up, ps, logs (+log alias), stop, kill, msg, scale, prompt, queue, run, doctor, tui.
- Global flags: -C/--chdir, --json/--jsonl, --no-color, --quiet.
- `forge msg` supports --now, --next-prompt, --template, --seq, selectors by loop/pool/state/tag.
- `forge prompt` manages .forge/prompts (add/edit/ls/set-default).
- `forge queue` manages loop queue (ls/clear/rm/move).

Acceptance:
- CLI matches PRD semantics with JSON output where applicable.
- Prompt precedence enforced and `--prompt-msg` supported.
- `forge logs` tails centralized logs and supports --follow/--since/--lines.



