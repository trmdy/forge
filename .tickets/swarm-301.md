---
id: swarm-301
status: closed
deps: []
links: []
created: 2025-12-21T22:24:09.279758857+01:00
type: task
priority: 1
---
# Handle poll failures gracefully

When poll fails (SSH timeout, etc.), preserve last known state. Mark as stale. Retry with backoff.


