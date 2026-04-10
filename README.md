# DBD

A small distributed database prototype built with Go, BoltDB, HashiCorp Raft, and etcd.

## What It Does

- Replicates write requests through Raft
- Stores table data locally in BoltDB
- Uses etcd for service discovery and leader lookup
- Supports explicit `remove` / `rejoin` membership operations
- Exposes an HTTP API and a CLI
- Includes unit tests and e2e tests

## Supported SQL

- `CREATE TABLE`
- `INSERT INTO ... VALUES ...`
- `SELECT ... FROM ... [WHERE ...]`
- `DELETE FROM ... WHERE ...`
- `SHOW TABLES`

## Repository Layout

- `cmd/server`: server entrypoint
- `cmd/cli`: CLI entrypoint
- `internal/api`: HTTP handlers
- `internal/app`: app assembly and lifecycle
- `internal/config`: config parsing
- `internal/discovery`: etcd integration
- `internal/model`: shared data models
- `internal/raftnode`: Raft node and FSM
- `internal/service`: business orchestration
- `internal/sql`: SQL parser
- `internal/storage`: BoltDB storage
- `test/e2e`: end-to-end tests
- `scripts`: local demo scripts

## Local Run

### 1. Start etcd

```bash
docker run -d --name dbd-etcd -p 2379:2379 \
  quay.io/coreos/etcd:v3.5.9 \
  etcd --advertise-client-urls=http://127.0.0.1:2379 \
       --listen-client-urls=http://0.0.0.0:2379
```

### 2. Start node1

```bash
go run ./cmd/server \
  --node-id=node1 \
  --http-addr=127.0.0.1:20080 \
  --raft-addr=127.0.0.1:20000 \
  --raft-dir=/tmp/dbd-node1/raft \
  --db-path=/tmp/dbd-node1/data.db \
  --bootstrap=true \
  --etcd=127.0.0.1:2379
```

### 3. Start node2 and node3

```bash
go run ./cmd/server \
  --node-id=node2 \
  --http-addr=127.0.0.1:20081 \
  --raft-addr=127.0.0.1:20001 \
  --raft-dir=/tmp/dbd-node2/raft \
  --db-path=/tmp/dbd-node2/data.db \
  --etcd=127.0.0.1:2379
```

```bash
go run ./cmd/server \
  --node-id=node3 \
  --http-addr=127.0.0.1:20082 \
  --raft-addr=127.0.0.1:20002 \
  --raft-dir=/tmp/dbd-node3/raft \
  --db-path=/tmp/dbd-node3/data.db \
  --etcd=127.0.0.1:2379
```

### 4. Use the CLI

```bash
go run ./cmd/cli --etcd=127.0.0.1:2379 cluster members
go run ./cmd/cli --etcd=127.0.0.1:2379 cluster leader
go run ./cmd/cli --etcd=127.0.0.1:2379 sql "CREATE TABLE books (id INT PRIMARY KEY, name TEXT)"
go run ./cmd/cli --etcd=127.0.0.1:2379 sql "INSERT INTO books VALUES (1, 'raft')"
go run ./cmd/cli --etcd=127.0.0.1:2379 sql "SELECT * FROM books"
```

### Read From a Specific Follower

```bash
go run ./cmd/cli --node-url=http://127.0.0.1:20081 sql "SELECT * FROM books"
go run ./cmd/cli --node-url=http://127.0.0.1:20082 sql "SELECT * FROM books"
```

## Membership Management

### Remove a node

```bash
go run ./cmd/cli --etcd=127.0.0.1:2379 cluster remove node3
```

### Rejoin a removed node

Start the node with `--rejoin=true`:

```bash
go run ./cmd/server \
  --node-id=node3 \
  --http-addr=127.0.0.1:20082 \
  --raft-addr=127.0.0.1:20002 \
  --raft-dir=/tmp/dbd-node3/raft \
  --db-path=/tmp/dbd-node3/data.db \
  --etcd=127.0.0.1:2379 \
  --rejoin=true
```

Then explicitly recover the logical member:

```bash
go run ./cmd/cli --etcd=127.0.0.1:2379 cluster rejoin node3 127.0.0.1:20002 127.0.0.1:20082
```

## Docker

Build the image:

```bash
docker build -t dbd:latest .
```

Start etcd + 3 nodes:

```bash
docker compose up --build
```

Useful ports:

- node1 HTTP: `http://127.0.0.1:18080`
- node2 HTTP: `http://127.0.0.1:18081`
- node3 HTTP: `http://127.0.0.1:18082`
- etcd: `127.0.0.1:2379`

## Scripts

- `scripts/demo-local.sh`: print a quick local demo flow
- `scripts/demo-compose.sh`: start the compose cluster and run a short demo

## Tests

Run unit tests:

```bash
go test ./...
```

Run coverage:

```bash
go test ./... -coverprofile=coverage.out
go tool cover -func=coverage.out
```

Run only e2e tests:

```bash
go test ./test/e2e -v
```

## Push to GitHub

Create an empty GitHub repository first, then run:

```bash
git init
git add .
git commit -m "init: distributed database MVP"
git branch -M main
git remote add origin git@github.com:<your-name>/dbd.git
git push -u origin main
```

If your local repository is already initialized, start from `git add .`.
