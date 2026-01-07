---
id: swarm-yo4s.2
status: closed
deps: []
links: []
created: 2025-12-27T07:08:34.099518208+01:00
type: task
priority: 1
parent: swarm-yo4s
---
# Implement multi-select and bulk actions

Add multi-selection support for agent cards with bulk action capabilities.

## Selection Hotkeys
- Space: Toggle selection on focused agent
- Shift+Space: Select range (from last selected to current)
- Ctrl+A: Select all agents in view
- Ctrl+Shift+A: Deselect all
- Esc: Clear selection (when in selection mode)

## Visual Feedback
- Selected agents: Highlighted border + checkbox icon
- Selection count in status bar: "3 agents selected"
- Selection mode indicator

## Bulk Actions (when agents selected)
- P: Pause all selected (with duration prompt)
- R: Resume all selected
- T: Open message palette targeting selected
- Q: Open queue editor in bulk mode
- K: Kill/terminate all selected (with confirmation)
- I: Interrupt all selected
- S: Send same message to all (opens input)

## Bulk Action UI
```
┌─────────────────────────────────────────────────┐
│ 󰄬 Bulk Action: 3 agents selected               │
├─────────────────────────────────────────────────┤
│ [P] Pause     [R] Resume    [T] Template       │
│ [Q] Queue     [K] Kill      [I] Interrupt      │
│ [S] Send message to all                        │
│                                                 │
│ [Esc] Cancel  [Space] Toggle selection         │
└─────────────────────────────────────────────────┘
```

## Implementation
- internal/tui/components/agent_list.go - Add selection state
- internal/tui/components/bulk_actions.go - Action bar
- Track selection as []string of agent IDs
- Batch operations in agent service


