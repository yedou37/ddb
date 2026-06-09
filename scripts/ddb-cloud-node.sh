#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="${SCRIPT_PROJECT_ROOT}/configs/cloud/node.local.json"
ACTION="status"
NAME=""

usage() {
  cat <<'USAGE'
usage:
  ./scripts/ddb-cloud-node.sh [-Config path] [-Action validate|list|status|start|stop|restart|join|rejoin|remove|remove-local|start-all|stop-all|restart-all|join-all] [-Name node]
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
    -Name)
      NAME="$2"
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
  validate|list|status|start|stop|restart|join|rejoin|remove|remove-local|start-all|stop-all|restart-all|join-all) ;;
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

wait_for_http() {
  local url="$1"
  local attempts="${2:-60}"
  local sleep_seconds="${3:-1}"
  local i
  for ((i = 0; i < attempts; i++)); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$sleep_seconds"
  done
  fail "timeout waiting for $url"
}

is_pid_running() {
  local pid="$1"
  [[ -n "$pid" ]] && kill -0 "$pid" >/dev/null 2>&1
}

normalize_http_url() {
  local addr="$1"
  if [[ "$addr" == http://* || "$addr" == https://* ]]; then
    printf '%s\n' "$addr"
    return 0
  fi
  printf 'http://%s\n' "$addr"
}

load_context() {
  local config_path="$1"
  local env_file targets_file
  env_file="$(mktemp)"
  targets_file="$(mktemp)"
  python3 - "$config_path" "$targets_file" >"$env_file" <<'PY'
import json
import os
import shlex
import sys

config_path = os.path.abspath(sys.argv[1])
targets_path = sys.argv[2]
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
data_root = resolve_path(cfg.get("data_root", ""), install_root, config_dir) or os.path.join(install_root, "data")
log_dir = resolve_path(cfg.get("log_dir", ""), install_root, config_dir) or os.path.join(install_root, "logs")
state_dir = resolve_path(cfg.get("state_dir", ""), install_root, config_dir) or os.path.join(install_root, "state")
machine_name = str(cfg.get("machine_name", "")).strip() or "cloud-node"
local_ip = str(cfg.get("local_ip", "")).strip()
etcd_host = str(cfg.get("etcd_host", "")).strip()
etcd_port = int(cfg.get("etcd_port", 2379))
default_join_host = str(cfg.get("default_join_host", "")).strip()
server_binary = resolve_path(cfg.get("server_binary", ""), install_root, config_dir) or os.path.join(install_root, "bin", "ddb-server")
cli_binary = resolve_path(cfg.get("cli_binary", ""), install_root, config_dir) or os.path.join(install_root, "bin", "ddb-cli")
build_server_binary = bool(cfg.get("build_server_binary", False))

def resolve_addr(explicit_addr, host, port):
    explicit_addr = str(explicit_addr or "").strip()
    if explicit_addr:
        return explicit_addr
    if not host or not port:
        return ""
    return f"{host}:{int(port)}"

lines = []
for item in cfg.get("targets", []):
    name = str(item.get("name", "")).strip()
    if not name:
        raise SystemExit("every target requires a name")
    runner = str(item.get("runner", "ddb-process")).strip()
    http_addr = resolve_addr(item.get("http_addr"), local_ip, item.get("http_port"))
    raft_addr = resolve_addr(item.get("raft_addr"), local_ip, item.get("raft_port"))
    join_host = str(item.get("join_host", "")).strip() or default_join_host
    join_addr = resolve_addr(item.get("join_addr"), join_host, item.get("join_port"))
    etcd_addr = resolve_addr(item.get("etcd"), etcd_host, item.get("etcd_port", etcd_port))
    raft_dir = resolve_path(item.get("raft_dir", ""), install_root, config_dir) or os.path.join(data_root, name, "raft")
    db_path = resolve_path(item.get("db_path", ""), install_root, config_dir) or os.path.join(data_root, name, "data.db")
    health_url = str(item.get("health_url", "")).strip() or f"http://{http_addr}/health"
    membership_addr = str(item.get("membership_addr", "")).strip() or join_addr or http_addr
    log_path = os.path.join(log_dir, f"{name}.log")
    node_id = str(item.get("node_id", "")).strip() or name
    role = str(item.get("role", "")).strip()
    group_id = str(item.get("group_id", "")).strip()
    bootstrap = "true" if bool(item.get("bootstrap", False)) else "false"
    rejoin = "true" if bool(item.get("rejoin", False)) else "false"
    row = [
        name,
        runner,
        node_id,
        role,
        group_id,
        http_addr,
        raft_addr,
        bootstrap,
        rejoin,
        join_addr,
        etcd_addr,
        health_url,
        membership_addr,
        raft_dir,
        db_path,
        log_path,
    ]
    lines.append("\t".join(row))

with open(targets_path, "w", encoding="utf-8") as handle:
    for line in lines:
        handle.write(line + "\n")

values = {
    "CONFIG_PATH": config_path,
    "CONFIG_DIR": config_dir,
    "INSTALL_ROOT": install_root,
    "DATA_ROOT": data_root,
    "LOG_DIR": log_dir,
    "STATE_DIR": state_dir,
    "MACHINE_NAME": machine_name,
    "LOCAL_IP": local_ip,
    "ETCD_HOST": etcd_host,
    "ETCD_PORT": str(etcd_port),
    "DEFAULT_JOIN_HOST": default_join_host,
    "SERVER_BINARY": server_binary,
    "CLI_BINARY": cli_binary,
    "BUILD_SERVER_BINARY": "true" if build_server_binary else "false",
    "TARGETS_FILE": targets_path,
}
for key, value in values.items():
    print(f"{key}={shlex.quote(str(value))}")
PY
  # shellcheck disable=SC1090
  source "$env_file"
  rm -f "$env_file"
}

build_server_if_needed() {
  if [[ "$BUILD_SERVER_BINARY" == "true" ]]; then
    require_command go
    require_path_exists "$INSTALL_ROOT"
    log "building ddb-server"
    ensure_dir "$(dirname "$SERVER_BINARY")"
    (cd "$INSTALL_ROOT" && go build -o "$SERVER_BINARY" ./cmd/server)
  fi
}

target_pid_file() {
  printf '%s/%s.%s.pid\n' "$STATE_DIR" "$MACHINE_NAME" "$1"
}

target_line_by_name() {
  local target_name="$1"
  local line
  line="$(awk -F '\t' -v name="$target_name" '$1 == name { print; exit }' "$TARGETS_FILE")"
  [[ -n "$line" ]] || fail "target not found: $target_name"
  printf '%s\n' "$line"
}

parse_target_line() {
  local line="$1"
  local sep=$'\x1f'
  IFS="$sep" read -r TARGET_NAME TARGET_RUNNER TARGET_NODE_ID TARGET_ROLE TARGET_GROUP_ID TARGET_HTTP_ADDR TARGET_RAFT_ADDR TARGET_BOOTSTRAP TARGET_REJOIN TARGET_JOIN_ADDR TARGET_ETCD TARGET_HEALTH_URL TARGET_MEMBERSHIP_ADDR TARGET_RAFT_DIR TARGET_DB_PATH TARGET_LOG_PATH <<< "${line//$'\t'/$sep}"
  TARGET_PID_FILE="$(target_pid_file "$TARGET_NAME")"
}

run_target() {
  parse_target_line "$1"
  local mode="${2:-start}"
  [[ "$TARGET_RUNNER" == "ddb-process" ]] || fail "unsupported runner: $TARGET_RUNNER"
  ensure_dir "$STATE_DIR"
  ensure_dir "$(dirname "$TARGET_LOG_PATH")"
  ensure_dir "$TARGET_RAFT_DIR"
  ensure_dir "$(dirname "$TARGET_DB_PATH")"
  if [[ -f "$TARGET_PID_FILE" ]] && is_pid_running "$(cat "$TARGET_PID_FILE")"; then
    log "$TARGET_NAME already running with pid $(cat "$TARGET_PID_FILE")"
    return 0
  fi
  rm -f "$TARGET_PID_FILE"
  local cmd
  cmd=(
    "$SERVER_BINARY"
    --role="$TARGET_ROLE"
    --node-id="$TARGET_NODE_ID"
    --group-id="$TARGET_GROUP_ID"
    --http-addr="$TARGET_HTTP_ADDR"
    --raft-addr="$TARGET_RAFT_ADDR"
    --raft-dir="$TARGET_RAFT_DIR"
    --db-path="$TARGET_DB_PATH"
    --bootstrap="$TARGET_BOOTSTRAP"
  )
  if [[ "$mode" == "join" || "$mode" == "rejoin" ]] && [[ -n "$TARGET_JOIN_ADDR" ]]; then
    cmd+=(--join="$TARGET_JOIN_ADDR")
  fi
  if [[ -n "$TARGET_ETCD" ]]; then
    cmd+=(--etcd="$TARGET_ETCD")
  fi
  if [[ "$mode" == "rejoin" || ( "$mode" == "join" && "$TARGET_REJOIN" == "true" ) ]]; then
    cmd+=(--rejoin=true)
  fi
  log "starting $TARGET_NAME at $TARGET_HTTP_ADDR (mode=$mode)"
  nohup "${cmd[@]}" >>"$TARGET_LOG_PATH" 2>&1 &
  echo $! >"$TARGET_PID_FILE"
  wait_for_http "$TARGET_HEALTH_URL"
}

stop_target() {
  parse_target_line "$1"
  if [[ -f "$TARGET_PID_FILE" ]]; then
    local pid
    pid="$(cat "$TARGET_PID_FILE")"
    if is_pid_running "$pid"; then
      kill "$pid" >/dev/null 2>&1 || true
      sleep 1
      is_pid_running "$pid" && kill -9 "$pid" >/dev/null 2>&1 || true
    fi
    rm -f "$TARGET_PID_FILE"
  fi
}

remove_local_target() {
  parse_target_line "$1"
  stop_target "$1"
  log "removing local state for $TARGET_NAME"
  rm -rf "$TARGET_RAFT_DIR"
  rm -rf "$TARGET_DB_PATH"
  rm -f "$TARGET_LOG_PATH"
  rm -f "$TARGET_PID_FILE"
}

status_target() {
  parse_target_line "$1"
  local status="stopped"
  if [[ -f "$TARGET_PID_FILE" ]] && is_pid_running "$(cat "$TARGET_PID_FILE")"; then
    status="running"
    if curl -fsS "$TARGET_HEALTH_URL" >/dev/null 2>&1; then
      status="healthy"
    fi
  fi
  printf '%s\t%s\t%s\t%s\n' "$TARGET_NAME" "$TARGET_GROUP_ID" "$TARGET_HTTP_ADDR" "$status"
}

remove_target() {
  parse_target_line "$1"
  require_path_exists "$CLI_BINARY"
  [[ -n "$TARGET_MEMBERSHIP_ADDR" ]] || fail "$TARGET_NAME has no membership_addr/join_addr/http_addr to drive remove"
  local base_url
  base_url="$(normalize_http_url "$TARGET_MEMBERSHIP_ADDR")"
  log "removing $TARGET_NAME from group $TARGET_GROUP_ID via $base_url"
  "$CLI_BINARY" --node-url="$base_url" cluster remove "$TARGET_NODE_ID"
  stop_target "$1"
}

validate_environment() {
  require_command python3
  require_command curl
  load_context "$CONFIG"
  require_path_exists "$INSTALL_ROOT"
  if [[ "$BUILD_SERVER_BINARY" == "true" ]]; then
    require_command go
  else
    require_path_exists "$SERVER_BINARY"
  fi
  require_path_exists "$CLI_BINARY"
  ensure_dir "$LOG_DIR"
  ensure_dir "$STATE_DIR"

  log "config ok: $CONFIG_PATH"
  log "install_root=$INSTALL_ROOT"
  log "server_binary=$SERVER_BINARY"
  log "cli_binary=$CLI_BINARY"
  printf 'name\tgroup\thttp\traft\tjoin\tmembership\tetcd\n'
  while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    parse_target_line "$line"
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$TARGET_NAME" "$TARGET_GROUP_ID" "$TARGET_HTTP_ADDR" "$TARGET_RAFT_ADDR" "$TARGET_JOIN_ADDR" "$TARGET_MEMBERSHIP_ADDR" "$TARGET_ETCD"
  done < "$TARGETS_FILE"
}

load_context "$CONFIG"
require_command python3
require_command curl
if [[ "$BUILD_SERVER_BINARY" == "true" ]]; then
  require_command go
fi
ensure_dir "$LOG_DIR"
ensure_dir "$STATE_DIR"

case "$ACTION" in
  validate)
    validate_environment
    ;;
  list)
    awk -F '\t' '{ printf "%s\t%s\t%s\n", $1, $4, $6 }' "$TARGETS_FILE"
    ;;
  status)
    printf 'name\tgroup\thttp\tstatus\n'
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      status_target "$line"
    done < "$TARGETS_FILE"
    ;;
  start)
    [[ -n "$NAME" ]] || fail "-Name is required for start"
    build_server_if_needed
    run_target "$(target_line_by_name "$NAME")" start
    ;;
  stop)
    [[ -n "$NAME" ]] || fail "-Name is required for stop"
    stop_target "$(target_line_by_name "$NAME")"
    ;;
  restart)
    [[ -n "$NAME" ]] || fail "-Name is required for restart"
    stop_target "$(target_line_by_name "$NAME")"
    build_server_if_needed
    run_target "$(target_line_by_name "$NAME")" start
    ;;
  join)
    [[ -n "$NAME" ]] || fail "-Name is required for join"
    build_server_if_needed
    run_target "$(target_line_by_name "$NAME")" join
    ;;
  rejoin)
    [[ -n "$NAME" ]] || fail "-Name is required for rejoin"
    build_server_if_needed
    run_target "$(target_line_by_name "$NAME")" rejoin
    ;;
  remove)
    [[ -n "$NAME" ]] || fail "-Name is required for remove"
    remove_target "$(target_line_by_name "$NAME")"
    ;;
  remove-local)
    [[ -n "$NAME" ]] || fail "-Name is required for remove-local"
    remove_local_target "$(target_line_by_name "$NAME")"
    ;;
  start-all)
    build_server_if_needed
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      run_target "$line" start
    done < "$TARGETS_FILE"
    ;;
  stop-all)
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      stop_target "$line"
    done < "$TARGETS_FILE"
    ;;
  restart-all)
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      stop_target "$line"
    done < "$TARGETS_FILE"
    build_server_if_needed
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      run_target "$line" start
    done < "$TARGETS_FILE"
    ;;
  join-all)
    build_server_if_needed
    while IFS= read -r line; do
      [[ -z "$line" ]] && continue
      run_target "$line" join
    done < "$TARGETS_FILE"
    ;;
esac
