# DDB / ShardDB

A distributed database prototype built with Go, BoltDB, HashiCorp Raft, and etcd.

The repository now contains two closely related layers:

- a replicated single-group DDB
- a ShardDB control plane and data plane with `controller`, `apiserver`, and multi-group shard replicas

## Current Scope

### Core Building Blocks

- Raft-based replication inside each replica group
- BoltDB-backed local state storage
- etcd-based service discovery and leader lookup
- explicit membership operations such as `remove` and `rejoin`
- HTTP API, CLI, demo scripts, and dashboard

### ShardDB Features

- role-based startup: `controller`, `apiserver`, `shard`
- shared control-plane config via etcd
- shard placement and shard-group metadata
- `move-shard` and `rebalance`
- migration safety using shard-level locks
- retry semantics during migration with `503 + Retry-After`
- a browser dashboard for topology, shard map, health, and table browsing

## Supported SQL

Current MVP SQL coverage:

- `CREATE TABLE`
- `INSERT INTO ... VALUES (...)`
- `SELECT ... FROM ... [WHERE ...]`
- `DELETE FROM ... WHERE ...`
- `SHOW TABLES`
- restricted equality `JOIN` support in ShardDB tests and coordinator flow

Notes:

- routing is primary-key oriented for the MVP path
- migration safety is prioritized over transparent access during shard moves

## Repository Layout

- `cmd/server`: server entrypoint for all roles
- `cmd/cli`: CLI entrypoint
- `internal/api`: DDB HTTP handlers
- `internal/apiserver`: ShardDB API server handlers and dashboard
- `internal/app`: app assembly and lifecycle
- `internal/config`: config parsing
- `internal/controller`: control-plane logic and shard management
- `internal/coordinator`: SQL routing and control-plane execution
- `internal/discovery`: etcd integration
- `internal/model`: shared API models
- `internal/raftnode`: Raft node wrapper and FSM
- `internal/router`: shard router
- `internal/shardmeta`: shard metadata types
- `internal/sql`: SQL parser
- `internal/storage`: BoltDB storage
- `scripts`: local demo scripts
- `test/e2e`: in-process and black-box end-to-end tests
- `docs`: demo and course-oriented runbooks

## Quick Start

### Build

```bash
go build -o ./bin/ddb-server ./cmd/server
go build -o ./bin/ddb-cli ./cmd/cli
```

### Start Single-Host ShardDB Demo

Clean the local demo environment:

```bash
./scripts/demo-single-host-sharddb.sh cleanup-only
```

Start the environment without built-in verification:

```bash
./scripts/demo-single-host-sharddb.sh start-only
```

Seed demo data into the running environment:

```bash
./scripts/demo-single-host-sharddb.sh seed-only
```

Open the dashboard:

```bash
open http://127.0.0.1:18100/dashboard/
```

The dashboard currently shows:

- cluster topology
- node and group health
- shard map and migrating shards
- table browser with manual `SELECT *` loading

## CLI Examples

Inspect the control plane:

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control config
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control groups
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control shards
```

Read and write application data:

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "INSERT INTO users VALUES (1, 'alice')"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 1"
```

Move one shard:

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control move-shard 6 g3
```

Rebalance across groups:

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control rebalance g1 g2 g3
```

Scale out to a new group and rebalance:

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control rebalance g1 g2 g3 g4
```

## Membership Management

### Remove a Node

```bash
./bin/ddb-cli --etcd=127.0.0.1:2379 cluster remove node3
```

### Rejoin a Removed Node

Start the node with `--rejoin=true`:

```bash
go run ./cmd/server \
  --node-id=node3 \
  --http-addr=127.0.0.1:20082 \
  --raft-addr=127.0.0.1:20002 \
  --raft-dir=/tmp/ddb-node3/raft \
  --db-path=/tmp/ddb-node3/data.db \
  --etcd=127.0.0.1:2379 \
  --rejoin=true
```

Then recover the logical member:

```bash
go run ./cmd/cli --etcd=127.0.0.1:2379 cluster rejoin node3 127.0.0.1:20002 127.0.0.1:20082
```

## Dashboard

The ShardDB dashboard is served directly by `apiserver` and uses polling from:

- `/dashboard/api/overview`
- `/dashboard/api/table-data?table=<name>`

Highlights:

- no separate frontend build step
- static assets embedded with `go:embed`
- polling-based topology and health updates
- manual table browsing to avoid heavy periodic data scans

## Demo Scripts

- `scripts/demo-single-host-sharddb.sh`
  - `cleanup-only`
  - `start-only`
  - `verify-only`
  - `seed-only`
- `scripts/run-sharddb-node.sh`
  - start one `controller`, `apiserver`, or `shard` process
- `scripts/demo-local.sh`
  - quick local flow
- `scripts/demo-compose.sh`
  - compose-based demo

## Documentation

- [single-host-sharddb-dashboard-demo.md](file:///Users/bytedance/dbd/docs/single-host-sharddb-dashboard-demo.md)
  - single-host dashboard demo runbook
- [three-machine-sharddb-demo.md](file:///Users/bytedance/dbd/docs/three-machine-sharddb-demo.md)
  - recommended three-machine classroom demo topology
- `docs/assignment.txt`
  - course assignment requirements

## Tests

Run all tests:

```bash
go test ./...
```

Run only e2e tests:

```bash
go test ./test/e2e -v
```

The repository now includes both:

- in-process e2e tests for fast regression coverage
- black-box e2e tests that build `cmd/server`, spawn real processes, and validate real HTTP/Raft/discovery behavior

Covered scenarios include:

- replication and leader failover
- quorum loss rejection
- remove/rejoin and catch-up
- discovery-based auto join
- members state transitions
- ShardDB control-plane config sharing
- apiserver restart and config reload
- shard movement and rebalance flows

## Docker

Build the image:

```bash
docker build -t ddb:latest .
```

Bring up the compose environment:

```bash
docker compose up --build
```

## Notes

- The current course demo recommendation is a single control-plane instance plus multiple shard groups.
- The main availability story is in shard replica groups, not in a multi-controller HA control plane.
- The dashboard is intended for live demos and observability, not for production-grade administration.
