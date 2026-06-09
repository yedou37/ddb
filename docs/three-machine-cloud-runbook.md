# 三机云服务器部署初步手册

这份文档用于记录一个更贴近真实验收方式的三机部署方案。

目标不是压测，而是确保下面这些流程可以顺利演示：

- 通过 SSH 登录集群
- 启动 `etcd`、`apiserver` 和 9 个 shard 节点
- 使用 `ddb-cli interact` 执行 `sql`、`control groups`、`control shards`
- 使用 `inspect` 直连具体 node 检查本地数据
- 演示 `move-shard`、单节点故障、恢复等流程

## 结论先说

当前建议的方案是：

- 三台云服务器
- 全部节点之间只走内网
- 只给控制面机器开公网 SSH
- `ddb-cli` 放在控制面机器上运行
- 所有 `inspect` 操作也在控制面机器上执行
- 本地提前编译 `linux/amd64` 二进制，再上传到云服务器
- cloud sample 模板默认按 `3 group × 5 replica` 组织，可按需删减回 `3 group × 3 replica`

推荐资源规格：

- 更稳妥：`1 台 2C4G + 2 台 2C2G`
- 预算优先：`3 台 2C2G`

如果只是课程展示和功能验收，`3 台 2C2G` 也可以接受。

## 机器分工

建议按下面方式分工：

- `control`
  - `etcd`
  - `apiserver`
  - `g1-n1`
  - `g2-n1`
  - `g3-n1`
- `node-b`
  - `g1-n2`
  - `g2-n2`
  - `g3-n2`
- `node-c`
  - `g1-n3`
  - `g2-n3`
  - `g3-n3`

也就是说，控制面机器不仅承担控制平面，还承担每个 group 的一个 bootstrap 节点。

当前 `configs/cloud/*.sample.json` 已经给出一个 `3 group × 5 replica` 的分布示例：

- `node-a.sample.json`
  - `g1-n1`
  - `g2-n1`
  - `g3-n1`
  - `g1-n4`
  - `g2-n4`
- `node-b.sample.json`
  - `g1-n2`
  - `g2-n2`
  - `g3-n2`
  - `g3-n4`
  - `g1-n5`
- `node-c.sample.json`
  - `g1-n3`
  - `g2-n3`
  - `g3-n3`
  - `g2-n5`
  - `g3-n5`

## 网络建议

### 公网

只给控制面机器开公网 `22`：

- `control`
  - 公网入站：`22`
  - 来源：只允许你的演示机公网 IP
- `node-b`
  - 不开公网入站
- `node-c`
  - 不开公网入站

### 内网

三台机器之间需要通过内网互相访问：

- `2379`：`etcd`
- `18100`：`apiserver`
- `30100`：`apiserver` Raft
- `21080/21180/21280`
- `21081/21181/21281`
- `21082/21182/21282`
- `22080/22180/22280`
- `22081/22181/22281`
- `22082/22182/22282`

配置中所有地址都建议写内网 IP，不要混用公网 IP。

## 为什么两台数据机不用公网

因为：

- 日常 `sql`、`control groups`、`control shards` 可以通过控制面入口完成
- `inspect` 虽然本质是 CLI 直连具体 node，但 CLI 可以放在控制面机器上运行
- 控制面机器通过内网就能访问另外两台上的 shard 节点

所以最推荐的方式是：

- 你的电脑只 SSH 到控制面机器
- 再从控制面机器通过内网 SSH 到另外两台
- 或者直接在控制面机器上远程执行 SSH 命令启动另外两台上的进程

## 目录约定

建议三台机器统一目录结构：

```text
/opt/ddb/
  bin/
  configs/
  logs/
  data/
```

建议二进制放到：

```text
/opt/ddb/bin/ddb-server
/opt/ddb/bin/ddb-cli
/opt/ddb/bin/etcd
```

建议把本仓库里的下面这些文件也一起上传到控制面机器，再从控制面机器分发到另外两台：

- `scripts/ddb-cloud-control.sh`
- `scripts/ddb-cloud-node.sh`
- `configs/cloud/control-plane.sample.json`
- `configs/cloud/node-a.sample.json`
- `configs/cloud/node-b.sample.json`
- `configs/cloud/node-c.sample.json`

推荐放到：

```text
/opt/ddb/scripts/
/opt/ddb/configs/
```

## 本地需要准备什么

先在你自己的电脑上准备 Linux 二进制。

假设目标云服务器架构是 `linux/amd64`，建议执行：

```bash
mkdir -p ./artifacts/linux-amd64/bin

CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./artifacts/linux-amd64/bin/ddb-server ./cmd/server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./artifacts/linux-amd64/bin/ddb-cli ./cmd/cli
```

`etcd` 可以用两种方式准备：

- 直接在控制面机器上安装
- 本地下载 Linux 版 `etcd`，也放进 `./artifacts/linux-amd64/bin/`

如果你想统一上传，建议最终本地目录至少长这样：

```text
artifacts/linux-amd64/
  bin/
    ddb-server
    ddb-cli
    etcd
```

## 如何把程序放到三台机器上

### 第一步：先传到控制面机器

先在控制面机器上建目录：

```bash
ssh ubuntu@<control-public-ip> 'mkdir -p /opt/ddb/bin /opt/ddb/configs /opt/ddb/logs /opt/ddb/data'
```

上传二进制：

```bash
scp ./artifacts/linux-amd64/bin/ddb-server ubuntu@<control-public-ip>:/opt/ddb/bin/
scp ./artifacts/linux-amd64/bin/ddb-cli ubuntu@<control-public-ip>:/opt/ddb/bin/
scp ./artifacts/linux-amd64/bin/etcd ubuntu@<control-public-ip>:/opt/ddb/bin/
```

设权限：

```bash
ssh ubuntu@<control-public-ip> 'chmod +x /opt/ddb/bin/ddb-server /opt/ddb/bin/ddb-cli /opt/ddb/bin/etcd'
```

### 第二步：从控制面机器分发到另外两台

登录控制面机器：

```bash
ssh ubuntu@<control-public-ip>
```

在控制面机器里建目录：

```bash
ssh ubuntu@<node-b-internal-ip> 'mkdir -p /opt/ddb/bin /opt/ddb/configs /opt/ddb/logs /opt/ddb/data'
ssh ubuntu@<node-c-internal-ip> 'mkdir -p /opt/ddb/bin /opt/ddb/configs /opt/ddb/logs /opt/ddb/data'
```

分发二进制：

```bash
scp /opt/ddb/bin/ddb-server ubuntu@<node-b-internal-ip>:/opt/ddb/bin/
scp /opt/ddb/bin/ddb-cli ubuntu@<node-b-internal-ip>:/opt/ddb/bin/

scp /opt/ddb/bin/ddb-server ubuntu@<node-c-internal-ip>:/opt/ddb/bin/
scp /opt/ddb/bin/ddb-cli ubuntu@<node-c-internal-ip>:/opt/ddb/bin/
```

通常只有控制面机器需要 `etcd`，所以 `node-b` 和 `node-c` 不一定要分发 `etcd`。

最后设权限：

```bash
ssh ubuntu@<node-b-internal-ip> 'chmod +x /opt/ddb/bin/ddb-server /opt/ddb/bin/ddb-cli'
ssh ubuntu@<node-c-internal-ip> 'chmod +x /opt/ddb/bin/ddb-server /opt/ddb/bin/ddb-cli'
```

脚本和配置模板也建议一起传：

```bash
scp ./scripts/ddb-cloud-control.sh ubuntu@<control-public-ip>:/opt/ddb/scripts/
scp ./scripts/ddb-cloud-node.sh ubuntu@<control-public-ip>:/opt/ddb/scripts/
scp ./configs/cloud/*.sample.json ubuntu@<control-public-ip>:/opt/ddb/configs/
ssh ubuntu@<control-public-ip> 'chmod +x /opt/ddb/scripts/ddb-cloud-control.sh /opt/ddb/scripts/ddb-cloud-node.sh'
```

然后在控制面机器上复制出真正要用的配置：

```bash
cp /opt/ddb/configs/control-plane.sample.json /opt/ddb/configs/control-plane.json
cp /opt/ddb/configs/node-a.sample.json /opt/ddb/configs/node-a.json
cp /opt/ddb/configs/node-b.sample.json /opt/ddb/configs/node-b.json
cp /opt/ddb/configs/node-c.sample.json /opt/ddb/configs/node-c.json
```

你只需要把这 4 份配置里的占位值替换掉：

- `<control-private-ip>`
- `<node-b-private-ip>`
- `<node-c-private-ip>`

如果 `install_root` 不是 `/opt/ddb`，也一起改掉。

## 如何启动程序

### 用脚本启动

推荐直接用新脚本，不要现场手敲长命令。

控制面机器先执行：

```bash
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.json -Action validate
/opt/ddb/scripts/ddb-cloud-control.sh -Config /opt/ddb/configs/control-plane.json -Action start
```

然后同一台机器继续启动本机承载的 bootstrap shard：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.json -Action validate
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-a.json -Action start-all
```

接着从控制面机器 SSH 到 `node-b`，执行：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.json -Action validate
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.json -Action start-all
```

最后 SSH 到 `node-c`，执行：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.json -Action validate
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-c.json -Action start-all
```

常用动作包括：

- `validate`
- `status`
- `start`
- `stop`
- `restart`
- `join`
- `rejoin`
- `remove`
- `remove-local`
- `start-all`
- `stop-all`
- `restart-all`
- `join-all`

语义建议这样理解：

- `start/stop`
  - 只负责进程启停
  - 适合已经在副本组里的节点做宕机/恢复演示
- `join`
  - 用于让一个新节点加入副本组
  - 默认依赖 `etcd` 自动发现当前 leader，不再写死 bootstrap 地址
  - 适合第一次拉起 follower，或新增 `n4/n5`
- `rejoin`
  - 用于一个已经 `remove` 过、但仍保留本地 `raft/db` 状态的旧节点重新回到副本组
  - 不需要先清理本地目录
- `remove`
  - 用于把节点从副本组成员关系中移除
  - 脚本内部会调用 `ddb-cli cluster remove`
  - 成功后会顺手停掉对应进程
- `remove-local`
  - 只清理本地 `raft_dir/db_path/log/pid`
  - 不修改副本组成员关系
  - 不清除 etcd 中的 removed 标记

如果只想单独操作某一个 shard，可以这样：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.json -Action restart -Name g1-n2
```

如果要把一个新节点加入某个副本组，可以这样：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.json -Action join -Name g1-n5
```

如果要把它永久移出副本组：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.json -Action remove -Name g1-n5
```

如果要让一个被 `remove` 的旧节点保留本地状态回来：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.json -Action rejoin -Name g1-n5
```

如果要清理本地状态：

```bash
/opt/ddb/scripts/ddb-cloud-node.sh -Config /opt/ddb/configs/node-b.json -Action remove-local -Name g1-n5
```

如果这个 `node_id` 已经执行过 `remove`，即使再执行 `remove-local`，后续也仍然应该使用 `rejoin`，而不是普通 `join`。

### 登录方式

推荐开 3 个 SSH 会话：

- 会话 1：控制面机器本机 shell
- 会话 2：从控制面机器 SSH 到 `node-b`
- 会话 3：从控制面机器 SSH 到 `node-c`

### 保活方式

不要直接前台运行。

建议至少使用：

```bash
nohup <command> >/opt/ddb/logs/<name>.log 2>&1 &
```

如果你更习惯，也可以用 `tmux`。

## 启动顺序

顺序建议固定，不要现场改：

1. 控制面机器启动 `etcd`
2. 控制面机器启动本机 bootstrap shard
3. 控制面机器启动 `apiserver`
4. `node-b`、`node-c` 上的 follower 通过 `join` 加入各自副本组

对应到脚本动作可以理解成：

1. `ddb-cloud-control.sh ... -Action start`
2. `node-a.json` 用 `start-all`
3. `node-b.json` 和 `node-c.json` 首次部署时用 `join-all`

后续如果只是模拟节点宕机恢复，就不要再用 `join`，而是直接 `start` 或 `restart`。

如果节点已经执行过 `remove`，则不要直接 `start`：

- 保留旧本地状态回来：用 `rejoin`
- 移出后即使清空本地状态，原 `node_id` 回来仍然用 `rejoin`

### 为什么 follower 放后面

因为 `g1-n2/g1-n3` 这类 follower 要先等各自副本组已经选出 leader。

当前三机预设里：

- follower 启动时只要配置了 `etcd`
- 并且对应副本组已经有可用 leader
- 节点就会通过 etcd 自动发现 leader 再发起 join

这样即使最初的 bootstrap 节点停了，只要该副本组仍然保持 quorum 并重新选主，新的 follower 仍然可以继续加入。

## `ddb-cli` 应该放哪

建议只放在控制面机器使用。

这样你可以在控制面机器上同时做两类操作：

- 连 `apiserver` 做普通交互
- 连具体 shard node 做 `inspect`

### 普通交互

例如：

```bash
/opt/ddb/bin/ddb-cli --node-url=http://127.0.0.1:18100 interact
```

### Inspect

`inspect` 不是借助 `apiserver` 转发，而是 CLI 直接连具体 node。

所以在控制面机器上可以这样做：

```bash
/opt/ddb/bin/ddb-cli --node-url=http://<node-b-internal-ip>:21180 inspect "SELECT * FROM users WHERE id = 201"
```

这也是为什么两台数据机不需要公网：只要控制面机器能通过内网打到这些节点即可。

## 现场最重要的检查项

在正式演示前，至少确认下面这些都成立：

- 三台机器之间内网互通
- 控制面机器可以 SSH 到 `node-b`、`node-c`
- `/opt/ddb/bin/ddb-server` 可执行
- `/opt/ddb/bin/ddb-cli` 可执行
- `etcd` 已起
- `apiserver` 已起
- 9 个 shard 节点都已起
- `ddb-cli --node-url=http://127.0.0.1:18100 interact` 能进入交互模式
- `control groups`
- `control shards`
- 对任意一个具体 node 的 `inspect` 成功

## 现场最容易踩坑的地方

- 二进制架构编错，例如上传了 macOS 二进制或 `arm64` 到 `amd64` 机器
- 配置里混用了公网 IP 和内网 IP
- follower 比 leader 更早启动，或者副本组尚未完成选主，导致 join 失败
- 只开了公网 `22`，但内网安全组没放通 shard/raft 端口
- `ddb-cli` 放在本地演示机上直接打 node，结果本地根本访问不到数据机内网
- 只用前台 shell 起进程，断线后进程一起没了

## 当前推荐方案总结

如果你现在要一个“稳、能讲清楚、且接近真实验收”的方案，建议就按下面收敛：

- 三台云服务器
- 全部业务流量走内网
- 只给控制面机器开公网 SSH
- 控制面机器承担 `etcd + apiserver + g1-n1/g2-n1/g3-n1`
- `node-b` 承担 `g1-n2/g2-n2/g3-n2`
- `node-c` 承担 `g1-n3/g2-n3/g3-n3`
- 本地提前编译 `linux/amd64` 二进制
- 先传控制面机器，再由控制面机器分发到另外两台
- 所有 `ddb-cli` 操作都在控制面机器执行

## 后续可以继续补什么

这份文档现在只是初稿，后续还可以继续补：

- 三台机器各自的精确启动命令
- 各台机器的配置文件模板
- 一份安全组规则清单
- 一份 `inspect` 演示命令清单
- 一份“演示前 5 分钟检查单”
