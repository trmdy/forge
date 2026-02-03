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

source_dir="${repo_root}/.agent-skills"
dest_dir="${repo_root}/internal/skills/builtin"

if [ ! -d "${source_dir}" ]; then
  fail "source skills directory not found: ${source_dir}"
fi

mkdir -p "${dest_dir}"
if command -v rsync >/dev/null 2>&1; then
  rsync -a --delete "${source_dir}/" "${dest_dir}/"
else
  rm -rf "${dest_dir}"
  mkdir -p "${dest_dir}"
  cp -R "${source_dir}/." "${dest_dir}/"
fi

log "synced embedded skills from ${source_dir} -> ${dest_dir}"
