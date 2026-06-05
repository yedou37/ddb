#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
IMAGE="${DDB_MOCK_IMAGE:-alpine:3.20}"
NETWORK_NAME="${DDB_MOCK_NETWORK:-ddb-five-container-net}"
SUBNET_CIDR="${DDB_MOCK_SUBNET:-10.10.9.0/24}"
GATEWAY_IP="${DDB_MOCK_GATEWAY:-10.10.9.254}"

log() {
  printf '[mock-up] %s\n' "$*"
}

fail() {
  printf '[mock-up][error] %s\n' "$*" >&2
  exit 1
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

remove_container_if_exists() {
  local name="$1"
  docker rm -f "$name" >/dev/null 2>&1 || true
}

ensure_network() {
  if docker network inspect "$NETWORK_NAME" >/dev/null 2>&1; then
    return 0
  fi
  log "creating docker network $NETWORK_NAME ($SUBNET_CIDR, gateway $GATEWAY_IP)"
  docker network create --driver bridge --subnet "$SUBNET_CIDR" --gateway "$GATEWAY_IP" "$NETWORK_NAME" >/dev/null
}

ensure_image() {
  if docker image inspect "$IMAGE" >/dev/null 2>&1; then
    return 0
  fi
  log "pulling image $IMAGE"
  docker pull "$IMAGE" >/dev/null
}

prepare_runtime_dirs() {
  mkdir -p \
    "$ROOT_DIR/.mock-data/five-container" \
    "$ROOT_DIR/.mock-logs/five-container"

  local container_name
  for container_name in t9-ct1 t9-ct2 t9-ct3 t9-ct4 t9-ct5; do
    mkdir -p \
      "$ROOT_DIR/.mock-data/five-container/$container_name" \
      "$ROOT_DIR/.mock-logs/five-container/$container_name"
  done
}

run_container() {
  local name="$1"
  local ip="$2"

  log "starting $name ($ip)"
  docker run -d \
    --name "$name" \
    --hostname "$name" \
    --network "$NETWORK_NAME" \
    --ip "$ip" \
    -v "$ROOT_DIR/.mock-data/five-container/$name:/data" \
    -v "$ROOT_DIR/.mock-logs/five-container/$name:/logs" \
    "$IMAGE" \
    sh -lc 'mkdir -p /data /logs && while true; do sleep 3600; done' >/dev/null
}

main() {
  cd "$ROOT_DIR"
  require_command docker

  ensure_image
  ensure_network
  prepare_runtime_dirs

  remove_container_if_exists t9-ct1
  remove_container_if_exists t9-ct2
  remove_container_if_exists t9-ct3
  remove_container_if_exists t9-ct4
  remove_container_if_exists t9-ct5

  run_container t9-ct1 10.10.9.1
  run_container t9-ct2 10.10.9.2
  run_container t9-ct3 10.10.9.3
  run_container t9-ct4 10.10.9.4
  run_container t9-ct5 10.10.9.5

  cat <<'EOF'

empty mock environment started

containers:
  t9-ct1 10.10.9.1
  t9-ct2 10.10.9.2
  t9-ct3 10.10.9.3
  t9-ct4 10.10.9.4
  t9-ct5 10.10.9.5

attach shells:
  docker exec -it t9-ct1 sh
  docker exec -it t9-ct2 sh
  docker exec -it t9-ct3 sh
  docker exec -it t9-ct4 sh
  docker exec -it t9-ct5 sh

runtime dirs inside every container:
  /data
  /logs

note:
  containers do not see local source code or binaries by default
  upload artifacts explicitly, e.g. ./mock/five-container/push-artifacts.sh <artifacts-dir>
EOF
}

main "$@"
