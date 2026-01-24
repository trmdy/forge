# Grand Vision (2026-01-20)

## Summary

Forge evolves from loop runner to workflow orchestration plane for multi-agent, multi-node work.

## How it integrates with current Forge

- **Loop** stays core primitive. `forge up/ps/msg/queue` map to `forge loop` subcommands.
- **Profiles/pools** continue as harness selection for agent steps.
- **Ledger** expands from per-loop to per-workflow/job audit trail.
- **Repo config** (`.forge/forge.yaml`) remains. New workflow files live alongside prompts.
- **TUI/CLI** gain workflow + job views, but loop views remain first-class.

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

**Examples**:

- `forge loop up --count 2`
- `forge workflow run spec-to-ship --input repo=.`
- `forge job run nightly-qa --trigger cron:0 2 * * *`
- `forge trigger add webhooks/ship --job spec-to-ship`

## Open questions

- Workflow stop conditions: deterministic only, or allow LLM evals?
- Human-in-loop: where approvals live (repo vs global db)?
- Master election: static vs automatic failover for mesh.
