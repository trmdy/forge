---
id: f-0fd1
status: closed
deps: []
links: []
created: 2026-01-10T20:06:26Z
type: task
priority: 1
assignee: Tormod Haugland
parent: f-c2d0
---
# Scaffold fmail CLI (cmd/fmail)

Create the new standalone fmail CLI binary described in docs/forge-mail.

Scope:
- Add cmd/fmail/main.go entrypoint
- Establish command structure + help output for: send, log, watch, who, status, topics, gc, init
- Establish shared plumbing for:
  - locating project root
  - resolving agent identity
  - consistent error/exit code handling

Notes:
- Repo already uses Cobra for forge; spec suggests urfave/cli. Decide what to use and keep fmail independent from forge config.

## Acceptance Criteria

- `go run ./cmd/fmail --help` shows the expected commands and usage (even if some are stubbed)
- fmail does not require forge config files; it relies only on env vars and the filesystem
- CLI skeleton is ready for standalone mode implementation (core package can be added next)


## Notes

**2026-01-11T06:49:58Z**

Scaffolded cmd/fmail and internal/fmail skeleton; go run blocked by network (cobra download) in sandbox.
