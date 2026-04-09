# Go 分布式数据库设计文档

## 1. 项目概述

**项目名称**: Go Distributed MiniSQL  
**项目性质**: 课程演示项目 - 分布式 SQL 数据库系统  
**技术栈**: Go + BoltDB + Raft + etcd + sqlparser + Docker

---

## 2. 技术选型分析

### 2.1 核心技术栈选择

| 技术 | 用途 | 选型理由 |
|-----|------|---------|
| **Go** | 开发语言 | 并发编程简单、部署方便、生态成熟 |
| **BoltDB** | 本地存储 | 纯 Go 实现、嵌入式 KV 存储、ACID 事务、零依赖 |
| **Raft** | 一致性协议 | 易于理解和实现、保证强一致性、成熟 Go 库 |
| **etcd** | 服务发现 | 轻量级、Go 原生、临时节点机制完美适配服务发现 |
| **sqlparser** | SQL 解析 | 轻量级 Go 库，支持基本 SQL 语法解析 |
| **Docker** | 部署 | 跨平台、易于在多物理机部署、便于演示 |

### 2.2 方案合理性分析

✅ **方案合理，原因如下：

1. **架构清晰**：etcd + Raft，职责分离明确
2. **强一致性保证**：Raft 协议天然保证线性一致性
3. **自动服务发现**：etcd 临时节点机制，节点自动注册和感知
4. **SQL 支持**：在 KV 基础上实现简单 SQL，更像真实数据库
5. **部署简单**：Docker 镜像，一行命令启动
6. **适合课程演示**：架构简洁，概念清晰，便于讲解

---

## 3. 系统架构

### 3.1 整体架构（课程演示版）

```
                    ┌─────────────────────────────────────────┐
                    │           etcd (服务发现)              │
                    │         192.168.1.10:2379          │
                    │  /raft-cluster/nodes/                 │
                    │    node1, node2, node3...             │
                    └─────────────────────────────────────────┘
                              │
         ┌────────────────────┼────────────────────┐
         │                    │                    │
         ▼                    ▼                    ▼
  ┌──────────────┐   ┌──────────────┐   ┌──────────────┐
  │   Node 1     │   │   Node 2     │   │   Node 3     │
  │ 192.168.1.11│   │ 192.168.1.12│   │ 192.168.1.13│
  ├──────────────┤   ├──────────────┤   ├──────────────┤
  │ SQL Parser   │   │ SQL Parser   │   │ SQL Parser   │
  │ Raft FSM     │   │ Raft FSM     │   │ Raft FSM     │
  │   BoltDB     │   │   BoltDB     │   │   BoltDB     │
  └──────────────┘   └──────────────┘   └──────────────┘
         │                    │                    │
         └────────────────────┴────────────────────┘
                              │
                              ▼
                    ┌───────────────────┐
                    │   CLI 客户端     │
                    │ (任意电脑运行)    │
                    └───────────────────┘
```

### 3.2 核心组件

#### 3.2.1 etcd 服务发现
- 中心化的服务注册中心
- 所有节点启动时自动注册
- 节点通过 etcd 发现彼此
- 临时节点机制实现健康检查

#### 3.2.2 Raft 节点
每个节点包含：
- **SQL Parser**：解析 SQL 语句
- **Raft 状态机**：处理选举、日志复制
- **FSM (有限状态机)**：应用 Raft 日志到本地存储
- **HTTP API**：提供客户端接口
- **etcd 客户端**：服务注册和发现

#### 3.2.3 存储层 (BoltDB)
- **表存储映射**：
  - 每个表对应一个 BoltDB Bucket
  - 表元数据（schema）存储在 `_schemas` Bucket
  - 每行数据：key = 主键值, value = JSON 序列化的行数据
- 支持事务
- 持久化到磁盘

#### 3.2.4 CLI 客户端
- 从 etcd 自动发现所有节点
- 查询任意节点的状态
- 执行 SQL 语句
- 读请求：可从任意节点读
- 写请求：自动重定向到 Leader

### 3.3 网络通信

| 通信类型 | 协议 | 端口 | 用途 |
|---------|------|------|------|
| 节点 ↔ etcd | gRPC | 2379 | 服务注册、发现、健康检查 |
| 节点 ↔ 节点 | Raft RPC | 9090 | 选举、日志复制 |
| CLI ↔ 节点 | HTTP | 8080 | SQL 执行、状态查询 |

---

## 4. SQL 支持设计

### 4.1 支持的 SQL 语法

| 语句类型 | 示例 | 说明 |
|---------|------|------|
| **CREATE TABLE** | `CREATE TABLE users (id INT PRIMARY KEY, name TEXT, age INT);` | 创建表 |
| **INSERT** | `INSERT INTO users VALUES (1, 'Alice', 25);` | 插入数据 |
| **SELECT** | `SELECT * FROM users;` 或 `SELECT name FROM users WHERE id = 1;` | 查询数据 |
| **DELETE** | `DELETE FROM users WHERE id = 1;` | 删除数据 |
| **DROP TABLE** | `DROP TABLE users;` | 删除表 |

### 4.2 SQL 解析与执行流程

```
SQL 语句
    │
    ▼
SQL Parser (sqlparser 库)
    │
    ├─→ CREATE TABLE → 解析表结构 → 写入 _schemas Bucket
    │
    ├─→ INSERT → 解析数据 → 序列化为 JSON → 写入表 Bucket
    │
    ├─→ SELECT → 解析条件 → 扫描表 Bucket → 过滤/投影 → 返回结果
    │
    ├─→ DELETE → 解析条件 → 扫描表 Bucket → 删除匹配行
    │
    └─→ DROP TABLE → 删除表 Bucket → 删除 _schemas 中的表定义
```

### 4.3 存储结构示例

#### BoltDB Bucket 结构
```
┌─────────────────────────────────────────┐
│  _schemas (系统 Bucket)                  │
│  "users" → {                            │
│    "columns": [{"name": "id", ...}, ...]│
│  }                                        │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│  users (表 Bucket)                       │
│  "1" → {"id": 1, "name": "Alice", ...}  │
│  "2" → {"id": 2, "name": "Bob", ...}    │
└─────────────────────────────────────────┘
```

---

## 5. 功能设计

### 5.1 核心功能

#### 5.1.1 SQL 操作
| 功能 | 说明 | 执行节点 |
|-----|------|---------|
| **CREATE TABLE** | 创建表 | Leader |
| **INSERT** | 插入数据 | Leader |
| **SELECT** | 查询数据 | 任意节点 |
| **DELETE** | 删除数据 | Leader |
| **DROP TABLE** | 删除表 | Leader |

#### 5.1.2 集群管理
| 功能 | 说明 |
|-----|------|
| **自动注册** | 节点启动自动注册到 etcd |
| **自动发现** | 节点从 etcd 获取集群成员 |
| **健康检查** | etcd 临时节点自动检测节点离线 |
| **Leader 查询** | 查询当前 Leader 节点 |
| **状态查询** | 查询任意节点状态、表列表 |

### 5.2 etcd 服务发现机制

#### 5.2.1 节点注册流程
```
1. 节点启动
2. 连接 etcd
3. 创建会话（带 keepalive）
4. 在 /raft-cluster/nodes/{node-id} 创建临时节点
5. 节点信息写入：{"raft_addr": "...", "http_addr": "...", "role": "follower/leader"}
```

#### 5.2.2 节点发现流程
```
1. 节点启动
2. 连接 etcd
3. 列出 /raft-cluster/nodes/ 目录
4. 获取所有已注册节点
5. 监听该目录的变化事件
6. 动态更新本地节点列表
```

#### 5.2.3 节点离线检测
```
节点挂掉
    │
    │ 与 etcd 会话超时
    │
    ▼
etcd 自动删除临时节点
    │
    │
    ▼
其他节点收到删除事件
    │
    │
    ▼
更新本地节点列表
```

### 5.3 数据流程

#### 5.3.1 写操作流程 (CREATE/INSERT/DELETE/DROP)
```
1. CLI 从 etcd 获取节点列表
2. CLI 查询当前 Leader
3. CLI 向 Leader 发送 SQL
4. Leader 解析 SQL，生成操作
5. Leader 将操作封装为 Raft 日志条目
6. Leader 将日志复制到大多数节点
7. 一旦大多数节点确认，日志被提交
8. FSM 应用日志到 BoltDB（执行实际的表/数据操作）
9. 返回结果给 CLI
```

#### 5.3.2 读操作流程 (SELECT)
```
1. CLI 从 etcd 获取节点列表
2. 用户选择任意节点（或自动选择）
3. CLI 向该节点发送 SQL
4. 节点使用 ReadIndex 保证线性一致性
5. 节点解析 SQL，从 BoltDB 读取数据
6. 返回结果给 CLI
```

---

## 6. 仓库架构设计

### 6.1 设计原则

- **入口清晰**：服务端和 CLI 分离，便于演示和部署
- **模块分层**：协议层、执行层、存储层、共识层分开，避免耦合
- **便于替换**：以后如果把 BoltDB 换成 BadgerDB、把 HTTP 换成 gRPC，改动范围可控
- **便于课程展示**：目录名直接对应系统组件，老师一眼能看懂

### 6.2 推荐仓库目录

```text
go-distributed-minisql/
├── cmd/
│   ├── server/
│   │   └── main.go
│   └── cli/
│       └── main.go
├── internal/
│   ├── config/
│   │   └── config.go
│   ├── model/
│   │   ├── cluster.go
│   │   ├── schema.go
│   │   └── sql.go
│   ├── discovery/
│   │   └── etcd.go
│   ├── raftnode/
│   │   ├── node.go
│   │   ├── fsm.go
│   │   ├── transport.go
│   │   └── membership.go
│   ├── sql/
│   │   ├── parser.go
│   │   ├── planner.go
│   │   └── executor.go
│   ├── storage/
│   │   ├── boltdb.go
│   │   ├── catalog.go
│   │   ├── table.go
│   │   └── codec.go
│   ├── service/
│   │   ├── query_service.go
│   │   ├── cluster_service.go
│   │   └── leader_service.go
│   ├── api/
│   │   ├── handler.go
│   │   ├── middleware.go
│   │   └── response.go
│   └── app/
│       └── app.go
├── deployments/
│   ├── docker/
│   │   └── Dockerfile
│   └── compose/
│       └── docker-compose.yml
├── scripts/
│   ├── demo.sh
│   ├── start-node.sh
│   └── start-etcd.sh
├── test/
│   ├── integration/
│   └── e2e/
├── .gitignore
├── go.mod
├── go.sum
└── README.md
```

### 6.3 各目录职责

| 目录 | 职责 | 是否对外暴露 |
|-----|------|-------------|
| `cmd/server` | 节点进程入口，解析参数并启动服务 | 是 |
| `cmd/cli` | 演示和运维用客户端入口 | 是 |
| `internal/config` | 统一读取环境变量和启动参数 | 否 |
| `internal/model` | 公共数据结构，如表结构、节点信息、SQL 请求体 | 否 |
| `internal/discovery` | etcd 注册、续租、watch、节点发现 | 否 |
| `internal/raftnode` | Raft 节点生命周期、FSM、成员变更 | 否 |
| `internal/sql` | SQL 解析、简单执行计划、语句分发 | 否 |
| `internal/storage` | BoltDB 读写、schema 管理、行编码 | 否 |
| `internal/service` | 业务编排层，连接 API、Raft、SQL、存储 | 否 |
| `internal/api` | HTTP 路由、请求解析、响应输出 | 否 |
| `internal/app` | 应用装配层，初始化所有组件 | 否 |
| `deployments` | Docker 与本地/课程演示部署文件 | 是 |
| `scripts` | 一键启动和演示脚本 | 是 |
| `test` | 集成测试和端到端测试 | 是 |

### 6.4 为什么这样拆

#### 6.4.1 不把所有逻辑堆在 `server`

如果把 SQL、Raft、BoltDB、etcd 全都写进一个 `server.go`，短期能跑，但后面会出现：

- API 处理和数据库逻辑耦合
- Leader 转发和 SQL 执行耦合
- 后续加 `SHOW TABLES`、`JOIN NODE`、`status` 很难维护

所以这里建议把 `server` 只当作入口，把真正逻辑下沉到 `service`、`raftnode`、`storage`、`sql`。

#### 6.4.2 为什么要有 `service` 层

这个项目里最容易乱的地方不是底层存储，而是“一个请求到底要不要走 Raft”。

例如：

- `INSERT` 必须走 Leader，再进入 Raft 日志复制
- `SELECT` 可以在任意节点执行，但要先做一致性读
- `cluster status` 不走 SQL 执行器，而是读节点状态

这些逻辑放在 `service` 层最合适。

#### 6.4.3 为什么把 `sql` 和 `storage` 分开

这两层职责不同：

- `sql` 负责理解用户语句
- `storage` 负责把结构化结果落到 BoltDB

比如一条：

```sql
INSERT INTO students VALUES (1, 'Alice', 95);
```

执行链应该是：

```text
api -> service -> sql parser/planner -> raft log -> fsm -> storage
```

而不是直接在 HTTP handler 里操作 BoltDB。

### 6.5 建议保留的最小文件集合

如果你想先做一个能演示的第一版，不需要一开始就把所有目录都写满，最小可实现版本建议先落这几个文件：

```text
cmd/server/main.go
cmd/cli/main.go
internal/config/config.go
internal/discovery/etcd.go
internal/raftnode/node.go
internal/raftnode/fsm.go
internal/sql/parser.go
internal/sql/executor.go
internal/storage/boltdb.go
internal/storage/catalog.go
internal/service/query_service.go
internal/api/handler.go
internal/app/app.go
```

这样既不会过度设计，也能保证后续自然扩展。

---

## 7. HTTP API 设计

### 7.1 SQL 执行 API

| 方法 | 路径 | 说明 |
|-----|------|------|
| POST | /sql | 执行 SQL 语句 |

**请求示例**:
```json
POST /sql
{
  "sql": "INSERT INTO users VALUES (1, 'Alice', 25)"
}
```

**响应示例**:
```json
{
  "success": true,
  "result": {
    "type": "insert",
    "rows_affected": 1
  }
}
```

```json
{
  "success": true,
  "result": {
    "type": "select",
    "columns": ["id", "name", "age"],
    "rows": [
      [1, "Alice", 25],
      [2, "Bob", 30]
    ]
  }
}
```

### 7.2 集群状态 API

| 方法 | 路径 | 说明 |
|-----|------|------|
| GET | /status | 获取节点状态 |
| GET | /leader | 获取当前 Leader |
| GET | /members | 获取集群成员列表 |
| GET | /tables | 获取表列表 |

**响应示例**:
```json
// GET /status
{
  "node_id": "node1",
  "raft_addr": "192.168.1.11:9090",
  "http_addr": "192.168.1.11:8080",
  "role": "leader",
  "leader": "node1",
  "tables": ["users", "products"]
}
```

---

## 8. CLI 客户端设计

### 8.1 命令设计

```bash
# 全局选项
--etcd-addr string    etcd 地址 (默认 "127.0.0.1:2379")

# SQL 操作
sql "<statement>"     执行 SQL 语句（写操作自动找 Leader）
sql "<statement>" --node <node-id>  在指定节点执行（用于读操作）

# 集群操作
cluster status [node-id]  查看节点状态（默认所有节点）
cluster leader           查看当前 Leader
cluster members          查看集群成员
cluster tables           查看所有表
```

### 8.2 使用示例

```bash
# 执行写操作（自动找 Leader）
cli sql "CREATE TABLE users (id INT PRIMARY KEY, name TEXT, age INT)"
cli sql "INSERT INTO users VALUES (1, 'Alice', 25)"

# 执行读操作（可指定节点）
cli sql "SELECT * FROM users" --node node2
cli sql "SELECT name FROM users WHERE id = 1" --node node3

# 查看集群状态
cli cluster status
cli cluster leader
cli cluster tables
```

### 8.3 CLI 工作流程

```
1. 连接 etcd
2. 发现所有节点
3. 如果是写操作：
   - 查询当前 Leader
   - 向 Leader 发送 SQL
4. 如果是读操作：
   - 向指定节点（或任意节点）发送 SQL
5. 执行操作
6. 输出结果
```

---

## 9. Docker 部署设计

### 9.1 Dockerfile

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server
RUN CGO_ENABLED=0 go build -o cli ./cmd/cli

FROM alpine:latest
COPY --from=builder /app/server /server
COPY --from=builder /app/cli /cli
EXPOSE 8080 9090
ENTRYPOINT ["/server"]
```

### 9.2 环境变量

| 环境变量 | 说明 | 示例 |
|---------|------|------|
| `NODE_ID` | 节点唯一标识 | `node1` |
| `RAFT_ADDR` | Raft 通信地址 | `192.168.1.11:9090` |
| `HTTP_ADDR` | HTTP API 地址 | `192.168.1.11:8080` |
| `ETCD_ADDR` | etcd 地址 | `192.168.1.10:2379` |
| `DATA_DIR` | 数据存储目录 | `/data` |

### 9.3 多物理机部署（课程演示方案）

#### 步骤 1：启动 etcd（固定在一台机器上）
```bash
# 机器 A (192.168.1.10)
docker run -d \
  -p 2379:2379 \
  --name etcd \
  quay.io/coreos/etcd:v3.5.9 \
  etcd --listen-client-urls http://0.0.0.0:2379 \
  --advertise-client-urls http://192.168.1.10:2379
```

#### 步骤 2：三个同学分别启动各自的节点
```bash
# 同学 1 (192.168.1.11)
docker run -d \
  -e NODE_ID=node1 \
  -e RAFT_ADDR=192.168.1.11:9090 \
  -e HTTP_ADDR=192.168.1.11:8080 \
  -e ETCD_ADDR=192.168.1.10:2379 \
  -p 8080:8080 \
  -p 9090:9090 \
  go-distributed-minisql:latest

# 同学 2 (192.168.1.12)
docker run -d \
  -e NODE_ID=node2 \
  -e RAFT_ADDR=192.168.1.12:9090 \
  -e HTTP_ADDR=192.168.1.12:8080 \
  -e ETCD_ADDR=192.168.1.10:2379 \
  -p 8080:8080 \
  -p 9090:9090 \
  go-distributed-minisql:latest

# 同学 3 (192.168.1.13)
docker run -d \
  -e NODE_ID=node3 \
  -e RAFT_ADDR=192.168.1.13:9090 \
  -e HTTP_ADDR=192.168.1.13:8080 \
  -e ETCD_ADDR=192.168.1.10:2379 \
  -p 8080:8080 \
  -p 9090:9090 \
  go-distributed-minisql:latest
```

#### 步骤 3：任意电脑运行 CLI
```bash
docker run --rm -it \
  go-distributed-minisql:latest \
  /cli --etcd-addr=192.168.1.10:2379 cluster status
```

---

## 10. 课程演示流程设计

### 10.1 演示前准备
1. 三台物理机，确保网络互通
2. 一台机器预先启动 etcd
3. 所有机器拉取 Docker 镜像

### 10.2 演示步骤

#### 步骤 1：启动集群（三个同学同时操作）
```bash
# 各自运行一行命令启动节点
```

#### 步骤 2：查看集群状态
```bash
# 任意电脑运行 CLI
cli cluster status
```

#### 步骤 3：创建表并插入数据（演示强一致性）
```bash
# 创建表
cli sql "CREATE TABLE students (id INT PRIMARY KEY, name TEXT, score INT)"

# 插入数据
cli sql "INSERT INTO students VALUES (1, 'Alice', 95)"
cli sql "INSERT INTO students VALUES (2, 'Bob', 88)"
cli sql "INSERT INTO students VALUES (3, 'Charlie', 92)"
```

#### 步骤 4：从不同节点查询（验证一致性）
```bash
# 从 node2 查询
cli sql "SELECT * FROM students" --node node2

# 从 node3 查询
cli sql "SELECT * FROM students" --node node3

# 两者结果应该完全一致！
```

#### 步骤 5：模拟节点故障
```bash
# 停止一个节点
docker stop <container-id>

# 查看集群状态，验证节点被自动感知
cli cluster status

# 验证数据仍然可以读写
cli sql "SELECT * FROM students"
```

#### 步骤 6：恢复节点
```bash
# 重新启动节点
docker start <container-id>

# 验证节点自动重新加入
```

---

## 11. 关键技术点

### 11.1 Raft 实现
使用 hashicorp/raft 库（成熟的 Go Raft 实现）

### 11.2 线性一致性读
对于读取操作，使用 Raft 的 ReadIndex 机制保证线性一致性

### 11.3 etcd 服务发现
- 临时节点 + 会话 + keepalive
- 监听节点变化事件
- 自动节点发现和离线检测

### 11.4 SQL 引擎
- 使用 sqlparser 解析 SQL
- BoltDB 表映射：Bucket = 表，Key = 主键
- JSON 序列化行数据

### 11.5 Leader 自动发现
- CLI 从 etcd 获取节点列表
- 查询任意节点的 /leader 接口
- 写请求自动重定向

---

## 12. 与参考项目对比

| 特性 | 参考项目 | 本项目 |
|-----|---------|-------|
| 架构 | Master-Slave | Raft 对等节点 |
| 一致性 | 无明确保证 | Raft 强一致性 |
| 存储 | MySQL | BoltDB |
| 查询接口 | SQL | SQL |
| 服务发现 | ZooKeeper | etcd |
| 自动发现 | 需要手动配置 | 自动注册/发现 |
| 部署复杂度 | 高（多组件） | 低（单一镜像） |
| 演示友好度 | 一般 | 优秀 |
| CLI 客户端 | 无 | 有 |
