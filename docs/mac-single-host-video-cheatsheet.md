# macOS 单机录屏 Cheat Sheet

## 适用场景

- 一台 macOS 机器
- 仓库路径不固定，只要能写对 `project_root`
- 已安装 Go
- 已安装本地 `etcd`
- 需要录一个“分别启动控制平面与节点，再通过 CLI 交互展示功能”的视频

本方案使用现在的 macOS 配置驱动脚本：

- `scripts/ddb-mac-control.sh`
- `scripts/ddb-mac.sh`

## 演示前先编译

建议在正式测试或录屏前先编译出这两个程序：

```bash
go build -o ./bin/ddb-server ./cmd/server
go build -o ./bin/ddb-cli ./cmd/cli
```

说明：

- `ddb-mac-control.sh` / `ddb-mac.sh` 在缺少 `ddb-server` 时会尝试自动编译
- 但提前编译更稳，能避免第一次启动时再临时 build
- `ddb-cli` 不会被脚本自动编译，演示 `interact` / `inspect` 前最好先手工编译好

## 环境检测命令

建议在正式测试前先单独跑一遍这些检查：

```bash
go version
python3 --version
curl --version | head -n 1
/opt/homebrew/bin/etcd --version
test -x ./bin/ddb-server && echo "ddb-server ok"
test -x ./bin/ddb-cli && echo "ddb-cli ok"
./scripts/ddb-mac-control.sh -Action validate
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action validate
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-b.local.json -Action validate
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-c.local.json -Action validate
```

预期：

- `go`、`python3`、`curl`、`etcd` 都能正常输出版本
- `ddb-server` 和 `ddb-cli` 的存在性检查都能输出 `ok`
- `validate` 不报错，并打印出配置摘要和节点地址

核心思路：

- 先准备 1 份控制平面配置
- 再准备 3 份 shard 配置
- 控制平面通过一个脚本启动 `etcd + apiserver`
- shard 节点通过 3 次脚本启动
- 最后用 `ddb-cli interact` 连 `apiserver` 演示功能

## 需要的 4 个配置文件

### 1. 控制平面配置

推荐直接编辑：

- `configs/macos/control-plane.local.json`

一般只需要改：

- `project_root`
- `local_ip`

单机演示时推荐：

```json
"local_ip": "127.0.0.1"
```

### 2. 三份 shard 配置

推荐直接编辑这 3 份本地单机配置：

- `configs/macos/three-machine/mac-a.local.json`
- `configs/macos/three-machine/mac-b.local.json`
- `configs/macos/three-machine/mac-c.local.json`

单机演示时建议你直接改这几项：

- 三份配置都把 `project_root` 改成你的仓库目录
- 三份配置都把 `local_ip` 改成 `127.0.0.1`
- 三份配置都把 `etcd_host` 改成 `127.0.0.1`
- `mac-b.local.json` / `mac-c.local.json` 里的 `default_join_host` 也改成 `127.0.0.1`

其余端口、节点名、`join_port`、`group_id` 一般不用动。

注意：

- 这份单机文档默认你使用 `*.local.json`
- `three-machine/*.sample.json` 保留给样例配置；在本机单机演示时，优先改 `*.local.json`

## 容易踩坑

- `ddb-mac-control.sh` 默认读取 `configs/macos/control-plane.local.json`
- `ddb-mac.sh` 默认读取 `configs/macos/local.json`，但这份单机方案统一显式传 `mac-a.local.json` / `mac-b.local.json` / `mac-c.local.json`，不要混用默认 `local.json`
- 4 份 `local` 配置里的 `project_root` 都要改；如果你现在就在这台机器上测试，也可以直接使用我已经填好的本地路径
- 如果你的 mac 不能用 Docker，就把控制平面配置里的 `etcd.runner` 设成 `native`，并确保 `binary_path` 指向本地 `etcd`
- 必须先启动控制平面，再启动 3 份 shard 配置
- 如果你之前跑过 demo，旧的 `.ddb-data` / `.ddb-state` / `.ddb-logs` 可能会影响这次测试，建议先清理
- 如果节点启动失败，优先看 `.ddb-logs/*.log`

本仓库当前这台 mac 已经按本地 `etcd` 配好了：

- `configs/macos/control-plane.local.json`
- `etcd.runner = native`
- `etcd.binary_path = /opt/homebrew/bin/etcd`

## 演示前准备

建议打开 3 个窗口：

1. 终端窗口 A：启动控制平面
2. 终端窗口 B：启动 shard 节点
3. 浏览器窗口：展示 dashboard

如果你想更稳一点，录屏前先做一次清理：

```bash
pkill -f '/bin/ddb-server' 2>/dev/null || true
pkill -f 'bin/ddb-server' 2>/dev/null || true
pkill -x etcd 2>/dev/null || true
rm -rf ./.ddb-data ./.ddb-state ./.ddb-logs
```

## 最稳主流程

### 1. 进入仓库

```bash
cd /Users/yourname/ddb
```

进入仓库后，先确认下面 4 个文件里的 `project_root` 都已经改成当前仓库路径：

- `configs/macos/control-plane.local.json`
- `configs/macos/three-machine/mac-a.local.json`
- `configs/macos/three-machine/mac-b.local.json`
- `configs/macos/three-machine/mac-c.local.json`

如果你想先做一遍控制平面前置检查，可以先执行：

```bash
/opt/homebrew/bin/etcd --version
```

### 2. 启动控制平面

```bash
./scripts/ddb-mac-control.sh -Action validate
./scripts/ddb-mac-control.sh -Action start
```

预期：

- `etcd` 和 `apiserver` 都显示为运行中
- 输出 dashboard 地址

### 3. 启动三个 shard 配置

按顺序执行：

```bash
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action start-all
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-b.local.json -Action start-all
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-c.local.json -Action start-all
```

如果你想强调“分别启动节点”的过程，也可以改成逐个节点启动。

例如：

```bash
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action start -Name g1-n1
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action start -Name g2-n1
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action start -Name g3-n1
```

### 4. 打开 dashboard

```bash
open http://127.0.0.1:18100/dashboard/
```

### 5. 健康检查

```bash
./scripts/ddb-mac-control.sh -Action status
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action status
curl -fsS http://127.0.0.1:18100/health
```

### 6. 进入 CLI 交互模式

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:18100 interact
```

进入后，你可以连续输入命令。

## 交互模式演示脚本

进入 `interact` 后，推荐按这个顺序演示：

```text
control groups
control shards
sql CREATE TABLE users (id INT PRIMARY KEY, name TEXT)
sql INSERT INTO users VALUES (1, 'user-001')
sql INSERT INTO users VALUES (20, 'user-020')
sql SELECT * FROM users WHERE id = 1
sql SELECT * FROM users WHERE id = 20
sql INSERT INTO users VALUES (101, 'video-user-101')
sql SELECT * FROM users WHERE id = 101
sql SELECT * FROM users ORDER BY id DESC LIMIT 2
```

讲解词建议：

- 系统当前有 3 个 shard group
- `apiserver` 是统一入口
- `cli` 只连一个地址，后端自动路由
- 写请求会根据主键自动落到目标 shard group
- 读取结果再由统一入口返回
- `ORDER BY` 和 `LIMIT` 已经支持，可以顺手展示 SQL 子集能力

## Inspect 演示

这一段专门用来展示：

1. 先通过统一入口插入一条数据
2. 再分别直连 9 个节点检查本地数据
3. 最终看到 3 个副本有且一致，另外 6 个节点没有

推荐做法是：

- 保持当前这个 `interact` 窗口继续连 `apiserver`
- 再额外开一个终端窗口，专门执行 `inspect`

### 1. 在交互模式里插入一条数据

例如继续执行：

```text
sql INSERT INTO users VALUES (201, 'inspect-user-201')
sql SELECT * FROM users WHERE id = 201
```

### 2. 在另一个窗口逐个 inspect 9 个节点

下面这些命令都是“直连具体节点本地检查”，不会再走 `apiserver` 自动路由。

```bash
./bin/ddb-cli --node-url=http://127.0.0.1:21080 inspect "SELECT * FROM users WHERE id = 201"
./bin/ddb-cli --node-url=http://127.0.0.1:21180 inspect "SELECT * FROM users WHERE id = 201"
./bin/ddb-cli --node-url=http://127.0.0.1:21280 inspect "SELECT * FROM users WHERE id = 201"

./bin/ddb-cli --node-url=http://127.0.0.1:21081 inspect "SELECT * FROM users WHERE id = 201"
./bin/ddb-cli --node-url=http://127.0.0.1:21181 inspect "SELECT * FROM users WHERE id = 201"
./bin/ddb-cli --node-url=http://127.0.0.1:21281 inspect "SELECT * FROM users WHERE id = 201"

./bin/ddb-cli --node-url=http://127.0.0.1:21082 inspect "SELECT * FROM users WHERE id = 201"
./bin/ddb-cli --node-url=http://127.0.0.1:21182 inspect "SELECT * FROM users WHERE id = 201"
./bin/ddb-cli --node-url=http://127.0.0.1:21282 inspect "SELECT * FROM users WHERE id = 201"
```

### 3. 讲解词建议

- 这一步不再请求统一入口，而是直接请求具体 shard 节点
- `inspect` 只看该节点本地存储里的数据
- 因为一条记录只属于一个 shard group，所以只有对应 group 的 3 个副本会有这条记录
- 另外 6 个节点不会有这条记录
- 这也说明系统不是“全量复制到所有节点”，而是“分片 + 组内复制”

退出交互模式：

```text
exit
```

## Dashboard 展示点

建议在页面上依次展示：

1. `Cluster Topology`
2. `Group Health`
3. `Shard Map`
4. `Table Browser`

在 `Table Browser` 中：

1. 选择 `users`
2. 点击查询
3. 按 `id` 排序
4. 展示刚才插入的 `101`

## 可选扩展流程 A：展示分片迁移

如果你时间够，可以继续演示控制平面能力。

进入交互模式后继续执行：

```text
control move-shard 6 g3
control shards
sql SELECT * FROM users WHERE id = 1
sql INSERT INTO users VALUES (102, 'after-move-102')
sql SELECT * FROM users WHERE id = 102
```

## 可选扩展流程 B：展示副本故障不影响整体服务

如果你想录“容错容灾”效果，可以额外做这一段。

先停一个节点：

```bash
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action stop -Name g1-n1
```

然后继续在交互模式里执行：

```text
control groups
sql SELECT * FROM users WHERE id = 1
sql INSERT INTO users VALUES (103, 'after-failure-103')
sql SELECT * FROM users WHERE id = 103
```

恢复节点：

```bash
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action start -Name g1-n1
```

## 一次性复制版

```bash
cd /Users/yourname/ddb
./scripts/ddb-mac-control.sh -Action validate
./scripts/ddb-mac-control.sh -Action start
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action start-all
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-b.local.json -Action start-all
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-c.local.json -Action start-all
open http://127.0.0.1:18100/dashboard/
./bin/ddb-cli --node-url=http://127.0.0.1:18100 interact
```

## 收尾清理

```bash
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-c.local.json -Action stop-all
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-b.local.json -Action stop-all
./scripts/ddb-mac.sh -Config ./configs/macos/three-machine/mac-a.local.json -Action stop-all
./scripts/ddb-mac-control.sh -Action stop
```
