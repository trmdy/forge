# Grand Vision — Forge

> A composable, agent-native execution system for software work
> combining workflows, teams, jobs, nodes, and always-on loops.

---

## 1. Core Concepts Overview

Forge is built from a small set of primitives:

* **Agents** — autonomous LLM-backed workers
* **Prompts** — instructions and behavior definitions
* **Workflows** — directed graphs of execution
* **Loops** — persistent task execution
* **Teams** — long-lived agent groups
* **Jobs** — triggerable units of work
* **Nodes** — machines that execute work
* **Mesh** — distributed coordination layer

Everything composes.

---

## 2. Forge Taxonomy

### 2.1 Agents

Agents are execution units backed by a model + prompt.

**Agent profile (`agent.md`)**

* Profile / Harness
* Memory
* Ledger
* Execution history

**Supported agent backends**

* Codex
* Claude Code
* Pi
* OpenCode
* AMP
* Droid

---

### 2.2 Prompts

**Prompt definition (`prompt.md`)**

* System prompt
* Autodetection rules
* Loop behavior
* Metadata

```yaml
id
name
ledger
sequence
```

Agents + prompts form executable units.

---

### 2.3 Workflow Runs

A workflow run is a concrete execution instance.

```yaml
id
name
steps
current_step
ledger
```

A workflow run may execute:

* Agent steps
* Bash commands
* Logic (if / branching / fan-out)
* Other workflows

---

## 3. Execution Primitives

### 3.1 Step

A **step** is a single unit of execution:

* Agent + prompt
* Bash command
* Logic node
* External call

Steps may have:

* Pre-hooks
* Post-hooks
* Stop conditions

---

### 3.2 Workflow

A **workflow** is a directed graph of steps.

**Key properties**

* Defined in a self-contained `toml` file
* Shareable and reusable
* Deterministic structure, nondeterministic execution

**Supported constructs**

* Linear sequences
* Fan-out / fan-in
* Conditional branching
* Loops (finite or infinite)
* Human-in-the-loop steps
* Parasitic steps (exist only while another step exists)

> Example: a committer agent that runs every 5 minutes while a build loop is active.

---

## 4. Loops (Ralph Loops)

A **loop** is a persistent execution pattern whose purpose is repetition and improvement.

Typical use cases:

* Task execution
* QA passes
* Refactoring cycles
* Long-running swarms

### Loop CLI

```bash
forge loop            # flp
flp up
flp up --alias <cmd>
flp up --pool <pool>
flp ps
flp rm
flp stop
flp resume
flp send
```

---

## 5. Canonical Workflow Example

### Initial Specification Phase

```
START
  ↓
[Agent] Read PRD and create tasks
  ↓
STOP CONDITION: count(tasks) > 0
  ↓
[Agent] Review tasks and refine
  ↓
STOP CONDITION: LLM confirms coverage
```

---

### Task Execution Loop

```
LOOP:
  [Agent] Pick task and implement
  STOP WHEN: count(tasks.open) == 0
```

(This loop may run N times or infinitely.)

---

### QA & Expansion Phase

```
[Agent] Deep code review → qa.md
  ↓
[Agent] Create new tasks from qa.md
```

---

### Fan-Out Rule

```
IF tasks > 20:
  fan out to multiple agents
ELSE:
  continue single-agent loop
```

---

### Finalization Phase

```
[Agent] Design review
[Agent] Fix design issues
[Agent] Commit work
[Agent] Create GitHub PR
```

---

## 6. Teams

A **team** is a persistent group of agents with identity, memory, and roles.

### Key Characteristics

* Long-lived agents (unlike workflows)
* Named agents with IDs
* Persistent memory
* Continuous execution possible
* Communicate via `fmail`

> Think of teams as departments, agents as employees.

---

### Teams vs Workflows

| Concept  | Teams        | Workflows         |
| -------- | ------------ | ----------------- |
| Lifetime | Persistent   | Ephemeral         |
| Identity | Named agents | Temporary agents  |
| Memory   | Long-term    | Execution-scoped  |
| Purpose  | Ongoing work | One-off execution |

Teams may **invoke workflows**, but workflows do not own teams.

---

### Task Delegation

Tasks can be sent via:

* CLI
* Forge UI
* Webhooks
* Cron jobs
* Direct agent messaging

**Task formats**

* Simple strings
* Arbitrary JSON

Delegation rules are configurable.

**Example**

```json
{
  "type": "design",
  "payload": { ... }
}
```

→ Routed automatically to the Designer agent.

---

### Heartbeats & Always-On Swarms

Teams can be configured as **always-on**:

* Listening to Linear
* Watching Slack
* Monitoring repos
* Continuously improving work

This enables real swarms.

---

### Team Leaders

Teams may designate **leaders**.

* Regular agents do not assume authority
* Suggestions are advisory
* Leader instructions carry higher weight
* Authority is explicit, not implicit

---

## 7. Jobs

A **job** is a triggerable unit of work.

Jobs may:

* Spawn workflows
* Run workflows in parallel or sequence
* Execute bash scripts
* Perform meta-operations on Forge itself
* Run on multiple nodes

**Job properties**

```yaml
input
output
trigger
```

---

## 8. Triggers

A **trigger** starts a job.

Supported triggers:

* CLI
* Cron
* HTTP (webhook)

---

## 9. Nodes

A **node** is a registered machine capable of executing Forge work.

Requirements:

1. Forge installed
2. `forged` daemon running

Nodes execute agent runs locally.

Communication is done over HTTPS with bearer tokens.

---

## 10. Mesh

The **mesh** is the distributed coordination layer.

### Properties

* A node belongs to exactly one mesh
* A mesh has exactly one active master
* Master coordinates task routing
* Automatic master re-election on failure

Forge uses a **client → master → node** model.

---

## 11. Summary Mental Model

* **Workflows** describe *how*
* **Teams** own *who*
* **Loops** provide *persistence*
* **Jobs** define *what*
* **Triggers** define *when*
* **Nodes** define *where*
* **Mesh** defines *coordination*

Everything is composable.

---

If you want, next strong moves would be:

* Split this into `/concepts`, `/execution`, `/cli`, `/architecture`
* Convert workflows into actual TOML examples
* Add one **end-to-end narrative** (“PRD → merged PR”)
* Turn loops into a formal state machine spec

If you want, I can do any of those next.

_Source file referenced: 
