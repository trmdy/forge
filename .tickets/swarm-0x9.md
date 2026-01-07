---
id: swarm-0x9
status: closed
deps: [swarm-njf]
links: []
created: 2025-12-21T22:24:33.688650833+01:00
type: task
priority: 0
---
# Check agent state before dispatch

Before sending queue item, verify agent is idle. Skip if busy, retry later.


