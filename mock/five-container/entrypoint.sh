#!/usr/bin/env bash
set -euo pipefail

CONTAINER_NAME="${MOCK_CONTAINER_NAME:-$(hostname)}"
CONTAINER_IP="${MOCK_CONTAINER_IP:-}"
RUN_ETCD="${RUN_ETCD:-false}"
ETCD_HTTP_ADDR="${ETCD_HTTP_ADDR:-http://10.10.9.1:2379}"
DDB_PROCESSES="${DDB_PROCESSES:-}"
DATA_ROOT="${MOCK_DATA_ROOT:-/data}"
LOG_ROOT="${MOCK_LOG_ROOT:-/logs}"
RESTART_DELAY_SECONDS="${RESTART_DELAY_SECONDS:-2}"

declare -a SUPERVISOR_PIDS=()

log() {
  printf '[mock][%s] %s\n' "$CONTAINER_NAME" "$*"
}

fail() {
  printf '[mock][%s][error] %s\n' "$CONTAINER_NAME" "$*" >&2
  exit 1
}

wait_for_http() {
  local url="$1"
  local attempts="${2:-120}"
  local sleep_seconds="${3:-1}"
  local try

  for ((try = 1; try <= attempts; try++)); do
    if curl -fsS "$url" >/dev/null 2>&1; then
      return 0
    fi
    sleep "$sleep_seconds"
  done
  return 1
}

cleanup() {
  trap - EXIT INT TERM
  for pid in "${SUPERVISOR_PIDS[@]:-}"; do
    kill "$pid" >/dev/null 2>&1 || true
  done
  wait || true
}

start_etcd() {
  mkdir -p "$DATA_ROOT/etcd" "$LOG_ROOT"
  log "starting etcd on ${CONTAINER_IP}:2379"
  etcd \
    --name "$CONTAINER_NAME" \
    --data-dir "$DATA_ROOT/etcd" \
    --advertise-client-urls "http://${CONTAINER_IP}:2379" \
    --listen-client-urls "http://0.0.0.0:2379" \
    >"$LOG_ROOT/etcd.log" 2>"$LOG_ROOT/etcd.err.log" &
  SUPERVISOR_PIDS+=("$!")

  if ! wait_for_http "${ETCD_HTTP_ADDR}/health" 60 1; then
    fail "etcd did not become healthy at ${ETCD_HTTP_ADDR}"
  fi
}

start_ddb_supervisor() {
  local definition="$1"
  local node_id role group_id http_addr raft_addr bootstrap join_addr controller_addrs

  IFS='|' read -r node_id role group_id http_addr raft_addr bootstrap join_addr controller_addrs _ <<< "${definition}|"

  if [[ -z "$node_id" || -z "$role" || -z "$http_addr" || -z "$raft_addr" ]]; then
    fail "invalid DDB_PROCESSES entry: ${definition}"
  fi

  (
    local db_path raft_dir exit_code
    db_path="${DATA_ROOT}/${node_id}/data.db"
    raft_dir="${DATA_ROOT}/${node_id}/raft"
    mkdir -p "$raft_dir" "$LOG_ROOT"

    while true; do
      if [[ -n "$ETCD_HTTP_ADDR" ]]; then
        wait_for_http "${ETCD_HTTP_ADDR}/health" 120 1 || true
      fi

      if [[ -n "$join_addr" ]]; then
        if ! wait_for_http "${join_addr%/}/health" 120 1; then
          log "join target not ready for ${node_id}: ${join_addr}"
          sleep "$RESTART_DELAY_SECONDS"
          continue
        fi
      fi

      log "starting ${node_id} role=${role} group=${group_id:-<none>}"
      env \
        NODE_ID="$node_id" \
        ROLE="$role" \
        GROUP_ID="$group_id" \
        HTTP_ADDR="$http_addr" \
        RAFT_ADDR="$raft_addr" \
        RAFT_DIR="$raft_dir" \
        DB_PATH="$db_path" \
        BOOTSTRAP="${bootstrap:-false}" \
        JOIN_ADDR="$join_addr" \
        CONTROLLER_ADDRS="$controller_addrs" \
        ETCD_ADDR="${ETCD_HTTP_ADDR#http://}" \
        ddb-server \
        >"$LOG_ROOT/${node_id}.log" \
        2>"$LOG_ROOT/${node_id}.err.log"
      exit_code=$?
      log "${node_id} exited with code ${exit_code}; restarting in ${RESTART_DELAY_SECONDS}s"
      sleep "$RESTART_DELAY_SECONDS"
    done
  ) &

  SUPERVISOR_PIDS+=("$!")
}

main() {
  [[ -n "$CONTAINER_IP" ]] || fail "MOCK_CONTAINER_IP is required"

  mkdir -p "$DATA_ROOT" "$LOG_ROOT" /run/mock
  trap cleanup EXIT INT TERM

  if [[ "$RUN_ETCD" == "true" ]]; then
    start_etcd
  fi

  while IFS= read -r definition || [[ -n "$definition" ]]; do
    [[ -z "${definition// }" ]] && continue
    [[ "$definition" == \#* ]] && continue
    start_ddb_supervisor "$definition"
  done <<< "$DDB_PROCESSES"

  if [[ "${#SUPERVISOR_PIDS[@]}" -eq 0 ]]; then
    fail "no child processes configured"
  fi

  wait -n "${SUPERVISOR_PIDS[@]}"
}

main "$@"
