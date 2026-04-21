#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="/Users/bytedance/dbd"
IMAGE="${DDB_IMAGE:-ghcr.io/yedou37/ddb:latest}"
NETWORK_NAME="${DDB_DOCKER_NETWORK:-ddb-demo-net}"
ETCD_CONTAINER="${DDB_ETCD_CONTAINER:-ddb-demo-etcd}"
API_URL="${DDB_API_URL:-http://127.0.0.1:18100}"
ETCD_HOST_PORT="${DDB_ETCD_HOST_PORT:-2379}"
API_HOST_PORT="${DDB_API_HOST_PORT:-18100}"
SEED_ROW_COUNT="${DDB_SEED_ROW_COUNT:-40}"
TMP_DIR="/tmp/ddb-demo-docker"

usage() {
  cat <<'EOF'
usage:
  ./scripts/demo-single-host-sharddb-docker.sh [start-only|verify-only|seed-only|cleanup-only]

description:
  run the single-host ShardDB demo entirely with Docker containers.

modes:
  start-only   clean up and start the environment, but skip verification
  verify-only  only run verification against an already running environment
  seed-only    create the demo table and insert enough test data into a running environment
  cleanup-only only remove demo containers, network, and temporary files

environment:
  DDB_IMAGE            image to use, default ghcr.io/yedou37/ddb:latest
  DDB_DOCKER_NETWORK   docker network name, default ddb-demo-net
  DDB_SEED_ROW_COUNT   row count for seed-only, default 40
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
  usage
  exit 0
fi

MODE="${1:-}"
START_ONLY=false
VERIFY_ONLY=false
SEED_ONLY=false
CLEANUP_ONLY=false

case "$MODE" in
  "" ) ;;
  start-only) START_ONLY=true ;;
  verify-only) VERIFY_ONLY=true ;;
  seed-only) SEED_ONLY=true ;;
  cleanup-only) CLEANUP_ONLY=true ;;
  *)
    usage
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
  local name="$1"
  command -v "$name" >/dev/null 2>&1 || fail "required command not found: $name"
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

json_get() {
  local file="$1"
  local expression="$2"
  python3 - "$file" "$expression" <<'PY'
import json
import sys

with open(sys.argv[1], "r", encoding="utf-8") as handle:
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
value = eval(sys.argv[2], safe_globals, {"data": payload})
if isinstance(value, (dict, list)):
    print(json.dumps(value))
elif value is None:
    print("")
else:
    print(value)
PY
}

sql_payload() {
  python3 - "$1" <<'PY'
import json
import sys
print(json.dumps({"sql": sys.argv[1]}))
PY
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

seed_users_rows() {
  python3 - "$SEED_ROW_COUNT" <<'PY'
import sys

count = int(sys.argv[1])
if count < 1:
    count = 1

for i in range(1, count + 1):
    print(f"{i}|user-{i:03d}")
PY
}

container_name() {
  printf 'ddb-demo-%s' "$1"
}

docker_rm_if_exists() {
  local name="$1"
  docker rm -f "$name" >/dev/null 2>&1 || true
}

docker_volume_rm_if_exists() {
  local name="$1"
  docker volume rm "$name" >/dev/null 2>&1 || true
}

ensure_network() {
  if ! docker network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
    docker network create "$NETWORK_NAME" >/dev/null
  fi
}

ensure_image() {
  if ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
    log "pull image $IMAGE"
    docker pull "$IMAGE"
  fi
}

cleanup_demo() {
  log "cleanup demo containers"
  local names=(
    "$ETCD_CONTAINER"
    "$(container_name api-1)"
    "$(container_name g1-n1)" "$(container_name g1-n2)" "$(container_name g1-n3)"
    "$(container_name g2-n1)" "$(container_name g2-n2)" "$(container_name g2-n3)"
    "$(container_name g3-n1)" "$(container_name g3-n2)" "$(container_name g3-n3)"
  )
  local name
  for name in "${names[@]}"; do
    docker_rm_if_exists "$name"
  done

  log "cleanup demo volumes"
  local volumes=(
    "ddb-demo-api-1-data"
    "ddb-demo-g1-n1-data" "ddb-demo-g1-n2-data" "ddb-demo-g1-n3-data"
    "ddb-demo-g2-n1-data" "ddb-demo-g2-n2-data" "ddb-demo-g2-n3-data"
    "ddb-demo-g3-n1-data" "ddb-demo-g3-n2-data" "ddb-demo-g3-n3-data"
  )
  for name in "${volumes[@]}"; do
    docker_volume_rm_if_exists "$name"
  done

  log "cleanup docker network"
  docker network rm "$NETWORK_NAME" >/dev/null 2>&1 || true

  rm -rf "$TMP_DIR"
}

run_server() {
  local name="$1"
  local host_http_port="$2"
  local host_raft_port="$3"
  local volume_name="$4"
  shift 4

  docker run -d \
    --name "$name" \
    --restart unless-stopped \
    --network "$NETWORK_NAME" \
    -p "${host_http_port}:${host_http_port}" \
    -p "${host_raft_port}:${host_raft_port}" \
    -v "${volume_name}:/data" \
    "$@" \
    "$IMAGE" >/dev/null
}

start_etcd() {
  log "start etcd container"
  docker run -d \
    --name "$ETCD_CONTAINER" \
    --restart unless-stopped \
    --network "$NETWORK_NAME" \
    -p "${ETCD_HOST_PORT}:2379" \
    quay.io/coreos/etcd:v3.5.9 \
    etcd \
    --advertise-client-urls=http://${ETCD_CONTAINER}:2379 \
    --listen-client-urls=http://0.0.0.0:2379 >/dev/null

  wait_for_http "http://127.0.0.1:${ETCD_HOST_PORT}/health" 60 1
}

start_api_server() {
  log "start apiserver"
  run_server \
    "$(container_name api-1)" \
    "$API_HOST_PORT" \
    "30100" \
    "ddb-demo-api-1-data" \
    -e ROLE=apiserver \
    -e NODE_ID=api-1 \
    -e GROUP_ID=control \
    -e HTTP_ADDR="$(container_name api-1):${API_HOST_PORT}" \
    -e RAFT_ADDR="$(container_name api-1):30100" \
    -e RAFT_DIR=/data/raft \
    -e DB_PATH=/data/apiserver.db \
    -e BOOTSTRAP=false \
    -e ETCD_ADDR="${ETCD_CONTAINER}:2379"

  wait_for_http "$API_URL/health" 60 1
}

start_shard_nodes() {
  log "start g1"
  run_server \
    "$(container_name g1-n1)" "21080" "22080" "ddb-demo-g1-n1-data" \
    -e ROLE=shard -e NODE_ID=g1-n1 -e GROUP_ID=g1 \
    -e HTTP_ADDR="$(container_name g1-n1):21080" -e RAFT_ADDR="$(container_name g1-n1):22080" \
    -e RAFT_DIR=/data/raft -e DB_PATH=/data/data.db \
    -e BOOTSTRAP=true -e ETCD_ADDR="${ETCD_CONTAINER}:2379"
  run_server \
    "$(container_name g1-n2)" "21180" "22180" "ddb-demo-g1-n2-data" \
    -e ROLE=shard -e NODE_ID=g1-n2 -e GROUP_ID=g1 \
    -e HTTP_ADDR="$(container_name g1-n2):21180" -e RAFT_ADDR="$(container_name g1-n2):22180" \
    -e RAFT_DIR=/data/raft -e DB_PATH=/data/data.db \
    -e BOOTSTRAP=false -e JOIN_ADDR="http://$(container_name g1-n1):21080" \
    -e ETCD_ADDR="${ETCD_CONTAINER}:2379"
  run_server \
    "$(container_name g1-n3)" "21280" "22280" "ddb-demo-g1-n3-data" \
    -e ROLE=shard -e NODE_ID=g1-n3 -e GROUP_ID=g1 \
    -e HTTP_ADDR="$(container_name g1-n3):21280" -e RAFT_ADDR="$(container_name g1-n3):22280" \
    -e RAFT_DIR=/data/raft -e DB_PATH=/data/data.db \
    -e BOOTSTRAP=false -e JOIN_ADDR="http://$(container_name g1-n1):21080" \
    -e ETCD_ADDR="${ETCD_CONTAINER}:2379"

  log "start g2"
  run_server \
    "$(container_name g2-n1)" "21081" "22081" "ddb-demo-g2-n1-data" \
    -e ROLE=shard -e NODE_ID=g2-n1 -e GROUP_ID=g2 \
    -e HTTP_ADDR="$(container_name g2-n1):21081" -e RAFT_ADDR="$(container_name g2-n1):22081" \
    -e RAFT_DIR=/data/raft -e DB_PATH=/data/data.db \
    -e BOOTSTRAP=true -e ETCD_ADDR="${ETCD_CONTAINER}:2379"
  run_server \
    "$(container_name g2-n2)" "21181" "22181" "ddb-demo-g2-n2-data" \
    -e ROLE=shard -e NODE_ID=g2-n2 -e GROUP_ID=g2 \
    -e HTTP_ADDR="$(container_name g2-n2):21181" -e RAFT_ADDR="$(container_name g2-n2):22181" \
    -e RAFT_DIR=/data/raft -e DB_PATH=/data/data.db \
    -e BOOTSTRAP=false -e JOIN_ADDR="http://$(container_name g2-n1):21081" \
    -e ETCD_ADDR="${ETCD_CONTAINER}:2379"
  run_server \
    "$(container_name g2-n3)" "21281" "22281" "ddb-demo-g2-n3-data" \
    -e ROLE=shard -e NODE_ID=g2-n3 -e GROUP_ID=g2 \
    -e HTTP_ADDR="$(container_name g2-n3):21281" -e RAFT_ADDR="$(container_name g2-n3):22281" \
    -e RAFT_DIR=/data/raft -e DB_PATH=/data/data.db \
    -e BOOTSTRAP=false -e JOIN_ADDR="http://$(container_name g2-n1):21081" \
    -e ETCD_ADDR="${ETCD_CONTAINER}:2379"

  log "start g3"
  run_server \
    "$(container_name g3-n1)" "21082" "22082" "ddb-demo-g3-n1-data" \
    -e ROLE=shard -e NODE_ID=g3-n1 -e GROUP_ID=g3 \
    -e HTTP_ADDR="$(container_name g3-n1):21082" -e RAFT_ADDR="$(container_name g3-n1):22082" \
    -e RAFT_DIR=/data/raft -e DB_PATH=/data/data.db \
    -e BOOTSTRAP=true -e ETCD_ADDR="${ETCD_CONTAINER}:2379"
  run_server \
    "$(container_name g3-n2)" "21182" "22182" "ddb-demo-g3-n2-data" \
    -e ROLE=shard -e NODE_ID=g3-n2 -e GROUP_ID=g3 \
    -e HTTP_ADDR="$(container_name g3-n2):21182" -e RAFT_ADDR="$(container_name g3-n2):22182" \
    -e RAFT_DIR=/data/raft -e DB_PATH=/data/data.db \
    -e BOOTSTRAP=false -e JOIN_ADDR="http://$(container_name g3-n1):21082" \
    -e ETCD_ADDR="${ETCD_CONTAINER}:2379"
  run_server \
    "$(container_name g3-n3)" "21282" "22282" "ddb-demo-g3-n3-data" \
    -e ROLE=shard -e NODE_ID=g3-n3 -e GROUP_ID=g3 \
    -e HTTP_ADDR="$(container_name g3-n3):21282" -e RAFT_ADDR="$(container_name g3-n3):22282" \
    -e RAFT_DIR=/data/raft -e DB_PATH=/data/data.db \
    -e BOOTSTRAP=false -e JOIN_ADDR="http://$(container_name g3-n1):21082" \
    -e ETCD_ADDR="${ETCD_CONTAINER}:2379"
}

seed_demo_data() {
  local create_json="$TMP_DIR/seed-create-users.json"
  local select_json="$TMP_DIR/seed-select-users.json"
  local insert_json
  local delete_json
  local inserted=0

  mkdir -p "$TMP_DIR"
  log "wait for apiserver before seeding"
  wait_for_http "$API_URL/health" 60 1

  log "ensure users table exists"
  if ! sql_request "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)" "$create_json"; then
    warn "CREATE TABLE users returned non-zero; continuing"
  fi

  log "clear existing users demo rows in range 1..$SEED_ROW_COUNT"
  while IFS='|' read -r row_id row_name; do
    [[ -z "$row_id" ]] && continue
    delete_json="$TMP_DIR/seed-delete-${row_id}.json"
    sql_request "DELETE FROM users WHERE id = ${row_id}" "$delete_json" || true
  done < <(seed_users_rows)

  log "insert $SEED_ROW_COUNT demo rows into users"
  while IFS='|' read -r row_id row_name; do
    [[ -z "$row_id" ]] && continue
    insert_json="$TMP_DIR/seed-insert-${row_id}.json"
    sql_request "INSERT INTO users VALUES (${row_id}, '${row_name}')" "$insert_json"
    assert_json "$insert_json" 'data["success"] is True' "insert succeeds for id ${row_id}"
    inserted=$((inserted + 1))
  done < <(seed_users_rows)

  sql_request "SELECT * FROM users WHERE id = 1" "$select_json"
  assert_json "$select_json" 'data["success"] is True and len(data["result"]["rows"]) == 1' "seeded row 1 is readable"

  cat <<EOF

== seeded ==
Seeded table: users
Inserted rows: $inserted
Dashboard:
  ${API_URL}/dashboard/
EOF
}

verify_flow() {
  mkdir -p "$TMP_DIR"
  log "verify control plane"
  curl -fsS "$API_URL/health" >"$TMP_DIR/health.json"
  curl -fsS "$API_URL/groups" >"$TMP_DIR/groups.json"
  curl -fsS "$API_URL/shards" >"$TMP_DIR/shards.json"
  assert_json "$TMP_DIR/groups.json" 'len(data) == 3' "three shard groups are visible"
  assert_json "$TMP_DIR/shards.json" 'len(data["assignments"]) > 0' "shard assignments exist"

  log "verify seeded data path"
  sql_request "SELECT * FROM users WHERE id = 1" "$TMP_DIR/verify-user-1.json"
  assert_json "$TMP_DIR/verify-user-1.json" 'data["success"] is True and len(data["result"]["rows"]) == 1' "users row id=1 is readable"
}

start_demo() {
  ensure_image
  ensure_network
  mkdir -p "$TMP_DIR"
  start_etcd
  start_shard_nodes
  sleep 3
  start_api_server
}

main() {
  cd "$ROOT_DIR"
  require_command docker
  require_command curl
  require_command python3

  if [[ "$CLEANUP_ONLY" == "true" ]]; then
    cleanup_demo
    return 0
  fi

  if [[ "$VERIFY_ONLY" == "true" ]]; then
    verify_flow
    return 0
  fi

  if [[ "$SEED_ONLY" == "true" ]]; then
    seed_demo_data
    return 0
  fi

  cleanup_demo
  start_demo

  if [[ "$START_ONLY" == "true" ]]; then
    cat <<EOF

== environment started ==
Containers:
  $ETCD_CONTAINER
  $(container_name api-1)
  $(container_name g1-n1) $(container_name g1-n2) $(container_name g1-n3)
  $(container_name g2-n1) $(container_name g2-n2) $(container_name g2-n3)
  $(container_name g3-n1) $(container_name g3-n2) $(container_name g3-n3)
Dashboard:
  ${API_URL}/dashboard/
You can seed data later with:
  ./scripts/demo-single-host-sharddb-docker.sh seed-only
EOF
    return 0
  fi

  seed_demo_data
  verify_flow
}

main "$@"
