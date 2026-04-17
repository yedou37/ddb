# Three-Machine ShardDB Demo Guide

This document describes the recommended three-machine demo topology for the current ShardDB MVP.

## Topology

Use three physical machines on the same LAN:

- `MACHINE_A=192.168.1.10`
- `MACHINE_B=192.168.1.11`
- `MACHINE_C=192.168.1.12`

Run a single-node `etcd` on `MACHINE_A`:

- `ETCD=192.168.1.10:2379`

Recommended control plane for the current demo:

- `etcd` on `MACHINE_A`
- `controller-1` on `MACHINE_A`, HTTP `:18080`
- `apiserver` on `MACHINE_A`, HTTP `:18100`

For the current MVP, use a **single control plane instance**. Shard group high availability is the main availability story in the demo.

Recommended shard groups:

- `group1`: `g1-n1`, `g1-n2`, `g1-n3`
- `group2`: `g2-n1`, `g2-n2`, `g2-n3`
- `group3`: `g3-n1`, `g3-n2`, `g3-n3`

Each group should place one replica on each machine.

## Port Plan

Suggested port layout:

```text
Machine A
  etcd          2379
  controller-1  18080
  apiserver     18100
  g1-n1         http 21080 / raft 22080
  g2-n1         http 21081 / raft 22081
  g3-n1         http 21082 / raft 22082

Machine B
  g1-n2         http 21180 / raft 22180
  g2-n2         http 21181 / raft 22181
  g3-n2         http 21182 / raft 22182

Machine C
  g1-n3         http 21280 / raft 22280
  g2-n3         http 21281 / raft 22281
  g3-n3         http 21282 / raft 22282
```

## Build Once

On each machine:

```bash
go build -o ./bin/ddb-server ./cmd/server
go build -o ./bin/ddb-cli ./cmd/cli
```

## Start etcd

On `MACHINE_A`:

```bash
docker rm -f ddb-etcd 2>/dev/null || true
docker run -d \
  --name ddb-etcd \
  -p 2379:2379 \
  quay.io/coreos/etcd:v3.5.9 \
  etcd \
  --advertise-client-urls=http://192.168.1.10:2379 \
  --listen-client-urls=http://0.0.0.0:2379
```

## Start Controller and API Server

The current MVP stores controller config in:

- etcd, when `--etcd` is set
- a local JSON snapshot, using `--db-path` as the base path

Start a single controller and a single apiserver on `MACHINE_A`.

Machine A:

```bash
./scripts/run-sharddb-node.sh controller ctrl-1 192.168.1.10:18080 127.0.0.1:30080 /tmp/ctrl-1/raft /tmp/ctrl-1/controller.db control false "" 192.168.1.10:2379
./scripts/run-sharddb-node.sh apiserver api-1 192.168.1.10:18100 127.0.0.1:30100 /tmp/api-1/raft /tmp/api-1/controller.db control false "" 192.168.1.10:2379
```

## Start Shard Groups

Start all three shard groups. `group3` starts with zero assigned shards and is used as the expansion target in the demo.

Machine A:

```bash
./scripts/run-sharddb-node.sh shard g1-n1 192.168.1.10:21080 192.168.1.10:22080 /tmp/g1-n1/raft /tmp/g1-n1/data.db g1 true "" 192.168.1.10:2379
./scripts/run-sharddb-node.sh shard g2-n1 192.168.1.10:21081 192.168.1.10:22081 /tmp/g2-n1/raft /tmp/g2-n1/data.db g2 true "" 192.168.1.10:2379
./scripts/run-sharddb-node.sh shard g3-n1 192.168.1.10:21082 192.168.1.10:22082 /tmp/g3-n1/raft /tmp/g3-n1/data.db g3 true "" 192.168.1.10:2379
```

Machine B:

```bash
./scripts/run-sharddb-node.sh shard g1-n2 192.168.1.11:21180 192.168.1.11:22180 /tmp/g1-n2/raft /tmp/g1-n2/data.db g1 false http://192.168.1.10:21080 192.168.1.10:2379
./scripts/run-sharddb-node.sh shard g2-n2 192.168.1.11:21181 192.168.1.11:22181 /tmp/g2-n2/raft /tmp/g2-n2/data.db g2 false http://192.168.1.10:21081 192.168.1.10:2379
./scripts/run-sharddb-node.sh shard g3-n2 192.168.1.11:21182 192.168.1.11:22182 /tmp/g3-n2/raft /tmp/g3-n2/data.db g3 false http://192.168.1.10:21082 192.168.1.10:2379
```

Machine C:

```bash
./scripts/run-sharddb-node.sh shard g1-n3 192.168.1.12:21280 192.168.1.12:22280 /tmp/g1-n3/raft /tmp/g1-n3/data.db g1 false http://192.168.1.10:21080 192.168.1.10:2379
./scripts/run-sharddb-node.sh shard g2-n3 192.168.1.12:21281 192.168.1.12:22281 /tmp/g2-n3/raft /tmp/g2-n3/data.db g2 false http://192.168.1.10:21081 192.168.1.10:2379
./scripts/run-sharddb-node.sh shard g3-n3 192.168.1.12:21282 192.168.1.12:22282 /tmp/g3-n3/raft /tmp/g3-n3/data.db g3 false http://192.168.1.10:21082 192.168.1.10:2379
```

## Demo Flow

### 1. Verify control plane

```bash
./bin/ddb-cli --node-url=http://192.168.1.10:18100 control config
./bin/ddb-cli --node-url=http://192.168.1.10:18100 control groups
./bin/ddb-cli --node-url=http://192.168.1.10:18100 control shards
```

### 2. Create table and insert rows

```bash
./bin/ddb-cli --node-url=http://192.168.1.10:18100 sql "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)"
./bin/ddb-cli --node-url=http://192.168.1.10:18100 sql "INSERT INTO users VALUES (1, 'alice')"
./bin/ddb-cli --node-url=http://192.168.1.10:18100 sql "INSERT INTO users VALUES (2, 'bob')"
```

### 3. Show initial shard placement

```bash
./bin/ddb-cli --node-url=http://192.168.1.10:18100 control groups
./bin/ddb-cli --node-url=http://192.168.1.10:18100 control shards
```

### 4. Move one shard to group3

Pick a shard ID from `control shards`, then:

```bash
./bin/ddb-cli --node-url=http://192.168.1.10:18100 control move-shard 6 g3
```

Observe:

- `control shards` shows the shard assigned to `g3`
- `control groups` shows `g3` now carries shards
- follow-up reads by primary key still succeed through the apiserver

### 5. Rebalance across three groups

```bash
./bin/ddb-cli --node-url=http://192.168.1.10:18100 control rebalance g1 g2 g3
```

Observe:

- shard counts become more even
- later writes route according to the new mapping

### 6. Fault tolerance demo

Pick one machine and stop all shard processes on it.

Expected:

- the remaining two replicas in each shard group still form a majority
- reads and writes through the apiserver continue to succeed

Important note:

- this applies to shard groups today
- the control plane is intentionally single-instance in the current demo
- if `MACHINE_A` fails, control commands and the unified SQL entrypoint stop, but shard group replication semantics remain the main HA story

## Current MVP Boundaries

What the current implementation supports:

- shared controller config through etcd plus local snapshot persistence
- single controller/apiserver demo topology
- single-shard SQL routing
- `move-shard` and `rebalance`
- minimal row migration semantics for moved shards

What is still simplified:

- controller is not yet a full Raft-backed metadata group
- migration is row-level and synchronous, not full online streaming migration
- cross-shard Join is not implemented
