---
id: swarm-8egp.3
status: closed
deps: [swarm-8egp.1]
links: []
created: 2025-12-22T12:25:33.827885096+01:00
type: task
priority: 1
parent: swarm-8egp
---
# Agent inspector: bind to selection + search jump

Goal: inspector always mirrors selected agent and search jump. Requirements: inspector reads only from current selection (no most-recent fallback); Enter in search sets selection to first match and updates inspector; if no match, show clear empty-state message. Acceptance: inspector content always matches focused card and search jump feedback is obvious.


