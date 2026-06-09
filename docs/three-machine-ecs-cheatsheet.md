# 三机 ECS 命令清单

## 0. 机器信息

- `control-plane`
  - public: `118.196.4.119`
  - private: `192.168.0.131`
- `node-b`
  - private: `192.168.0.129`
- `node-c`
  - private: `192.168.0.130`

## 1. 本地 SSH 准备

### 私钥权限

```bash
chmod 600 ~/.ssh/ddb.pem
```

### 加载到 agent

```bash
ssh-add ~/.ssh/ddb.pem
ssh-add -L
```

### 本地 `~/.ssh/config`

```sshconfig
Host ddb-control
  HostName 118.196.4.119
  User <your-ecs-user>
  IdentityFile ~/.ssh/ddb.pem
  ForwardAgent yes
  ServerAliveInterval 30
  ServerAliveCountMax 6

Host ddb-node-b
  HostName 192.168.0.129
  User <your-ecs-user>
  ProxyJump ddb-control
  ForwardAgent yes

Host ddb-node-c
  HostName 192.168.0.130
  User <your-ecs-user>
  ProxyJump ddb-control
  ForwardAgent yes
```

### 本地登录测试

```bash
ssh ddb-control
ssh ddb-node-b
ssh ddb-node-c
```

## 2. 本地编译

```bash
mkdir -p ./artifacts/linux-amd64/bin

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./artifacts/linux-amd64/bin/ddb-server ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./artifacts/linux-amd64/bin/ddb-cli ./cmd/cli
```

### 检查二进制架构

```bash
file ./artifacts/linux-amd64/bin/ddb-server
file ./artifacts/linux-amd64/bin/ddb-cli
file ./artifacts/linux-amd64/bin/etcd
```

## 3. 上传到控制面机

### 创建目录

```bash
ssh ddb-control 'mkdir -p /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs /opt/ddb/logs /opt/ddb/data /opt/ddb/state'
```

### 上传二进制

```bash
scp ./artifacts/linux-amd64/bin/ddb-server ddb-control:/opt/ddb/bin/
scp ./artifacts/linux-amd64/bin/ddb-cli ddb-control:/opt/ddb/bin/
scp ./artifacts/linux-amd64/bin/etcd ddb-control:/opt/ddb/bin/
```

### 上传脚本

```bash
scp ./scripts/ddb-cloud-control.sh ddb-control:/opt/ddb/scripts/
scp ./scripts/ddb-cloud-node.sh ddb-control:/opt/ddb/scripts/
scp ./scripts/ddb-cloud-wipe.sh ddb-control:/opt/ddb/scripts/
```

### 上传配置

```bash
scp ./configs/cloud/control-plane.ecs.json ddb-control:/opt/ddb/configs/
scp ./configs/cloud/node-a.ecs.json ddb-control:/opt/ddb/configs/
scp ./configs/cloud/node-b.ecs.json ddb-control:/opt/ddb/configs/
scp ./configs/cloud/node-c.ecs.json ddb-control:/opt/ddb/configs/
```

### 设置权限

```bash
ssh ddb-control 'chmod +x /opt/ddb/bin/ddb-server /opt/ddb/bin/ddb-cli /opt/ddb/bin/etcd /opt/ddb/scripts/ddb-cloud-control.sh /opt/ddb/scripts/ddb-cloud-node.sh /opt/ddb/scripts/ddb-cloud-wipe.sh'
```

### 检查上传结果

```bash
ssh ddb-control 'ls -l /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs'
```

## 4. 登录控制面机前先确认 agent

```bash
ssh-add ~/.ssh/ddb.pem
ssh-add -L
ssh -A ddb-control
```

### 控制面机内验证转发

```bash
ssh-add -L
```

## 5. 从控制面机分发到另外两台

### 创建目录

```bash
ssh 192.168.0.129 'mkdir -p /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs /opt/ddb/logs /opt/ddb/data /opt/ddb/state'
ssh 192.168.0.130 'mkdir -p /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs /opt/ddb/logs /opt/ddb/data /opt/ddb/state'
```

### 分发二进制

```bash
scp /opt/ddb/bin/ddb-server 192.168.0.129:/opt/ddb/bin/
scp /opt/ddb/bin/ddb-cli 192.168.0.129:/opt/ddb/bin/
scp /opt/ddb/bin/ddb-server 192.168.0.130:/opt/ddb/bin/
scp /opt/ddb/bin/ddb-cli 192.168.0.130:/opt/ddb/bin/
```

### 分发脚本

```bash
scp /opt/ddb/scripts/ddb-cloud-node.sh 192.168.0.129:/opt/ddb/scripts/
scp /opt/ddb/scripts/ddb-cloud-node.sh 192.168.0.130:/opt/ddb/scripts/
scp /opt/ddb/scripts/ddb-cloud-wipe.sh 192.168.0.129:/opt/ddb/scripts/
scp /opt/ddb/scripts/ddb-cloud-wipe.sh 192.168.0.130:/opt/ddb/scripts/
```

### 分发配置

```bash
scp /opt/ddb/configs/node-b.ecs.json 192.168.0.129:/opt/ddb/configs/
scp /opt/ddb/configs/node-c.ecs.json 192.168.0.130:/opt/ddb/configs/
```

### 设置权限

```bash
ssh 192.168.0.129 'chmod +x /opt/ddb/bin/ddb-server /opt/ddb/bin/ddb-cli /opt/ddb/scripts/ddb-cloud-node.sh /opt/ddb/scripts/ddb-cloud-wipe.sh'
ssh 192.168.0.130 'chmod +x /opt/ddb/bin/ddb-server /opt/ddb/bin/ddb-cli /opt/ddb/scripts/ddb-cloud-node.sh /opt/ddb/scripts/ddb-cloud-wipe.sh'
```

### 检查分发结果

```bash
ssh 192.168.0.129 'ls -l /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs'
ssh 192.168.0.130 'ls -l /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs'
```

## 6. 控制面机登录另外两台

```bash
ssh 192.168.0.129
ssh 192.168.0.130
```

## 7. 本地看 dashboard

### 本地建立端口转发

```bash
ssh -L 18101:192.168.0.131:18100 ddb-control
```

### 本地访问

```text
http://127.0.0.1:18101/dashboard/
```

## 8. 启动控制面

```bash
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action validate
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action start
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action status
curl http://192.168.0.131:18100/health
```

## 9. 启动 `node-a` bootstrap

### 校验

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action validate
```

### 启动 `3x3` bootstrap

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start -Name g1-n1
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start -Name g2-n1
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start -Name g3-n1
```

### 启动 `node-a` 全部预设

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start-all
```

## 10. 启动 `node-b`

### 登录

```bash
ssh 192.168.0.129
```

### 校验

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action validate
```

### 加入 `3x3`

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g1-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g2-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g3-n2
```

### 加入 `node-b` 全部预设

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join-all
```

## 11. 启动 `node-c`

### 登录

```bash
ssh 192.168.0.130
```

### 校验

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action validate
```

### 加入 `3x3`

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join -Name g1-n3
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join -Name g2-n3
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join -Name g3-n3
```

### 加入 `node-c` 全部预设

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join-all
```

## 12. 控制面机验证

### 交互

```bash
/opt/ddb/bin/ddb-cli --node-url=http://192.168.0.131:18100 interact
```

### 常用 `interact` 命令流

```text
help
cluster status
cluster leader
cluster members
cluster tables
control config
control groups
control shards
sql CREATE TABLE users (id INT PRIMARY KEY, name TEXT, sample FLOAT)
sql INSERT INTO users VALUES (1, 'alice', RAND())
sql INSERT INTO users VALUES (2, 'bob', RAND())
sql SELECT * FROM users WHERE id = 1
sql SELECT * FROM users WHERE id = 2
control move-shard 6 g3
control shards
control rebalance g1 g2 g3
control groups
control shards
cluster status
```

### inspect

```bash
/opt/ddb/bin/ddb-cli --node-url=http://192.168.0.129:21180 inspect "SELECT id, name, sample FROM users WHERE id = 1"
/opt/ddb/bin/ddb-cli --node-url=http://192.168.0.130:21280 inspect "SELECT id, name, sample FROM users WHERE id = 1"
/opt/ddb/bin/ddb-cli --node-url=http://192.168.0.129:21181 inspect "SELECT id, name, sample FROM users WHERE id = 2"
/opt/ddb/bin/ddb-cli --node-url=http://192.168.0.130:21281 inspect "SELECT id, name, sample FROM users WHERE id = 2"
```

```text
同一个副本组里，RAND() 的值应保持一致。
当前实现是 leader 先把 RAND() 物化成具体字面量，再复制到同组副本。
```

### `g4` 启动

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start -Name g4-n1
ssh 192.168.0.129 '/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g4-n2'
ssh 192.168.0.130 '/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join -Name g4-n3'
```

### `g4` 启动后查看 group 和 shard

```text
control groups
control shards
```

### `g4` rebalance

```text
control rebalance g1 g2 g3 g4
control groups
control shards
```

### 定向迁移 shard 到 `g4`

```text
control move-shard 6 g4
control shards
```

### 迁移后验证数据

```text
sql INSERT INTO users VALUES (3, 'carol')
sql SELECT * FROM users WHERE id = 3
```

## 13. 常用状态和日志

### 控制面状态

```bash
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action status
```

### 节点状态

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action status
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action status
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action status
```

### 看日志

```bash
tail -f /opt/ddb/logs/control-plane/apiserver.log
tail -f /opt/ddb/logs/node-a/g1-n1.log
tail -f /opt/ddb/logs/node-b/g1-n2.log
tail -f /opt/ddb/logs/node-c/g1-n3.log
```

## 14. 常用运维动作

### 宕机和恢复

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action stop -Name g1-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action start -Name g1-n2
```

### 移出和回组

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action remove -Name g1-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action rejoin -Name g1-n2
```

### 清理本地状态

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action remove-local -Name g1-n2
```

### 移出后清理本地状态再以原 `node_id` 回组

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action remove -Name g1-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action remove-local -Name g1-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action rejoin -Name g1-n2
```

### 新增预设节点

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action join -Name g1-n4
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g1-n5
```

### `g4` 注意事项

```text
- `g4-n1` 是新 group 的第一个节点，必须先 `start`
- `g4-n2` 和 `g4-n3` 再 `join`
- `g4` 起起来后默认不会自动拿到 shard
- 需要执行 `control rebalance g1 g2 g3 g4` 或 `control move-shard <shard-id> g4`
```

## 15. 停止全部

### 控制面机

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action stop-all
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action stop
```

### `node-b`

```bash
ssh 192.168.0.129 '/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action stop-all'
```

### `node-c`

```bash
ssh 192.168.0.130 '/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action stop-all'
```

## 16. 彻底清空运行痕迹

### 先看清理计划

```bash
/opt/ddb/scripts/ddb-cloud-wipe.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action plan
ssh 192.168.0.129 '/opt/ddb/scripts/ddb-cloud-wipe.sh -Config /opt/ddb/configs/node-b.ecs.json -Action plan'
ssh 192.168.0.130 '/opt/ddb/scripts/ddb-cloud-wipe.sh -Config /opt/ddb/configs/node-c.ecs.json -Action plan'
```

### 执行清理

```bash
/opt/ddb/scripts/ddb-cloud-wipe.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action wipe -Confirm WIPE
ssh 192.168.0.129 '/opt/ddb/scripts/ddb-cloud-wipe.sh -Config /opt/ddb/configs/node-b.ecs.json -Action wipe -Confirm WIPE'
ssh 192.168.0.130 '/opt/ddb/scripts/ddb-cloud-wipe.sh -Config /opt/ddb/configs/node-c.ecs.json -Action wipe -Confirm WIPE'
```

## 17. 全新重建顺序

```bash
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action start
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start -Name g1-n1
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start -Name g2-n1
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start -Name g3-n1
ssh 192.168.0.129 '/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g1-n2'
ssh 192.168.0.129 '/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g2-n2'
ssh 192.168.0.129 '/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g3-n2'
ssh 192.168.0.130 '/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join -Name g1-n3'
ssh 192.168.0.130 '/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join -Name g2-n3'
ssh 192.168.0.130 '/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join -Name g3-n3'
```

## 18. 备注

- 本地终端可以用 `ddb-control`、`ddb-node-b`、`ddb-node-c`
- 控制面机终端里直接用 `192.168.0.129`、`192.168.0.130`
- `remove-local` 只清本地状态，不清 removed 标记
- 同一个被 `remove` 过的 `node_id`，后续仍然应该 `rejoin`
- 全部 shard 都 `stop-all` 后，不适合直接 `start-all` 原样冷启动；如果要完全重来，先 wipe 再按“全新重建顺序”执行
