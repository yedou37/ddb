#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 7 ]]; then
  cat <<'EOF'
usage:
  run-sharddb-node.sh <role> <node-id> <http-addr> <raft-addr> <raft-dir> <db-path> <group-id> [bootstrap] [join-addr] [etcd]

examples:
  # single control plane on machine1
  ./scripts/run-sharddb-node.sh apiserver api-1 192.168.1.10:18100 127.0.0.1:30100 /tmp/api-1/raft /tmp/api-1/controller.db control false "" 192.168.1.10:2379
  ./scripts/run-sharddb-node.sh shard g1-n1 192.168.1.10:21080 192.168.1.10:22080 /tmp/g1-n1/raft /tmp/g1-n1/data.db g1 true "" 192.168.1.10:2379
  ./scripts/run-sharddb-node.sh controller ctrl-1 192.168.1.10:18080 127.0.0.1:0 /tmp/ctrl-1/raft /tmp/ctrl-1/controller.db control false "" 192.168.1.10:2379
EOF
  exit 1
fi

ROLE="$1"
NODE_ID="$2"
HTTP_ADDR="$3"
RAFT_ADDR="$4"
RAFT_DIR="$5"
DB_PATH="$6"
GROUP_ID="$7"
BOOTSTRAP="${8:-false}"
JOIN_ADDR="${9:-}"
ETCD_ADDR="${10:-}"

exec go run ./cmd/server \
  --role="${ROLE}" \
  --node-id="${NODE_ID}" \
  --http-addr="${HTTP_ADDR}" \
  --raft-addr="${RAFT_ADDR}" \
  --raft-dir="${RAFT_DIR}" \
  --db-path="${DB_PATH}" \
  --group-id="${GROUP_ID}" \
  --bootstrap="${BOOTSTRAP}" \
  --join="${JOIN_ADDR}" \
  --etcd="${ETCD_ADDR}"
