# 三机 ECS 从搭建环境到执行手册

这份文档面向当前已经申请好的三台 ECS，目标是把下面整条链路串起来：

- 本地准备 SSH 和 Linux 二进制
- 上传文件到控制面机
- 从控制面机分发到另外两台机器
- 启动 `etcd`、`apiserver` 和 `3 group x 3 replica`
- 用 `ddb-cli interact` 和 `inspect` 做功能验证
- 继续演示 `stop/start`、`remove/rejoin`、`remove-local/join`

## 一页版流程

第一次从零搭环境，建议严格按这个顺序：

1. 本地准备私钥，并执行 `ssh-add ~/.ssh/ddb.pem`
2. 本地检查 `ssh-add -L`，确认 agent 里能看到公钥
3. 本地配置 `~/.ssh/config`
4. 本地编译 `linux/amd64` 版本 `ddb-server`、`ddb-cli`、`etcd`
5. 本地把二进制、脚本、配置上传到 `ddb-control`
6. 用 `ssh -A ddb-control` 登录控制面机
7. 在控制面机检查 `ssh-add -L`，确认 agent forwarding 生效
8. 从控制面机把文件分发到 `node-b`、`node-c`
9. 在控制面机启动 `etcd + apiserver`
10. 在控制面机启动 `g1-n1/g2-n1/g3-n1`
11. 在 `node-b` 上 `join g1-n2/g2-n2/g3-n2`
12. 在 `node-c` 上 `join g1-n3/g2-n3/g3-n3`
13. 回到控制面机执行 `ddb-cli interact`
14. 执行 `control groups`、`control shards`、建表、插入、查询、`inspect`

如果你只是现场演示，先只跑 `3x3`，不要一开始就启动 `n4/n5`。

这份文档对应当前已经申请好的三台 ECS：

- `control-plane`
  - 公网：`118.196.4.119`
  - 私网：`192.168.0.131`
- `node-b`
  - 私网：`192.168.0.129`
- `node-c`
  - 私网：`192.168.0.130`

## 配置文件

仓库里已经生成了 4 份正式配置：

- `configs/cloud/control-plane.ecs.json`
- `configs/cloud/node-a.ecs.json`
- `configs/cloud/node-b.ecs.json`
- `configs/cloud/node-c.ecs.json`

它们对应的机器分工是：

- `control-plane.ecs.json`
  - `etcd + apiserver`
- `node-a.ecs.json`
  - 跑在 `192.168.0.131`
  - `g1-n1`
  - `g2-n1`
  - `g3-n1`
  - `g1-n4`
  - `g2-n4`
- `node-b.ecs.json`
  - 跑在 `192.168.0.129`
  - `g1-n2`
  - `g2-n2`
  - `g3-n2`
  - `g3-n4`
  - `g1-n5`
- `node-c.ecs.json`
  - 跑在 `192.168.0.130`
  - `g1-n3`
  - `g2-n3`
  - `g3-n3`
  - `g2-n5`
  - `g3-n5`

如果你这次只想先跑 `3 group × 3 replica`，可以只启动：

- `node-a` 上的 `g1-n1`、`g2-n1`、`g3-n1`
- `node-b` 上的 `g1-n2`、`g2-n2`、`g3-n2`
- `node-c` 上的 `g1-n3`、`g2-n3`、`g3-n3`

## 私钥放哪里

推荐把私钥只放在你本地机器，不要复制到 ECS 上。

建议路径：

```text
~/.ssh/ddb.pem
```

权限要收紧：

```bash
chmod 600 ~/.ssh/ddb.pem
```

## 为什么不把私钥放到控制面机器

更推荐用 `ssh -A` 做 agent forwarding：

- 私钥只保留在你的本地电脑
- 先 SSH 到控制面机
- 再从控制面机 SSH 到 `node-b` 和 `node-c`
- 不需要把私钥文件复制到远端

## 先确认 SSH Agent 正常

在本地机器执行：

```bash
ssh-add ~/.ssh/ddb.pem
ssh-add -L
```

如果这里显示 `The agent has no identities.`，说明你还没有把私钥加入本地 agent。

然后再登录控制面机：

```bash
ssh -A ddb-control
ssh-add -L
```

如果你在控制面机里看到的仍然是 `The agent has no identities.`，说明这次 SSH 没有把本地 agent 转发过去。常见原因只有两个：

- 你是在本地忘了先执行 `ssh-add`
- `ddb-control` 的 SSH 配置里没有开启 `ForwardAgent yes`

只有当控制面机里 `ssh-add -L` 也能看到公钥时，才说明它可以继续免密访问 `node-b` 和 `node-c`。

## 建议先配一个本地 SSH config

编辑本地 `~/.ssh/config`：

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

说明：

- `<your-ecs-user>` 请替换成你的 ECS 登录用户
- 常见可能是 `root`、`ecs-user`、`ubuntu`
- 你用哪个用户能正常 SSH，就把它写进去

配置完之后，本地可以这样登录：

```bash
ssh ddb-control
ssh ddb-node-b
ssh ddb-node-c
```

## 第一步：本地编译 Linux 二进制

在仓库根目录执行：

```bash
mkdir -p ./artifacts/linux-amd64/bin

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./artifacts/linux-amd64/bin/ddb-server ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./artifacts/linux-amd64/bin/ddb-cli ./cmd/cli
```

准备 `etcd`：

- 方式 A：本地下载 Linux 版 `etcd` 到 `./artifacts/linux-amd64/bin/etcd`
- 方式 B：登录控制面机后单独安装

建议最终本地目录是：

```text
artifacts/linux-amd64/bin/
  ddb-server
  ddb-cli
  etcd
```

建议本地先自检一遍：

```bash
file ./artifacts/linux-amd64/bin/ddb-server
file ./artifacts/linux-amd64/bin/ddb-cli
file ./artifacts/linux-amd64/bin/etcd
```

目标应该是 `ELF 64-bit` 的 Linux 可执行文件，而不是 macOS Mach-O。

## 第二步：先传到控制面机

先创建目录：

```bash
ssh ddb-control 'mkdir -p /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs /opt/ddb/logs /opt/ddb/data /opt/ddb/state'
```

上传二进制：

```bash
scp ./artifacts/linux-amd64/bin/ddb-server ddb-control:/opt/ddb/bin/
scp ./artifacts/linux-amd64/bin/ddb-cli ddb-control:/opt/ddb/bin/
scp ./artifacts/linux-amd64/bin/etcd ddb-control:/opt/ddb/bin/
```

上传脚本：

```bash
scp ./scripts/ddb-cloud-control.sh ddb-control:/opt/ddb/scripts/
scp ./scripts/ddb-cloud-node.sh ddb-control:/opt/ddb/scripts/
```

上传配置：

```bash
scp ./configs/cloud/control-plane.ecs.json ddb-control:/opt/ddb/configs/
scp ./configs/cloud/node-a.ecs.json ddb-control:/opt/ddb/configs/
scp ./configs/cloud/node-b.ecs.json ddb-control:/opt/ddb/configs/
scp ./configs/cloud/node-c.ecs.json ddb-control:/opt/ddb/configs/
```

设置权限：

```bash
ssh ddb-control 'chmod +x /opt/ddb/bin/ddb-server /opt/ddb/bin/ddb-cli /opt/ddb/bin/etcd /opt/ddb/scripts/ddb-cloud-control.sh /opt/ddb/scripts/ddb-cloud-node.sh'
```

上传后可以先检查一遍：

```bash
ssh ddb-control 'ls -l /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs'
```

## 第三步：从控制面机分发到另外两台

先登录控制面机：

```bash
ssh -A ddb-control
```

先确认 agent forwarding 正常：

```bash
ssh-add -L
```

如果这里还是 `The agent has no identities.`，不要继续分发，先回本地修复 `ssh-add` 和 `ForwardAgent`。

在控制面机里创建目录：

```bash
ssh 192.168.0.129 'mkdir -p /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs /opt/ddb/logs /opt/ddb/data /opt/ddb/state'
ssh 192.168.0.130 'mkdir -p /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs /opt/ddb/logs /opt/ddb/data /opt/ddb/state'
```

分发二进制：

```bash
scp /opt/ddb/bin/ddb-server 192.168.0.129:/opt/ddb/bin/
scp /opt/ddb/bin/ddb-cli 192.168.0.129:/opt/ddb/bin/

scp /opt/ddb/bin/ddb-server 192.168.0.130:/opt/ddb/bin/
scp /opt/ddb/bin/ddb-cli 192.168.0.130:/opt/ddb/bin/
```

分发脚本：

```bash
scp /opt/ddb/scripts/ddb-cloud-node.sh 192.168.0.129:/opt/ddb/scripts/
scp /opt/ddb/scripts/ddb-cloud-node.sh 192.168.0.130:/opt/ddb/scripts/
```

分发配置：

```bash
scp /opt/ddb/configs/node-b.ecs.json 192.168.0.129:/opt/ddb/configs/
scp /opt/ddb/configs/node-c.ecs.json 192.168.0.130:/opt/ddb/configs/
```

设置权限：

```bash
ssh 192.168.0.129 'chmod +x /opt/ddb/bin/ddb-server /opt/ddb/bin/ddb-cli /opt/ddb/scripts/ddb-cloud-node.sh'
ssh 192.168.0.130 'chmod +x /opt/ddb/bin/ddb-server /opt/ddb/bin/ddb-cli /opt/ddb/scripts/ddb-cloud-node.sh'
```

分发后建议检查：

```bash
ssh 192.168.0.129 'ls -l /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs'
ssh 192.168.0.130 'ls -l /opt/ddb/bin /opt/ddb/scripts /opt/ddb/configs'
```

## 第四步：启动控制面

仍然在控制面机执行：

```bash
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action validate
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action start
```

检查状态：

```bash
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.ecs.json -Action status
```

建议额外检查：

```bash
curl http://192.168.0.131:18100/health
```

## 第五步：启动 node-a 上的 bootstrap shard

在控制面机执行：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action validate
```

如果你先跑 `3*3`，逐个启动 bootstrap：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start -Name g1-n1
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start -Name g2-n1
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start -Name g3-n1
```

如果你要把 `node-a` 上所有预设都拉起来：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.ecs.json -Action start-all
```

## 第六步：启动 node-b 和 node-c

首次加入副本组，推荐用 `join` 语义。

登录 `node-b`：

```bash
ssh ddb-node-b
```

先校验：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action validate
```

如果你先跑 `3*3`，逐个加入：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g1-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g2-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join -Name g3-n2
```

如果你要把 `node-b` 上所有预设都加进去：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action join-all
```

登录 `node-c`：

```bash
ssh ddb-node-c
```

先校验：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action validate
```

如果你先跑 `3*3`，逐个加入：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join -Name g1-n3
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join -Name g2-n3
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join -Name g3-n3
```

如果你要把 `node-c` 上所有预设都加进去：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.ecs.json -Action join-all
```

## 第七步：控制面机上验证

在控制面机执行：

```bash
/opt/ddb/bin/ddb-cli --node-url=http://127.0.0.1:18100 interact
```

进入后建议先跑：

```text
control groups
control shards
sql CREATE TABLE users (id INT PRIMARY KEY, name TEXT)
sql INSERT INTO users VALUES (1, 'alice')
sql SELECT * FROM users WHERE id = 1
```

如果你想按演示顺序来，推荐用这套最小验证流：

```text
control groups
control shards
sql CREATE TABLE users (id INT PRIMARY KEY, name TEXT)
sql INSERT INTO users VALUES (1, 'alice')
sql INSERT INTO users VALUES (2, 'bob')
sql SELECT * FROM users WHERE id = 1
sql SELECT * FROM users WHERE id = 2
```

## inspect 怎么做

`inspect` 本质上是 CLI 直连具体 node，所以仍然建议在控制面机执行：

```bash
/opt/ddb/bin/ddb-cli --node-url=http://192.168.0.129:21180 inspect "SELECT * FROM users WHERE id = 1"
```

如果你要分别检查三个 group，当前 `3x3` 的常用端口可以直接记成：

- `g1-n1` -> `http://192.168.0.131:21080`
- `g1-n2` -> `http://192.168.0.129:21180`
- `g1-n3` -> `http://192.168.0.130:21280`
- `g2-n1` -> `http://192.168.0.131:21081`
- `g2-n2` -> `http://192.168.0.129:21181`
- `g2-n3` -> `http://192.168.0.130:21281`
- `g3-n1` -> `http://192.168.0.131:21082`
- `g3-n2` -> `http://192.168.0.129:21182`
- `g3-n3` -> `http://192.168.0.130:21282`

## 宕机、恢复、remove、rejoin、remove-local

例如模拟 `g1-n2` 宕机：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action stop -Name g1-n2
```

恢复：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action start -Name g1-n2
```

永久移出：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action remove -Name g1-n2
```

保留本地状态重新加入：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action rejoin -Name g1-n2
```

如果想清理它的本地状态：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action remove-local -Name g1-n2
```

这里的 `remove-local` 表示：

- 只清理本地 `raft/db/log/pid`
- 不修改副本组成员关系
- 不清除 etcd 中的 removed 标记

所以如果同一个 `node_id` 已经执行过 `remove`，即使你又执行了 `remove-local`，后续也仍然应该使用 `rejoin`，而不是普通 `join`。

## 你现在最推荐的实际操作顺序

第一次搭环境时，建议严格按这个顺序：

1. 本地准备私钥到 `~/.ssh/ddb.pem`
2. 本地配置 `~/.ssh/config`
3. 本地编译 `linux/amd64` 二进制
4. 上传到 `ddb-control`
5. 从 `ddb-control` 分发到 `ddb-node-b`、`ddb-node-c`
6. 控制面机启动 `etcd + apiserver`
7. 控制面机启动 `g1-n1/g2-n1/g3-n1`
8. `node-b` 执行 `join` 加入 `g1-n2/g2-n2/g3-n2`
9. `node-c` 执行 `join` 加入 `g1-n3/g2-n3/g3-n3`
10. 控制面机运行 `ddb-cli interact` 验证

## 演示时常用动作

最常见的三类演示动作如下。

### 1. 宕机与恢复

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action stop -Name g1-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action start -Name g1-n2
```

### 2. 节点移出后保留旧状态回组

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action remove -Name g1-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action rejoin -Name g1-n2
```

### 3. 节点移出后清空本地状态再以原 node_id 回组

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action remove -Name g1-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action remove-local -Name g1-n2
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.ecs.json -Action rejoin -Name g1-n2
```

## 演示前检查单

正式执行前，建议快速过一遍：

- 本地 `ssh-add -L` 能看到公钥
- 控制面机里 `ssh-add -L` 也能看到同一把公钥
- `/opt/ddb/bin`、`/opt/ddb/scripts`、`/opt/ddb/configs` 三台机器都齐全
- `ddb-cloud-control.sh -Action status` 正常
- `ddb-cloud-node.sh -Action status` 正常
- `curl http://192.168.0.131:18100/health` 正常
- `ddb-cli --node-url=http://127.0.0.1:18100 interact` 能进入交互模式
- `control groups` 和 `control shards` 输出正常
- 至少一个 `inspect` 成功

## 注意

- 所有集群地址一律使用私网 IP
- 公网 IP `118.196.4.119` 只用于你本地 SSH 登录控制面机
- 不建议把私钥复制到 ECS 上
- 更推荐 `ssh -A`，让控制面机通过 agent forwarding 访问内网两台
- 如果只演示 `3*3`，先不要起 `n4/n5`
- `stop -> start` 演示的是宕机恢复，不是成员重加
- `remove -> rejoin` 演示的是保留旧状态重新回组
- `remove -> remove-local -> rejoin` 演示的是把旧节点移出、清空本地状态后，再以原 `node_id` 回组
