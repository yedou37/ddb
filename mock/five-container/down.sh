#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
NETWORK_NAME="${DDB_MOCK_NETWORK:-ddb-five-container-net}"

log() {
  printf '[mock-down] %s\n' "$*"
}

remove_container_if_exists() {
  local name="$1"
  docker rm -f "$name" >/dev/null 2>&1 || true
}

main() {
  cd "$ROOT_DIR"

  log "removing containers"
  remove_container_if_exists t9-ct1
  remove_container_if_exists t9-ct2
  remove_container_if_exists t9-ct3
  remove_container_if_exists t9-ct4
  remove_container_if_exists t9-ct5

  log "removing network $NETWORK_NAME"
  docker network rm "$NETWORK_NAME" >/dev/null 2>&1 || true

  cat <<'EOF'

mock environment stopped

runtime state remains on disk:
  .mock-data/five-container
  .mock-logs/five-container
EOF
}

main "$@"
