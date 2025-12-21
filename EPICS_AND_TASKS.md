# Swarm - Epics and Task Breakdown

This document breaks down the Swarm product specification into actionable epics and tasks. Tasks are organized by dependency order and MVP priority.

---

## EPIC 1: Project Foundation & Core Infrastructure

**Goal:** Establish the Go project structure, build system, configuration framework, and storage layer.

**Priority:** HIGH (Foundation - everything depends on this)  
**MVP:** Yes

### Tasks

#### 1.1 Project Scaffolding
- [ ] **1.1.1** Initialize Go module (`go mod init github.com/your-org/swarm`)
- [ ] **1.1.2** Create directory structure:
  ```
  swarm/
  ├── cmd/
  │   ├── swarm/          # CLI/TUI binary
  │   └── swarmd/         # Daemon binary (stub for now)
  ├── internal/
  │   ├── config/         # Configuration management
  │   ├── db/             # SQLite storage layer
  │   ├── models/         # Domain models (Node, Workspace, Agent, etc.)
  │   ├── tmux/           # tmux wrapper
  │   ├── ssh/            # SSH execution backends
  │   ├── scheduler/      # Queue and scheduler
  │   ├── state/          # State detection engine
  │   ├── adapters/       # Agent CLI adapters
  │   ├── events/         # Event log and pub/sub
  │   ├── cli/            # Cobra commands
  │   └── tui/            # Bubble Tea TUI
  ├── pkg/                # Public APIs (if any)
  ├── scripts/            # Bootstrap and utility scripts
  ├── testdata/           # Test fixtures
  └── docs/
  ```
- [ ] **1.1.3** Set up Makefile with standard targets (`build`, `test`, `lint`, `install`)
- [ ] **1.1.4** Configure GoReleaser for cross-compilation and releases
- [ ] **1.1.5** Set up CI pipeline (GitHub Actions: lint, test, build)

#### 1.2 Configuration System
- [ ] **1.2.1** Define configuration file schema (YAML)
  - Global config: `~/.config/swarm/config.yaml`
  - Per-workspace overrides
- [ ] **1.2.2** Implement config loader with Viper
  - XDG base directory support
  - Environment variable overrides
  - CLI flag precedence
- [ ] **1.2.3** Define configuration structures:
  - `SwarmConfig` (global settings)
  - `NodeConfig` (per-node settings)
  - `AccountConfig` (provider credentials reference)
- [ ] **1.2.4** Implement config validation and defaults

#### 1.3 Storage Layer (SQLite)
- [ ] **1.3.1** Set up SQLite with `modernc.org/sqlite` (pure Go)
- [ ] **1.3.2** Design database schema:
  ```sql
  -- Core entities
  nodes (id, name, ssh_target, status, created_at, updated_at)
  workspaces (id, node_id, repo_path, tmux_session, status, created_at)
  agents (id, workspace_id, agent_type, tmux_pane, profile_id, state, created_at)
  
  -- Queues and scheduling
  queue_items (id, agent_id, item_type, payload, position, status, created_at)
  
  -- Accounts
  accounts (id, provider, profile_name, is_active, cooldown_until)
  
  -- Events (append-only log)
  events (id, timestamp, event_type, entity_type, entity_id, payload)
  ```
- [ ] **1.3.3** Implement golang-migrate for schema migrations
- [ ] **1.3.4** Create repository interfaces and implementations:
  - `NodeRepository`
  - `WorkspaceRepository`
  - `AgentRepository`
  - `QueueRepository`
  - `AccountRepository`
  - `EventRepository`
- [ ] **1.3.5** Add database connection pooling and transaction helpers

#### 1.4 Logging Infrastructure
- [ ] **1.4.1** Set up zerolog for structured logging
- [ ] **1.4.2** Configure log levels and output formats
- [ ] **1.4.3** Implement secret redaction in logs
- [ ] **1.4.4** Add context propagation for request tracing

#### 1.5 Domain Models
- [ ] **1.5.1** Define core domain types:
  ```go
  type Node struct { ... }
  type Workspace struct { ... }
  type Agent struct { ... }
  type AgentState (Working|Idle|AwaitingApproval|RateLimited|Error|Paused)
  type QueueItem struct { ... }
  type Account struct { ... }
  type Event struct { ... }
  ```
- [ ] **1.5.2** Implement model validation methods
- [ ] **1.5.3** Add JSON serialization tags for CLI output

---

## EPIC 2: Node Management & SSH/Remote Execution

**Goal:** Implement node registration and dual SSH execution backends (native Go SSH + system ssh fallback).

**Priority:** HIGH (Required for remote operations)  
**MVP:** Yes

### Tasks

#### 2.1 SSH Abstraction Layer
- [ ] **2.1.1** Define SSH executor interface:
  ```go
  type SSHExecutor interface {
      Exec(ctx context.Context, cmd string) (stdout, stderr []byte, err error)
      ExecInteractive(ctx context.Context, cmd string, stdin io.Reader) error
      StartSession() (SSHSession, error)
      Close() error
  }
  ```
- [ ] **2.1.2** Implement connection options struct:
  - Host, port, user
  - Key path / agent forwarding
  - ProxyJump / bastion support
  - Timeout settings

#### 2.2 Native Go SSH Backend
- [ ] **2.2.1** Implement executor using `golang.org/x/crypto/ssh`
- [ ] **2.2.2** Add SSH key loading (file-based, ssh-agent)
- [ ] **2.2.3** Implement connection multiplexing/pooling
- [ ] **2.2.4** Add keep-alive and reconnection logic
- [ ] **2.2.5** Handle known_hosts verification

#### 2.3 System SSH Fallback Backend
- [ ] **2.3.1** Implement executor that shells out to `ssh` binary
- [ ] **2.3.2** Support ControlMaster multiplexing
- [ ] **2.3.3** Parse and respect `~/.ssh/config`
- [ ] **2.3.4** Handle ProxyJump transparently
- [ ] **2.3.5** Capture and parse exit codes correctly

#### 2.4 Node Service
- [ ] **2.4.1** Implement `NodeService`:
  - `AddNode(name, sshTarget string) (*Node, error)`
  - `RemoveNode(id string) error`
  - `ListNodes() ([]*Node, error)`
  - `GetNode(id string) (*Node, error)`
  - `TestConnection(id string) error`
- [ ] **2.4.2** Implement node health checking:
  - SSH connectivity test
  - tmux availability check
  - Agent runtime checks (is OpenCode installed?)
- [ ] **2.4.3** Store node connection preferences (which SSH backend to use)
- [ ] **2.4.4** Implement `swarm node doctor` diagnostics:
  - Check SSH connectivity
  - Verify tmux version
  - Check agent CLI availability
  - Verify disk space and resources

#### 2.5 Local Node Support
- [ ] **2.5.1** Implement "local" pseudo-node that executes directly
- [ ] **2.5.2** Auto-register local node on first run
- [ ] **2.5.3** Use same interface as remote nodes for consistency

---

## EPIC 3: Workspace Lifecycle Management

**Goal:** Implement workspace create, import, list, status, attach, unmanage, and destroy operations.

**Priority:** HIGH (Core abstraction)  
**MVP:** Yes

### Tasks

#### 3.1 Workspace Service Core
- [ ] **3.1.1** Implement `WorkspaceService`:
  - `CreateWorkspace(nodeID, repoPath string) (*Workspace, error)`
  - `ImportWorkspace(nodeID, tmuxSession string) (*Workspace, error)`
  - `ListWorkspaces(filters ...Filter) ([]*Workspace, error)`
  - `GetWorkspace(id string) (*Workspace, error)`
  - `GetWorkspaceStatus(id string) (*WorkspaceStatus, error)`
- [ ] **3.1.2** Validate repo path exists on target node
- [ ] **3.1.3** Detect git repository and extract branch info

#### 3.2 Workspace Creation
- [ ] **3.2.1** Generate unique tmux session name from repo
- [ ] **3.2.2** Create tmux session via node executor
- [ ] **3.2.3** Set working directory to repo path
- [ ] **3.2.4** Create "human pane" (pane 0) as reserved
- [ ] **3.2.5** Store workspace record in database

#### 3.3 Workspace Import
- [ ] **3.3.1** List existing tmux sessions on node
- [ ] **3.3.2** Inspect panes for working directory info (`tmux display -p -t pane '#{pane_current_path}'`)
- [ ] **3.3.3** Detect repo root from pane paths
- [ ] **3.3.4** Handle ambiguous repo detection (prompt user)
- [ ] **3.3.5** Bind existing session into Swarm model
- [ ] **3.3.6** Discover existing agents in panes (heuristic detection)

#### 3.4 Workspace Status
- [ ] **3.4.1** Aggregate agent states
- [ ] **3.4.2** Detect git status (branch, dirty/clean, ahead/behind)
- [ ] **3.4.3** Calculate "progress pulse" (recent activity metrics)
- [ ] **3.4.4** Identify top alerts (approval needed, cooldown, errors)
- [ ] **3.4.5** Count agents by state

#### 3.5 Workspace Lifecycle
- [ ] **3.5.1** Implement `AttachWorkspace(id string)` - returns tmux attach command
- [ ] **3.5.2** Implement `UnmanageWorkspace(id string)` - remove from DB, keep tmux
- [ ] **3.5.3** Implement `DestroyWorkspace(id string)`:
  - Archive transcripts and logs
  - Kill tmux session
  - Remove from database
- [ ] **3.5.4** Handle graceful agent shutdown on destroy

---

## EPIC 4: tmux Integration Layer

**Goal:** Robust tmux control for session/window/pane management, screen capture, and command injection.

**Priority:** HIGH (Core substrate)  
**MVP:** Yes

### Tasks

#### 4.1 tmux Command Wrapper
- [ ] **4.1.1** Create `TmuxClient` struct with executor backend
- [ ] **4.1.2** Implement core operations:
  ```go
  type TmuxClient interface {
      // Sessions
      NewSession(name, workDir string) error
      KillSession(name string) error
      ListSessions() ([]Session, error)
      HasSession(name string) (bool, error)
      
      // Windows and Panes
      SplitWindow(session string, horizontal bool) (paneID string, error)
      SelectPane(target string) error
      KillPane(target string) error
      ListPanes(session string) ([]Pane, error)
      
      // Interaction
      SendKeys(target, keys string, literal bool) error
      CapturePane(target string, opts CaptureOptions) (string, error)
      
      // Info
      GetPaneInfo(target string) (*PaneInfo, error)
  }
  ```
- [ ] **4.1.3** Use stable tmux format strings for parsing output
- [ ] **4.1.4** Handle tmux version differences gracefully

#### 4.2 Pane Management
- [ ] **4.2.1** Implement pane target parsing (`session:window.pane`)
- [ ] **4.2.2** Create pane layout manager (for spawning multiple agents)
- [ ] **4.2.3** Track pane-to-agent mapping
- [ ] **4.2.4** Handle pane resize events

#### 4.3 Screen Capture
- [ ] **4.3.1** Implement `capture-pane` with options:
  - Start/end line specification
  - History buffer access
  - Escape sequences (optional)
- [ ] **4.3.2** Create screen snapshot hashing for change detection
- [ ] **4.3.3** Implement transcript accumulator (rolling buffer)
- [ ] **4.3.4** Handle large scrollback efficiently

#### 4.4 Command Injection
- [ ] **4.4.1** Implement `SendKeys` with proper escaping
- [ ] **4.4.2** Support literal mode for special characters
- [ ] **4.4.3** Implement "send and wait for idle" pattern
- [ ] **4.4.4** Handle multi-line input injection
- [ ] **4.4.5** Add interrupt signal sending (Ctrl-C)

#### 4.5 Session Persistence
- [ ] **4.5.1** Verify sessions survive SSH disconnection
- [ ] **4.5.2** Implement session recovery on reconnect
- [ ] **4.5.3** Handle "session not found" errors gracefully

---

## EPIC 5: Agent Orchestration Core

**Goal:** Implement agent spawning, lifecycle management, and the core orchestration loop.

**Priority:** HIGH  
**MVP:** Yes

### Tasks

#### 5.1 Agent Service
- [ ] **5.1.1** Implement `AgentService`:
  ```go
  type AgentService interface {
      SpawnAgent(wsID string, agentType string, opts SpawnOptions) (*Agent, error)
      ListAgents(wsID string) ([]*Agent, error)
      GetAgent(id string) (*Agent, error)
      GetAgentState(id string) (*AgentState, error)
      InterruptAgent(id string) error
      RestartAgent(id string, opts RestartOptions) error
      TerminateAgent(id string) error
  }
  ```
- [ ] **5.1.2** Define `SpawnOptions`:
  - Agent type
  - Account profile
  - Initial prompt (optional)
  - Environment overrides

#### 5.2 Agent Spawning
- [ ] **5.2.1** Create new pane in workspace tmux session
- [ ] **5.2.2** Set working directory to workspace repo path
- [ ] **5.2.3** Invoke agent CLI via adapter
- [ ] **5.2.4** Store agent record with pane mapping
- [ ] **5.2.5** Wait for agent to reach "ready" state

#### 5.3 Multi-Agent Spawning
- [ ] **5.3.1** Implement `--count N` for batch spawning
- [ ] **5.3.2** Distribute panes efficiently in tmux layout
- [ ] **5.3.3** Handle partial spawn failures gracefully

#### 5.4 Agent Lifecycle
- [ ] **5.4.1** Implement interrupt (Ctrl-C injection)
- [ ] **5.4.2** Implement restart:
  - Kill current agent process
  - Re-spawn with same or different profile
- [ ] **5.4.3** Implement graceful termination
- [ ] **5.4.4** Clean up pane on agent termination
- [ ] **5.4.5** Archive agent logs/transcript on termination

#### 5.5 Agent Message Sending
- [ ] **5.5.1** Implement `SendMessage(agentID, message string) error`
- [ ] **5.5.2** Verify agent is in correct state (idle) before sending
- [ ] **5.5.3** Log message dispatch as event
- [ ] **5.5.4** Handle send failure (agent not ready, etc.)

---

## EPIC 6: Agent Adapters (OpenCode-first)

**Goal:** Implement adapter layer for agent CLIs with OpenCode as the primary (Tier 3) integration.

**Priority:** HIGH  
**MVP:** Yes (OpenCode adapter required)

### Tasks

#### 6.1 Adapter Interface
- [ ] **6.1.1** Define adapter interface:
  ```go
  type AgentAdapter interface {
      // Identity
      Name() string
      Tier() AdapterTier  // Tier1, Tier2, Tier3
      
      // Lifecycle
      SpawnCommand(opts SpawnOptions) (cmd string, args []string)
      DetectReady(screen string) (bool, error)
      DetectState(screen string, meta any) (AgentState, StateReason, error)
      
      // Control
      SendMessage(tmux TmuxClient, pane, message string) error
      Interrupt(tmux TmuxClient, pane string) error
      
      // Capabilities
      SupportsApprovals() bool
      SupportsUsageMetrics() bool
      SupportsDiffMetadata() bool
  }
  ```
- [ ] **6.1.2** Define `AdapterTier` enum with capability implications
- [ ] **6.1.3** Create adapter registry

#### 6.2 OpenCode Adapter (Tier 3 - Native)
- [ ] **6.2.1** Research OpenCode CLI interface and event stream
- [ ] **6.2.2** Implement spawn command construction
- [ ] **6.2.3** Implement state detection from OpenCode events:
  - Connect to OpenCode's structured interface if available
  - Parse state from screen if events unavailable
- [ ] **6.2.4** Implement approval handling:
  - Detect approval requests
  - Route to approvals inbox
  - Send approval/denial
- [ ] **6.2.5** Extract diff/progress metadata
- [ ] **6.2.6** Extract usage metrics (if exposed)
- [ ] **6.2.7** Implement account profile injection (env vars)

#### 6.3 Generic tmux-only Adapter (Tier 1)
- [ ] **6.3.1** Implement fallback adapter for unknown CLIs
- [ ] **6.3.2** Use heuristic state detection (screen hash changes)
- [ ] **6.3.3** Implement basic send-keys messaging
- [ ] **6.3.4** Mark all states as "Low confidence"

#### 6.4 Claude Code Adapter (Tier 2)
- [ ] **6.4.1** Research Claude Code CLI interface
- [ ] **6.4.2** Implement spawn command
- [ ] **6.4.3** Implement state detection from logs/screen
- [ ] **6.4.4** Implement approval handling (if supported)

#### 6.5 Codex CLI Adapter (Tier 2)
- [ ] **6.5.1** Research Codex CLI interface
- [ ] **6.5.2** Implement spawn and state detection

#### 6.6 Gemini CLI Adapter (Tier 2)
- [ ] **6.6.1** Research Gemini CLI interface
- [ ] **6.6.2** Implement spawn and state detection

---

## EPIC 7: State Detection Engine

**Goal:** Implement the state engine that determines agent states with confidence levels and reasons.

**Priority:** HIGH  
**MVP:** Yes (basic detection required)

### Tasks

#### 7.1 State Engine Core
- [ ] **7.1.1** Define state model:
  ```go
  type AgentState string
  const (
      StateWorking          AgentState = "working"
      StateIdle             AgentState = "idle"
      StateAwaitingApproval AgentState = "awaiting_approval"
      StateRateLimited      AgentState = "rate_limited"
      StateError            AgentState = "error"
      StatePaused           AgentState = "paused"
  )
  
  type StateConfidence string  // High, Medium, Low
  
  type StateResult struct {
      State      AgentState
      Confidence StateConfidence
      Reason     string
      Evidence   []string  // Supporting evidence
      Timestamp  time.Time
  }
  ```
- [ ] **7.1.2** Implement `StateEngine`:
  ```go
  type StateEngine interface {
      UpdateState(agentID string) (*StateResult, error)
      GetState(agentID string) (*StateResult, error)
      Subscribe(agentID string) <-chan StateResult
  }
  ```

#### 7.2 Evidence Collectors
- [ ] **7.2.1** Implement screen snapshot collector:
  - Capture pane content periodically
  - Hash content for change detection
  - Detect "idle" patterns (prompt visible, no activity)
- [ ] **7.2.2** Implement transcript parser:
  - Identify output patterns indicating state
  - Detect error messages
  - Detect rate limit messages
  - Detect approval requests
- [ ] **7.2.3** Implement process stats collector (optional):
  - CPU usage of agent process
  - Memory usage
  - I/O activity

#### 7.3 State Inference
- [ ] **7.3.1** Implement rule-based state inference:
  - "Screen unchanged for X seconds" → likely idle
  - "Error pattern detected" → error state
  - "Rate limit pattern detected" → rate limited
  - "Approval pattern detected" → awaiting approval
- [ ] **7.3.2** Combine evidence sources with confidence weighting
- [ ] **7.3.3** Integrate adapter-specific state detection
- [ ] **7.3.4** Handle conflicting evidence gracefully

#### 7.4 State Polling Loop
- [ ] **7.4.1** Implement polling scheduler (configurable interval)
- [ ] **7.4.2** Prioritize recently active agents
- [ ] **7.4.3** Emit state change events
- [ ] **7.4.4** Handle poll failures gracefully

---

## EPIC 8: Queue & Scheduler System

**Goal:** Implement message queues, pause scheduling, and the dispatch scheduler.

**Priority:** HIGH  
**MVP:** Yes

### Tasks

#### 8.1 Queue Data Model
- [ ] **8.1.1** Define queue item types:
  ```go
  type QueueItemType string
  const (
      QueueItemMessage     QueueItemType = "message"
      QueueItemPause       QueueItemType = "pause"
      QueueItemConditional QueueItemType = "conditional"
  )
  
  type QueueItem struct {
      ID        string
      AgentID   string
      Type      QueueItemType
      Payload   json.RawMessage
      Position  int
      Status    QueueItemStatus  // pending, dispatched, completed, failed
      CreatedAt time.Time
  }
  ```

#### 8.2 Queue Service
- [ ] **8.2.1** Implement `QueueService`:
  ```go
  type QueueService interface {
      Enqueue(agentID string, items ...QueueItem) error
      Dequeue(agentID string) (*QueueItem, error)
      Peek(agentID string) (*QueueItem, error)
      ListQueue(agentID string) ([]*QueueItem, error)
      ReorderQueue(agentID string, ordering []string) error
      ClearQueue(agentID string) error
      InsertAt(agentID string, position int, item QueueItem) error
      RemoveItem(itemID string) error
  }
  ```
- [ ] **8.2.2** Implement queue persistence
- [ ] **8.2.3** Implement queue reordering with drag-drop support

#### 8.3 Queue Item Types
- [ ] **8.3.1** Implement message item dispatch
- [ ] **8.3.2** Implement pause item (duration-based)
- [ ] **8.3.3** Implement conditional item:
  - "Only when idle"
  - "Only when cooldown cleared"
  - "Only after previous completes"
  - Custom condition expressions

#### 8.4 Scheduler Core
- [ ] **8.4.1** Implement `Scheduler`:
  ```go
  type Scheduler interface {
      Start(ctx context.Context) error
      Stop() error
      ScheduleNow(agentID string) error
      Pause(agentID string, duration time.Duration) error
      Resume(agentID string) error
  }
  ```
- [ ] **8.4.2** Run scheduling loop:
  - Check each agent's state
  - Check queue head eligibility
  - Dispatch if conditions met
  - Log all actions as events

#### 8.5 Dispatch Logic
- [ ] **8.5.1** Check agent state before dispatch (must be idle)
- [ ] **8.5.2** Check cooldown status
- [ ] **8.5.3** Check conditional gates
- [ ] **8.5.4** Execute dispatch via adapter
- [ ] **8.5.5** Update queue item status
- [ ] **8.5.6** Handle dispatch failures (retry logic)

#### 8.6 Pause Management
- [ ] **8.6.1** Implement agent pause with timer
- [ ] **8.6.2** Implement auto-resume on timer expiry
- [ ] **8.6.3** Insert cooldown pauses automatically when rate-limited

---

## EPIC 9: CLI Implementation

**Goal:** Full CLI parity with TUI using Cobra, supporting human-readable and machine-readable output.

**Priority:** HIGH  
**MVP:** Yes

### Tasks

#### 9.1 CLI Framework Setup
- [ ] **9.1.1** Set up Cobra root command with global flags:
  - `--json` for JSON output
  - `--jsonl` for streaming JSON lines
  - `--watch` for event streaming
  - `--config` for config file override
  - `--verbose` / `-v` for debug output
- [ ] **9.1.2** Implement output formatter (human/JSON/JSONL)
- [ ] **9.1.3** Set up Viper for config/env/flag binding
- [ ] **9.1.4** Implement command completion (bash, zsh, fish)

#### 9.2 Node Commands
- [ ] **9.2.1** `swarm node list` - list all nodes with status
- [ ] **9.2.2** `swarm node add --ssh user@host --name <node>` - register node
- [ ] **9.2.3** `swarm node remove <node>` - unregister node
- [ ] **9.2.4** `swarm node bootstrap --ssh root@host` - full bootstrap
- [ ] **9.2.5** `swarm node doctor <node>` - run diagnostics
- [ ] **9.2.6** `swarm node exec <node> -- <cmd>` - remote command execution

#### 9.3 Workspace Commands
- [ ] **9.3.1** `swarm ws create --node <node> --path <repo>` - create workspace
- [ ] **9.3.2** `swarm ws import --node <node> --tmux-session <name>` - import existing
- [ ] **9.3.3** `swarm ws list` - list workspaces
- [ ] **9.3.4** `swarm ws status <ws>` - detailed status
- [ ] **9.3.5** `swarm ws attach <ws>` - attach to tmux session
- [ ] **9.3.6** `swarm ws unmanage <ws>` - remove from Swarm, keep tmux
- [ ] **9.3.7** `swarm ws kill <ws>` - destroy workspace

#### 9.4 Agent Commands
- [ ] **9.4.1** `swarm agent spawn --ws <ws> --type opencode --count N` - spawn agents
- [ ] **9.4.2** `swarm agent list [--ws <ws>]` - list agents
- [ ] **9.4.3** `swarm agent status <agent>` - agent detail
- [ ] **9.4.4** `swarm agent send <agent> "message"` - send message
- [ ] **9.4.5** `swarm agent queue <agent> --file prompts.txt` - bulk queue
- [ ] **9.4.6** `swarm agent pause <agent> --minutes N` - pause agent
- [ ] **9.4.7** `swarm agent resume <agent>` - resume paused agent
- [ ] **9.4.8** `swarm agent interrupt <agent>` - send interrupt
- [ ] **9.4.9** `swarm agent restart <agent>` - restart agent
- [ ] **9.4.10** `swarm agent approve <agent> [--all]` - handle approvals

#### 9.5 Account Commands
- [ ] **9.5.1** `swarm accounts list` - list configured accounts
- [ ] **9.5.2** `swarm accounts add` - add new account (interactive)
- [ ] **9.5.3** `swarm accounts import-caam` - import from caam
- [ ] **9.5.4** `swarm accounts rotate` - rotate account for agent
- [ ] **9.5.5** `swarm accounts cooldown list|set|clear` - manage cooldowns

#### 9.6 Export/Integration Commands
- [ ] **9.6.1** `swarm export status --json` - export full status
- [ ] **9.6.2** `swarm export events --since 1h --jsonl` - export events
- [ ] **9.6.3** `swarm hook on-event --cmd <script>` - register webhooks

#### 9.7 TUI Launch
- [ ] **9.7.1** `swarm` (no subcommand) - launch TUI
- [ ] **9.7.2** `swarm ui` - explicit TUI launch
- [ ] **9.7.3** Handle TTY detection for auto-mode selection

---

## EPIC 10: TUI Fleet Dashboard

**Goal:** Implement the main TUI Fleet Dashboard using Bubble Tea and Lip Gloss.

**Priority:** HIGH  
**MVP:** Yes

### Tasks

#### 10.1 TUI Framework Setup
- [ ] **10.1.1** Set up Bubble Tea application structure
- [ ] **10.1.2** Create main model with view routing
- [ ] **10.1.3** Set up Lip Gloss theme and color palette:
  - State colors (Working=green, Idle=gray, Error=red, etc.)
  - Alert colors
  - Accent colors
- [ ] **10.1.4** Implement responsive layout (terminal resize handling)

#### 10.2 Fleet Dashboard Layout
- [ ] **10.2.1** Implement left panel: Nodes list
  - Online/offline indicator
  - Agent count per node
  - Load/resource summary
  - Alerts badge
- [ ] **10.2.2** Implement main area: Workspace grid/cards
  - Repo name + node
  - Branch + dirty/clean status
  - Progress pulse animation
  - Agent counts by state
  - Top alerts
- [ ] **10.2.3** Implement bottom bar: Command palette trigger + status

#### 10.3 Workspace Cards
- [ ] **10.3.1** Design card component with state indicators
- [ ] **10.3.2** Implement progress pulse (activity sparkline)
- [ ] **10.3.3** Show agent state breakdown
- [ ] **10.3.4** Highlight blocking issues (approval, cooldown, error)
- [ ] **10.3.5** Handle card selection and navigation

#### 10.4 Command Palette
- [ ] **10.4.1** Implement fuzzy search overlay
- [ ] **10.4.2** Register global actions:
  - Navigate to workspace
  - Spawn agent
  - Pause/resume all
  - Refresh
  - Settings
- [ ] **10.4.3** Context-aware action filtering
- [ ] **10.4.4** Keyboard shortcut hints

#### 10.5 Real-time Updates
- [ ] **10.5.1** Subscribe to state change events
- [ ] **10.5.2** Implement efficient re-rendering on updates
- [ ] **10.5.3** Handle reconnection to event stream
- [ ] **10.5.4** Show "last updated" timestamp

#### 10.6 Navigation
- [ ] **10.6.1** Implement keyboard navigation:
  - Arrow keys for card selection
  - Enter to drill into workspace
  - `g` for go-to menu
  - `?` for help
  - `q` to quit
- [ ] **10.6.2** Implement breadcrumb trail
- [ ] **10.6.3** Support quick-jump via search

---

## EPIC 11: TUI Workspace & Agent Views

**Goal:** Implement Workspace detail view and Agent detail view in the TUI.

**Priority:** MEDIUM  
**MVP:** Partial (Workspace view required, Agent view nice-to-have)

### Tasks

#### 11.1 Workspace Screen
- [ ] **11.1.1** Implement pane-grid cards layout:
  - Each card = one agent
  - Agent type + model
  - Account profile
  - State + reason
  - Queue length
  - Last activity
- [ ] **11.1.2** Implement side inspector panel (toggle):
  - Transcript/screen tail
  - Queue editor
  - Git status/diff summary
  - Approvals inbox
  - Integrations

#### 11.2 Agent Cards
- [ ] **11.2.1** Design agent card component
- [ ] **11.2.2** Show state with color and icon
- [ ] **11.2.3** Show confidence indicator
- [ ] **11.2.4** Show "why" reason text
- [ ] **11.2.5** Quick actions on hover/select

#### 11.3 Transcript Viewer
- [ ] **11.3.1** Implement scrollable transcript pane
- [ ] **11.3.2** Syntax highlighting for code blocks
- [ ] **11.3.3** Search within transcript
- [ ] **11.3.4** Auto-scroll to bottom (toggle)
- [ ] **11.3.5** Timestamp annotations

#### 11.4 Queue Editor
- [ ] **11.4.1** Display queue items as list
- [ ] **11.4.2** Inline editing of messages
- [ ] **11.4.3** Reorder via keyboard
- [ ] **11.4.4** Insert pause/conditional items
- [ ] **11.4.5** Delete items
- [ ] **11.4.6** Clear queue action

#### 11.5 Agent Detail Screen
- [ ] **11.5.1** Full transcript viewer with search
- [ ] **11.5.2** Queue list with full editing
- [ ] **11.5.3** Account profile switcher
- [ ] **11.5.4** Usage panel (from adapter)
- [ ] **11.5.5** Action buttons: interrupt, restart, export

#### 11.6 Approvals Inbox
- [ ] **11.6.1** List pending approvals for workspace/agent
- [ ] **11.6.2** Show approval request details
- [ ] **11.6.3** Approve/deny actions
- [ ] **11.6.4** Bulk approve option

---

## EPIC 12: Accounts & Usage Management

**Goal:** Implement multi-account configuration, rotation, and cooldown management.

**Priority:** MEDIUM  
**MVP:** Basic account config required

### Tasks

#### 12.1 Account Model
- [ ] **12.1.1** Define account configuration:
  ```go
  type Account struct {
      ID           string
      Provider     string  // anthropic, openai, google, etc.
      ProfileName  string
      CredPath     string  // path to credential file/env var
      IsActive     bool
      CooldownUntil *time.Time
  }
  ```
- [ ] **12.1.2** Implement secure credential storage (encrypted file)
- [ ] **12.1.3** Support environment variable references

#### 12.2 Account Service
- [ ] **12.2.1** Implement `AccountService`:
  - `AddAccount(cfg AccountConfig) error`
  - `ListAccounts(provider string) ([]*Account, error)`
  - `GetAccount(id string) (*Account, error)`
  - `DeleteAccount(id string) error`
  - `SetCooldown(id string, until time.Time) error`
  - `ClearCooldown(id string) error`
  - `GetNextAvailable(provider string) (*Account, error)`

#### 12.3 caam Import
- [ ] **12.3.1** Parse caam configuration format
- [ ] **12.3.2** Import accounts with credential references
- [ ] **12.3.3** Handle migration of existing sessions

#### 12.4 Account Rotation
- [ ] **12.4.1** Detect rate limit events from agents
- [ ] **12.4.2** Set cooldown on current account
- [ ] **12.4.3** Select next available account
- [ ] **12.4.4** Restart agent with new account
- [ ] **12.4.5** Log rotation event

#### 12.5 Cooldown Management
- [ ] **12.5.1** Track cooldown timers per account
- [ ] **12.5.2** Automatic cooldown clear on timer expiry
- [ ] **12.5.3** Manual cooldown override commands
- [ ] **12.5.4** Display cooldown status in TUI

#### 12.6 Usage Tracking (Best-effort)
- [ ] **12.6.1** Collect usage metrics from adapters (where available)
- [ ] **12.6.2** Store usage history
- [ ] **12.6.3** Display usage estimates in TUI
- [ ] **12.6.4** Warn on approaching limits

---

## EPIC 13: Observability & Event Log

**Goal:** Implement append-only event log, state engine, and event streaming for TUI/CLI.

**Priority:** MEDIUM  
**MVP:** Basic event logging required

### Tasks

#### 13.1 Event Model
- [ ] **13.1.1** Define event types:
  ```go
  type EventType string
  const (
      EventNodeOnline        EventType = "node.online"
      EventNodeOffline       EventType = "node.offline"
      EventWorkspaceCreated  EventType = "workspace.created"
      EventAgentSpawned      EventType = "agent.spawned"
      EventAgentStateChanged EventType = "agent.state_changed"
      EventMessageQueued     EventType = "message.queued"
      EventMessageDispatched EventType = "message.dispatched"
      EventApprovalRequested EventType = "approval.requested"
      EventRateLimitDetected EventType = "rate_limit.detected"
      EventError             EventType = "error"
      // ... etc
  )
  ```
- [ ] **13.1.2** Define event structure with payload

#### 13.2 Event Store
- [ ] **13.2.1** Implement append-only event log in SQLite
- [ ] **13.2.2** Add event indexing for efficient queries
- [ ] **13.2.3** Implement event retention policy (configurable)
- [ ] **13.2.4** Support cursor-based pagination

#### 13.3 Event Publishing
- [ ] **13.3.1** Implement event publisher:
  ```go
  type EventPublisher interface {
      Publish(event Event) error
      Subscribe(filter EventFilter) <-chan Event
      Unsubscribe(ch <-chan Event)
  }
  ```
- [ ] **13.3.2** Integrate event publishing throughout services
- [ ] **13.3.3** Ensure atomic event creation with state changes

#### 13.4 Event Streaming
- [ ] **13.4.1** Implement `--watch` streaming for CLI
- [ ] **13.4.2** JSONL output format for events
- [ ] **13.4.3** Filter by event type, workspace, agent
- [ ] **13.4.4** Replay from timestamp

#### 13.5 Event Export
- [ ] **13.5.1** Export events by time range
- [ ] **13.5.2** Export events by entity
- [ ] **13.5.3** Format as JSON/JSONL

---

## EPIC 14: Node Bootstrap System

**Goal:** Implement automated server provisioning from root login to ready Swarm node.

**Priority:** MEDIUM  
**MVP:** Basic bootstrap required

### Tasks

#### 14.1 Bootstrap Script
- [ ] **14.1.1** Create idempotent bootstrap script:
  - Create non-root user with sudo
  - Configure SSH keys
  - Install system dependencies (tmux, git, curl, etc.)
  - Install language runtimes (Node.js for OpenCode, etc.)
  - Configure sane shell defaults
- [ ] **14.1.2** Support major Linux distros (Ubuntu, Debian, RHEL/CentOS)
- [ ] **14.1.3** Make script available via URL for curl-pipe-bash

#### 14.2 Agent Runtime Installation
- [ ] **14.2.1** Install OpenCode CLI
- [ ] **14.2.2** Install Claude Code CLI (optional)
- [ ] **14.2.3** Install other agent runtimes (optional)
- [ ] **14.2.4** Verify installations

#### 14.3 Bootstrap CLI Command
- [ ] **14.3.1** Implement `swarm node bootstrap --ssh root@host`:
  - Copy bootstrap script
  - Execute with sudo
  - Run node doctor
  - Register node
- [ ] **14.3.2** Support `--install-extras` for optional components
- [ ] **14.3.3** Interactive mode for configuration questions
- [ ] **14.3.4** Non-interactive mode with defaults

#### 14.4 swarmd Installation (Future)
- [ ] **14.4.1** Install swarmd binary
- [ ] **14.4.2** Configure systemd service
- [ ] **14.4.3** Set up SSH tunnel for communication

---

## EPIC 15: Optional Integrations (beads/bv, Agent Mail)

**Goal:** Integrate optional tooling for enhanced workspace visibility and agent coordination.

**Priority:** LOW  
**MVP:** No

### Tasks

#### 15.1 beads/bv Integration
- [ ] **15.1.1** Detect beads project in workspace
- [ ] **15.1.2** Parse beads task status
- [ ] **15.1.3** Display task panel in workspace view
- [ ] **15.1.4** Export status reports

#### 15.2 Agent Mail Integration
- [ ] **15.2.1** Detect Agent Mail MCP in workspace
- [ ] **15.2.2** Display mailbox view in workspace
- [ ] **15.2.3** Integrate file/path claims for conflict prevention
- [ ] **15.2.4** Show claim status in agent cards

---

## EPIC 16: swarmd Daemon (Post-MVP)

**Goal:** Implement the optional per-node daemon for real-time performance and scalability.

**Priority:** LOW  
**MVP:** No (SSH-only mode for MVP)

### Tasks

#### 16.1 Daemon Core
- [ ] **16.1.1** Create `swarmd` binary structure
- [ ] **16.1.2** Implement gRPC service definitions (Protocol Buffers)
- [ ] **16.1.3** Local tmux orchestration (fast path)
- [ ] **16.1.4** Screen snapshotting and hashing
- [ ] **16.1.5** Transcript/log collection

#### 16.2 Event Streaming
- [ ] **16.2.1** Implement server-push events to control plane
- [ ] **16.2.2** Event replay from cursor
- [ ] **16.2.3** Efficient delta updates

#### 16.3 Control Plane Integration
- [ ] **16.3.1** SSH tunnel setup for gRPC channel
- [ ] **16.3.2** Seamless fallback to SSH-only mode
- [ ] **16.3.3** Hybrid mode (some nodes with daemon, some without)

#### 16.4 Local Policies
- [ ] **16.4.1** Rate limiting enforcement
- [ ] **16.4.2** Resource caps (CPU/memory per agent)
- [ ] **16.4.3** Disk space monitoring

---

## Dependency Graph (Suggested Build Order)

```
Phase 1 - Foundation (Week 1-2)
├── Epic 1: Project Foundation (required first)
└── Epic 4: tmux Integration (can parallel)

Phase 2 - Core Operations (Week 2-4)
├── Epic 2: Node Management (depends on 1)
├── Epic 3: Workspace Management (depends on 1, 2, 4)
└── Epic 5: Agent Orchestration (depends on 1, 3, 4)

Phase 3 - Intelligence (Week 4-5)
├── Epic 6: Agent Adapters (depends on 5)
├── Epic 7: State Detection (depends on 4, 5, 6)
└── Epic 8: Queue & Scheduler (depends on 5, 7)

Phase 4 - Interfaces (Week 5-7)
├── Epic 9: CLI (depends on 2, 3, 5, 8)
├── Epic 10: TUI Dashboard (depends on 2, 3, 7)
└── Epic 11: TUI Detail Views (depends on 10)

Phase 5 - Polish (Week 7-8)
├── Epic 12: Accounts & Usage (depends on 5, 6)
├── Epic 13: Observability (depends on all)
└── Epic 14: Bootstrap (depends on 2)

Phase 6 - Future
├── Epic 15: Integrations
└── Epic 16: swarmd
```

---

## MVP Checklist

The minimum viable product includes:

- [ ] Project foundation with config and SQLite storage
- [ ] Local node support (remote nodes nice-to-have for MVP)
- [ ] Workspace create/import/list/status/attach
- [ ] tmux session management and pane spawn
- [ ] Agent spawn with OpenCode adapter
- [ ] Agent send + basic queue
- [ ] Basic state detection (screen snapshots)
- [ ] Fleet Dashboard TUI
- [ ] Workspace view TUI
- [ ] CLI parity for core operations

---

## Notes

- **OpenCode-first**: The OpenCode adapter is the primary integration target. Other adapters are secondary.
- **SSH-only MVP**: Use SSH-based remote execution for MVP. swarmd daemon is post-MVP.
- **State confidence**: Always show confidence and reason for state determinations.
- **Keyboard-first**: TUI must be fully keyboard navigable with command palette.
