#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="/Users/bytedance/dbd"
BIN_SERVER="$ROOT_DIR/bin/ddb-server"
BIN_CLI="$ROOT_DIR/bin/ddb-cli"
TMP_DIR="/tmp/ddb-demo"
ETCD_ENDPOINT="127.0.0.1:2379"
API_URL="http://127.0.0.1:18100"
ETCD_CONTAINER="ddb-etcd"
LOG_DIR="$TMP_DIR/logs"
LAUNCHER_DIR="$TMP_DIR/launchers"
TERMINAL_PREFERENCE="${DDB_TERM_APP:-auto}"
TERMINAL_APP=""
PRIMARY_KEY_A="${DDB_PRIMARY_KEY_A:-1}"
PRIMARY_KEY_B="${DDB_PRIMARY_KEY_B:-2}"
KEEP_ETCD_MODE="${DDB_KEEP_ETCD:-auto}"
REUSE_EXISTING_ETCD=false
ETCD_BIN="${DDB_ETCD_BIN:-}"

usage() {
  cat <<'EOF'
usage:
  ./scripts/demo-single-host-sharddb.sh [verify-only|cleanup-only]

description:
  clean up the local demo environment, start etcd, open each shard node and the
  apiserver in a separate Terminal.app shell, then run verification commands.

notes:
  - designed for macOS with iTerm2 or Terminal.app
  - shard nodes always start in separate Terminal shells
  - etcd uses Docker if available, otherwise falls back to local `etcd`
  - set DDB_TERM_APP=iterm2 to force iTerm2
  - set DDB_TERM_APP=terminal to force Terminal.app
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

VERIFY_ONLY=false
if [[ "${1:-}" == "verify-only" ]]; then
  VERIFY_ONLY=true
fi

CLEANUP_ONLY=false
if [[ "${1:-}" == "cleanup-only" ]]; then
  CLEANUP_ONLY=true
fi

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

json_get() {
  local file="$1"
  local expression="$2"
  python3 - "$file" "$expression" <<'PY'
import json
import sys

file_path = sys.argv[1]
expression = sys.argv[2]
with open(file_path, "r", encoding="utf-8") as handle:
    payload = json.load(handle)
safe_globals = {
    "__builtins__": {},
    "len": len,
    "all": all,
    "any": any,
    "sorted": sorted,
    "next": next,
    "str": str,
    "int": int,
    "min": min,
    "max": max,
}
value = eval(expression, safe_globals, {"data": payload})
if isinstance(value, (dict, list)):
    print(json.dumps(value))
elif value is None:
    print("")
else:
    print(value)
PY
}

assert_json() {
  local file="$1"
  local expression="$2"
  local message="$3"
  python3 - "$file" "$expression" "$message" <<'PY'
import json
import sys

file_path = sys.argv[1]
expression = sys.argv[2]
message = sys.argv[3]
with open(file_path, "r", encoding="utf-8") as handle:
    payload = json.load(handle)
safe_globals = {
    "__builtins__": {},
    "len": len,
    "all": all,
    "any": any,
    "sorted": sorted,
    "next": next,
    "str": str,
    "int": int,
    "min": min,
    "max": max,
}
result = eval(expression, safe_globals, {"data": payload})
if not result:
    raise SystemExit(message)
print(f"[PASS] {message}")
PY
}

sql_payload() {
  python3 - "$1" <<'PY'
import json
import sys

print(json.dumps({"sql": sys.argv[1]}))
PY
}

http_get_json() {
  local url="$1"
  local output="$2"
  curl -fsS "$url" >"$output"
}

http_post_json() {
  local url="$1"
  local payload="$2"
  local output="$3"
  curl -fsS \
    -X POST \
    -H 'Content-Type: application/json' \
    -d "$payload" \
    "$url" >"$output"
}

sql_request() {
  local statement="$1"
  local output="$2"
  http_post_json "$API_URL/sql" "$(sql_payload "$statement")" "$output"
}

describe_port_usage() {
  local port="$1"
  local details
  details=$(lsof -nP -iTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
  if [[ -n "$details" ]]; then
    printf '%s\n' "$details"
  fi
}

kill_demo_processes() {
  local pattern="$1"
  local matches
  matches=$(pgrep -af "$pattern" 2>/dev/null || true)
  if [[ -n "$matches" ]]; then
    printf '[INFO] killing demo processes matching %s\n%s\n' "$pattern" "$matches"
    pkill -f "$pattern" 2>/dev/null || true
  fi
}

diagnose_ports() {
  log "port diagnostics"
  for port in \
    18080 18100 2379 \
    21080 21081 21082 21180 21181 21182 21280 21281 21282 21380 21381 21382 \
    22080 22081 22082 22180 22181 22182 22280 22281 22282 22380 22381 22382
  do
    if [[ "$REUSE_EXISTING_ETCD" == "true" && "$port" == "2379" ]]; then
      continue
    fi
    local details
    details=$(describe_port_usage "$port")
    if [[ -n "$details" ]]; then
      printf '[WARN] port %s is still in use\n%s\n' "$port" "$details" >&2
    fi
  done
}

diagnose_docker() {
  if ! command -v docker >/dev/null 2>&1; then
    warn "docker cli not found; local etcd fallback will be used if available"
    return 0
  fi
  if docker info >/dev/null 2>&1; then
    log "docker daemon is available"
    return 0
  fi
  warn "docker cli exists but Docker Desktop daemon is not running"
}

detect_terminal_app() {
  case "$TERMINAL_PREFERENCE" in
    auto)
      if osascript -e 'tell application "iTerm2" to version' >/dev/null 2>&1; then
        TERMINAL_APP="iTerm2"
        return 0
      fi
      if osascript -e 'tell application "iTerm" to version' >/dev/null 2>&1; then
        TERMINAL_APP="iTerm"
        return 0
      fi
      if osascript -e 'tell application "Terminal" to version' >/dev/null 2>&1; then
        TERMINAL_APP="Terminal"
        return 0
      fi
      ;;
    iterm2)
      if osascript -e 'tell application "iTerm2" to version' >/dev/null 2>&1; then
        TERMINAL_APP="iTerm2"
        return 0
      fi
      if osascript -e 'tell application "iTerm" to version' >/dev/null 2>&1; then
        TERMINAL_APP="iTerm"
        return 0
      fi
      fail "DDB_TERM_APP=iterm2 was requested, but iTerm2 is not available"
      ;;
    terminal)
      if osascript -e 'tell application "Terminal" to version' >/dev/null 2>&1; then
        TERMINAL_APP="Terminal"
        return 0
      fi
      fail "DDB_TERM_APP=terminal was requested, but Terminal.app is not available"
      ;;
    *)
      fail "unsupported DDB_TERM_APP value: $TERMINAL_PREFERENCE"
      ;;
  esac
  fail "no supported terminal app found; install iTerm2 or use Terminal.app"
}

require_command() {
  local name="$1"
  command -v "$name" >/dev/null 2>&1 || fail "required command not found: $name"
}

etcd_is_healthy() {
  curl -fsS "http://127.0.0.1:2379/health" >/dev/null 2>&1
}

should_reuse_existing_etcd() {
  case "$KEEP_ETCD_MODE" in
    true)
      return 0
      ;;
    false)
      return 1
      ;;
    auto)
      if etcd_is_healthy && ! docker info >/dev/null 2>&1 && ! command -v etcd >/dev/null 2>&1; then
        return 0
      fi
      return 1
      ;;
    *)
      fail "unsupported DDB_KEEP_ETCD value: $KEEP_ETCD_MODE"
      ;;
  esac
}

preflight_checks() {
  log "preflight checks"
  require_command bash
  require_command curl
  require_command lsof
  require_command osascript
  require_command python3
  require_command go
  detect_terminal_app
  log "terminal app: $TERMINAL_APP"
  diagnose_docker
  if [[ -z "$ETCD_BIN" ]] && command -v etcd >/dev/null 2>&1; then
    ETCD_BIN="$(command -v etcd)"
    log "local etcd binary: $ETCD_BIN"
  fi
  if ! command -v etcd >/dev/null 2>&1 && ! command -v docker >/dev/null 2>&1; then
    fail "neither local etcd nor docker is available"
  fi
  if etcd_is_healthy; then
    log "detected a running etcd on $ETCD_ENDPOINT"
  fi
}

dump_recent_logs() {
  if [[ ! -d "$LOG_DIR" ]]; then
    return 0
  fi
  warn "recent logs from $LOG_DIR"
  local found=false
  while IFS= read -r log_file; do
    found=true
    printf '[WARN] tail %s\n' "$log_file" >&2
    tail -n 20 "$log_file" >&2 || true
  done < <(find "$LOG_DIR" -type f -name '*.log' | sort)
  if [[ "$found" == false ]]; then
    warn "no log files found yet"
  fi
}

on_error() {
  local exit_code="$1"
  local line_no="$2"
  warn "script failed at line $line_no with exit code $exit_code"
  diagnose_docker
  diagnose_ports
  dump_recent_logs
  exit "$exit_code"
}

trap 'on_error $? $LINENO' ERR

cleanup_demo() {
  REUSE_EXISTING_ETCD=false
  if should_reuse_existing_etcd; then
    REUSE_EXISTING_ETCD=true
    warn "reusing existing etcd on $ETCD_ENDPOINT; old etcd keys will be kept"
  fi

  log "cleanup demo processes"
  kill_demo_processes "/tmp/ddb-demo"
  kill_demo_processes "./bin/ddb-server"
  kill_demo_processes "go run ./cmd/server"
  if [[ "$REUSE_EXISTING_ETCD" == "false" ]]; then
    kill_demo_processes "etcd --advertise-client-urls=http://127.0.0.1:2379"
  fi

  log "cleanup demo ports"
  for p in \
    18080 18100 2379 \
    21080 21081 21082 \
    21180 21181 21182 \
    21280 21281 21282 \
    21380 21381 21382 \
    22080 22081 22082 \
    22180 22181 22182 \
    22280 22281 22282 \
    22380 22381 22382
  do
    if [[ "$REUSE_EXISTING_ETCD" == "true" && "$p" == "2379" ]]; then
      continue
    fi
    local details
    details=$(describe_port_usage "$p")
    if [[ -n "$details" ]]; then
      printf '[INFO] closing listeners on port %s\n%s\n' "$p" "$details"
    fi
    kill -9 $(lsof -tiTCP:$p -sTCP:LISTEN) 2>/dev/null || true
  done

  log "cleanup docker etcd"
  if [[ "$REUSE_EXISTING_ETCD" == "true" ]]; then
    warn "skip docker etcd cleanup because existing etcd is being reused"
  elif command -v docker >/dev/null 2>&1; then
    if docker info >/dev/null 2>&1; then
      if ! docker rm -f "$ETCD_CONTAINER" >/dev/null 2>&1; then
        warn "docker cleanup skipped or container not found: $ETCD_CONTAINER"
      fi
    else
      warn "docker cli exists but daemon is unreachable; skipped docker etcd cleanup"
    fi
  else
    warn "docker cli not found; skipped docker etcd cleanup"
  fi

  log "cleanup temp files"
  rm -rf "$TMP_DIR"
  find "$ROOT_DIR" -name "*.controller.json" -type f -print -delete || true

  log "recreate temp dirs"
  mkdir -p "$TMP_DIR"/{api-1,etcd,g1-n1,g1-n2,g1-n3,g2-n1,g2-n2,g2-n3,g3-n1,g3-n2,g3-n3}/raft
  mkdir -p "$LOG_DIR" "$LAUNCHER_DIR"
  diagnose_ports
}

build_binaries() {
  log "build binaries"
  cd "$ROOT_DIR"
  go build -o "$BIN_SERVER" ./cmd/server
  go build -o "$BIN_CLI" ./cmd/cli
}

start_etcd() {
  log "start etcd"
  if etcd_is_healthy; then
    if [[ "$REUSE_EXISTING_ETCD" == "true" ]]; then
      warn "reusing already running etcd on $ETCD_ENDPOINT"
    else
      log "etcd is already healthy on $ETCD_ENDPOINT"
    fi
    return 0
  fi

  if docker info >/dev/null 2>&1; then
    docker run -d \
      --name "$ETCD_CONTAINER" \
      -p 2379:2379 \
      quay.io/coreos/etcd:v3.5.9 \
      etcd \
      --advertise-client-urls=http://127.0.0.1:2379 \
      --listen-client-urls=http://0.0.0.0:2379 >/dev/null
    log "started etcd via Docker container $ETCD_CONTAINER"
    return 0
  fi

  if [[ -n "$ETCD_BIN" ]]; then
    open_terminal "ddb-etcd" \
      "\"$ETCD_BIN\" --data-dir=$TMP_DIR/etcd --advertise-client-urls=http://127.0.0.1:2379 --listen-client-urls=http://0.0.0.0:2379"
    log "started etcd via local binary in a separate terminal: $ETCD_BIN"
    return 0
  fi

  fail "could not start etcd; start Docker Desktop, install etcd locally, or export DDB_KEEP_ETCD=true to reuse an already running clean etcd"
}

create_launcher_script() {
  local title="$1"
  local command="$2"
  local launcher="$LAUNCHER_DIR/${title}.sh"
  local log_file="$LOG_DIR/${title}.log"
  cat >"$launcher" <<EOF
#!/usr/bin/env bash
set -uo pipefail
cd "$ROOT_DIR" || exit 1
printf '\\033]0;%s\\007' "$title"
echo "[launcher] $title"
echo "[cwd] $ROOT_DIR"
echo "[command] $command"
echo "[log] $log_file"
{
  eval '$command'
} 2>&1 | tee -a "$log_file"
status=\${PIPESTATUS[0]}
echo "[exit] \$status" | tee -a "$log_file"
if [[ \$status -ne 0 ]]; then
  echo "command failed; inspect $log_file" | tee -a "$log_file"
fi
exec bash -i
EOF
  chmod +x "$launcher"
  printf '%s\n' "$launcher"
}

open_with_terminal_app() {
  local launcher="$1"
  local title="$2"
  local command_text="bash '$launcher'"
  local escaped
  local error_log="$TMP_DIR/terminal-launch.err"
  escaped=$(printf '%s' "$command_text" | sed 's/\\/\\\\/g; s/"/\\"/g')
  case "$TERMINAL_APP" in
    iTerm2|iTerm)
      if ! osascript <<EOF > /dev/null 2>"$error_log"
tell application "$TERMINAL_APP"
  activate
  create window with default profile command "$escaped"
end tell
EOF
      then
        warn "$(cat "$error_log")"
        fail "failed to open $TERMINAL_APP; grant Automation permission to your terminal in System Settings -> Privacy & Security -> Automation"
      fi
      ;;
    Terminal)
      if ! osascript <<EOF > /dev/null 2>"$error_log"
tell application "Terminal"
  activate
  do script "$escaped"
end tell
EOF
      then
        warn "$(cat "$error_log")"
        fail "failed to open Terminal.app; grant Automation permission to your terminal in System Settings -> Privacy & Security -> Automation"
      fi
      ;;
    *)
      fail "terminal app not initialized"
      ;;
  esac
  log "opened $title in $TERMINAL_APP"
}

open_terminal() {
  local title="$1"
  local command="$2"
  local launcher
  launcher=$(create_launcher_script "$title" "$command")
  open_with_terminal_app "$launcher" "$title"
}

wait_for_http() {
  local url="$1"
  local retries="${2:-60}"
  for ((i = 0; i < retries; i++)); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  fail "timeout waiting for $url"
}

wait_for_file_condition() {
  local description="$1"
  local expression="$2"
  local file="$3"
  local retries="${4:-60}"
  for ((i = 0; i < retries; i++)); do
    if [[ -f "$file" ]] && python3 - "$file" "$expression" <<'PY'
import json
import sys

file_path = sys.argv[1]
expression = sys.argv[2]
with open(file_path, "r", encoding="utf-8") as handle:
    payload = json.load(handle)
safe_globals = {
    "__builtins__": {},
    "len": len,
    "all": all,
    "any": any,
    "sorted": sorted,
    "next": next,
    "str": str,
    "int": int,
    "min": min,
    "max": max,
}
if eval(expression, safe_globals, {"data": payload}):
    raise SystemExit(0)
raise SystemExit(1)
PY
    then
      log "ready: $description"
      return 0
    fi
    sleep 1
    curl -fsS "$API_URL/groups" >"$file" 2>/dev/null || true
  done
  fail "timeout waiting for $description"
}

route_shard_for_key() {
  python3 - "$1" "$2" "$3" <<'PY'
import sys

table = sys.argv[1]
primary_key = sys.argv[2]
total_shards = int(sys.argv[3])

def fnv1a32(value: str) -> int:
    h = 2166136261
    for byte in value.encode("utf-8"):
        h ^= byte
        h = (h * 16777619) & 0xFFFFFFFF
    return h

points = []
for shard in range(total_shards):
    for replica in range(32):
        points.append((fnv1a32(f"shard:{shard}:{replica}"), shard))
points.sort(key=lambda item: item[0])
target = fnv1a32(f"{table}:{primary_key}")
for point_hash, shard in points:
    if point_hash >= target:
        print(shard)
        raise SystemExit(0)
print(points[0][1])
PY
}

verify_group_layout() {
  local file="$1"
  assert_json "$file" 'len(data) == 3' "group list exposes g1/g2/g3"
  assert_json "$file" 'sorted(item["group_id"] for item in data) == ["g1", "g2", "g3"]' "group ids are g1/g2/g3"
  assert_json "$file" 'all(len(item.get("nodes", [])) >= 3 for item in data)' "each group reports at least 3 nodes"
  assert_json "$file" 'next(item for item in data if item["group_id"] == "g3")["shard_count"] == 0' "g3 starts empty"
}

verify_shard_layout() {
  local file="$1"
  assert_json "$file" 'data["total_shards"] == 8' "total shard count is 8"
  assert_json "$file" 'len(data["assignments"]) == 8' "all shard assignments are visible"
}

verify_select_result() {
  local file="$1"
  local expected_id="$2"
  local expected_name="$3"
  assert_json "$file" 'data["success"] is True' "SQL call succeeds for id '"$expected_id"'"
  assert_json "$file" 'len(data["result"]["rows"]) >= 1' "SQL response returns at least one row for id '"$expected_id"'"
  assert_json "$file" 'any(str(row[0]) == "'"$expected_id"'" and row[1] == "'"$expected_name"'" for row in data["result"]["rows"])' "row id='"$expected_id"' name='"$expected_name"' is present"
}

verify_rebalance_result() {
  local groups_file="$1"
  local before_shards_file="$2"
  local after_shards_file="$3"
  assert_json "$groups_file" 'sorted(item["group_id"] for item in data) == ["g1", "g2", "g3"]' "rebalance keeps g1/g2/g3 online"
  assert_json "$groups_file" 'all(item["shard_count"] > 0 for item in data)' "rebalance gives each group at least one shard"
  python3 - "$before_shards_file" "$after_shards_file" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
    before = json.load(handle)
with open(sys.argv[2], "r", encoding="utf-8") as handle:
    after = json.load(handle)

before_map = {item["shard_id"]: item["group_id"] for item in before["assignments"]}
after_map = {item["shard_id"]: item["group_id"] for item in after["assignments"]}
moved_to_g3 = [shard for shard, group in after_map.items() if group == "g3" and before_map.get(shard) != "g3"]
if not moved_to_g3:
    raise SystemExit("rebalance did not move any shard into g3")
print(f"[PASS] rebalance moved shards into g3: {moved_to_g3}")
PY
}

start_shard_nodes() {
  log "start shard nodes in separate terminal shells"

  open_terminal "g1-n1" "./scripts/run-sharddb-node.sh shard g1-n1 127.0.0.1:21080 127.0.0.1:22080 $TMP_DIR/g1-n1/raft $TMP_DIR/g1-n1/data.db g1 true \"\" $ETCD_ENDPOINT"
  sleep 1
  open_terminal "g1-n2" "./scripts/run-sharddb-node.sh shard g1-n2 127.0.0.1:21180 127.0.0.1:22180 $TMP_DIR/g1-n2/raft $TMP_DIR/g1-n2/data.db g1 false http://127.0.0.1:21080 $ETCD_ENDPOINT"
  open_terminal "g1-n3" "./scripts/run-sharddb-node.sh shard g1-n3 127.0.0.1:21280 127.0.0.1:22280 $TMP_DIR/g1-n3/raft $TMP_DIR/g1-n3/data.db g1 false http://127.0.0.1:21080 $ETCD_ENDPOINT"

  sleep 1
  open_terminal "g2-n1" "./scripts/run-sharddb-node.sh shard g2-n1 127.0.0.1:21081 127.0.0.1:22081 $TMP_DIR/g2-n1/raft $TMP_DIR/g2-n1/data.db g2 true \"\" $ETCD_ENDPOINT"
  sleep 1
  open_terminal "g2-n2" "./scripts/run-sharddb-node.sh shard g2-n2 127.0.0.1:21181 127.0.0.1:22181 $TMP_DIR/g2-n2/raft $TMP_DIR/g2-n2/data.db g2 false http://127.0.0.1:21081 $ETCD_ENDPOINT"
  open_terminal "g2-n3" "./scripts/run-sharddb-node.sh shard g2-n3 127.0.0.1:21281 127.0.0.1:22281 $TMP_DIR/g2-n3/raft $TMP_DIR/g2-n3/data.db g2 false http://127.0.0.1:21081 $ETCD_ENDPOINT"

  sleep 1
  open_terminal "g3-n1" "./scripts/run-sharddb-node.sh shard g3-n1 127.0.0.1:21082 127.0.0.1:22082 $TMP_DIR/g3-n1/raft $TMP_DIR/g3-n1/data.db g3 true \"\" $ETCD_ENDPOINT"
  sleep 1
  open_terminal "g3-n2" "./scripts/run-sharddb-node.sh shard g3-n2 127.0.0.1:21182 127.0.0.1:22182 $TMP_DIR/g3-n2/raft $TMP_DIR/g3-n2/data.db g3 false http://127.0.0.1:21082 $ETCD_ENDPOINT"
  open_terminal "g3-n3" "./scripts/run-sharddb-node.sh shard g3-n3 127.0.0.1:21282 127.0.0.1:22282 $TMP_DIR/g3-n3/raft $TMP_DIR/g3-n3/data.db g3 false http://127.0.0.1:21082 $ETCD_ENDPOINT"
}

start_apiserver() {
  log "start apiserver in a separate terminal shell"
  open_terminal "apiserver" "./scripts/run-sharddb-node.sh apiserver api-1 127.0.0.1:18100 127.0.0.1:30100 $TMP_DIR/api-1/raft $TMP_DIR/api-1/controller.db control false \"\" $ETCD_ENDPOINT"
}

verify_flow() {
  local groups_json="$TMP_DIR/groups.json"
  local shards_json="$TMP_DIR/shards.json"
  local sql_create_json="$TMP_DIR/sql-create.json"
  local sql_insert_1_json="$TMP_DIR/sql-insert-1.json"
  local sql_insert_2_json="$TMP_DIR/sql-insert-2.json"
  local sql_select_1_json="$TMP_DIR/sql-select-1.json"
  local sql_select_2_json="$TMP_DIR/sql-select-2.json"
  local rebalance_groups_json="$TMP_DIR/rebalance-groups.json"
  local rebalance_shards_json="$TMP_DIR/rebalance-shards.json"
  local sql_insert_3_json="$TMP_DIR/sql-insert-3.json"
  local sql_insert_4_json="$TMP_DIR/sql-insert-4.json"
  local sql_select_3_json="$TMP_DIR/sql-select-3.json"
  local sql_select_4_json="$TMP_DIR/sql-select-4.json"

  log "wait for shard node health"
  wait_for_http "http://127.0.0.1:21080/health"
  wait_for_http "http://127.0.0.1:21081/health"
  wait_for_http "http://127.0.0.1:21082/health"

  log "wait for apiserver"
  wait_for_http "$API_URL/health"
  curl -fsS "$API_URL/groups" >"$groups_json" 2>/dev/null || true
  wait_for_file_condition "all shard groups to appear in control plane" 'len(data) == 3 and all(len(item.get("nodes", [])) >= 3 for item in data)' "$groups_json" 90

  echo
  log "verify control plane"
  "$BIN_CLI" --node-url="$API_URL" control groups
  "$BIN_CLI" --node-url="$API_URL" control shards
  http_get_json "$API_URL/groups" "$groups_json"
  http_get_json "$API_URL/shards" "$shards_json"
  verify_group_layout "$groups_json"
  verify_shard_layout "$shards_json"

  echo
  log "verify SQL base flow"
  sql_request "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)" "$sql_create_json"
  sql_request "INSERT INTO users VALUES ($PRIMARY_KEY_A, 'alice')" "$sql_insert_1_json"
  sql_request "INSERT INTO users VALUES ($PRIMARY_KEY_B, 'bob')" "$sql_insert_2_json"
  sql_request "SELECT * FROM users WHERE id = $PRIMARY_KEY_A" "$sql_select_1_json"
  sql_request "SELECT * FROM users WHERE id = $PRIMARY_KEY_B" "$sql_select_2_json"
  cat "$sql_create_json"
  cat "$sql_insert_1_json"
  cat "$sql_insert_2_json"
  cat "$sql_select_1_json"
  cat "$sql_select_2_json"
  assert_json "$sql_create_json" 'data["success"] is True' "create table succeeds"
  assert_json "$sql_insert_1_json" 'data["success"] is True' "insert for key A succeeds"
  assert_json "$sql_insert_2_json" 'data["success"] is True' "insert for key B succeeds"
  verify_select_result "$sql_select_1_json" "$PRIMARY_KEY_A" "alice"
  verify_select_result "$sql_select_2_json" "$PRIMARY_KEY_B" "bob"
  log "key $PRIMARY_KEY_A routes to shard $(route_shard_for_key "users" "$PRIMARY_KEY_A" 8)"
  log "key $PRIMARY_KEY_B routes to shard $(route_shard_for_key "users" "$PRIMARY_KEY_B" 8)"

  echo
  log "verify rebalance"
  "$BIN_CLI" --node-url="$API_URL" control rebalance g1 g2 g3
  "$BIN_CLI" --node-url="$API_URL" control groups
  "$BIN_CLI" --node-url="$API_URL" control shards
  http_get_json "$API_URL/groups" "$rebalance_groups_json"
  http_get_json "$API_URL/shards" "$rebalance_shards_json"
  sql_request "SELECT * FROM users WHERE id = $PRIMARY_KEY_A" "$sql_select_1_json"
  sql_request "SELECT * FROM users WHERE id = $PRIMARY_KEY_B" "$sql_select_2_json"
  sql_request "INSERT INTO users VALUES (3, 'charlie')" "$sql_insert_3_json"
  sql_request "INSERT INTO users VALUES (4, 'david')" "$sql_insert_4_json"
  sql_request "SELECT * FROM users WHERE id = 3" "$sql_select_3_json"
  sql_request "SELECT * FROM users WHERE id = 4" "$sql_select_4_json"
  cat "$rebalance_groups_json"
  cat "$rebalance_shards_json"
  cat "$sql_select_1_json"
  cat "$sql_select_2_json"
  cat "$sql_insert_3_json"
  cat "$sql_insert_4_json"
  cat "$sql_select_3_json"
  cat "$sql_select_4_json"
  verify_rebalance_result "$rebalance_groups_json" "$shards_json" "$rebalance_shards_json"
  verify_shard_layout "$rebalance_shards_json"
  verify_select_result "$sql_select_1_json" "$PRIMARY_KEY_A" "alice"
  verify_select_result "$sql_select_2_json" "$PRIMARY_KEY_B" "bob"
  assert_json "$sql_insert_3_json" 'data["success"] is True' "insert after rebalance succeeds for id 3"
  assert_json "$sql_insert_4_json" 'data["success"] is True' "insert after rebalance succeeds for id 4"
  verify_select_result "$sql_select_3_json" "3" "charlie"
  verify_select_result "$sql_select_4_json" "4" "david"

  cat <<'EOF'

== done ==
Node terminals stay open for manual inspection.
Logs are stored under /tmp/ddb-demo/logs.
If you want to test horizontal scaling further, manually start g4-n1/g4-n2/g4-n3 in new Terminal shells and run:
  ./bin/ddb-cli --node-url=http://127.0.0.1:18100 control rebalance g1 g2 g3 g4
EOF
}

main() {
  cd "$ROOT_DIR"
  if [[ "$CLEANUP_ONLY" == "true" ]]; then
    preflight_checks
    cleanup_demo
    log "cleanup-only completed"
    return 0
  fi
  if [[ "$VERIFY_ONLY" == "false" ]]; then
    preflight_checks
    cleanup_demo
    build_binaries
    start_etcd
    wait_for_http "http://127.0.0.1:2379/health" 30
    start_shard_nodes
    sleep 3
    start_apiserver
  fi
  verify_flow
}

main "$@"
