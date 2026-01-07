---
id: swarm-yo4s.4
status: closed
deps: []
links: []
created: 2025-12-27T07:09:26.726495457+01:00
type: task
priority: 1
parent: swarm-yo4s
---
# Implement Launchpad wizard for agent spawning

Create a guided modal wizard for spawning and tasking agents in one flow.

## Trigger
- N: Open launchpad (from main view)
- Command palette: "Spawn agents..."

## Wizard Steps

### Step 1: Workspace Selection
```
┌─────────────────────────────────────────────────┐
│ 󰕍 Launchpad: Spawn Agents (Step 1/5)           │
├─────────────────────────────────────────────────┤
│ Select or create workspace:                     │
│                                                 │
│ ● my-project      /home/user/my-project        │
│ ○ api-server      /home/user/api-server        │
│ ○ frontend        /home/user/frontend          │
│ ─────────────────────────────────               │
│ ○ [Create new from current directory]          │
│ ○ [Create new from path...]                    │
│                                                 │
│ [Enter] Continue  [Esc] Cancel  [Tab] Options  │
└─────────────────────────────────────────────────┘
```

### Step 2: Agent Type
```
│ Select agent type:                              │
│                                                 │
│ ● OpenCode       Fast, hackable                │
│ ○ Claude Code    Multi-file, agentic           │
│ ○ Codex          OpenAI reasoning              │
│ ○ Gemini         Google multimodal             │
```

### Step 3: Count
```
│ How many agents?                                │
│                                                 │
│ Count: [4_______]                               │
│                                                 │
│ Presets: [1] [2] [4] [8] [16]                  │
```

### Step 4: Account Rotation
```
│ Account rotation mode:                          │
│                                                 │
│ ● Round-robin   Cycle through profiles         │
│ ○ Single        Use one profile for all        │
│ ○ Balanced      Prefer least-used profiles     │
│                                                 │
│ Profiles to use:                               │
│ [x] personal   [x] work   [ ] backup           │
```

### Step 5: Initial Sequence
```
│ Initial sequence (optional):                    │
│                                                 │
│ ○ None          Start with no prompt           │
│ ● continue      Resume current task            │
│ ○ bugfix        Bug fix workflow               │
│ ○ [Custom...]   Enter message now              │
```

### Confirmation
```
│ Ready to spawn:                                 │
│                                                 │
│ Workspace:  my-project                         │
│ Agents:     4 × opencode                       │
│ Rotation:   round-robin (personal, work)       │
│ Sequence:   continue                           │
│                                                 │
│ [Enter] Spawn  [Backspace] Go Back  [Esc] Cancel│
```

## Implementation
- internal/tui/components/launchpad.go
- Multi-step wizard state machine
- Integrate with workspace, agent, and recipe services
- Show spawn progress with live updates


