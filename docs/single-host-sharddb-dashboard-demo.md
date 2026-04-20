# Single-Host ShardDB Dashboard Demo Guide

这份文档总结了当前仓库在本机单机环境下已经验证通过的一套完整演示流程，适合课堂展示或答辩彩排。

目标包括：

- 清理环境
- 启动单机版 `ShardDB`
- 打开 dashboard
- 注入演示数据
- 验证读写
- 在线新增 `g4`
- 执行 `rebalance`
- 验证历史数据和新写入数据
- 模拟节点故障与恢复
- 最后清理环境

本文档中的命令默认在仓库根目录执行：

```bash
cd /Users/bytedance/dbd
```

## 1. 清理环境

先把旧的 demo 环境清理掉：

```bash
./scripts/demo-single-host-sharddb.sh cleanup-only
```

这一步会清理：

- demo 相关进程
- demo 占用端口
- `/tmp/ddb-demo` 下的临时文件
- 手动新增的 `g4` 数据目录

## 2. 启动环境

只启动环境，不跑内置验证：

```bash
./scripts/demo-single-host-sharddb.sh start-only
```

默认会启动：

- `etcd`
- `g1-n1`, `g1-n2`, `g1-n3`
- `g2-n1`, `g2-n2`, `g2-n3`
- `g3-n1`, `g3-n2`, `g3-n3`
- `apiserver`

脚本会尽量在当前终端窗口中打开新标签页。

## 3. 打开 Dashboard

打开 dashboard：

```bash
open http://127.0.0.1:18100/dashboard/
```

如果页面有缓存，强刷浏览器：

```text
Cmd + Shift + R
```

dashboard 聚合接口：

```bash
curl -s http://127.0.0.1:18100/dashboard/api/overview
```

## 4. 注入演示数据

向已经启动好的环境创建 `users` 表并插入默认 40 行测试数据：

```bash
./scripts/demo-single-host-sharddb.sh seed-only
```

如果想插入更多数据，例如 100 行：

```bash
DDB_SEED_ROW_COUNT=100 ./scripts/demo-single-host-sharddb.sh seed-only
```

当前 `seed-only` 的行为：

- 确保 `users` 表存在
- 清空 `users` 表中 `1..SEED_ROW_COUNT` 范围内的数据
- 重新插入 `user-001 ... user-040`

## 5. 初始状态验证

### 5.1 查看控制面状态

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control config
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control groups
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control shards
```

### 5.2 验证数据可读

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 1"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 10"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 20"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 40"
```

### 5.3 在 dashboard 中查看表数据

页面操作：

1. 打开 `Table Browser`
2. 选择 `users`
3. 点击 `查询数据`
4. 可点击列名做升序/降序排序

## 6. 在线新增 g4

### 6.1 创建目录

```bash
mkdir -p /tmp/ddb-demo/g4-n1/raft /tmp/ddb-demo/g4-n2/raft /tmp/ddb-demo/g4-n3/raft
```

### 6.2 启动 g4-n1

在一个新的终端标签页执行：

```bash
cd /Users/bytedance/dbd
./scripts/run-sharddb-node.sh shard g4-n1 127.0.0.1:21083 127.0.0.1:22083 /tmp/ddb-demo/g4-n1/raft /tmp/ddb-demo/g4-n1/data.db g4 true "" 127.0.0.1:2379
```

### 6.3 启动 g4-n2

在一个新的终端标签页执行：

```bash
cd /Users/bytedance/dbd
./scripts/run-sharddb-node.sh shard g4-n2 127.0.0.1:21183 127.0.0.1:22183 /tmp/ddb-demo/g4-n2/raft /tmp/ddb-demo/g4-n2/data.db g4 false http://127.0.0.1:21083 127.0.0.1:2379
```

### 6.4 启动 g4-n3

在一个新的终端标签页执行：

```bash
cd /Users/bytedance/dbd
./scripts/run-sharddb-node.sh shard g4-n3 127.0.0.1:21283 127.0.0.1:22283 /tmp/ddb-demo/g4-n3/raft /tmp/ddb-demo/g4-n3/data.db g4 false http://127.0.0.1:21083 127.0.0.1:2379
```

### 6.5 观察 g4 已加入

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control groups
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control shards
```

此时 dashboard 中应该能看到：

- `Cluster Topology` 多出 `g4-n1`, `g4-n2`, `g4-n3`
- `Group Health` 多出 `g4`

## 7. 执行 Rebalance

### 7.1 执行 rebalance

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control rebalance g1 g2 g3 g4
```

### 7.2 查看 rebalance 结果

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control groups
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control shards
```

此时 dashboard 中应该能看到：

- `Shard Map` 中部分 shard 迁移到 `g4`
- `Group Health` 中 `g4` 的 shard 数量增加
- 如果迁移过程中某些 shard 被锁，会在 `Migration & Alerts` 中看到对应 shard

## 8. Rebalance 后验证数据

### 8.1 历史数据仍可读

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 1"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 10"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 20"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 40"
```

### 8.2 Rebalance 后继续写入

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "INSERT INTO users VALUES (101, 'after-rebalance-101')"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "INSERT INTO users VALUES (102, 'after-rebalance-102')"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "INSERT INTO users VALUES (103, 'after-rebalance-103')"
```

### 8.3 查询新写入的数据

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 101"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 102"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 103"
```

### 8.4 在 dashboard 中验证表数据

页面操作：

1. `Table Browser` 选择 `users`
2. 点击 `查询数据`
3. 点击 `id` 列名排序
4. 确认能看到 `101`, `102`, `103`

## 9. 节点故障演示

这里以 `g1-n1` 为例。

### 9.1 杀掉 g1-n1

```bash
pkill -f "g1-n1"
```

或者在 `g1-n1` 所在终端标签页直接按：

```text
Ctrl + C
```

### 9.2 故障后观察 dashboard

你应该能看到：

- `g1-n1` 保留在拓扑中
- 节点状态变成 `offline`
- 对应 group 可能变成 `degraded`

### 9.3 故障后继续验证读写

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control groups
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 1"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "INSERT INTO users VALUES (104, 'after-g1n1-down')"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 104"
```

## 10. 节点恢复演示

### 10.1 重新启动 g1-n1

普通故障恢复时，保留原目录，直接按原命令重启：

```bash
cd /Users/bytedance/dbd
./scripts/run-sharddb-node.sh shard g1-n1 127.0.0.1:21080 127.0.0.1:22080 /tmp/ddb-demo/g1-n1/raft /tmp/ddb-demo/g1-n1/data.db g1 true "" 127.0.0.1:2379
```

注意：

- 不要删除 `/tmp/ddb-demo/g1-n1/raft`
- 不要删除 `/tmp/ddb-demo/g1-n1/data.db`

### 10.2 恢复后验证

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control groups
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 1"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 104"
```

dashboard 中你应该能看到：

- `g1-n1` 从 `offline` 变回 `online`
- `g1` 状态恢复

## 11. 可选的 remove / rejoin 演示

如果你想演示“安全移除后再恢复”，这是另一条链路，不等同于普通故障恢复。

### 11.1 remove 节点

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control remove g1-n1
```

### 11.2 停掉节点进程

```bash
pkill -f "g1-n1"
```

### 11.3 注意

- 此时如果直接按普通方式重启，应该会被拒绝
- `remove` 后恢复应走专门的 `rejoin` 语义

## 12. 结束后清理环境

```bash
./scripts/demo-single-host-sharddb.sh cleanup-only
```

## 13. 最短答辩演示顺序

如果你时间有限，可以按下面这个顺序：

### 13.1 清理并启动

```bash
./scripts/demo-single-host-sharddb.sh cleanup-only
./scripts/demo-single-host-sharddb.sh start-only
./scripts/demo-single-host-sharddb.sh seed-only
open http://127.0.0.1:18100/dashboard/
```

### 13.2 验证原始状态

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 control groups
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 1"
```

### 13.3 新增 g4 并 rebalance

```bash
mkdir -p /tmp/ddb-demo/g4-n1/raft /tmp/ddb-demo/g4-n2/raft /tmp/ddb-demo/g4-n3/raft

./scripts/run-sharddb-node.sh shard g4-n1 127.0.0.1:21083 127.0.0.1:22083 /tmp/ddb-demo/g4-n1/raft /tmp/ddb-demo/g4-n1/data.db g4 true "" 127.0.0.1:2379
./scripts/run-sharddb-node.sh shard g4-n2 127.0.0.1:21183 127.0.0.1:22183 /tmp/ddb-demo/g4-n2/raft /tmp/ddb-demo/g4-n2/data.db g4 false http://127.0.0.1:21083 127.0.0.1:2379
./scripts/run-sharddb-node.sh shard g4-n3 127.0.0.1:21283 127.0.0.1:22283 /tmp/ddb-demo/g4-n3/raft /tmp/ddb-demo/g4-n3/data.db g4 false http://127.0.0.1:21083 127.0.0.1:2379

./bin/ddb-cli --node-url=http://127.0.0.1:18100 control rebalance g1 g2 g3 g4
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 1"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "INSERT INTO users VALUES (101, 'after-rebalance-101')"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 101"
```

### 13.4 故障与恢复

```bash
pkill -f "g1-n1"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 1"
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "INSERT INTO users VALUES (104, 'after-g1n1-down')"

./scripts/run-sharddb-node.sh shard g1-n1 127.0.0.1:21080 127.0.0.1:22080 /tmp/ddb-demo/g1-n1/raft /tmp/ddb-demo/g1-n1/data.db g1 true "" 127.0.0.1:2379
./bin/ddb-cli --node-url=http://127.0.0.1:18100 sql "SELECT * FROM users WHERE id = 104"
```

### 13.5 结束清理

```bash
./scripts/demo-single-host-sharddb.sh cleanup-only
```
