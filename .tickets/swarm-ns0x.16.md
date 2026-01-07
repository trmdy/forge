---
id: swarm-ns0x.16
status: closed
deps: []
links: []
created: 2026-01-06T20:02:57.956280039+01:00
type: task
priority: 2
parent: swarm-ns0x
---
# Add tests for loop engine + CLI

Add unit/integration tests for the loop runtime and CLI.

Scope:
- Loop iteration lifecycle tests (stop/kill, queue application, prompt precedence).
- Profile/pool selection and cooldown enforcement.
- CLI smoke tests for `forge init`, `forge up`, `forge msg`, `forge logs`.

Acceptance:
- Tests cover critical loop behaviors and regressions.
- CI can run without external harness binaries (use stubs/mocks).



