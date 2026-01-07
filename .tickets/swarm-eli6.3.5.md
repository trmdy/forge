---
id: swarm-eli6.3.5
status: closed
deps: [swarm-eli6.1.2, swarm-h4jd]
links: [swarm-0qgb]
created: 2025-12-22T10:17:12.905606892+01:00
type: task
priority: 1
parent: swarm-eli6.3
---
# Add fleet summary command (`swarm status`)

Add swarm status (or swarm overview) that shows fleet summary: node counts, workspace counts, agent state breakdown, and top alerts (approval needed, errors, cooldowns). Should reuse the same underlying data model as swarm export status to avoid duplication. Support --watch with JSONL for dashboards.

## Acceptance Criteria

- summary command outputs consistent tables and JSON
- watch mode streams updates until Ctrl+C
- top alerts limited to a small, scannable list


