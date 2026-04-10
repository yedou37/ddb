#!/usr/bin/env bash
set -euo pipefail

echo "[1/4] Starting etcd + 3 nodes"
docker compose up --build -d

echo "[2/4] Waiting for cluster to stabilize"
sleep 8

echo "[3/4] Running demo commands"
go run ./cmd/cli --etcd=127.0.0.1:2379 cluster members
go run ./cmd/cli --etcd=127.0.0.1:2379 cluster leader
go run ./cmd/cli --etcd=127.0.0.1:2379 sql "CREATE TABLE books (id INT PRIMARY KEY, name TEXT)"
go run ./cmd/cli --etcd=127.0.0.1:2379 sql "INSERT INTO books VALUES (1, 'raft')"
go run ./cmd/cli --node-url=http://127.0.0.1:18081 sql "SELECT * FROM books"
go run ./cmd/cli --node-url=http://127.0.0.1:18082 sql "SELECT * FROM books"

echo "[4/4] Done"
echo "Stop the cluster with: docker compose down -v"
