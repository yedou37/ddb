# DDB Two-Host Demo Guide

This document describes how to run a 5-node DDB cluster across two physical machines:

- one macOS host
- one Windows host

Demo flow:

1. Start `etcd` on macOS
2. Start 2 nodes on macOS
3. Start 3 nodes on Windows
4. Kill 1 Windows node and 1 macOS node, then observe
5. Restart them, then observe
6. Remove 1 Windows node and 1 macOS node, then observe the 3-node cluster
7. Rejoin 1 Windows node and 1 macOS node, then observe
8. Stop the whole cluster

## Important Note

For this cross-host demo, the recommended setup is:

- run `etcd` in Docker on macOS
- run DDB nodes as native processes on each host

Reason:

- DDB currently uses one address for both "listen" and "advertise"
- on Docker Desktop for macOS and Windows, container port mapping across two physical hosts is awkward for this pattern
- the limitation is not "mac vs Windows", but the current network model of the app

If all nodes run on the same host, Docker is fine.
If nodes run across two physical hosts, native processes are the safest demo path.

## Topology

Replace the IPs below with your real LAN IPs.

- `MAC_IP=192.168.1.10`
- `WIN_IP=192.168.1.20`

Node layout:

- macOS:
  - `node1`: HTTP `192.168.1.10:20080`, Raft `192.168.1.10:21000`
  - `node2`: HTTP `192.168.1.10:20081`, Raft `192.168.1.10:21001`
- Windows:
  - `node3`: HTTP `192.168.1.20:20082`, Raft `192.168.1.20:21002`
  - `node4`: HTTP `192.168.1.20:20083`, Raft `192.168.1.20:21003`
  - `node5`: HTTP `192.168.1.20:20084`, Raft `192.168.1.20:21004`

## Prerequisites

- both machines are on the same LAN and can reach each other
- firewall allows these ports:
  - macOS: `2379`, `20080`, `20081`, `21000`, `21001`
  - Windows: `20082-20084`, `21002-21004`
- Go is installed on both machines
- Docker Desktop is installed on macOS for `etcd`

### Optional Network Preflight Check

This repo includes a cross-platform connectivity checker at `cmd/netcheck`.
Run it on each host before starting the demo to verify:

- which local IPv4 addresses are active
- whether the peer IP falls into one of the local subnets
- whether `ping` works
- whether required TCP ports are reachable

Example from macOS to Windows:

```bash
go run ./cmd/netcheck --peer=$WIN_IP --ports=20082,20083,20084,21002,21003,21004
```

Example from Windows to macOS:

```powershell
go run .\cmd\netcheck --peer=$env:MAC_IP --ports=2379,20080,20081,21000,21001
```

If `Summary: FAIL` is shown, fix networking first before continuing with the cluster demo.

## 0. Build Binaries

### macOS

```bash
cd /path/to/ddb
go build -o ./bin/ddb-server ./cmd/server
go build -o ./bin/ddb-cli ./cmd/cli
```

### Windows PowerShell

```powershell
cd C:\path\to\ddb
go build -o .\bin\ddb-server.exe .\cmd\server
go build -o .\bin\ddb-cli.exe .\cmd\cli
```

## 1. Set IP Variables

### macOS

```bash
export MAC_IP=192.168.1.10
export WIN_IP=192.168.1.20
```

### Windows PowerShell

```powershell
$env:MAC_IP="192.168.1.10"
$env:WIN_IP="192.168.1.20"
```

## 2. Start etcd on macOS

Run this on macOS:

```bash
docker rm -f ddb-etcd 2>/dev/null || true
docker run -d \
  --name ddb-etcd \
  -p 2379:2379 \
  quay.io/coreos/etcd:v3.5.9 \
  etcd \
  --advertise-client-urls=http://$MAC_IP:2379 \
  --listen-client-urls=http://0.0.0.0:2379
```

Quick check:

```bash
curl http://$MAC_IP:2379/health
```

## 3. Start 2 Nodes on macOS

Create local data dirs first:

```bash
mkdir -p /tmp/ddb-demo/node1 /tmp/ddb-demo/node2
```

### macOS Terminal 1: `node1`

```bash
./bin/ddb-server \
  --node-id=node1 \
  --http-addr=$MAC_IP:20080 \
  --raft-addr=$MAC_IP:21000 \
  --raft-dir=/tmp/ddb-demo/node1/raft \
  --db-path=/tmp/ddb-demo/node1/ddb.db \
  --etcd=$MAC_IP:2379 \
  --bootstrap=true
```

### macOS Terminal 2: `node2`

```bash
./bin/ddb-server \
  --node-id=node2 \
  --http-addr=$MAC_IP:20081 \
  --raft-addr=$MAC_IP:21001 \
  --raft-dir=/tmp/ddb-demo/node2/raft \
  --db-path=/tmp/ddb-demo/node2/ddb.db \
  --etcd=$MAC_IP:2379
```

## 4. Start 3 Nodes on Windows

Create local data dirs first:

```powershell
New-Item -ItemType Directory -Force C:\ddb-demo\node3 | Out-Null
New-Item -ItemType Directory -Force C:\ddb-demo\node4 | Out-Null
New-Item -ItemType Directory -Force C:\ddb-demo\node5 | Out-Null
```

### Windows PowerShell 1: `node3`

```powershell
.\bin\ddb-server.exe --node-id=node3 --http-addr=$env:WIN_IP:20082 --raft-addr=$env:WIN_IP:21002 --raft-dir=C:\ddb-demo\node3\raft --db-path=C:\ddb-demo\node3\ddb.db --etcd=$env:MAC_IP:2379
```

### Windows PowerShell 2: `node4`

```powershell
.\bin\ddb-server.exe --node-id=node4 --http-addr=$env:WIN_IP:20083 --raft-addr=$env:WIN_IP:21003 --raft-dir=C:\ddb-demo\node4\raft --db-path=C:\ddb-demo\node4\ddb.db --etcd=$env:MAC_IP:2379
```

### Windows PowerShell 3: `node5`

```powershell
.\bin\ddb-server.exe --node-id=node5 --http-addr=$env:WIN_IP:20084 --raft-addr=$env:WIN_IP:21004 --raft-dir=C:\ddb-demo\node5\raft --db-path=C:\ddb-demo\node5\ddb.db --etcd=$env:MAC_IP:2379
```

## 5. Base Observation Commands

Run the observation commands on macOS. This machine already has `ddb-cli`.

### Check leader and members

```bash
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster leader
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster members
```

### Create table and seed data

```bash
./bin/ddb-cli --etcd=$MAC_IP:2379 sql "CREATE TABLE demo_books (id INT PRIMARY KEY, name TEXT)"
./bin/ddb-cli --etcd=$MAC_IP:2379 sql "INSERT INTO demo_books VALUES (1, 'raft')"
./bin/ddb-cli --etcd=$MAC_IP:2379 sql "INSERT INTO demo_books VALUES (2, 'etcd')"
./bin/ddb-cli --etcd=$MAC_IP:2379 sql "INSERT INTO demo_books VALUES (3, 'replica')"
```

### Query from one leader-side node and two follower-side nodes

At this point `node1` should be the leader because it bootstrapped the cluster.

```bash
./bin/ddb-cli --node-url=http://$MAC_IP:20080 sql "SELECT * FROM demo_books"
./bin/ddb-cli --node-url=http://$WIN_IP:20082 sql "SELECT * FROM demo_books"
./bin/ddb-cli --node-url=http://$WIN_IP:20084 sql "SELECT * FROM demo_books"
```

Expected result:

- all three queries return the same rows
- `cluster members` shows 5 voters

## 6. Kill 1 Windows Node and 1 macOS Node, Then Observe

For a stable demo, kill follower nodes:

- stop `node2` on macOS
- stop `node4` on Windows

Use `Ctrl+C` in the `node2` and `node4` terminals.

### Observe cluster state

```bash
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster members
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster leader
```

### Perform write operations

Current SQL support includes:

- `CREATE TABLE`
- `INSERT`
- `SELECT`
- `DELETE`

`UPDATE` is not implemented in the current MVP, so for "modify" in the demo use `DELETE + INSERT`.

```bash
./bin/ddb-cli --etcd=$MAC_IP:2379 sql "INSERT INTO demo_books VALUES (4, 'after-kill')"
./bin/ddb-cli --etcd=$MAC_IP:2379 sql "DELETE FROM demo_books WHERE id = 2"
```

### Query remaining online nodes

```bash
./bin/ddb-cli --node-url=http://$MAC_IP:20080 sql "SELECT * FROM demo_books"
./bin/ddb-cli --node-url=http://$WIN_IP:20082 sql "SELECT * FROM demo_books"
./bin/ddb-cli --node-url=http://$WIN_IP:20084 sql "SELECT * FROM demo_books"
```

Expected result:

- writes still succeed
- the three online nodes return the same result
- `node2` and `node4` remain in Raft membership but are offline

## 7. Restart the Stopped Nodes, Then Observe

Restart the same two nodes with the original commands.

### macOS Terminal 2: restart `node2`

```bash
./bin/ddb-server \
  --node-id=node2 \
  --http-addr=$MAC_IP:20081 \
  --raft-addr=$MAC_IP:21001 \
  --raft-dir=/tmp/ddb-demo/node2/raft \
  --db-path=/tmp/ddb-demo/node2/ddb.db \
  --etcd=$MAC_IP:2379
```

### Windows PowerShell 2: restart `node4`

```powershell
.\bin\ddb-server.exe --node-id=node4 --http-addr=$env:WIN_IP:20083 --raft-addr=$env:WIN_IP:21003 --raft-dir=C:\ddb-demo\node4\raft --db-path=C:\ddb-demo\node4\ddb.db --etcd=$env:MAC_IP:2379
```

### Observe catch-up

```bash
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster members
./bin/ddb-cli --node-url=http://$MAC_IP:20081 sql "SELECT * FROM demo_books"
./bin/ddb-cli --node-url=http://$WIN_IP:20083 sql "SELECT * FROM demo_books"
```

Expected result:

- `node2` and `node4` catch up to the same rows as the rest of the cluster
- the cluster returns to 5 online nodes

## 8. Remove 1 Windows Node and 1 macOS Node, Then Observe the 3-Node Cluster

Now explicitly remove:

- `node2`
- `node4`

Run on macOS:

```bash
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster remove node2
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster remove node4
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster members
```

Expected result:

- the active cluster becomes `node1 + node3 + node5`
- removed nodes are no longer voters
- removed node processes should stop themselves after the removed state is observed

### Verify data on the remaining 3-node cluster

```bash
./bin/ddb-cli --etcd=$MAC_IP:2379 sql "INSERT INTO demo_books VALUES (5, 'after-remove')"
./bin/ddb-cli --etcd=$MAC_IP:2379 sql "DELETE FROM demo_books WHERE id = 1"
./bin/ddb-cli --node-url=http://$MAC_IP:20080 sql "SELECT * FROM demo_books"
./bin/ddb-cli --node-url=http://$WIN_IP:20082 sql "SELECT * FROM demo_books"
./bin/ddb-cli --node-url=http://$WIN_IP:20084 sql "SELECT * FROM demo_books"
```

Expected result:

- the 3 remaining nodes still return the same rows
- the cluster continues to accept writes

## 9. Rejoin 1 Windows Node and 1 macOS Node, Then Observe

### Step 9.1 Restart removed nodes in rejoin mode

#### macOS Terminal 2: rejoin `node2`

```bash
./bin/ddb-server \
  --node-id=node2 \
  --http-addr=$MAC_IP:20081 \
  --raft-addr=$MAC_IP:21001 \
  --raft-dir=/tmp/ddb-demo/node2/raft \
  --db-path=/tmp/ddb-demo/node2/ddb.db \
  --etcd=$MAC_IP:2379 \
  --rejoin=true
```

#### Windows PowerShell 2: rejoin `node4`

```powershell
.\bin\ddb-server.exe --node-id=node4 --http-addr=$env:WIN_IP:20083 --raft-addr=$env:WIN_IP:21003 --raft-dir=C:\ddb-demo\node4\raft --db-path=C:\ddb-demo\node4\ddb.db --etcd=$env:MAC_IP:2379 --rejoin=true
```

### Step 9.2 Issue explicit rejoin commands from macOS

```bash
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster rejoin node2 $MAC_IP:21001 $MAC_IP:20081
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster rejoin node4 $WIN_IP:21003 $WIN_IP:20083
./bin/ddb-cli --etcd=$MAC_IP:2379 cluster members
```

### Step 9.3 Verify the cluster after rejoin

```bash
./bin/ddb-cli --etcd=$MAC_IP:2379 sql "INSERT INTO demo_books VALUES (6, 'after-rejoin')"
./bin/ddb-cli --node-url=http://$MAC_IP:20081 sql "SELECT * FROM demo_books"
./bin/ddb-cli --node-url=http://$WIN_IP:20083 sql "SELECT * FROM demo_books"
```

Expected result:

- `node2` and `node4` return to the cluster
- they catch up to the latest dataset
- the cluster returns to 5 voters

## 10. Stop the Whole Cluster

### Stop nodes

Use `Ctrl+C` in these five terminals:

- macOS Terminal 1: `node1`
- macOS Terminal 2: `node2`
- Windows PowerShell 1: `node3`
- Windows PowerShell 2: `node4`
- Windows PowerShell 3: `node5`

### Stop etcd on macOS

```bash
docker stop ddb-etcd
docker rm ddb-etcd
```

## 11. Optional Cleanup

### macOS

```bash
rm -rf /tmp/ddb-demo
```

### Windows PowerShell

```powershell
Remove-Item -Recurse -Force C:\ddb-demo
```

## 12. What to Observe During the Demo

At each stage, focus on these checks:

- `cluster leader`: who is the current leader
- `cluster members`: which nodes are online, offline, removed, or rejoined
- `INSERT` and `DELETE`: whether writes still succeed
- `SELECT` on different nodes: whether leader-side and follower-side reads return the same rows

Recommended observation checklist:

1. after all 5 nodes start, all queries match
2. after `node2` and `node4` are killed, writes still succeed on the remaining 3 nodes
3. after restart, `node2` and `node4` catch up
4. after remove, the cluster shrinks to 3 members and still accepts writes
5. after rejoin, the cluster grows back and the data remains consistent
