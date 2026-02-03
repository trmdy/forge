#!/usr/bin/env bash
set -euo pipefail
IFS=$'\n\t'

log() {
  printf '%s\n' "[forge-skills] $*"
}

fail() {
  printf '%s\n' "[forge-skills] ERROR: $*" >&2
  exit 1
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/.." && pwd)"

source_dir="${FORGE_SKILLS_SOURCE:-${repo_root}/.agent-skills}"
config_path=""
dry_run="false"
delete_extras="false"

usage() {
  cat <<'EOF'
Usage: scripts/install-skills.sh [--config PATH] [--source DIR] [--dry-run] [--delete]

Installs repo skills into harness-specific locations based on Forge config.

Defaults:
  --config  $XDG_CONFIG_HOME/forge/config.yaml, ~/.config/forge/config.yaml, or ./config.yaml
  --source  .agent-skills (repo root)
  --delete  remove files in destination not present in source (requires rsync)
EOF
}

while [ $# -gt 0 ]; do
  case "$1" in
    --config)
      config_path="${2:-}"
      shift 2
      ;;
    --source)
      source_dir="${2:-}"
      shift 2
      ;;
    --dry-run)
      dry_run="true"
      shift
      ;;
    --delete)
      delete_extras="true"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
done

if [ -z "${config_path}" ]; then
  if [ -n "${XDG_CONFIG_HOME:-}" ] && [ -f "${XDG_CONFIG_HOME}/forge/config.yaml" ]; then
    config_path="${XDG_CONFIG_HOME}/forge/config.yaml"
  elif [ -f "${HOME}/.config/forge/config.yaml" ]; then
    config_path="${HOME}/.config/forge/config.yaml"
  elif [ -f "./config.yaml" ]; then
    config_path="./config.yaml"
  fi
fi

if [ -z "${config_path}" ] || [ ! -f "${config_path}" ]; then
  fail "config.yaml not found; pass --config or create ~/.config/forge/config.yaml"
fi

if [ ! -d "${source_dir}" ]; then
  fail "source skills directory not found: ${source_dir}"
fi

extract_profiles() {
  local cfg="$1"

  if python3 - <<'PY' "$cfg" >/dev/null 2>&1; then
import sys
try:
    import yaml  # type: ignore
except Exception:
    sys.exit(1)
PY
  then
    python3 - <<'PY' "$cfg"
import json
import sys
try:
    import yaml  # type: ignore
except Exception:
    sys.exit(1)

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as fh:
    data = yaml.safe_load(fh) or {}

profiles = data.get("profiles") or []
for profile in profiles:
    harness = (profile or {}).get("harness") or ""
    auth_home = (profile or {}).get("auth_home") or ""
    if harness:
        print(f"{harness}|{auth_home}")
PY
    return 0
  fi

  awk '
    function ltrim(s) { sub(/^[ \t]+/, "", s); return s }
    BEGIN { in_profiles=0; harness=""; auth_home="" }
    /^[^ \t]/ { if ($1 != "profiles:") in_profiles=0 }
    /^profiles:/ { in_profiles=1; next }
    in_profiles && /^  - / {
      if (harness != "") {
        print harness "|" auth_home
      }
      harness=""
      auth_home=""
      next
    }
    in_profiles && /^[ \t]+harness:/ {
      harness=ltrim($0)
      sub(/^harness:[ \t]*/, "", harness)
      next
    }
    in_profiles && /^[ \t]+auth_home:/ {
      auth_home=ltrim($0)
      sub(/^auth_home:[ \t]*/, "", auth_home)
      next
    }
    END {
      if (harness != "") {
        print harness "|" auth_home
      }
    }
  ' "$cfg"
}

resolve_dest() {
  local harness="$1"
  local auth_home="$2"

  case "$harness" in
    codex)
      if [ -n "$auth_home" ]; then
        printf '%s/skills' "$auth_home"
      else
        printf '%s/.codex/skills' "$HOME"
      fi
      ;;
    claude|claude_code)
      if [ -n "$auth_home" ]; then
        printf '%s/skills' "$auth_home"
      else
        printf '%s/.claude/skills' "$HOME"
      fi
      ;;
    opencode)
      if [ -n "$auth_home" ]; then
        printf '%s/skills' "$auth_home"
      else
        printf '%s/.config/opencode/skills' "$HOME"
      fi
      ;;
    pi)
      if [ -n "$auth_home" ]; then
        printf '%s/skills' "$auth_home"
      else
        printf '%s/.pi/skills' "$HOME"
      fi
      ;;
    *)
      printf ''
      ;;
  esac
}

sync_dir() {
  local src="$1"
  local dest="$2"

  if [ "$dry_run" = "true" ]; then
    log "would install skills from ${src} -> ${dest}"
    return 0
  fi

  mkdir -p "$dest"
  if command -v rsync >/dev/null 2>&1; then
    if [ "$delete_extras" = "true" ]; then
      rsync -a --delete "${src}/" "${dest}/"
    else
      rsync -a "${src}/" "${dest}/"
    fi
  else
    if [ "$delete_extras" = "true" ]; then
      fail "--delete requires rsync"
    fi
    cp -R "${src}/." "${dest}/"
  fi
}

log "using config: ${config_path}"
log "using skills source: ${source_dir}"

installed=0
skipped=0
while IFS="|" read -r harness auth_home; do
  dest="$(resolve_dest "$harness" "$auth_home")"
  if [ -z "$dest" ]; then
    log "skipping harness '${harness}' (no skills destination configured)"
    skipped=$((skipped + 1))
    continue
  fi

  sync_dir "$source_dir" "$dest"
  log "installed skills for ${harness} -> ${dest}"
  installed=$((installed + 1))
done < <(extract_profiles "$config_path")

log "done (installed: ${installed}, skipped: ${skipped})"
