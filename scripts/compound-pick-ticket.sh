#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

cd "$REPO_ROOT"

payload=$(sv task ready --json --limit 1 || true)
if [[ -z "$payload" ]]; then
  exit 0
fi

python3 - "$payload" <<'PY'
import json
import sys

try:
    data = json.loads(sys.argv[1])
except json.JSONDecodeError:
    sys.exit(1)

tasks = data.get("data", {}).get("tasks", [])
if not tasks:
    sys.exit(0)

item = tasks[0]
ident = item.get("id", "")
title = item.get("title", "")

if not ident or not title:
    sys.exit(1)

print(f"{ident}\t{title}")
PY
