#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
REMOTE_ROOT="${DDB_REMOTE_ROOT:-/opt/ddb}"
TARGETS=(t9-ct1 t9-ct2 t9-ct3 t9-ct4 t9-ct5)

usage() {
  cat <<'EOF'
usage:
  ./mock/five-container/push-artifacts.sh <artifacts-dir>

description:
  simulate a real upload step by explicitly copying local artifacts into all five mock containers.

expected layout inside <artifacts-dir>:
  ddb-server
  ddb-cli
  etcd                  optional
  configs/              optional

environment:
  DDB_REMOTE_ROOT       remote install root inside containers, default /opt/ddb
EOF
}

log() {
  printf '[push-artifacts] %s\n' "$*"
}

fail() {
  printf '[push-artifacts][error] %s\n' "$*" >&2
  exit 1
}

require_container() {
  local name="$1"
  docker inspect "$name" >/dev/null 2>&1 || fail "container not found: $name, run ./mock/five-container/up.sh first"
}

copy_if_exists() {
  local source_path="$1"
  local target_name="$2"
  local remote_dir="$3"

  if [[ ! -e "$source_path" ]]; then
    return 0
  fi

  docker exec "$target_name" sh -lc "mkdir -p '$remote_dir'"
  docker cp "$source_path" "$target_name:$remote_dir/"
}

main() {
  local artifacts_dir
  artifacts_dir="${1:-}"

  [[ -n "$artifacts_dir" ]] || {
    usage
    exit 1
  }

  if [[ "${artifacts_dir}" != /* ]]; then
    artifacts_dir="${ROOT_DIR}/${artifacts_dir}"
  fi
  [[ -d "$artifacts_dir" ]] || fail "artifacts dir not found: $artifacts_dir"
  [[ -f "$artifacts_dir/ddb-server" ]] || fail "missing required file: $artifacts_dir/ddb-server"
  [[ -f "$artifacts_dir/ddb-cli" ]] || fail "missing required file: $artifacts_dir/ddb-cli"

  local target
  for target in "${TARGETS[@]}"; do
    require_container "$target"
    log "uploading artifacts to $target"
    docker exec "$target" sh -lc "mkdir -p '$REMOTE_ROOT/bin' '$REMOTE_ROOT/configs' '$REMOTE_ROOT/logs' '$REMOTE_ROOT/data'"
    copy_if_exists "$artifacts_dir/ddb-server" "$target" "$REMOTE_ROOT/bin"
    copy_if_exists "$artifacts_dir/ddb-cli" "$target" "$REMOTE_ROOT/bin"
    copy_if_exists "$artifacts_dir/etcd" "$target" "$REMOTE_ROOT/bin"
    if [[ -d "$artifacts_dir/configs" ]]; then
      docker exec "$target" sh -lc "rm -rf '$REMOTE_ROOT/configs'/*"
      docker cp "$artifacts_dir/configs/." "$target:$REMOTE_ROOT/configs/"
    fi
    docker exec "$target" sh -lc "chmod +x '$REMOTE_ROOT/bin/'* 2>/dev/null || true"
  done

  cat <<EOF

artifacts uploaded

remote root:
  $REMOTE_ROOT

example checks:
  docker exec t9-ct1 sh -lc 'ls -R $REMOTE_ROOT'
  docker exec t9-ct1 sh -lc '$REMOTE_ROOT/bin/ddb-cli --help'
EOF
}

main "$@"
