---
id: swarm-8bub
status: closed
deps: []
links: []
created: 2025-12-27T07:06:00.872729156+01:00
type: epic
priority: 1
---
# EPIC: Template and Sequence System

First-class support for message templates and command sequences.

## Problem
Without templates and sequences, users constantly copy/paste the same prompts and manually coordinate multi-step workflows.

## Solution
Add stored templates (single messages with variables) and sequences (ordered lists of messages, pauses, and conditions) as first-class CLI and TUI concepts.

## Core Concepts

### Templates
Single message with optional variable substitution
```yaml
# ~/.config/swarm/templates/continue.yaml
name: continue
description: Resume work on current task
message: |
  Continue working on the current task. If you are blocked,
  explain what you need to proceed.
```

### Sequences
Ordered list of operations (messages, pauses, conditions)
```yaml
# ~/.config/swarm/sequences/bugfix-loop.yaml
name: bugfix-loop
description: Standard bug fix workflow
steps:
  - type: message
    content: "Find and fix the bug described in issue {{issue_id}}"
  - type: pause
    duration: 30s
    reason: "Wait for initial analysis"
  - type: conditional
    when: idle
    message: "Run the tests and report results"
  - type: pause
    duration: 60s
  - type: message
    content: "Commit the fix with a descriptive message"
```

## CLI Commands
- `swarm template add|edit|list|show|run|delete`
- `swarm seq add|edit|list|show|run|delete`

## TUI Integration
- Message Palette (Ctrl+P) shows templates and sequences
- Quick selection → target agent(s) → enqueue

## Storage
- User templates: ~/.config/swarm/templates/
- User sequences: ~/.config/swarm/sequences/
- Project templates: .swarm/templates/
- Project sequences: .swarm/sequences/

## Success Criteria
- Common workflows are two keystrokes
- New users can use curated templates
- Power users can build custom sequences
- Templates work in both CLI and TUI


