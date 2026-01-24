# Forge Orchestrator Product Specification (v0.1)

Working name: **Forge**
Components: **TUI dashboard**, **CLI**, optional per-node **daemon (`forged`)**
Core substrate: **tmux + ssh**
Core abstraction: **Workspace = (node, repo path) + tmux session + agents**

This document consolidates everything we’ve discussed so far into a single cohesive spec we can iterate on.

---

## 1) Product overview

Forge is a control plane for running and supervising many AI coding agents across many repositories and servers-locally and remotely-with:

* A **“sexy,” fast TUI** that makes it obvious what’s progressing vs blocked.
* A fully interoperable **CLI** (machine-readable output) for external automation (e.g., Empire Cockpit).
* Deep integration with **tmux** (agents live in panes) and **ssh** (nodes are remote servers).
* **Account/session orchestration** across providers (many accounts; cooldowns; fast switching).
* Easy **node bootstrapping** (root login → ready node) and adding nodes into a mesh.

**Key strategic direction:**
Forge will support multiple agent CLIs, but will be **OpenCode-first** as the “native integration tier” because it’s hackable and exposes a structured control surface. Other CLIs will be supported via adapters with best-effort telemetry/state detection.

---

## 2) Goals

### 2.1 Core goals

1. **Full control over all agents**

   * Spawn agents into tmux panes (local or remote).
   * Send instructions (inject message + Enter).
   * Queue many messages and pauses.
   * Interrupt/restart agents.
2. **Accurate progress tracking**

   * Know if each agent is: Working / Idle / AwaitingApproval / RateLimited / Error / Paused.
   * Visible reasons for “why we think this state is true.”
3. **Workspace-first organization**

   * Dashboard divided into workspaces.
   * Each workspace maps to a single repo folder on a specific node.
   * Each workspace has a dedicated tmux session.
4. **Accounts and usage management**

   * Configure many accounts per provider.
   * Swap agent auth sessions quickly.
   * Insert required pauses/cooldowns when needed.
5. **Great UX**

   * User can immediately see which projects progressed and which have not.
   * Keyboard-first workflow, command palette, minimal friction.
6. **Mesh scaling**

   * Add new nodes easily (automated bootstrap with root login).
   * Workspaces can exist on any node.
   * Future: multi-node workspaces.

### 2.2 Non-goals (for v1)

* Replacing tmux with a custom terminal multiplexer.
* Full “enterprise” multi-user RBAC from day one.
* Perfect provider quota accounting (we aim for practical cooldown + failover).

---

## 2.3 Next iteration vision (workflows + mesh)

This iteration extends Forge from a loop runner to a workflow orchestration plane.

**Additions**

* **Workflow**: DAG of steps (agent, bash, logic, job, human) with pre/post hooks, stop conditions, and fan-out.
* **Job**: higher-level unit that starts workflows, runs scripts, or dispatches work across nodes. Has input/output.
* **Trigger**: CLI, cron, or HTTP webhook starts a job.
* **Node + mesh**: registered computers running `forged`. Mesh has a single active master; master routes commands.
* **Parasitic steps**: steps that stay alive only while another step exists (e.g., committer agent).

**Continuity**

* Loops remain the core execution primitive and map to workflow step type `loop`.
* Profiles/pools remain the harness selection layer for agent steps.
* Ledgers remain the audit trail, now extended to workflow/job execution.

---

## 3) Key concepts and glossary

### 3.1 Node

A machine Forge can control (local or remote) via ssh and tmux.

### 3.2 Workspace

A managed unit that binds together:

* `node_id`
* `repo_path` (folder on that node)
* `tmux_session`
* zero or more agents
* optional integrations (beads/bv, Agent Mail, etc.)

### 3.3 Agent instance

A single running agent process tied to:

* an **agent type** (OpenCode / Claude Code / Codex / Gemini / etc.)
* a **runtime mode** (interactive-in-pane vs exec/headless)
* a **tmux pane** (session:window.pane)
* an **account profile** (provider credentials/profile)

### 3.4 Control plane state

Forge’s authoritative record:

* nodes, workspaces, agents
* queues and schedules
* agent states
* event history

---

## 4) User experience specification (TUI)

### 4.1 Fleet Dashboard (home screen)

Shows at-a-glance operational truth.

**Layout**

* Left: Nodes list (online/offline, load, agent count, alerts)
* Main: Workspace grid/cards

  * Repo + node
  * branch + dirty/clean
  * progress pulse (recent activity)
  * counts of agents by state (Working/Idle/Blocked/etc.)
  * top alerts (approval needed, cooldown, error)
* Bottom or overlay: Command palette (fuzzy search actions)

**Must-feel behaviors**

* Snappy refresh (no “laggy terminal app” vibe)
* Clear color semantics for states
* One keystroke to jump from fleet → workspace → agent → pane attach

### 4.2 Workspace screen

A workspace is where you do orchestration.

**Pane-grid cards**
Each agent card shows:

* Agent type + model (if known)
* Account profile
* State + “reason”
* Queue length
* Last activity timestamp
* Optional: “diff touched files” summary (strong in OpenCode mode)

**Side inspector**
Toggle between:

* Transcript/screen tail
* Queue editor
* Git status/diff summary
* Approvals inbox (workspace-scoped)
* Integrations (beads/bv, Agent Mail)

### 4.3 Agent detail screen

Focus mode for one agent:

* Full transcript/screen viewer with search
* Queue list with reorder/insert/pause/conditions
* Account profile switcher
* Usage panel (best-effort; depends on adapter)
* Actions: interrupt, restart, fork (if supported), export logs

### 4.4 High-level UX rules

* Don’t bury critical states. “Approval needed” must scream.
* Don’t hide automation. Every action is logged and inspectable.
* “Why” is always visible: show why the system thinks the agent is idle/blocked.

---

## 5) CLI specification

Everything in the TUI must be doable from the CLI.

### 5.1 Output modes

* Human-readable by default
* `--json` for machine use
* `--watch` for streaming events (JSONL)

### 5.2 Command families (initial)

**Nodes**

* `forge node list`
* `forge node add --ssh user@host --name <node>`
* `forge node bootstrap --ssh root@host [--install-extras]`
* `forge node doctor <node>`
* `forge node exec <node> -- <cmd>`

**Workspaces**

* `forge ws create --node <node> --path <repo>`
* `forge ws import --node <node> --tmux-session <name>`
* `forge ws list`
* `forge ws status <ws> [--json]`
* `forge ws attach <ws>`
* `forge ws unmanage <ws>` (leave tmux session intact)
* `forge ws kill <ws>` (kill session, archive logs)

**Agents**

* `forge agent spawn --ws <ws> --type opencode --count 3 [--profile <acct>]`
* `forge agent list [--ws <ws>]`
* `forge agent send <agent_id> "…" `
* `forge agent queue <agent_id> --file prompts.txt`
* `forge agent pause <agent_id> --minutes 20`
* `forge agent interrupt <agent_id>`
* `forge agent restart <agent_id> [--profile <acct>]`
* `forge agent approve <agent_id> [--all]` (when adapter supports approvals)

**Accounts**

* `forge accounts import-caam`
* `forge accounts list [--provider X]`
* `forge accounts rotate --provider X --mode auto`
* `forge accounts cooldown list|set|clear …`

**Export/integration**

* `forge export status --json`
* `forge export events --since 1h --jsonl`
* `forge hook on-event --cmd <script>`

---

## 6) Runtime architecture

Forge must work both locally and against remote nodes.

### 6.1 Mode A: SSH-only (minimal install)

Control plane runs locally:

* Uses ssh to run tmux operations and capture panes.
* Polling-based status inference.

Pros: simplest, minimal footprint
Cons: less scalable for many panes, less real-time

### 6.2 Mode B: Node daemon (`forged`) (recommended)

A lightweight agent runs on each node:

* tmux orchestration locally (fast)
* screen snapshotting + hashing
* transcript/log collection
* structured event emission to control plane (over ssh tunnel)
* enforcement of local policies (rate limiting, resource caps)

Pros: scales, feels real-time, cleaner integration
Cons: more setup

**Design choice:** implement Mode A first for MVP, but keep interfaces compatible so Mode B slots in cleanly.

---

## 7) Workspace lifecycle

### 7.1 Create workspace

1. Validate repo path exists.
2. Create or reuse tmux session.
3. Create a reserved “human pane” (pane 0).
4. Spawn agents into additional panes per config/recipe.

### 7.2 Import workspace from existing tmux session

Requirement: bootstrap from existing tmux.

Mechanism:

* Inspect panes for working directory info.
* If repo root ambiguous, prompt in TUI/CLI.
* Bind the session into Forge’s model without disrupting panes.

### 7.3 Destroy/unmanage

* **Unmanage:** remove from Forge but keep tmux session alive.
* **Destroy:** kill tmux session; archive transcripts and event log.

---

## 8) Agent orchestration model

### 8.1 Message injection

Universal baseline:

* `tmux send-keys -t pane "message"` + Enter

OpenCode-native:

* prefer API-based prompt submission (more reliable, supports async)

### 8.2 Queue semantics

Each agent has a queue of items:

* `message`: text to send
* `pause`: duration
* `conditional`: gates (“send only when idle”, “only when cooldown cleared”, etc.)

Rules:

* One active dispatch at a time per agent
* Scheduler checks agent state + policy gates before sending next item

### 8.3 Scheduler

Continuously:

* updates agent state
* dispatches next eligible queue item
* inserts cooldown pauses or rotates accounts when needed
* logs everything as events

---

## 9) Agent capability tiers (how Forge stays consistent)

Forge should normalize agents into a common interface, but not pretend all backends are equal.

### 9.1 Capability matrix (initial)

| Capability                            | Tier 3: Native (OpenCode-first) | Tier 2: Telemetry/Logs (Claude/Gemini/Codex depending) | Tier 1: tmux-only (generic) |
| ------------------------------------- | ------------------------------- | ------------------------------------------------------ | --------------------------- |
| Reliable state (idle/working/blocked) | ✅ deterministic events          | ⚠️ good, varies                                        | ⚠️ heuristic                |
| Send/queue commands                   | ✅ API + queue                   | ✅ send-keys / exec loop                                | ✅ send-keys                 |
| Approval routing                      | ✅ structured inbox              | ⚠️ partial                                             | ❌ mostly guess              |
| Diff/progress metadata                | ✅ direct                        | ⚠️ via git polling                                     | ⚠️ via git polling          |
| Usage metrics                         | ✅ if exposed                    | ⚠️ partial                                             | ❌                           |

**Product stance:**
TUI stays consistent, but shows “confidence” and “reason” per state. Users can prefer Tier 3 agents for heavy automation.

---

## 10) OpenCode-first strategy

### 10.1 Why OpenCode-first

* Hackable, source available, controllable.
* Provides a structured session model and event stream (so we avoid brittle UI scraping).
* Allows routing to arbitrary models through one CLI/runtime.

### 10.2 How Forge uses OpenCode

Two viable deployment patterns:

**Pattern A (recommended for multi-account): one OpenCode server per agent/profile**

* Each agent instance has its own OpenCode server process (loopback bound).
* Credentials are isolated per agent/profile.
* Forge talks to each agent’s server via ssh tunnel or local forged proxy.
* This cleanly supports many accounts and avoids “global auth store” issues.

**Pattern B: one OpenCode server per workspace**

* Many sessions per workspace.
* More efficient, but multi-account routing becomes harder (may require upstream work).

### 10.3 Forge-specific OpenCode plugin pack (optional)

Provide a plugin that:

* emits “heartbeat” + enriched events (agent_id, ws_id)
* implements policy hooks (deny risky commands by default)
* helps integrate with Forge’s approvals inbox

Forking OpenCode is not required for MVP; treat upstream contributions/fork as phase 2 if gaps appear.

---

## 11) Supporting other agent CLIs (compatibility adapters)

We still want Claude Code, Codex CLI, Gemini CLI, etc. for user choice and redundancy.

**Approach**

* Adapter layer per agent type defines:

  * spawn command
  * how to detect state
  * how to extract usage (if possible)
  * how to handle approvals (if possible)
  * how to resume/restart

**Practical policy**

* Where possible, prefer “exec/headless” modes for reliable orchestration
* Interactive full-screen UIs are supported, but may be “lower confidence” for state

---

## 12) Accounts and usage management

### 12.1 Requirements

* Configure unlimited accounts per provider.
* Quickly swap an agent between accounts.
* Track cooldown windows and schedule pauses.
* Prefer automatic rotation/failover when rate-limited.

### 12.2 Integration approach

* Reuse the account manager philosophy from `coding_agent_account_manager`:

  * store multiple provider profiles
  * expose “rotate” and “cooldown” controls
* Forge scheduler uses this information to:

  * pause agents
  * restart/rehydrate under different profile when required
  * keep a clear audit trail of why swaps happened

### 12.3 Reality check

“Perfect quota remaining” is not guaranteed across vendors. Forge aims for:

* best-effort usage metering where available
* robust cooldown + failover driven by observed rate-limit signals

---

## 13) Node bootstrap and mesh scaling

### 13.1 Bootstrap requirement

Given root login, it should be dead simple to turn a new server into a Forge node.

**Bootstrap tasks**

* create non-root user with sudo
* install core deps (tmux, git, etc.)
* install selected agent runtimes (OpenCode, etc.)
* install forged (if using daemon mode)
* configure ssh keys, sane defaults
* verify with `forge node doctor`

### 13.2 Adding nodes

* `forge node add` + `forge node bootstrap`
* Nodes appear in Fleet Dashboard
* Create workspaces on any node with one command

---

## 14) Optional integrations: beads/bv and Agent Mail

These are not mandatory, but when present should feel native.

### 14.1 beads/bv

* Workspace detects beads project and shows task/status panels.
* Export status reports easily.

### 14.2 Agent Mail (MCP)

* Workspace mailbox view
* Optional file/path claims to prevent agent conflicts
* Great primitive for future multi-node workspaces and safe parallelization

---

## 15) Observability and event log

### 15.1 Event model (append-only)

Events capture:

* node online/offline
* workspace created/imported
* agent spawned/restarted/terminated
* message queued/dispatched
* approval requested/approved/denied
* rate limit detected / cooldown inserted
* errors/crashes
* git change summaries (optional)

### 15.2 State engine

State is derived from:

* OpenCode events (best)
* transcript/log parsing
* tmux screen snapshots + hash changes
* process stats (supporting evidence)

State always includes:

* state value (Working/Idle/etc.)
* confidence (High/Medium/Low)
* reason (human-readable)

---

## 16) Security posture (v0.1)

* Nodes accessed via ssh keys.
* Prefer loopback-bound local services on nodes.
* Any remote API access via ssh port forwarding or forged proxy.
* Audit log is always available.
* Later: optional multi-user mode and RBAC.

---

## 17) Roadmap and MVP acceptance criteria

### MVP (v0.1 target)

* Workspaces (create/import/list/status)
* tmux session management + pane spawn
* Agent send + queue + pause
* Fleet Dashboard + Workspace view in TUI
* CLI parity for the above
* Basic state detection (tmux snapshots)
* Basic node bootstrap
* OpenCode adapter **as primary** (at least spawn + queue + state via its structured interface where possible)

### v0.2-v1.0

* forged for real-time scaling and better performance
* approvals inbox (especially strong for OpenCode)
* usage + cooldown scheduling integrated with account manager
* recipes/roles (planner/implementer/reviewer)
* worktree isolation per agent
* integrations (beads/bv, Agent Mail)

### Future

* multi-node workspaces
* conflict prevention via path claims
* supervisor AI that proposes actions (human-confirm by default)

---

## 18) Open questions for next iteration

These are the decisions worth locking down early:

1. **OpenCode deployment model**

   * per-agent server (best for multi-account) vs per-workspace server (simpler topology)
2. **Control plane topology**

   * SSH-only MVP vs forged-from-day-one (I lean: SSH-only MVP with forged-ready interfaces)
3. **Standard agent “roles”**

   * do we ship recipes in MVP or after?
4. **Approval policy defaults**

   * strict by default (ask) vs permissive with per-workspace policy?
5. **Logging/transcripts retention**

   * how much history do we keep locally vs on node?

## 19) Tech stack

Forge will be implemented primarily in **Go (Golang)**, with a focus on shipping a fast, reliable single-binary CLI/TUI and an optional lightweight per-node daemon.

### 19.1 Language and build

* **Language:** Go
* **Build/Dependency mgmt:** Go modules
* **Release packaging:** **GoReleaser** (cross-compile CLI + forged, create checksums, GitHub releases, etc.)
* **Target OS:** Linux-first (because tmux + ssh + servers), but keep the **control plane CLI/TUI** portable where practical (macOS/Linux).

### 19.2 TUI framework

We’ll use modern Go TUI libraries with excellent UX primitives:

**Primary recommendation: Charmbracelet stack**

* **Bubble Tea**: main TUI architecture (model-update-view), great for complex interactive apps
* **Bubbles**: reusable UI components (lists, tables, viewports, spinners, text inputs, etc.)
* **Lip Gloss**: styling/layout, consistent visual polish (“sexy” factor)
* Optional: **Harmonica** for animations/spring effects if we want extra “feel”

This stack is a strong fit for:

* real-time dashboards
* command palette / fuzzy search
* responsive layouts
* modal workflows (approvals inbox, queue editor, inspector panes)

**Secondary option (if needed):**

* **tview** (more traditional widget framework). We likely won’t need it if Bubble Tea meets our needs, but it’s a viable fallback for certain table-heavy layouts.

### 19.3 CLI framework

* **Cobra** (command structure) + **Viper** (config/env flags) is the conventional Go choice for a large CLI surface area.
* Alternative: **urfave/cli** (simpler) if we want less boilerplate.

CLI output standards:

* Human-readable by default
* `--json` and `--jsonl` for automation
* `--watch` for event streaming

### 19.4 Process execution, tmux control, and terminal plumbing

Forge will integrate deeply with tmux; the most robust approach is to **treat tmux as an external dependency** and control it via commands:

* `tmux new-session`, `split-window`, `select-pane`, `send-keys`, `capture-pane`, etc.
* Parse outputs deterministically (prefer stable formats and explicit tmux format strings)

Go components:

* `os/exec` for calling `tmux`, agent CLIs, and bootstrap scripts
* Optional: `creack/pty` if we ever need pseudo-terminal control (mostly for edge cases; tmux already gives us what we need)

### 19.5 SSH and remote execution

Forge must work across local and remote nodes.

We’ll support **two SSH execution backends**:

1. **Native Go SSH client**

* `golang.org/x/crypto/ssh`
* Pros: structured control, easier multiplexing, fewer shell quirks
* Cons: re-implementing some “real ssh” conveniences (ProxyJump, agent forwarding) can be annoying

2. **System `ssh` fallback (recommended to keep)**

* Call the user’s `ssh` binary for maximum compatibility with:

  * existing `~/.ssh/config`
  * ProxyJump / bastions
  * agent forwarding
  * ControlMaster multiplexing
* This is extremely pragmatic for power users

Forge can choose the backend per node (configurable).

### 19.6 Daemon and control plane communications (optional forged)

For real-time scale and responsiveness, we’ll optionally run **forged** on nodes.

* **Transport:** gRPC over an SSH tunnel (clean, secure, no public ports required)
* Alternative: WebSocket over SSH tunnel (fine, but gRPC gives stronger typing and tooling)

Serialization:

* **Protocol Buffers** for forged ↔ control plane messages
* Event stream: server push (subscribe) + replay (since cursor)

### 19.7 Storage and state

Forge needs a durable local state store for:

* nodes/workspaces/agents
* queues and schedules
* event log
* account/profile mapping (but secrets handled carefully)

**Recommended: SQLite**

* Local control plane DB: SQLite is perfect (fast, simple, portable)
* Go driver options:

  * **modernc.org/sqlite** (pure Go; easiest distribution)
  * or `mattn/go-sqlite3` (cgo; faster in some cases but complicates builds)

DB access:

* Use `database/sql` + migrations (golang-migrate)
* Optional: **sqlc** for type-safe queries if the schema gets large

### 19.8 Configuration and secrets

* **Config files:** YAML or TOML (user-friendly)
* **Config discovery:** XDG base dirs for Linux, standard user dirs elsewhere
* **Secrets / credentials:**

  * Prefer OS keychains where feasible (optional)
  * Otherwise: encrypted local vault file (Forge-managed) + support importing from existing account managers (e.g., caam philosophy)
  * Never log secrets; strict redaction

### 19.9 Logging and observability

* Structured logging: **zerolog** or **zap**
* Optional observability:

  * **OpenTelemetry Go SDK** for forged/control plane metrics + traces (especially useful if we later ingest telemetry from certain agent tools)
* Always keep an append-only **event log** in the Forge DB for auditability and UI replay.

### 19.10 Testing strategy

* Unit tests for:

  * state engine (idle/working detection)
  * queue scheduler
  * tmux command wrappers (with golden outputs)
* Integration tests:

  * spin up a local tmux server in CI and validate “spawn agent → send → capture-pane → state update”
  * mock ssh backend for deterministic remote behavior
