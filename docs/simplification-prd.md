Below is an expanded **PRD for the Forge Loops rewrite**, with much more detail and a clearer, Docker-like CLI specification (including the exact command set and flags you reiterated). I’m also tightening the definitions around **Profiles and Pools** per your clarification: **a pool is a list of harness+auth combinations**, and the same combination can appear in multiple pools.

I’ll reference the Ralph loop semantics explicitly since Forge’s v1 runtime is intentionally modeled after your `ralph.sh` behavior (PID/log/stop-file, prompt delivery modes, alias mapping). 

---

# Forge Loops Rewrite PRD v1.1

## 1) Product overview

**Forge** is a headless “agent loop runner” for software repos. It is optimized for one outcome:

> “In any repo, I can start 1–N continuous development loops, steer them instantly, rotate across multiple subscriptions/accounts, and see everything clearly in a simple TUI.”

The rewrite intentionally **moves the golden path away from tmux panes/harness management** and toward an “agent loop” runtime that matches Ralph-driven development: a background process repeatedly runs an agent command against a prompt and sleeps between iterations; it supports graceful stop after an iteration completes; it logs all output. 

**Primary harness in v1:** OpenCode headless
**Planned harnesses:** Pi coding-agent (for true steering interrupts), OpenCode+Codex OAuth plugin

---

## 2) Target users and use cases

### Primary user

You (and power users like you) who:

* operate multiple repos in parallel,
* run many simultaneous agent sessions,
* have multiple subscriptions across providers,
* need **fast “start/stop/steer”** without babysitting,
* care deeply about **UX** and **DX**.

### Primary use cases

1. Start one loop in a repo and let it run forever.
2. Scale to 10–50 loops quickly.
3. Inject a message to one or many loops.
4. **Preempt** a loop: “STOP and do X now”.
5. Rotate across multiple subscriptions via pools.
6. Quickly switch default prompts and apply **one-shot “next iteration override prompt”**.

---

## 3) Core concepts and terminology

This rewrite intentionally reduces the “public” mental model to a few nouns.

### 3.1 Repository

A git repo folder containing a committed `.forge/` directory (project brain).

### 3.2 Loop

A background worker process that repeatedly runs a harness invocation in the repo directory:

* reads a prompt (base prompt or override)
* consumes queue items (messages, pauses, overrides)
* executes harness one-shot
* appends logs and writes a ledger entry
* sleeps
* repeats 

### 3.3 Harness

A runtime implementation. Examples:

* `opencode` (v1)
* `pi` (planned)
* future harnesses: other headless CLIs

### 3.4 Profile (Harness+Auth combination)

**This is your clarified requirement.**

A **Profile** is the atomic unit of execution identity:

* `harness`: `opencode` | `pi` | …
* `auth_kind`: `claude` | `codex` | … (not 1–1 with harness)
* `auth_home`: filesystem directory containing isolated auth/config state
* optional defaults:

  * `model` / `provider`
  * `extra_args`
  * `cooldown_policy`
  * `max_concurrency` (cap per profile)

**Key properties**

* A profile can be referenced by multiple pools.
* A profile is *not* a loop name (e.g., `oc1` is legacy-ish and maps to a profile, not a loop identity).

### 3.5 Pool

A named list of **profile references**.

* Pools are not “claude-only” or “opencode-only” by definition. They are **explicit lists** of harness+auth combos.
* A profile can appear in multiple pools.

Pools also define selection strategy:

* round-robin (default)
* least-recently-used
* weighted selection (optional, v1.1)

### 3.6 Prompt

A file that contains the base instructions for a loop iteration.

### 3.7 Template and Sequence

* **Template**: a stored message snippet (used in message palette)
* **Sequence**: an ordered list of queue actions (messages + pauses + overrides), e.g.

  * “review → pause 2m → continue”

### 3.8 Queue

Each loop has a queue of actions. v1 supports:

* `MessageAppend`: appended as operator message to the effective prompt
* `NextPromptOverrideOnce`: **replaces** the base prompt for the next iteration only, then reverts
* `Pause`: delay before next run / between actions
* `StopGraceful`: stops after current iteration
* `KillNow`: immediate stop

Planned:

* `SteerMessage`: true mid-flight interrupt (Pi harness likely)

---

## 4) System behavior

## 4.1 Loop runtime: iteration lifecycle

Each loop iteration:

1. **Stop check (graceful stop)**
   If stop marker exists, exit after writing a final log line and cleaning up state.
   This mirrors Ralph’s stop-file semantics. 

2. **Load base prompt**

   * default: repo root `PROMPT.md`
   * or configured base prompt

3. **Apply queue items**

   * `NextPromptOverrideOnce` (if present and due): uses override prompt instead of base prompt **for this iteration**, then marks override consumed.
   * `MessageAppend`: appended to effective prompt (e.g., under an “Operator Message” heading).
   * `Pause`: sleeps and marks consumed; iteration may continue or return (implementation detail; v1 can treat pause as an iteration boundary).

4. **Select profile**

   * if loop is pinned to a profile: use it
   * else: select from pool based on strategy and availability
   * if none available (all cooling down / at concurrency caps): loop waits until earliest is available

5. **Execute harness one-shot**

   * working directory: repo root
   * prompt delivery supports Ralph’s modes:

     * `{prompt}` substitution into command string
     * env var prompt content
     * stdin prompt content 

6. **Capture output**

   * append stdout/stderr to loop log
   * record exit code and timestamps
   * write a ledger entry (see §9)

7. **Sleep interval**

   * default: configurable (Ralph used 2 seconds; Forge default should be “human sane” like 10–30s). 

---

## 5) Auth and subscription handling

This is non-negotiable: Forge must support many concurrent loops using different subscriptions **without auth collisions**.

### 5.1 Profile isolation

Forge runs each harness process with environment overrides based on profile:

* `HOME=<profile.auth_home>` (baseline isolation for OpenCode-like tools)
* `CODEX_HOME=<...>` if needed for codex CLI compatibility (if used directly)
* pi: supports `--session-dir` and has its own storage conventions; profile should control those.

**Important:** Profiles are machine-local (paths differ per machine). Pools are also machine-local.

### 5.2 Legacy alias import (your current `~/.oc1`, `~/.codex2` setup)

Forge should provide a smooth migration path:

* You can create profiles pointing at existing dirs:

  * `forge profile add opencode --auth-kind claude --home ~/.oc1 --name claude-a`
  * `forge profile add opencode --auth-kind codex --home ~/.codex1 --name codex-a`

Optionally:

* `forge profile init` reads alias files similar to your Ralph script’s alias resolution behavior (shell-based + grep fallback). 

---

## 6) CLI redesign: minimal, semantic, discoverable

### 6.1 Design principles

* If you know Docker, you can use Forge.
* Default behavior should “just work” in the current repo.
* Common verbs are top-level:
  `init / up / ps / logs / stop / kill / msg / scale`
* Everything should have:

  * human output by default
  * `--json` for automation
* All “selectors” should be consistent across commands:

  * by loop name / id
  * by repo
  * by pool
  * by state
  * `--all`

### 6.2 Global flags (apply to all commands)

* `-C, --chdir <path>`: operate on a repo directory (like git)
* `--json`: output JSON (single object/array)
* `--jsonl`: output streaming JSON lines (for watch/status)
* `--quiet`: reduce noise
* `--no-color`: plain output
* `--config <path>`: override global config location

### 6.3 Top-level commands

#### `forge init`

Create `.forge/` skeleton (committed) and optional migration.

**What it does**

* creates:

  * `.forge/forge.yaml`
  * `.forge/prompts/`
  * `.forge/templates/`
  * `.forge/sequences/`
  * `.forge/ledgers/`
* optionally creates base `PROMPT.md` if missing
* optionally imports prompts from a directory
* optionally writes a `.gitignore` entry **only for non-committed runtime artifacts** (but `.forge/` stays committed)

**Examples**

* `forge init`
* `forge init --prompts-from ./prompts/`
* `forge init --no-create-prompt`

---

#### `forge up`

Start one or more loops (repo-scoped by default).

**Semantics**

* `forge up` starts **one** loop using:

  * default pool from config (or a default pool if none exists)
  * base prompt `PROMPT.md`
  * default interval
* `-n` starts N loops.
* `--name` gives a specific loop name; without it Forge auto-generates descriptive names.
* loops persist in runtime DB and are listed via `forge ps`.

**Examples**

* `forge up`
  (starts 1 loop using default harness+pool, base prompt PROMPT.md)
* `forge up -n 5 --pool claude`
  (start 5 loops, each picks an available profile from pool)
* `forge up -n 2 --pool codex --prompt .forge/prompts/bugfix.md`
* `forge up --name review --pool claude --prompt .forge/prompts/review.md`
* `forge up -C /path/to/other/repo -n 3 --pool claude`

**Important flags**

* `-n, --count <N>`
* `--pool <pool>`
* `--profile <profile>` (pin loop(s) to one profile instead of pool selection)
* `--prompt <path|prompt-name>` (base prompt for this loop)
* `--interval <duration>`
* `--name <name>` (single loop) / `--name-prefix <prefix>` (for many)
* `--tags <a,b,c>` (for later selection; useful for “review loops”)

---

#### `forge ps`

Show loops.

**Semantics**

* With no flags: shows loops across all repos (like `docker ps`).
* With `-C .`: shows loops for current repo only.

**Examples**

* `forge ps`
* `forge ps -C .`
* `forge ps --json`

**Useful filters**

* `--repo <path>`
* `--pool <pool>`
* `--profile <profile>`
* `--state running|sleeping|waiting|paused|stopped|error`
* `--tag <tag>`

---

#### `forge logs <loop>`

Tail loop logs.

**Examples**

* `forge logs review`
* `forge logs review -f`
* `forge logs --all` (repo)

**Flags**

* `-f, --follow`
* `--since <duration>`
* `--lines <n>`

---

#### `forge stop <loop|selector>`

Graceful stop: the loop stops after current iteration finishes, matching Ralph’s stop-file behavior. 

**Examples**

* `forge stop review`
* `forge stop --all`
* `forge stop --state error` (stop all errored loops)

---

#### `forge kill <loop|selector>`

Immediate stop.

**Examples**

* `forge kill review`
* `forge kill --all`

---

#### `forge msg <loop> "text..."`

Queue a follow-up message (default). This becomes a queue item consumed by the next iteration.

**Examples**

* `forge msg review "Please add tests and run the suite."`

**Flags**

* `--steer`
  If harness supports steering, inject immediately; otherwise fallback to preempt behavior (see `--now`).
* `--next-prompt <path|prompt-name>`
  One-shot override prompt for next iteration, then revert.
* `--template <name>`
  Use stored message template.
* `--seq <name>`
  Enqueue a stored sequence of queue items.
* `--now`
  Preempt: restart current iteration (or steer if possible) so change takes effect immediately.

**Examples**

* `forge msg review --next-prompt .forge/prompts/review.md --now`
* `forge msg oc-claude-02 --template stop-and-refocus --now`
* `forge msg oc-claude-03 --steer "STOP. Switch to code review on auth flow."`

**Selector extensions (v1.1 recommended)**

* `forge msg --pool claude --state idle --template review`
* `forge msg --tag review --now "STOP and handle urgent issue"`

This is key for “spin up many agents quickly and steer them in bulk”.

---

#### `forge scale`

Ensure a repo group has exactly N loops. This is your “fast parallelism” command.

**Examples**

* `forge scale --pool claude --count 10`
* `forge scale review --count 3`

**Semantics**

* If fewer loops exist → start more
* If more loops exist → stop extras (graceful by default; `--kill` optional)

**Flags**

* `--pool <pool>` / `--profile <profile>`
* `--count <N>`
* `--prompt <prompt>`
* `--interval <duration>`
* `--kill` (instead of graceful)
* `--name-prefix <prefix>`

---

#### `forge prompt`

Manage prompts.

**Examples**

* `forge prompt ls`
* `forge prompt add review ./PROMPT.review.md`
  (copies into `.forge/prompts/review.md`)
* `forge prompt edit review`
  (opens editor)
* `forge prompt set-default review`

**Notes**

* Prompt names resolve to `.forge/prompts/<name>.md`
* Allow `forge prompt add <name> --from-url <url>` later, but not required in v1

---

#### `forge profile`

Manage harness+auth profiles (machine-local).

**Examples**

* `forge profile ls`
* `forge profile add opencode --auth-kind claude --home ~/.oc1 --name claude-a`
* `forge profile add opencode --auth-kind claude --home ~/.oc2 --name claude-b`
* `forge profile add opencode --auth-kind claude --home ~/.oc3 --name claude-c`
* `forge profile add opencode --auth-kind codex --home ~/.codex1 --name codex-a`
* `forge profile add opencode --auth-kind codex --home ~/.codex2 --name codex-b`
* `forge profile add pi --auth-kind claude --home ~/.pi1 --name pi-claude-a`

**Required subcommands**

* `forge profile ls`
* `forge profile add …`
* `forge profile edit <name>`
* `forge profile rm <name>`
* `forge profile doctor <name>` (validate home dir structure, required binaries)
* `forge profile cooldown set <name> --until <time|duration>`
* `forge profile cooldown clear <name>`

---

#### `forge pool`

Manage pools (machine-local).

Your earlier message had `forge profile pool …`, but I recommend `forge pool …` as a cleaner top-level noun because pools are not a sub-type of profiles—they’re a composition of profiles.

**Examples**

* `forge pool create claude`
* `forge pool add claude claude-a claude-b claude-c`
* `forge pool add default claude-a codex-a`
* `forge pool ls`
* `forge pool show default`
* `forge pool set-default default`

**Key rule enforced**

* Pools accept any profile references (harness+auth combos), and profiles can be in multiple pools.

---

#### `forge tui`

Launch the dashboard (and optionally make `forge` with no args launch TUI).

---

### 6.4 “Missing but important” CLI commands (strongly recommended)

These are not “more complexity”; they make the CLI self-explanatory and debuggable.

#### `forge doctor`

Checks:

* repo setup (`.forge/` exists, base prompt exists)
* required binaries for configured profiles (opencode, pi, etc.)
* writable dirs, runtime DB health
* shows actionable fixes (“run this command…”)

#### `forge run <loop>`

Runs **one iteration** immediately (no daemon loop). Great for debugging prompt behavior, harness config, and profile auth.

#### `forge queue <loop>`

Inspect and manage queue:

* `forge queue ls <loop>`
* `forge queue clear <loop>`
* `forge queue rm <loop> <item-id>`
* `forge queue move <loop> <item-id> --to front`

This is essential for the “queueing up messages, waiting for usage, continuing” workflow.

---

## 7) Repo layout and config

### 7.1 Committed `.forge/` (project brain)

Recommended:

```
.forge/
  forge.yaml
  prompts/
    review.md
    implement.md
  templates/
    stop-and-refocus.md
    review-request.md
  sequences/
    review.seq.yaml
    implement.seq.yaml
  ledgers/
    loop-review.md
    loop-main-1.md
```

**Policy**

* `.forge/` is committed (not ignored).
* No secrets in `.forge/`.
* Runtime logs, PID, sqlite etc. are machine-local.

### 7.2 Machine-local runtime state (not committed)

* `~/.local/state/forge/` (or platform equivalent)

  * sqlite DB
  * loop logs
  * pid files
  * last-run metadata

This mirrors Ralph’s tmpdir approach but makes it persistent and multi-repo. 

### 7.3 Config separation

* Repo config (`.forge/forge.yaml`) describes:

  * default prompt
  * prompt registry (names)
  * templates/sequences registry
  * recommended pools to use (by name)
  * loop presets (like “review loop”)

* Global config (machine-local) describes:

  * profiles (harness+auth combos)
  * pools (lists of profile refs)
  * default pool name
  * editor preference

---

## 8) TUI requirements (simple, pretty, high-throughput)

The TUI is not “a second interface”; it’s the fastest way to operate many loops.

### 8.1 Core screens

1. **Overview**

   * all repos with active loops
   * last activity
   * counts by state: running / sleeping / waiting / error / stopped

2. **Repo view**

   * list loops in repo
   * each row shows:

     * loop name
     * harness + auth kind
     * profile used (or pool)
     * state
     * queue length
     * last run (time + duration + exit code)
     * cooldown/wait timer if applicable

3. **Loop detail**

   * logs tail
   * ledger view
   * queue editor (list with reorder + delete)
   * “send message” box with template/sequence insertion

### 8.2 Must-have interaction features

* **Command palette** (navigation + actions)
* **Message palette** (templates + sequences)
* **Multi-select** loops + apply message/sequence/pause/stop to many
* **Explain why blocked** (cooldown, concurrency cap, waiting on profile availability, paused)

This is how Forge becomes “impossible to misunderstand” in practice.

---

## 9) Ledger system (per repo, committed)

### 9.1 Why ledger exists

Headless loops lack persistent chat context. Ledger preserves continuity cheaply:

* what just happened
* what changed
* what to do next

### 9.2 Ledger defaults

* per repository
* stored in `.forge/ledgers/`
* committed

### 9.3 Ledger entry format (v1)

Each iteration appends:

* timestamp
* loop name
* profile used (harness/auth kind)
* prompt used (base vs override)
* exit code
* tail of output (N lines)
* optional git summary:

  * `git status --porcelain` summary
  * optionally `git diff --stat` (configurable)

---

## 10) Scheduling: pools, cooldowns, and waiting

### 10.1 Selection algorithm

Given a loop configured with a pool:

* filter pool profiles by:

  * not in cooldown
  * not at max concurrency
* pick by strategy:

  * round robin (default)
  * LRU (optional)
* if none available:

  * set loop to `WAITING_PROFILE`
  * compute next available time (earliest cooldown end)
  * sleep until that time

### 10.2 Cooldown model (v1)

Cooldown is initially manual + policy-based:

* per-profile cooldown after each run (optional)
* manual “set until” via CLI
* later: automatic detection (rate-limit strings)

---

## 11) Harness support v1 and roadmap

### v1: OpenCode headless

* must support:

  * command template with `{prompt}` or env or stdin (Ralph modes) 
* profile controls:

  * home dir isolation
  * model defaults and args

### v1.1: OpenCode + Codex OAuth plugin

* codex profiles are still `harness=opencode` but `auth_kind=codex`
* enable plugin inside that profile’s `auth_home`
* these profiles can sit beside Claude profiles in the same pool

### v2: Pi harness

* pi profiles can be `auth_kind=claude` or `auth_kind=codex`
* key feature: true steering interrupts (when supported)
* Forge behavior:

  * `--steer` uses injection API
  * fallback to restart/preempt if unsupported

---

## 12) Acceptance criteria

### CLI usability

* A new user can run:

  * `forge init`
  * `forge up -n 5`
  * `forge ps`
  * `forge msg <loop> "..."`
  * `forge stop --all`
    without reading docs.

### Throughput

* `forge up -n 20` starts loops within seconds (subject to CPU limits).
* Bulk message to 20 loops via selector finishes enqueue instantly.

### Correctness

* One-shot prompt override works exactly once and reverts.
* Graceful stop stops after current iteration. 
* Profiles isolate auth so 20 loops can run with different subscriptions concurrently.

### TUI

* You can:

  * see all loops and states at a glance
  * multi-select and apply templates
  * inspect queues and “why waiting”

---

## 13) Implementation milestones (pragmatic)

1. **M0 (CLI skeleton + init/up/ps/logs/stop/kill)**
2. **M1 (queue + msg + prompt override-once + scale)**
3. **M2 (profiles + pools + cooldown/wait)**
4. **M3 (TUI overview + repo view + message palette)**
5. **M4 (OpenCode Codex OAuth plugin profiles)**
6. **M5 (Pi harness + steering)**

---

## 14) CLI command recap (exact list you wanted, plus the missing essentials)

**Core**

* `forge init`
* `forge up`
* `forge ps`
* `forge logs`
* `forge stop`
* `forge kill`
* `forge msg`
* `forge scale`
* `forge prompt`
* `forge profile`
* `forge pool`
* `forge tui`

**Strongly recommended additions for clarity**

* `forge doctor`
* `forge queue`
* `forge run` (single iteration debug)
