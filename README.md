# DDB / ShardDB

一个基于 Go、BoltDB、HashiCorp Raft 和 etcd 的分布式数据库原型项目。

当前仓库包含两层紧密相关的能力：

- 单副本组的复制型 DDB
- 带 `controller`、`apiserver` 和多副本组分片能力的 ShardDB

## 项目包含什么

### 核心构件

- 每个副本组内部基于 Raft 做复制
- 基于 BoltDB 的本地状态存储
- 基于 etcd 的服务发现和控制平面元数据管理
- 显式 membership 操作，例如 `remove` 和 `rejoin`
- HTTP API、CLI、dashboard 和演示脚本

### ShardDB 特性

- 按角色启动：`controller`、`apiserver`、`shard`
- 通过 etcd 共享控制平面配置
- shard 放置与 shard group 元数据管理
- `move-shard` 和 `rebalance`
- 通过 shard 级别锁保证迁移安全
- 迁移期间通过 `503 + Retry-After` 暴露重试语义
- 浏览器 dashboard，可展示拓扑、分片映射、group 健康状态和表数据

## 环境准备

### 通用要求

在运行任何 demo 或课堂展示之前，先确保：

- 已安装 Go
- 仓库已拉到本地
- `ddb-server` 和 `ddb-cli` 已编译

编译所需二进制：

```bash
go build -o ./bin/ddb-server ./cmd/server
go build -o ./bin/ddb-cli ./cmd/cli
```

### macOS Requirements

当前 macOS 单机脚本默认假设：

- 已安装本地 `etcd`
- 本地 `etcd` 默认路径是 `/opt/homebrew/bin/etcd`
- 控制平面配置文件是 `configs/macos/control-plane.local.json`

建议先执行一遍快速检查：

```bash
go version
/opt/homebrew/bin/etcd --version
test -x ./bin/ddb-server && echo "ddb-server ok"
test -x ./bin/ddb-cli && echo "ddb-cli ok"
./scripts/ddb-mac-control.sh -Action validate
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action validate
```

### Windows Requirements

当前 Windows 单机脚本默认假设：

- 已安装并启动 Docker Desktop
- `etcd` 由控制平面脚本通过 Docker 启动
- 控制平面配置文件是 `configs/windows/control-plane.local.json`

建议先执行一遍快速检查：

```powershell
go version
docker version
docker info
Test-Path .\bin\ddb-server.exe
Test-Path .\bin\ddb-cli.exe
.\scripts\ddb-win-control.ps1 -Action validate
.\scripts\ddb-win.ps1 -Config .\configs\windows\three-machine\win-a.local.json -Action validate
```

## 单机运行文档

如果你是第一次验证环境，建议直接看下面两份 cheat sheet，而不是从脚本源码开始读。

- [macOS 单机 cheat sheet](./docs/mac-single-host-video-cheatsheet.md)
  - 使用 `scripts/ddb-mac-control.sh`
  - 使用 `scripts/ddb-mac.sh`
  - 默认读取 `configs/macos/control-plane.local.json`
  - 默认使用 `configs/macos/three-machine/mac-a.local.json`、`mac-b.local.json`、`mac-c.local.json`
- [Windows 单机 cheat sheet](./docs/windows-single-host-video-cheatsheet.md)
  - 使用 `scripts/ddb-win-control.ps1`
  - 使用 `scripts/ddb-win.ps1`
  - 默认读取 `configs/windows/control-plane.local.json`
  - 默认使用 `configs/windows/three-machine/win-a.local.json`、`win-b.local.json`、`win-c.local.json`

这两份文档已经包含：

- 预编译命令
- 环境检测命令
- 推荐启动顺序
- dashboard 和 CLI 演示命令
- 基于 inspect 的数据分布检查
- 收尾清理命令

## 多物理机运行说明

这个仓库也支持在多台物理机上运行。推荐的基础拓扑是：

- 一台机器负责控制平面
- 三台机器分别运行 shard 配置 `a`、`b`、`c`
- 每台机器只启动属于自己的那份配置

### 应该使用哪些配置

多机部署时，建议从 `three-machine` 目录下的样例配置出发：

- macOS control plane: `configs/macos/three-machine/control-plane.sample.json`
- macOS shard configs:
  - `configs/macos/three-machine/mac-a.sample.json`
  - `configs/macos/three-machine/mac-b.sample.json`
  - `configs/macos/three-machine/mac-c.sample.json`
- Windows control plane: `configs/windows/three-machine/control-plane.sample.json`
- Windows shard configs:
  - `configs/windows/three-machine/win-a.sample.json`
  - `configs/windows/three-machine/win-b.sample.json`
  - `configs/windows/three-machine/win-c.sample.json`

### 一般只需要改哪些字段

对于真实多机环境，最少通常只要修改：

- 每台机器自己的 `project_root`
- 每台机器自己的 `local_ip`
- shard 配置里的 `etcd_host` 继续指向控制平面机器
- 非 bootstrap 节点的 `default_join_host` 继续指向一个健康的 bootstrap 节点

如果你沿用样例里的默认拓扑，上面这些字段通常就是主要改动项。

### 网络要求

所有机器之间必须网络互通。至少要确认以下端口可达：

- `2379` for etcd
- `18100` for the `apiserver`
- shard HTTP ports such as `21080` to `21282`
- shard Raft ports such as `22080` to `22282`

### 启动顺序

强烈建议按下面顺序启动：

1. 先启动控制平面
2. 等待 `etcd` 和 `apiserver` 健康
3. 启动 bootstrap shard 所在机器
4. 再启动剩余 shard 机器

### 启动前校验

每台机器正式启动前，都建议先执行对应平台的 `validate`：

- macOS:

```bash
./scripts/ddb-mac-control.sh -Action validate
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.sample.json -Action validate
```

- Windows:

```powershell
.\scripts\ddb-win-control.ps1 -Action validate
.\scripts\ddb-win.ps1 -Config .\configs\windows\three-machine\win-a.sample.json -Action validate
```

## 当前支持的 SQL

当前 MVP 支持：

- `CREATE TABLE`
- `INSERT INTO ... VALUES (...)`
- `SELECT ... FROM ... [WHERE ...]`
- `DELETE FROM ... WHERE ...`
- `SHOW TABLES`
- 受限的等值 `JOIN`，已覆盖在 ShardDB 测试和 coordinator 路径中
- 当前 coordinator 路径下的 `ORDER BY` 和 `LIMIT`

说明：

- 当前 MVP 路径主要按主键路由
- shard 迁移期间优先保证迁移安全，而不是完全透明访问

## CLI 示例

查看控制平面：

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control config
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control groups
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control shards
```

读写业务数据：

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "INSERT INTO users VALUES (1, 'alice')"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 1"
```

迁移一个 shard：

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control move-shard 6 g3
```

在多个 group 之间做 rebalance：

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control rebalance g1 g2 g3
```

## Membership 管理

移除一个节点：

```bash
./bin/ddb-cli --etcd=127.0.0.1:2379 cluster remove node3
```

让一个已移除节点重新加入：

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

然后恢复逻辑成员：

```bash
go run ./cmd/cli --etcd=127.0.0.1:2379 cluster rejoin node3 127.0.0.1:20002 127.0.0.1:20082
```

## 仓库结构

- `cmd/server`：所有角色共用的 server 入口
- `cmd/cli`：CLI 入口
- `internal/apiserver`：ShardDB API server 和 dashboard
- `internal/controller`：控制平面逻辑与 shard 管理
- `internal/coordinator`：SQL 路由与控制平面执行
- `internal/discovery`：etcd 集成
- `internal/raftnode`：Raft 节点封装与 FSM
- `internal/router`：shard 路由
- `internal/sql`：SQL 解析
- `internal/storage`：BoltDB 存储
- `scripts`：本地和课堂演示脚本
- `configs`：本地配置和多机样例配置
- `docs`：演示文档和运行手册
- `test/e2e`：in-process 和 black-box 端到端测试

## 测试

运行全部测试：

```bash
go test ./...
```

只运行 e2e：

```bash
go test ./test/e2e -v
```

当前已覆盖的场景包括：

- 复制与 leader failover
- 多数派丢失时的拒绝写入
- remove/rejoin 与追赶
- 基于 discovery 的自动 join
- 控制平面配置共享
- apiserver 重启与配置重载
- shard 迁移与 rebalance 流程

## 说明

- 当前课程展示最推荐的形态，是一个控制平面实例加多个 shard group。
- 当前系统的主要可用性故事在 shard 副本组，而不是多控制器高可用控制平面。
- dashboard 的定位是演示和观测，不是生产级管理后台。
