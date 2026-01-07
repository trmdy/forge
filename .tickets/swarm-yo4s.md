---
id: swarm-yo4s
status: closed
deps: []
links: []
created: 2025-12-27T07:07:55.055011413+01:00
type: epic
priority: 1
---
# EPIC: TUI Enhancements - Elite Swarm Experience

Transform the TUI from functional to "elite" with message palette, multi-select, queue timeline, and launchpad wizard.

## Current State
- Command palette works
- Queue editor exists (demo-bound)
- Mailbox view exists
- Approval toggles work
- Inspector panel works

## Target State
"Best-in-class" TUI that makes managing 20+ agents feel effortless.

## Key Features

### 1. Message Palette (Ctrl+P)
Separate from command palette (Ctrl+K), specifically for templates and sequences.
- Lists templates and sequences
- Prompts for target agent(s)
- Prompts for variables
- Offers enqueue mode selection (end, front, after cooldown, when idle)

### 2. Multi-Select + Bulk Actions
Essential for real swarm management.
- Space: Toggle selection on agent card
- Shift+Space: Select range
- Bulk actions: Pause, Resume, Send template, Queue sequence

### 3. Queue Editor Timeline
Transform list editing into timeline semantics.
- Item type icons (message, pause, conditional)
- Gating reason display
- Cooldown countdown
- Dispatch attempts/errors
- Quick reorder with J/K

### 4. Launchpad Wizard
Dedicated modal for spawn + task flow.
- Choose workspace (or create)
- Choose agent type
- Choose count
- Choose account rotation mode
- Choose initial sequence
- Single-action spawn + enqueue

## Success Criteria
- Common operations are 2-3 keystrokes
- Agent selection scales to 50+ agents
- Queue management is visual and intuitive
- New agent creation is guided and fast


