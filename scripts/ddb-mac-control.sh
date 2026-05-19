#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCRIPT_PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CONFIG="${SCRIPT_PROJECT_ROOT}/configs/macos/control-plane.local.json"
ACTION="status"

usage() {
  cat <<'USAGE'
usage:
  ./scripts/ddb-mac-control.sh [-Config path] [-Action validate|status|start|stop|restart]
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
  validate|status|start|stop|restart) ;;
  *)
    echo "unsupported action: $ACTION" >&2
    exit 1
    ;;
esac

log() {
  printf '[INFO] %s\n' "$*"
}

warn() {
  printf '[WARN] %s\n' "$*" >&2
}

fail() {
  printf '[ERROR] %s\n' "$*" >&2
  exit 1
}

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required command not found: $1"
  fi
}

ensure_dir() {
  mkdir -p "$1"
}

wait_for_http() {
  local url="$1"
  local attempts="${2:-60}"
  local sleep_seconds="${3:-1}"
  local i
  for ((i=0; i<attempts; i++)); do
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

load_config() {
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

def resolve_path(value, project_root, config_dir):
    if not value:
        return ""
    value = os.path.expandvars(value)
    if os.path.isabs(value):
        return os.path.abspath(value)
    base = project_root
    if value.startswith("./") or value.startswith("../"):
        base = config_dir
    return os.path.abspath(os.path.join(base, value))

project_root_input = str(cfg.get("project_root", "")).strip()
if not project_root_input:
    raise SystemExit("config requires project_root")
project_root = resolve_path(project_root_input, config_dir, config_dir)
local_ip = str(cfg.get("local_ip", "")).strip()
if not local_ip:
    raise SystemExit("config requires local_ip")
machine_name = str(cfg.get("machine_name", "")).strip() or "control-plane"
data_root = resolve_path(cfg.get("data_root", ""), project_root, config_dir) or os.path.join(project_root, ".ddb-data")
log_dir = resolve_path(cfg.get("log_dir", ""), project_root, config_dir) or os.path.join(project_root, ".ddb-logs", "control-plane")
state_dir = resolve_path(cfg.get("state_dir", ""), project_root, config_dir) or os.path.join(project_root, ".ddb-state")
server_binary = resolve_path(cfg.get("server_binary", ""), project_root, config_dir) or os.path.join(project_root, "bin", "ddb-server")
build_server_binary = bool(cfg.get("build_server_binary", True))

etcd = cfg.get("etcd")
if not isinstance(etcd, dict):
    raise SystemExit("config requires etcd section")
etcd_runner = str(etcd.get("runner", "docker")).strip() or "docker"
if etcd_runner not in {"docker", "native"}:
    raise SystemExit("etcd.runner must be docker or native")
etcd_port = int(etcd.get("port", 2379))
etcd_data_dir = resolve_path(etcd.get("data_dir", ""), project_root, config_dir) or os.path.join(project_root, ".ddb-control", "etcd")
etcd_health_url = str(etcd.get("health_url", "")).strip() or f"http://{local_ip}:{etcd_port}/health"
etcd_binary = resolve_path(etcd.get("binary_path", ""), project_root, config_dir) or "etcd"
etcd_container_name = str(etcd.get("container_name", "")).strip() or "ddb-etcd"
etcd_image = str(etcd.get("image", "")).strip() or "quay.io/coreos/etcd:v3.5.9"
etcd_log_path = os.path.join(log_dir, "etcd.log")

api = cfg.get("apiserver")
if not isinstance(api, dict):
    raise SystemExit("config requires apiserver section")
api_node_id = str(api.get("node_id", "")).strip() or "api-1"
api_group_id = str(api.get("group_id", "")).strip() or "control"
api_http_port = int(api.get("http_port", 18100))
api_raft_port = int(api.get("raft_port", 30100))
api_bootstrap = bool(api.get("bootstrap", False))
api_raft_dir = resolve_path(api.get("raft_dir", ""), project_root, config_dir) or os.path.join(data_root, api_node_id, "raft")
api_db_path = resolve_path(api.get("db_path", ""), project_root, config_dir) or os.path.join(data_root, api_node_id, "apiserver.db")
api_http_addr = f"{local_ip}:{api_http_port}"
api_raft_addr = f"{local_ip}:{api_raft_port}"
api_health_url = str(api.get("health_url", "")).strip() or f"http://{local_ip}:{api_http_port}/health"
api_log_path = os.path.join(log_dir, "apiserver.log")
etcd_addr = f"{local_ip}:{etcd_port}"

values = {
    "CONFIG_PATH": config_path,
    "CONFIG_DIR": config_dir,
    "MACHINE_NAME": machine_name,
    "PROJECT_ROOT": project_root,
    "LOCAL_IP": local_ip,
    "DATA_ROOT": data_root,
    "LOG_DIR": log_dir,
    "STATE_DIR": state_dir,
    "SERVER_BINARY": server_binary,
    "BUILD_SERVER_BINARY": "true" if build_server_binary else "false",
    "ETCD_RUNNER": etcd_runner,
    "ETCD_PORT": str(etcd_port),
    "ETCD_DATA_DIR": etcd_data_dir,
    "ETCD_HEALTH_URL": etcd_health_url,
    "ETCD_BINARY": etcd_binary,
    "ETCD_CONTAINER_NAME": etcd_container_name,
    "ETCD_IMAGE": etcd_image,
    "ETCD_LOG_PATH": etcd_log_path,
    "ETCD_ADDR": etcd_addr,
    "API_NODE_ID": api_node_id,
    "API_GROUP_ID": api_group_id,
    "API_HTTP_ADDR": api_http_addr,
    "API_RAFT_ADDR": api_raft_addr,
    "API_BOOTSTRAP": "true" if api_bootstrap else "false",
    "API_RAFT_DIR": api_raft_dir,
    "API_DB_PATH": api_db_path,
    "API_HEALTH_URL": api_health_url,
    "API_LOG_PATH": api_log_path,
}
for key, value in values.items():
    print(f"{key}={shlex.quote(str(value))}")
PY
  # shellcheck disable=SC1090
  source "$env_file"
  rm -f "$env_file"
  ETCD_PID_FILE="$STATE_DIR/${MACHINE_NAME}.etcd.pid"
  APISERVER_PID_FILE="$STATE_DIR/${MACHINE_NAME}.apiserver.pid"
}

build_server_if_needed() {
  if [[ "$BUILD_SERVER_BINARY" == "true" ]]; then
    log "building ddb-server"
    ensure_dir "$(dirname "$SERVER_BINARY")"
    (cd "$PROJECT_ROOT" && go build -o "$SERVER_BINARY" ./cmd/server)
  fi
}

validate() {
  require_command python3
  require_command curl
  load_config "$CONFIG"
  require_command go
  if [[ "$ETCD_RUNNER" == "docker" ]]; then
    require_command docker
  else
    require_command "$ETCD_BINARY"
  fi
  ensure_dir "$LOG_DIR"
  ensure_dir "$STATE_DIR"
  ensure_dir "$ETCD_DATA_DIR"
  ensure_dir "$API_RAFT_DIR"
  ensure_dir "$(dirname "$API_DB_PATH")"
  log "config ok: $CONFIG_PATH"
  log "project_root=$PROJECT_ROOT"
  log "etcd=$ETCD_RUNNER $ETCD_ADDR"
  log "apiserver=$API_HTTP_ADDR"
}

start_etcd() {
  ensure_dir "$ETCD_DATA_DIR"
  ensure_dir "$(dirname "$ETCD_LOG_PATH")"
  if [[ "$ETCD_RUNNER" == "docker" ]]; then
    docker rm -f "$ETCD_CONTAINER_NAME" >/dev/null 2>&1 || true
    log "starting etcd container $ETCD_CONTAINER_NAME"
    docker run -d \
      --name "$ETCD_CONTAINER_NAME" \
      --restart unless-stopped \
      -p "$ETCD_PORT:2379" \
      -v "$ETCD_DATA_DIR:/etcd-data" \
      "$ETCD_IMAGE" \
      etcd \
      --data-dir=/etcd-data \
      --advertise-client-urls="http://$LOCAL_IP:$ETCD_PORT" \
      --listen-client-urls="http://0.0.0.0:2379" >/dev/null
  else
    if [[ -f "$ETCD_PID_FILE" ]] && is_pid_running "$(cat "$ETCD_PID_FILE")"; then
      log "etcd already running with pid $(cat "$ETCD_PID_FILE")"
      return 0
    fi
    rm -f "$ETCD_PID_FILE"
    log "starting native etcd"
    nohup "$ETCD_BINARY" \
      --data-dir "$ETCD_DATA_DIR" \
      --advertise-client-urls="http://$LOCAL_IP:$ETCD_PORT" \
      --listen-client-urls="http://0.0.0.0:$ETCD_PORT" \
      >>"$ETCD_LOG_PATH" 2>&1 &
    echo $! >"$ETCD_PID_FILE"
  fi
  wait_for_http "$ETCD_HEALTH_URL"
}

stop_etcd() {
  if [[ "$ETCD_RUNNER" == "docker" ]]; then
    docker rm -f "$ETCD_CONTAINER_NAME" >/dev/null 2>&1 || true
    return 0
  fi
  if [[ -f "$ETCD_PID_FILE" ]]; then
    local pid
    pid="$(cat "$ETCD_PID_FILE")"
    if is_pid_running "$pid"; then
      kill "$pid" >/dev/null 2>&1 || true
      sleep 1
      is_pid_running "$pid" && kill -9 "$pid" >/dev/null 2>&1 || true
    fi
    rm -f "$ETCD_PID_FILE"
  fi
}

start_apiserver() {
  ensure_dir "$API_RAFT_DIR"
  ensure_dir "$(dirname "$API_DB_PATH")"
  ensure_dir "$(dirname "$API_LOG_PATH")"
  if [[ -f "$APISERVER_PID_FILE" ]] && is_pid_running "$(cat "$APISERVER_PID_FILE")"; then
    log "apiserver already running with pid $(cat "$APISERVER_PID_FILE")"
    return 0
  fi
  rm -f "$APISERVER_PID_FILE"
  log "starting apiserver at $API_HTTP_ADDR"
  nohup "$SERVER_BINARY" \
    --role=apiserver \
    --node-id="$API_NODE_ID" \
    --group-id="$API_GROUP_ID" \
    --http-addr="$API_HTTP_ADDR" \
    --raft-addr="$API_RAFT_ADDR" \
    --raft-dir="$API_RAFT_DIR" \
    --db-path="$API_DB_PATH" \
    --bootstrap="$API_BOOTSTRAP" \
    --etcd="$ETCD_ADDR" \
    >>"$API_LOG_PATH" 2>&1 &
  echo $! >"$APISERVER_PID_FILE"
  wait_for_http "$API_HEALTH_URL"
}

stop_apiserver() {
  if [[ -f "$APISERVER_PID_FILE" ]]; then
    local pid
    pid="$(cat "$APISERVER_PID_FILE")"
    if is_pid_running "$pid"; then
      kill "$pid" >/dev/null 2>&1 || true
      sleep 1
      is_pid_running "$pid" && kill -9 "$pid" >/dev/null 2>&1 || true
    fi
    rm -f "$APISERVER_PID_FILE"
  fi
}

print_status() {
  local etcd_status="stopped"
  local api_status="stopped"
  if [[ "$ETCD_RUNNER" == "docker" ]]; then
    if docker inspect "$ETCD_CONTAINER_NAME" >/dev/null 2>&1; then
      etcd_status="container-present"
      if curl -fsS "$ETCD_HEALTH_URL" >/dev/null 2>&1; then
        etcd_status="healthy"
      fi
    fi
  elif [[ -f "$ETCD_PID_FILE" ]] && is_pid_running "$(cat "$ETCD_PID_FILE")"; then
    etcd_status="running"
    if curl -fsS "$ETCD_HEALTH_URL" >/dev/null 2>&1; then
      etcd_status="healthy"
    fi
  fi

  if [[ -f "$APISERVER_PID_FILE" ]] && is_pid_running "$(cat "$APISERVER_PID_FILE")"; then
    api_status="running"
    if curl -fsS "$API_HEALTH_URL" >/dev/null 2>&1; then
      api_status="healthy"
    fi
  fi

  printf 'machine: %s\n' "$MACHINE_NAME"
  printf 'etcd: %s (%s)\n' "$etcd_status" "$ETCD_ADDR"
  printf 'apiserver: %s (%s)\n' "$api_status" "$API_HTTP_ADDR"
  printf 'dashboard: http://%s/dashboard/\n' "$API_HTTP_ADDR"
}

load_config "$CONFIG"

case "$ACTION" in
  validate)
    validate
    ;;
  status)
    print_status
    ;;
  start)
    validate
    build_server_if_needed
    start_etcd
    start_apiserver
    print_status
    ;;
  stop)
    stop_apiserver
    stop_etcd
    print_status
    ;;
  restart)
    stop_apiserver
    stop_etcd
    validate
    build_server_if_needed
    start_etcd
    start_apiserver
    print_status
    ;;
esac
