#!/usr/bin/env bash
set -euo pipefail

REPO_ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
STATE_ROOT=${TMPDIR:-/tmp}
STATE_DIR="$STATE_ROOT/ralph-$(printf "%s" "$REPO_ROOT" | shasum -a 256 | awk '{print $1}')"
PID_FILE="$STATE_DIR/ralph.pid"
LOG_FILE="$STATE_DIR/ralph.log"
STOP_FILE="$STATE_DIR/ralph.stop"

usage() {
  cat <<'USAGE'
Usage: scripts/ralph.sh <start|stop|status|tail> [options]

Commands:
  start        Start the Ralph loop in the background
  stop         Gracefully stop after current loop finishes
  kill         Force stop immediately
  status       Show whether the loop is running
  tail         Tail the loop log

Start options:
  --cmd "..."        Command to run each loop (or set RALPH_CMD)
  --alias NAME       Use a shell alias from ~/.zsh_aliases (or RALPH_ALIAS_FILE)
  --prompt PATH      Prompt file (default: PROMPT.md or RALPH_PROMPT)
  --sleep SECONDS    Delay between loops (default: 2 or RALPH_SLEEP)

Notes:
  - The loop logs to $TMPDIR/ralph-<hash>/ralph.log
  - If the command contains "{prompt}" it will be replaced with the prompt path.
  - Otherwise the prompt is sent on stdin.
  - Alias names oc1/oc2/oc3, codex1/codex2, cc1/cc2/cc3 auto-map to headless mode.
    Use RALPH_OPENCODE_MODEL to override the default OpenCode model.
  - If --cmd is set to a known alias name, it will be resolved the same way as --alias.
  - RALPH_CODEX_CONFIG can force a specific Codex config.toml path.
  - RALPH_CODEX_SANDBOX can override CODEX_SANDBOX for the loop process.
  - If no sandbox flag is in the codex command, Ralph appends --sandbox from config.toml.

Examples:
  RALPH_CMD='codex' scripts/ralph.sh start
  RALPH_CMD='npx --yes @sourcegraph/amp' scripts/ralph.sh start
  scripts/ralph.sh start --alias oc1
  scripts/ralph.sh start --cmd 'codex' --prompt PROMPT.md --sleep 5
  scripts/ralph.sh stop
USAGE
}

is_running() {
  if [[ -f "$PID_FILE" ]]; then
    local pid
    pid=$(cat "$PID_FILE")
    if kill -0 "$pid" 2>/dev/null; then
      return 0
    fi
  fi
  return 1
}

resolve_alias() {
  local alias_name=$1
  local alias_output=""

  if ! alias_output=$(get_alias_output "$alias_name"); then
    echo "Alias not found: $alias_name" >&2
    exit 2
  fi

  parse_alias_command "$alias_output" "$alias_name"
}

get_alias_output() {
  local alias_name=$1
  local alias_file=${RALPH_ALIAS_FILE:-"$HOME/.zsh_aliases"}
  local shell_path=${RALPH_ALIAS_SHELL:-${SHELL:-/bin/zsh}}
  local alias_output=""

  if [[ -x "$shell_path" ]]; then
    alias_output=$("$shell_path" -lc "source \"$alias_file\" >/dev/null 2>&1; alias $alias_name" 2>/dev/null || true)
  fi

  if [[ -z "$alias_output" && -f "$alias_file" ]]; then
    alias_output=$(grep -E "^alias[[:space:]]+$alias_name=" "$alias_file" || true)
  fi

  if [[ -z "$alias_output" ]]; then
    return 1
  fi

  printf '%s\n' "$alias_output"
}

parse_alias_command() {
  local alias_output=$1
  local alias_name=$2

  alias_output=${alias_output%%$'\n'*}
  alias_output=${alias_output#alias }
  if [[ "$alias_output" == "$alias_name="* ]]; then
    alias_output=${alias_output#"$alias_name="}
  elif [[ "$alias_output" == *"="* ]]; then
    alias_output=${alias_output#*=}
  fi

  alias_output=${alias_output#\'}
  alias_output=${alias_output%\'}
  alias_output=${alias_output#\"}
  alias_output=${alias_output%\"}

  echo "$alias_output"
}

first_non_env_token() {
  local cmd=$1
  local token=""
  for token in $cmd; do
    if [[ "$token" == "env" ]]; then
      continue
    fi
    if [[ "$token" =~ ^[A-Za-z_][A-Za-z0-9_]*= ]]; then
      continue
    fi
    printf '%s\n' "$token"
    return 0
  done
  return 1
}

is_codex_cmd() {
  local cmd=$1
  local first=""
  first=$(first_non_env_token "$cmd" || true)
  [[ "$first" == "codex" || "$first" == */codex ]]
}

detect_codex_home() {
  if [[ -n "${CODEX_HOME:-}" && -d "${CODEX_HOME}" ]]; then
    printf '%s\n' "$CODEX_HOME"
    return 0
  fi

  if [[ -d "$HOME/.codex" ]]; then
    printf '%s\n' "$HOME/.codex"
    return 0
  fi

  if [[ -d "$HOME/codex" ]]; then
    printf '%s\n' "$HOME/codex"
    return 0
  fi

  if [[ -d "$HOME/Codex" ]]; then
    printf '%s\n' "$HOME/Codex"
    return 0
  fi

  find "$HOME" -maxdepth 2 -type d \( -name ".codex" -o -name "codex" -o -name "Codex" \) 2>/dev/null \
    | head -n 1
}

detect_codex_sandbox() {
  local cfg_path=$1
  if [[ -z "$cfg_path" || ! -f "$cfg_path" ]]; then
    return 1
  fi

  awk -F= '
    /^[[:space:]]*sandbox_mode[[:space:]]*=/ {
      val=$2
      gsub(/^[[:space:]]+|[[:space:]]+$/, "", val)
      gsub(/"/, "", val)
      if (val != "") {
        print val
        exit 0
      }
    }
  ' "$cfg_path"
}

headless_alias_cmd() {
  local alias_name=$1
  local alias_cmd=$2
  local opencode_model=${RALPH_OPENCODE_MODEL:-"anthropic/claude-opus-4-5"}
  local prompt_mode=$3

  case "$alias_name" in
    oc1|oc2|oc3)
      printf '%s\n%s\n' \
        "${alias_cmd} run --model ${opencode_model} \"\$RALPH_PROMPT_CONTENT\"" \
        "env"
      return 0
      ;;
    codex1|codex2)
      printf '%s\n%s\n' \
        "${alias_cmd} exec -" \
        "stdin"
      return 0
      ;;
    cc1|cc2|cc3)
      printf '%s\n%s\n' \
        "${alias_cmd} -p \"\$RALPH_PROMPT_CONTENT\"" \
        "env"
      return 0
      ;;
  esac

  printf '%s\n%s\n' "${alias_cmd}" "$prompt_mode"
}

start_loop() {
  local prompt=${RALPH_PROMPT:-"$REPO_ROOT/PROMPT.md"}
  local sleep_seconds=${RALPH_SLEEP:-2}
  local cmd=${RALPH_CMD:-}
  local alias_name=""
  local prompt_mode="stdin"
  local codex_home=""
  local codex_config=""
  local codex_sandbox=""

  while [[ $# -gt 0 ]]; do
    case "$1" in
      --cmd)
        cmd=$2
        shift 2
        ;;
      --alias)
        alias_name=$2
        shift 2
        ;;
      --prompt)
        prompt=$2
        shift 2
        ;;
      --sleep)
        sleep_seconds=$2
        shift 2
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        echo "Unknown option: $1" >&2
        usage
        exit 2
        ;;
    esac
  done

  if [[ -n "$alias_name" ]]; then
    if [[ -n "$cmd" ]]; then
      echo "Use either --cmd or --alias, not both." >&2
      exit 2
    fi
    cmd=$(resolve_alias "$alias_name")
    IFS=$'\n' read -r cmd prompt_mode < <(headless_alias_cmd "$alias_name" "$cmd" "$prompt_mode")
  elif [[ -n "$cmd" && "$cmd" != *[[:space:]]* ]]; then
    if alias_output=$(get_alias_output "$cmd"); then
      alias_name=$cmd
      cmd=$(parse_alias_command "$alias_output" "$alias_name")
      IFS=$'\n' read -r cmd prompt_mode < <(headless_alias_cmd "$alias_name" "$cmd" "$prompt_mode")
    fi
  fi

  if [[ -z "$cmd" ]]; then
    echo "Missing command. Set RALPH_CMD or pass --cmd/--alias." >&2
    exit 2
  fi

  if is_codex_cmd "$cmd"; then
    local first=${cmd%% *}
    if [[ "$cmd" == "$first" ]]; then
      cmd="$first exec -"
      prompt_mode="stdin"
    fi
    if [[ -n "${RALPH_CODEX_CONFIG:-}" ]]; then
      codex_config="$RALPH_CODEX_CONFIG"
    elif [[ -n "${CODEX_CONFIG:-}" ]]; then
      codex_config="$CODEX_CONFIG"
    else
      codex_home=$(detect_codex_home || true)
      if [[ -z "$codex_home" ]]; then
        echo "Codex home not found under $HOME. Skipping Codex config/sandbox detection." >&2
      else
        local cfg_path="$codex_home/config.toml"
        if [[ -f "$cfg_path" ]]; then
          codex_config="$cfg_path"
        fi
      fi
    fi
    if [[ -n "${RALPH_CODEX_SANDBOX:-}" ]]; then
      codex_sandbox="$RALPH_CODEX_SANDBOX"
    elif [[ -n "$codex_config" ]]; then
      codex_sandbox=$(detect_codex_sandbox "$codex_config" || true)
    elif [[ -n "${CODEX_SANDBOX:-}" ]]; then
      codex_sandbox="$CODEX_SANDBOX"
    fi
    if [[ -n "$codex_sandbox" && "$codex_sandbox" != "workspace-write" && "$cmd" == *"--full-auto"* ]]; then
      cmd=${cmd/--full-auto/}
      cmd=$(printf '%s\n' "$cmd" | awk '{$1=$1;print}')
    fi
    if [[ -n "$codex_sandbox" && "$cmd" == *"--dangerously-bypass-approvals-and-sandbox"* ]]; then
      cmd=${cmd/--dangerously-bypass-approvals-and-sandbox/}
      cmd=$(printf '%s\n' "$cmd" | awk '{$1=$1;print}')
    fi
    if [[ "$cmd" != *"--sandbox "* && "$cmd" != *"--sandbox="* ]]; then
      if [[ -n "$codex_sandbox" ]]; then
        if [[ "$cmd" == *" -" ]]; then
          cmd="${cmd% -} --sandbox $codex_sandbox -"
        else
          cmd="$cmd --sandbox $codex_sandbox"
        fi
      fi
    fi
  fi

  if [[ "$prompt" != /* ]]; then
    prompt="$REPO_ROOT/$prompt"
  fi

  if [[ ! -f "$prompt" ]]; then
    echo "Prompt file not found: $prompt" >&2
    exit 2
  fi

  if is_running; then
    echo "Ralph loop already running (pid $(cat "$PID_FILE"))."
    exit 0
  fi

  mkdir -p "$STATE_DIR"
  rm -f "$STOP_FILE"

  (
    cd "$REPO_ROOT"
    export RALPH_PROMPT_FILE="$prompt"
    if [[ -n "$codex_home" ]]; then
      export CODEX_HOME="$codex_home"
    fi
    if [[ -n "$codex_config" ]]; then
      export CODEX_CONFIG="$codex_config"
    fi
    if is_codex_cmd "$cmd"; then
      if [[ -n "${RALPH_CODEX_SANDBOX:-}" ]]; then
        export CODEX_SANDBOX="$RALPH_CODEX_SANDBOX"
      else
        unset CODEX_SANDBOX
      fi
    fi
    printf -- "Ralph loop started at %s\n" "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" >> "$LOG_FILE"
    local loop_count=0
    while :; do
      if [[ -f "$STOP_FILE" ]]; then
        printf -- "Ralph loop stopped gracefully at %s\n" "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" >> "$LOG_FILE"
        rm -f "$STOP_FILE" "$PID_FILE"
        exit 0
      fi
      loop_count=$((loop_count + 1))
      if [[ "$cmd" == *"{prompt}"* ]]; then
        bash -lc "${cmd//\{prompt\}/$prompt}" >> "$LOG_FILE" 2>&1
      elif [[ "$prompt_mode" == "env" ]]; then
        RALPH_PROMPT_CONTENT="$(cat "$prompt")" bash -lc "$cmd" >> "$LOG_FILE" 2>&1
      else
        bash -lc "$cmd" < "$prompt" >> "$LOG_FILE" 2>&1
      fi
      printf -- "---- loop %s finished %s ----\n" "$loop_count" "$(date -u +"%Y-%m-%dT%H:%M:%SZ")" >> "$LOG_FILE"
      sleep "$sleep_seconds"
    done
  ) &

  echo $! > "$PID_FILE"
  echo "Ralph loop started (pid $(cat "$PID_FILE")). Log: $LOG_FILE"
}

stop_loop() {
  if [[ ! -f "$PID_FILE" ]]; then
    echo "Ralph loop not running (no pid file)."
    exit 0
  fi

  local pid
  pid=$(cat "$PID_FILE")
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "Ralph loop not running (stale pid $pid)."
    rm -f "$PID_FILE"
    exit 0
  fi

  touch "$STOP_FILE"
  echo "Ralph loop will stop after current iteration (pid $pid). Use 'kill' to force stop."
}

kill_loop() {
  if [[ ! -f "$PID_FILE" ]]; then
    echo "Ralph loop not running (no pid file)."
    exit 0
  fi

  local pid
  pid=$(cat "$PID_FILE")
  if ! kill -0 "$pid" 2>/dev/null; then
    echo "Ralph loop not running (stale pid $pid)."
    rm -f "$PID_FILE" "$STOP_FILE"
    exit 0
  fi

  kill "$pid"
  sleep 0.2
  if kill -0 "$pid" 2>/dev/null; then
    echo "Ralph loop still running (pid $pid), sending SIGKILL."
    kill -9 "$pid" 2>/dev/null || true
    sleep 0.2
  fi

  rm -f "$PID_FILE" "$STOP_FILE"
  echo "Ralph loop killed."
}

status_loop() {
  if is_running; then
    echo "Ralph loop running (pid $(cat "$PID_FILE"))."
  else
    echo "Ralph loop not running."
  fi
}

tail_loop() {
  if [[ ! -f "$LOG_FILE" ]]; then
    echo "No log file at $LOG_FILE" >&2
    exit 1
  fi
  tail -f "$LOG_FILE"
}

case "${1:-}" in
  start)
    shift
    start_loop "$@"
    ;;
  stop)
    stop_loop
    ;;
  kill)
    kill_loop
    ;;
  status)
    status_loop
    ;;
  tail)
    tail_loop
    ;;
  -h|--help|help|"")
    usage
    ;;
  *)
    echo "Unknown command: $1" >&2
    usage
    exit 2
    ;;
esac
