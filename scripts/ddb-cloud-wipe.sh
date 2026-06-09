#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="${SCRIPT_PROJECT_ROOT}/configs/cloud/control-plane.local.json"
ACTION="plan"
CONFIRM=""

usage() {
  cat <<'USAGE'
usage:
  ./scripts/ddb-cloud-wipe.sh [-Config path] [-Action plan|wipe] [-Confirm WIPE]

description:
  Wipes machine-level runtime state under install_root while preserving:
  - bin/
  - scripts/
  - configs/

  Removed directories:
  - <install_root>/data
  - <install_root>/logs
  - <install_root>/state

examples:
  ./scripts/ddb-cloud-wipe.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action plan
  ./scripts/ddb-cloud-wipe.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action wipe -Confirm WIPE
USAGE
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -Config)
      CONFIG="$2"
      shift 2
      ;;
    -Action)
      ACTION="$2"
      shift 2
      ;;
    -Confirm)
      CONFIRM="$2"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "unknown argument: $1" >&2
      usage >&2
      exit 1
      ;;
  esac
done

case "$ACTION" in
  plan|wipe) ;;
  *)
    echo "unsupported action: $ACTION" >&2
    exit 1
    ;;
esac

log() {
  printf '[INFO] %s\n' "$*"
}

fail() {
  printf '[ERROR] %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

require_path_exists() {
  [[ -e "$1" ]] || fail "required path not found: $1"
}

ensure_dir() {
  mkdir -p "$1"
}

load_context() {
  local config_path="$1"
  local env_file
  env_file="$(mktemp)"
  python3 - "$config_path" >"$env_file" <<'PY'
import json
import os
import shlex
import sys

config_path = os.path.abspath(sys.argv[1])
with open(config_path, "r", encoding="utf-8") as handle:
    cfg = json.load(handle)

config_dir = os.path.dirname(config_path)

def resolve_path(value, root_dir, config_dir):
    if not value:
        return ""
    value = os.path.expandvars(value)
    if os.path.isabs(value):
        return os.path.abspath(value)
    base = root_dir
    if value.startswith("./") or value.startswith("../"):
        base = config_dir
    return os.path.abspath(os.path.join(base, value))

root_input = str(cfg.get("install_root", "")).strip() or str(cfg.get("project_root", "")).strip()
if not root_input:
    raise SystemExit("config requires install_root")

install_root = resolve_path(root_input, config_dir, config_dir)
machine_name = str(cfg.get("machine_name", "")).strip() or os.path.basename(config_path)
data_root = os.path.abspath(os.path.join(install_root, "data"))
logs_root = os.path.abspath(os.path.join(install_root, "logs"))
state_root = os.path.abspath(os.path.join(install_root, "state"))

values = {
    "CONFIG_PATH": config_path,
    "INSTALL_ROOT": install_root,
    "MACHINE_NAME": machine_name,
    "DATA_ROOT": data_root,
    "LOGS_ROOT": logs_root,
    "STATE_ROOT": state_root,
}

for key, value in values.items():
    print(f"{key}={shlex.quote(str(value))}")
PY
  # shellcheck disable=SC1090
  source "$env_file"
  rm -f "$env_file"
}

assert_safe_machine_root() {
  [[ -n "$INSTALL_ROOT" ]] || fail "install_root is empty"
  [[ "$INSTALL_ROOT" != "/" ]] || fail "refusing to wipe root directory"
  [[ -d "$INSTALL_ROOT" ]] || fail "install_root does not exist: $INSTALL_ROOT"
  [[ "$DATA_ROOT" == "$INSTALL_ROOT/"* ]] || fail "data root is outside install_root: $DATA_ROOT"
  [[ "$LOGS_ROOT" == "$INSTALL_ROOT/"* ]] || fail "logs root is outside install_root: $LOGS_ROOT"
  [[ "$STATE_ROOT" == "$INSTALL_ROOT/"* ]] || fail "state root is outside install_root: $STATE_ROOT"
}

running_processes() {
  ps -ef | grep -F "${INSTALL_ROOT}/bin/ddb-server" | grep -v grep || true
  ps -ef | grep -F "${INSTALL_ROOT}/bin/etcd" | grep -v grep || true
}

print_plan() {
  log "config=$CONFIG_PATH"
  log "machine=$MACHINE_NAME"
  log "install_root=$INSTALL_ROOT"
  printf 'will remove:\n'
  printf '  %s\n' "$DATA_ROOT"
  printf '  %s\n' "$LOGS_ROOT"
  printf '  %s\n' "$STATE_ROOT"
  printf 'will keep:\n'
  printf '  %s\n' "$INSTALL_ROOT/bin"
  printf '  %s\n' "$INSTALL_ROOT/scripts"
  printf '  %s\n' "$INSTALL_ROOT/configs"
}

wipe_directory() {
  local dir="$1"
  rm -rf "$dir"
  ensure_dir "$dir"
}

perform_wipe() {
  [[ "$CONFIRM" == "WIPE" ]] || fail "wipe requires -Confirm WIPE"
  local procs
  procs="$(running_processes)"
  if [[ -n "$procs" ]]; then
    printf '%s\n' "$procs" >&2
    fail "found running ddb-server/etcd processes under $INSTALL_ROOT; stop them first"
  fi

  print_plan
  log "wiping runtime state"
  wipe_directory "$DATA_ROOT"
  wipe_directory "$LOGS_ROOT"
  wipe_directory "$STATE_ROOT"
  log "wipe completed"
}

require_command python3
require_path_exists "$CONFIG"
load_context "$CONFIG"
assert_safe_machine_root

case "$ACTION" in
  plan)
    print_plan
    ;;
  wipe)
    perform_wipe
    ;;
esac
