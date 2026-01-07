---
id: swarm-yo4s.3
status: closed
deps: []
links: []
created: 2025-12-27T07:09:00.225385952+01:00
type: task
priority: 1
parent: swarm-yo4s
---
# Enhance queue editor with timeline semantics

Transform the queue editor from simple list editing to timeline-aware interface.

## Current State
Basic list editing for queue items.

## Target State
Rich timeline view with:
- Item type icons
- Status indicators
- Blocking reasons
- Countdown timers
- Dispatch history

## Queue Timeline View
```
┌────────────────────────────────────────────────────────────┐
│ Queue for agent abc123 (5 items)              [q] close   │
├────────────────────────────────────────────────────────────┤
│ POS │ TYPE       │ STATUS    │ CONTENT                    │
├────────────────────────────────────────────────────────────┤
│  1  │ 󰍉 message  │ 󰋙 blocked │ "Fix the lint..." (52c)   │
│     │            │ busy      │ dispatch in ~2m            │
├────────────────────────────────────────────────────────────┤
│  2  │ 󱎫 pause    │ 󰏤 pending │ 60s pause                  │
│     │            │           │                            │
├────────────────────────────────────────────────────────────┤
│  3  │ 󰔡 cond     │ 󰏤 pending │ when idle: "Continue..."   │
│     │            │ idle_gate │                            │
├────────────────────────────────────────────────────────────┤
│  4  │ 󰍉 message  │ 󰏤 pending │ "Then do this..." (89c)   │
├────────────────────────────────────────────────────────────┤
│  5  │ 󰍉 message  │ 󰏤 pending │ "Finally..." (45c)        │
└────────────────────────────────────────────────────────────┘
│ [j/k] move cursor  [J/K] reorder  [i] insert  [d] delete │
│ [p] pause  [g] gate  [t] template  [Enter] edit          │
└────────────────────────────────────────────────────────────┘
```

## Hotkeys
- j/k: Move cursor up/down
- J/K: Move selected item up/down (reorder)
- i: Insert message at cursor
- p: Insert pause at cursor
- g: Insert "when idle" gate
- t: Insert template (opens message palette)
- d: Delete item (with confirmation)
- Enter: Edit selected item
- e: Expand item to show full content
- r: Retry failed dispatch
- c: Copy item

## Status Icons
- 󰏤 pending: Waiting in queue
- 󰋙 blocked: Cannot dispatch (show reason)
- 󰓦 dispatched: Sent to agent
- 󰄬 completed: Successfully processed
- 󰅚 failed: Dispatch failed (show error)

## Block Reasons (shown inline)
- busy: Agent is working
- paused: Agent is paused
- cooldown: Account on cooldown (with countdown)
- idle_gate: Conditional waiting for idle
- dependency: Waiting on prior item


