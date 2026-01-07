---
id: swarm-z6e0
status: closed
deps: []
links: []
created: 2026-01-07T13:59:16.267650039+01:00
type: bug
priority: 1
---
# Fix OpenCode plugin import path

Update .opencode/plugin/swarm-mail.ts to import from @opencode-ai/plugin (current dependency) instead of opencode/plugin to avoid module resolution error.


