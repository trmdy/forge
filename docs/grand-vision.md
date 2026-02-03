# Grand Vision (2026-01-20)

## Summary

Forge evolves from loop runner to workflow orchestration plane for multi-agent, multi-node work.

## Core concepts (expanded)

- **Agent**: LLM-backed worker (model + prompt + profile/harness).
- **Prompt**: instruction + behavior definition; can include autodetection rules.
- **Workflow**: directed graph of steps (agent, bash, loop, logic, human, job, workflow).
- **Loop**: persistent execution pattern (aka ralph loop).
- **Team**: long-lived group of agents with identity + roles + delegation rules.
- **Job**: triggerable unit of work that runs one or more workflows.
- **Trigger**: cron/webhook/CLI start signal for a job.
- **Node**: registered machine that can execute forge work (runs `forged`).
- **Mesh**: set of nodes with one active master coordinating routing.

## Agents + prompts (proposed files)

**Central registry (recommended)**

- Global source of truth lives on each node (default: `~/.forge/registry/`)
- Repo export for sharing: `.forge/registry/` (commit-friendly)
- Commands: `forge registry export/import` (push/pull)
- Dashboard can query remote registry via master routing

**Agent profile (`agents/*.yaml`)**

- id, name
- harness/profile (codex, claude, opencode, amp, pi, droid)
- memory + ledger scope
- execution history metadata

**Prompt definition (`prompts/*.md`)**

- id, name
- system prompt + metadata
- selection rules (optional)
- loop behavior
- sequence

Agents + prompts combine into runnable units.

## Prompt selection (defaults)

Used when task/workflow does not specify `prompt_id`.

Precedence:

1. explicit `prompt_id` on task/workflow step
2. team rule match
3. repo default
4. global default

Rule order: higher priority → more specific → first-defined.

## Workflow runs (runtime model)

A workflow run is the concrete execution instance.

Fields (proposed):

- id
- name
- steps
- current_step
- ledger

## How it integrates with current Forge

- **Loop** stays core primitive. `forge up/ps/msg/queue` map to `forge loop` subcommands.
- **Profiles/pools** continue as harness selection for agent steps.
- **Ledger** expands from per-loop to per-workflow/job audit trail.
- **Repo config** (`.forge/forge.yaml`) remains. New workflow files live alongside prompts.
- **TUI/CLI** gain workflow + job views, but loop views remain first-class.

## Teams

Teams are persistent groups of agents with identity, memory, and roles.

Key traits:

- long-lived agents (vs workflows which are ephemeral)
- team IDs + names; optional leader role
- delegation rules route tasks to the right agent
- agents communicate via `fmail`
- persisted per node under `~/.forge` (default)

Task delegation:

- tasks can be sent to a team (CLI/UI/webhook/cron)
- tasks can be strings or JSON payloads
- rules can route based on payload keys (e.g. `type: "design"`)
- team members may spawn workflows via CLI using injected instructions

Task payload (JSON, minimal):

- `type`, `title`, `body`, `priority`
- optional: `repo`, `tags`, `workflow`, `inputs`, `routing_hint`, `meta`

Always-on teams:

- heartbeats keep teams active
- can ingest tasks from external systems (Linear/Slack/etc)
- heartbeat interval configurable per team

## Jobs + triggers

Jobs encapsulate work and can:

- spawn workflows (sequence or parallel)
- run bash scripts
- operate across nodes

Triggers:

- CLI, cron, webhook
- inputs/outputs stored per job run
- webhook auth: bearer token (rotation support)

## Nodes + mesh

- node = registered machine with `forged` daemon
- mesh = set of nodes with one active master
- client → master → node routing model
- HTTPS with bearer token for remote commands

## Loops (ralph)

CLI spec:

- `forge loop` (alias `flp`)
- `flp up|ps|rm|stop|resume|send`

## Workflow file format (TOML) - proposal

Location: `.forge/workflows/<name>.toml`

### Top-level

- `name` (string)
- `version` (string)
- `description` (string)
- `inputs` (table)
- `outputs` (table)
- `steps` (array of tables)
- `hooks` (table, optional)

### Step fields (common)

- `id` (string, unique)
- `name` (string, optional)
- `type` (string: `agent|loop|bash|logic|job|workflow|human`)
- `depends_on` (array of step ids)
- `when` (expr string, optional)
- `inputs` (table, optional)
- `outputs` (table, optional)
- `stop` (table, optional)
- `hooks` (table, optional)
- `alive_with` (array of step ids, optional; parasitic)

### Step type details

- `agent`
  - `prompt` (string or prompt ref)
  - `profile` or `pool`
  - `max_runtime` (duration)
- `loop`
  - `prompt` (string or prompt ref)
  - `profile` or `pool`
  - `interval` (duration)
  - `max_iterations` (int)
  - `stop` (table)
- `bash`
  - `cmd` (string)
  - `workdir` (string, optional)
- `logic`
  - `if` (expr string)
  - `then` (array of step ids)
  - `else` (array of step ids)
- `job`
  - `job_name` (string)
  - `params` (table)
- `workflow`
  - `workflow_name` (string)
  - `params` (table)
- `human`
  - `prompt` (string)
  - `timeout` (duration, optional)

### Stop conditions (examples)

- `stop.expr = "count(tasks.open) == 0"`
- `stop.tool = { name = "tk", args = ["ready"] }`
- `stop.llm = { rubric = "coverage", pass_if = "good" }`

### Hooks (examples)

- `hooks.pre = ["bash:./scripts/preflight.sh"]`
- `hooks.post = ["bash:./scripts/collect-logs.sh"]`

## Example workflow

```toml
name = "spec-to-ship"
version = "0.1"

type = "workflow"

[inputs]
repo = "."

[[steps]]
id = "read_prd"
type = "agent"
name = "Read PRD, create tasks"
prompt = "prompts/read-prd.md"
profile = "oc1"

[[steps]]
id = "review_tasks"
type = "agent"
name = "Review tasks, break down"
prompt = "prompts/review-tasks.md"
depends_on = ["read_prd"]

[[steps]]
id = "impl_loop_a"
type = "loop"
name = "Implement tasks"
prompt = "prompts/impl.md"
pool = "default"
interval = "30s"
stop.expr = "count(tasks.open) == 0"

[[steps]]
id = "impl_loop_b"
type = "loop"
name = "Implement tasks"
prompt = "prompts/impl.md"
pool = "default"
interval = "30s"
stop.expr = "count(tasks.open) == 0"

[[steps]]
id = "qa_review"
type = "agent"
name = "Deep code review -> qa.md"
prompt = "prompts/qa.md"
depends_on = ["impl_loop_a", "impl_loop_b"]

[[steps]]
id = "qa_tasks"
type = "agent"
name = "Create tasks from qa.md"
prompt = "prompts/qa-to-tasks.md"
depends_on = ["qa_review"]

[[steps]]
id = "fanout_gate"
type = "logic"
if = "count(tasks.open) > 20"
then = ["impl_loop_a", "impl_loop_b"]
else = ["design_review"]

[[steps]]
id = "design_review"
type = "agent"
name = "Design review"
prompt = "prompts/design-review.md"
depends_on = ["qa_tasks"]

[[steps]]
id = "fix_design"
type = "loop"
name = "Fix design issues"
prompt = "prompts/fix-design.md"
stop.expr = "count(tasks.open) == 0"

[[steps]]
id = "commit_pr"
type = "agent"
name = "Commit + PR"
prompt = "prompts/commit-pr.md"
depends_on = ["fix_design"]
```

## CLI mapping suggestion

**Keep compatibility**: `forge up/ps/msg/stop` remain as aliases to loop commands.

**Canonical structure**:

- `forge loop up|ps|msg|queue|stop|resume|rm|prune|scale|run`
- `forge workflow run|ls|show|graph|validate`
- `forge job run|ls|show|logs|cancel`
- `forge trigger add|ls|rm` (cron/webhook)
- `forge node add|ls|doctor|exec|bootstrap`
- `forge mesh status|promote|demote|join|leave`
- `forge team ls|new|rm|show|member add|rm`
- `forge task send|ls|show|assign|retry`
- `forge registry ls|export|import|status`
- `forge agent ls|show|validate`
- `forge prompt ls|show|validate`

**Examples**:

- `forge loop up --count 2`
- `forge workflow run spec-to-ship --input repo=.`
- `forge job run nightly-qa --trigger cron:0 2 * * *`
- `forge trigger add webhooks/ship --job spec-to-ship`

## Open questions

- Workflow stop conditions: deterministic only, or allow LLM evals?
- Human-in-loop: where approvals live (repo vs global db)?
- Master election: static vs automatic failover for mesh.
- Delegation rule language (final syntax).
- Registry conflict resolution (local vs repo).

## Autodetection (local harnesses + aliases)

Purpose: detect installed harness CLIs and alias patterns on the local node.

Inputs:

- CLI existence: `codex`, `claude`, `opencode`, `pi`, `droid`, `amp`
- Shell aliases from `~/.zsh_aliases` (optional: `zsh -ic 'alias'`)

Outputs:

- list of detected harnesses
- alias mapping (e.g. `cc1` → `claude --profile A`)
- proposed profile stubs (without credentials)

## Profiles + mesh sync (later)

Goal: fast provisioning of many profiles on new nodes without sharing credentials.

Concept:

- Central profile catalog defines how many profiles per harness type.
- Profiles are instantiated on each node as non-alias canonical IDs (e.g. `CC1`, `Codex1`).
- Nodes report auth status per profile (authenticated/expired/missing).
- Master can show overview across mesh; credentials never synced.

## Roadmap (sv milestones)

- **M1** Workflow spec + CLI (non-exec)
- **M2** Workflow runner (sequential)
- **M3** Stop conditions, IO binding, hooks, fan-out
- **M4** Human-in-loop steps + approvals
- **M5** Jobs + triggers (cron/webhook)
- **M6** Nodes + mesh routing
- **M7** Teams + task delegation (new)
- **M8** Agent profiles + prompt registry/autodetection (new)
- **M9** Always-on teams + external task ingestion (new)
