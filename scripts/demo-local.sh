#!/usr/bin/env bash
set -euo pipefail

cat <<'EOF'
Local demo flow
===============

1. Start etcd
docker run -d --name ddb-etcd -p 2379:2379 \
  quay.io/coreos/etcd:v3.5.9 \
  etcd --advertise-client-urls=http://127.0.0.1:2379 \
       --listen-client-urls=http://0.0.0.0:2379

2. Start node1
go run ./cmd/server \
  --node-id=node1 \
  --http-addr=127.0.0.1:20080 \
  --raft-addr=127.0.0.1:20000 \
  --raft-dir=/tmp/ddb-node1/raft \
  --db-path=/tmp/ddb-node1/data.db \
  --bootstrap=true \
  --etcd=127.0.0.1:2379

3. Start node2
go run ./cmd/server \
  --node-id=node2 \
  --http-addr=127.0.0.1:20081 \
  --raft-addr=127.0.0.1:20001 \
  --raft-dir=/tmp/ddb-node2/raft \
  --db-path=/tmp/ddb-node2/data.db \
  --etcd=127.0.0.1:2379

4. Start node3
go run ./cmd/server \
  --node-id=node3 \
  --http-addr=127.0.0.1:20082 \
  --raft-addr=127.0.0.1:20002 \
  --raft-dir=/tmp/ddb-node3/raft \
  --db-path=/tmp/ddb-node3/data.db \
  --etcd=127.0.0.1:2379

5. Run demo commands
go run ./cmd/cli --etcd=127.0.0.1:2379 cluster members
go run ./cmd/cli --etcd=127.0.0.1:2379 sql "CREATE TABLE books (id INT PRIMARY KEY, name TEXT)"
go run ./cmd/cli --etcd=127.0.0.1:2379 sql "INSERT INTO books VALUES (1, 'raft')"
go run ./cmd/cli --node-url=http://127.0.0.1:20081 sql "SELECT * FROM books"
go run ./cmd/cli --node-url=http://127.0.0.1:20082 sql "SELECT * FROM books"

6. Optional membership demo
go run ./cmd/cli --etcd=127.0.0.1:2379 cluster remove node3
EOF
