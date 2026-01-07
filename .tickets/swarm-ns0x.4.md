---
id: swarm-ns0x.4
status: closed
deps: []
links: []
created: 2026-01-06T20:00:47.963876621+01:00
type: task
priority: 1
parent: swarm-ns0x
---
# Implement loop runtime engine (iteration lifecycle)

Build the core loop runner per PRD sections 4.1, 5, and 10.

Scope:
- Background loop process with PID/stop/kill semantics (Ralph parity).
- Iteration lifecycle: stop-check, prompt load, queue apply, profile selection, harness run, log/ledger append, sleep.
- Base prompt precedence: --prompt > PROMPT.md > .forge/prompts/default.md.
- `--prompt-msg` sets base prompt for the loop (all iterations).
- On interrupt (`forge msg --now`), kill current run and immediately restart with ledger + log tail included.

Acceptance:
- Loop runs continuously and respects graceful stop + kill.
- Queue items are applied correctly (override-once consumes once).
- Profile selection handles pool availability and errors for pinned profile on cooldown/cap.



