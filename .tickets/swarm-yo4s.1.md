---
id: swarm-yo4s.1
status: closed
deps: []
links: []
created: 2025-12-27T07:08:14.991483651+01:00
type: task
priority: 1
parent: swarm-yo4s
---
# Implement Message Palette (Ctrl+P)

Add a dedicated message palette separate from the command palette.

## Trigger
- Ctrl+P: Open message palette
- Ctrl+K: Command palette (existing, unchanged)

## Message Palette UI
```
┌─────────────────────────────────────────────────┐
│ 󰍉 Message Palette                    [Ctrl+P]  │
├─────────────────────────────────────────────────┤
│ > Search templates and sequences...             │
├─────────────────────────────────────────────────┤
│ TEMPLATES                                       │
│   continue       Resume current task            │
│   commit         Commit changes                 │
│   explain        Ask agent to explain           │
│   test           Run tests                      │
│                                                 │
│ SEQUENCES                                       │
│   bugfix         Find → Fix → Test → Commit     │
│   feature        Implement feature workflow     │
│   review-loop    Review → Address → Re-review   │
└─────────────────────────────────────────────────┘
```

## Selection Flow
1. Select template or sequence
2. If selected agent(s): use those as target
3. If no selection: show agent picker modal
4. If template has variables: show variable input modal
5. Show enqueue mode picker:
   - End of queue (default)
   - Front of queue
   - After cooldown
   - When idle
6. Confirm and enqueue

## Implementation
- internal/tui/components/message_palette.go
- Integrate with templates and sequences packages
- Use Bubble Tea modal pattern
- Share styling with command palette

## Hotkeys in palette
- Enter: Select and proceed
- Tab: Cycle sections (templates → sequences)
- Esc: Close palette
- /: Focus search


