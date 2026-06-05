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

## 如何启动程序

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
2. 控制面机器启动 `g1-n1`、`g2-n1`、`g3-n1`
3. 控制面机器启动 `apiserver`
4. `node-b` 启动 `g1-n2`、`g2-n2`、`g3-n2`
5. `node-c` 启动 `g1-n3`、`g2-n3`、`g3-n3`

### 为什么 follower 放后面

因为 `g1-n2/g1-n3` 这类 follower 要 join 已经存在的 bootstrap leader。

当前三机预设里：

- `g1-n2`、`g1-n3` 需要 join `g1-n1`
- `g2-n2`、`g2-n3` 需要 join `g2-n1`
- `g3-n2`、`g3-n3` 需要 join `g3-n1`

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
- follower 比 leader 更早启动，导致 join 失败
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
