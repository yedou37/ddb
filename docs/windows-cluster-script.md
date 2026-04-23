# Windows Cluster Script Guide

这份文档说明如何在 Windows 物理机上，用一个配置文件和一个 PowerShell 脚本管理 `ShardDB` 演示节点。

这里默认只管理 `shard` 节点。

- `etcd` 你单独启动
- `apiserver` 你单独启动
- 这个脚本只负责各台机器上的 `g1/g2/g3` 节点

目标是解决这些线下操作问题：

- 不想每次新开 PowerShell 都重新设置环境变量
- 不想每次手敲很长的 `ddb-server.exe` 参数
- 需要把节点放到后台跑
- 杀掉进程模拟故障后，希望能用一条命令重新拉起
- 重启前希望自动清理旧进程残留、端口占用和 `.db` 文件占用

## 1. 方案结构

仓库里新增了：

- 主脚本：`scripts/manage-windows-cluster.ps1`
- 短命令包装脚本：`scripts/ddb-win.ps1`
- 三机样例配置：
  - `configs/windows/three-machine/win-a.sample.json`
  - `configs/windows/three-machine/win-b.sample.json`
  - `configs/windows/three-machine/win-c.sample.json`

脚本会做这些事情：

- 读取你指定的 JSON 配置文件
- 按配置把 `ddb-server.exe` 作为后台进程启动
- 记录每个目标的 `pid` 或容器名
- 支持查看状态、停止、重启、整机启动、整机停止
- 支持打开“每节点一个 PowerShell 窗口”的终端模式
- 在 `start` / `restart` 前自动清理目标节点的旧残留：
  - 清掉占用该节点 `http/raft` 端口的旧进程
  - 等待旧进程真正退出
  - 等待 `.db` 文件释放

## 2. 推荐使用方式

三台机器分别维护自己的配置文件。

例如：

- `WIN_A` 使用 `win-a.json`
- `WIN_B` 使用 `win-b.json`
- `WIN_C` 使用 `win-c.json`

为了做到“每次只打一条命令”，推荐每台机器都把自己的配置文件放成同一个固定路径：

- `configs/windows/local.json`

也就是说：

- `WIN_A` 把 `win-a.sample.json` 复制成 `local.json`
- `WIN_B` 把 `win-b.sample.json` 复制成 `local.json`
- `WIN_C` 把 `win-c.sample.json` 复制成 `local.json`

建议你们先从样例复制一份，再改成自己的真实路径和 IP：

```powershell
copy .\configs\windows\three-machine\win-a.sample.json .\configs\windows\local.json
```

在 `WIN_B` 上：

```powershell
copy .\configs\windows\three-machine\win-b.sample.json .\configs\windows\local.json
```

在 `WIN_C` 上：

```powershell
copy .\configs\windows\three-machine\win-c.sample.json .\configs\windows\local.json
```

然后分别修改：

- `project_root`
- 各机器真实 IP
- `raft_dir`
- `db_path`
- Docker target 的 volume 路径
- `etcd` 地址
- `join_addr`

## 3. 基本命令

下面命令都假设你已经在项目根目录，例如 `C:\ddb`。

### 3.1 查看当前机器有哪些目标

```powershell
.\scripts\ddb-win.ps1 -Action list
```

### 3.2 查看当前状态

```powershell
.\scripts\ddb-win.ps1 -Action status
```

### 3.3 启动当前机器上的所有目标

```powershell
.\scripts\ddb-win.ps1 -Action start-all
```

### 3.4 以“每节点一个终端窗口”的方式启动当前机器全部节点

这个模式更适合你们现场演示。

例如当前机器有 3 个 shard 节点，那么会自动打开 3 个 PowerShell 窗口，每个窗口绑定一个节点。

```powershell
.\scripts\ddb-win.ps1 -Action start-all-terminals
```

每个窗口会：

- 自动启动对应节点
- 保留这个窗口不关闭
- 节点以前台进程方式运行在这个窗口里
- 节点运行时，这个窗口会被节点占用并持续输出日志
- 节点退出后，这个窗口会回到命令提示

### 3.5 打开某个节点的专属终端窗口

```powershell
.\scripts\ddb-win.ps1 -Action open-terminal -Name g1-n2
```

### 3.6 只启动一个节点

```powershell
.\scripts\ddb-win.ps1 -Action start -Name g1-n2
```

### 3.7 停掉一个节点模拟故障

```powershell
.\scripts\ddb-win.ps1 -Action stop -Name g1-n2
```

### 3.8 重启一个节点

```powershell
.\scripts\ddb-win.ps1 -Action restart -Name g1-n2
```

### 3.9 查看某个节点最近日志

```powershell
.\scripts\ddb-win.ps1 -Action tail-log -Name g1-n2
```

### 3.10 停掉当前机器上的全部目标

```powershell
.\scripts\ddb-win.ps1 -Action stop-all
```

### 3.11 重启当前机器上的全部目标

```powershell
.\scripts\ddb-win.ps1 -Action restart-all
```

## 4. 推荐演示流程

先确保：

- `etcd` 已经是健康状态
- `apiserver` 已经启动
- `WIN_A`、`WIN_B`、`WIN_C` 间网络已通

### 4.1 第一次启动

按机器执行：

1. `WIN_A` 运行 `start-all-terminals`
2. `WIN_B` 运行 `start-all-terminals`
3. `WIN_C` 运行 `start-all-terminals`

这样你不需要手动开多个 PowerShell 窗口，而且每个节点都有自己固定的终端。

### 4.2 模拟故障

进入 `g2-n2` 对应的那个终端窗口。

当节点正在前台运行时，这个窗口会持续输出日志，此时没有命令提示符可输入。

要模拟故障，就在这个窗口里直接按：

```text
Ctrl+C
```

### 4.3 故障恢复

节点停掉以后，这个窗口会回到命令提示。

恢复时，在同一个终端窗口里输入：

```text
restart
```

如果你只是想重新启动而不先 stop，也可以输入：

```text
start
```

## 5. 日志和状态文件

脚本会自动写这些信息：

- 日志目录：`project_root\.ddb-logs\<machine-name>\`
- 状态目录：`project_root\.ddb-state\`

例如：

- `C:\ddb\.ddb-logs\win-b\g1-n2.log`
- `C:\ddb\.ddb-state\win-b.json`

这样即使你关掉当前 PowerShell，脚本下次仍然知道它之前启动过哪些进程。

## 6. 节点终端里能做什么

当你用 `start-all-terminals` 或 `open-terminal` 打开节点窗口后，每个窗口里可以直接输入：

```text
status
start
stop
kill
restart
tail
follow
clear
exit
```

说明：

- `start`: 在当前窗口以前台方式运行该节点，运行时窗口会被占用并持续输出日志
- `restart`: 和 `start` 类似，适合节点已经停掉后在同一窗口重新拉起
- `stop`: 只在当前有命令提示时可用，用来清理由脚本记录的残留进程
- `kill`: 和 `stop` 一样，专门给演示时按“杀掉节点”的说法准备
- `tail`: 看最近日志
- `follow`: 持续跟随日志输出，按 `Ctrl+C` 停止跟随并回到命令提示
- `exit`: 关闭这个节点终端窗口

## 7. 配置文件字段说明

顶层字段：

- `machine_name`: 当前机器名字，用于状态文件命名
- `project_root`: 仓库根目录
- `log_dir`: 日志目录，可写相对路径
- `state_dir`: 状态目录，可写相对路径
- `build_server_binary`: 如果本地没有 `bin\ddb-server.exe`，是否自动 `go build`
- `targets`: 当前机器上要管理的目标列表

`ddb-process` 类型目标常用字段：

- `name`
- `runner`: 固定写 `ddb-process`
- `node_id`
- `role`
- `group_id`
- `http_addr`
- `raft_addr`
- `raft_dir`
- `db_path`
- `bootstrap`
- `join_addr`
- `etcd`
- `health_url`

`docker` 类型目标常用字段：

- `name`
- `runner`: 固定写 `docker`
- `container_name`
- `image`
- `ports`
- `volumes`
- `command`
- `health_url`

## 8. 当前限制

- `stop` 主要依赖脚本记录的 `pid`，所以建议统一通过这个脚本启动和停止
- `start` / `restart` 会主动清理目标节点对应端口上的旧进程，因此这些端口最好专门留给对应 shard 节点使用
- 如果你手工在同一端口上跑了别的程序，脚本可能会把它当作残留进程清掉
- 当前样例默认只管理 shard 节点，不管理 `etcd` 和 `apiserver`

## 9. 最实用的一种落地方式

现在仓库里已经自带了这个包装脚本：`scripts/ddb-win.ps1`。

只要当前机器已经准备好 `configs/windows/local.json`，以后你只需要：

```text
Ctrl+C
restart
tail
kill
```

更准确地说，这就是你想要的这种方式：

- 每个节点一个固定终端
- 节点启动后前台占用这个终端并持续刷日志
- 你在这个终端按 `Ctrl+C` 模拟故障
- 节点退出后，还是这个终端，继续输入 `restart` 重新拉起
