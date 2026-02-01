#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
LOG_DIR="$REPO_ROOT/logs/compound"
LEDGER_DIR="$REPO_ROOT/.forge/ledgers"
TIMESTAMP=$(date +"%Y%m%d-%H%M%S")
LOG_JSON="$LOG_DIR/compound-review-$TIMESTAMP.jsonl"

mkdir -p "$LOG_DIR"

cd "$REPO_ROOT"

# Allow untracked logs/review notes without blocking automation
DIRTY=$(git status --porcelain)
DIRTY_FILTERED=$(echo "$DIRTY" | rg -v "^\?\? (logs/|docs/review/)" || true)
if [[ -n "$DIRTY_FILTERED" ]]; then
  echo "Working tree dirty; aborting compound review." >&2
  echo "$DIRTY_FILTERED" >&2
  exit 1
fi

git checkout main
git pull --rebase

SINCE=$(python3 - <<'PY'
from datetime import datetime, timedelta, timezone
print((datetime.now(timezone.utc) - timedelta(days=1)).strftime("%Y-%m-%dT%H:%M:%SZ"))
PY
)

LOG_FILES=$(find "$LOG_DIR" -type f -mtime -1 -print | sort || true)
LEDGER_FILES=""
if [[ -d "$LEDGER_DIR" ]]; then
  LEDGER_FILES=$(find "$LEDGER_DIR" -type f -mtime -1 -print | sort || true)
fi

cat <<EOF | codex exec --json -a never -s workspace-write -C "$REPO_ROOT" - > "$LOG_JSON"
You are running the nightly compound review for this repo.

Goals:
- Extract durable learnings from the last 24h of work.
- Update only AGENTS.md in this repo with concise bullets (patterns, pitfalls, conventions).
- Do not change code or tickets. No pushes.

Inputs:
- AGENTS.md
- Git log since: $SINCE (use: git log --since="$SINCE")
- If log files exist below, scan them for errors/gotchas:
$LOG_FILES
- If ledger files exist below, scan for learnings:
$LEDGER_FILES

Rules:
- Keep edits minimal and specific.
- Prefer actionable guidance.
