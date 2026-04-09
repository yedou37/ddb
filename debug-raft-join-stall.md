[OPEN]

# Debug Session: raft-join-stall

## Symptom

- node2 和 node3 加入集群时，终端出现：
  - `appendEntries rejected, sending older logs`
  - `failed to get previous log: previous-index=3/4 last-index=0 error="log not found"`
- 用户怀疑日志复制卡住。

## Hypotheses

1. 这是新 follower 从空日志开始追 leader 历史日志时的正常现象，并非故障。
2. join 已成功，但 follower 还没完成第一轮回放，所以短时间内只看到拒绝旧索引。
3. leader 使用内存 log store，节点重启后历史日志会丢失，可能导致后续更难判断复制状态。
4. 如果之后没有出现持续复制或查询结果不同，才说明复制链路真正有问题。

## Current Evidence

- leader 侧出现 `updating configuration: command=AddVoter` 和 `added peer, starting replication`
- 之后出现 `appendEntries rejected, sending older logs`
- follower 侧出现 `failed to get previous log ... last-index=0`
- 用户随后实际执行：
  - `CREATE TABLE t (...)`
  - `INSERT INTO t VALUES (1, 'alice')`
  - 在 `node2` 与 `node3` 上分别 `SELECT * FROM t`
- 两个 follower 都返回了同样的结果：`[1, "alice"]`

## Evidence Assessment

- 假设 1 confirmed：当前日志更像新 follower 追 leader 历史日志时的正常现象
- 假设 2 confirmed：join 成功，复制链路已经工作
- 假设 3 still open：当前 Raft log/stable store 仍是内存实现，重启后的行为还需要单独验证
- 假设 4 rejected：在本轮证据下，不存在“看起来 join 了但实际上没有复制”的情况

## Current Status

- 当前问题不是“是否复制成功”，而是“为什么终端没有继续打印复制日志”。
