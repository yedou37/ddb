# Three-Windows ShardDB Demo Guide

这份文档记录了如何在三台 Windows 物理机上拉起一个 `3 group / 9 node` 的 `ShardDB` 集群。

目标是：

- `WIN_A` 运行 `etcd`、`apiserver`、`g1-n1`、`g2-n1`、`g3-n1`
- `WIN_B` 运行 `g1-n2`、`g2-n2`、`g3-n2`
- `WIN_C` 运行 `g1-n3`、`g2-n3`、`g3-n3`

推荐方式：

- `WIN_A` 上的 `etcd` 用 Docker Desktop 跑
- `WIN_A` 上的 `apiserver` 也用 Docker Desktop 跑
- 所有 `shard` 节点使用 Windows 原生进程跑

这样做的原因是：

- `etcd` 和 `apiserver` 单实例集中放在 `WIN_A`，用 Docker 管理更方便
- 三物理机场景下，每个 `group` 的三个副本刚好一台机器一个，最容易做容错演示
- 所有 `shard` 节点直接使用宿主机真实 IP 跑原生进程，跨机器联通最稳
- 当前项目里地址既承担监听也承担对外通告，跨物理机时不建议把 `shard` 放进 Docker 再做额外端口映射
- 如果 `WIN_A` 无法使用 Docker Desktop，也可以退回到 `etcd` 原生进程 + `apiserver` 原生进程模式，本文档也给出完整命令

## 1. 拓扑

请把下面示例地址替换成你自己的真实局域网 IP：

```text
WIN_A = 192.168.1.10
WIN_B = 192.168.1.11
WIN_C = 192.168.1.12
ETCD  = 192.168.1.10:2379
API   = 192.168.1.10:18100
```

节点分布如下：

```text
WIN_A
  etcd          2379
  apiserver     http 18100 / raft 30100
  g1-n1         http 21080 / raft 22080
  g2-n1         http 21081 / raft 22081
  g3-n1         http 21082 / raft 22082

WIN_B
  g1-n2         http 21180 / raft 22180
  g2-n2         http 21181 / raft 22181
  g3-n2         http 21182 / raft 22182

WIN_C
  g1-n3         http 21280 / raft 22280
  g2-n3         http 21281 / raft 22281
  g3-n3         http 21282 / raft 22282
```

这套布局适合做三机场景下的完整演示：

- 控制面统一从 `WIN_A` 进入
- 每个 `group` 横跨三台机器
- 任意停掉 `WIN_B` 或 `WIN_C` 整台机器，三个 `group` 仍然都保留多数派
- 便于演示建表、写入、查询、`rebalance`、节点故障与恢复

## 2. 准备阶段

### 2.1 安装 Go

三台 Windows 机器都需要安装 Go。

推荐使用 `winget` 安装：

```powershell
winget install -e --id GoLang.Go
```

如果 `winget` 不可用，也可以手动下载：

- 打开 [https://go.dev/dl/](https://go.dev/dl/)
- 下载 Windows 安装包，通常选择 `Windows amd64 MSI installer`
- 双击安装，保持默认选项即可

安装完成后，重新打开 PowerShell，验证：

```powershell
go version
```

预期能看到类似：

```text
go version go1.xx.x windows/amd64
```

### 2.2 安装 Docker Desktop

默认方案里，只有 `WIN_A` 必须安装 Docker Desktop。

如果你希望三台机器都方便排查容器和镜像，也可以三台都装，但不是必须。

推荐先确保 WSL2 可用：

```powershell
wsl --install
```

执行后如果系统提示重启，就先重启机器。

然后安装 Docker Desktop：

```powershell
winget install -e --id Docker.DockerDesktop
```

如果 `winget` 不可用，也可以手动下载：

- 打开 [https://docs.docker.com/desktop/setup/install/windows-install/](https://docs.docker.com/desktop/setup/install/windows-install/)
- 下载 `Docker Desktop Installer.exe`
- 双击安装
- 安装过程中保持默认选项即可
- 安装完成后启动 Docker Desktop，等待状态变成 Running

验证 Docker 是否可用：

```powershell
docker version
docker info
```

如果 `docker version` 能返回客户端和服务端信息，就说明 Docker Desktop 已经正常工作。

### 2.3 如果不能使用 Docker

如果 `WIN_A` 无法安装或运行 Docker Desktop，也可以继续做这套三机实验：

- `etcd` 改为 Windows 原生进程
- `apiserver` 改为 Windows 原生进程
- 所有 `shard` 节点仍然用 Windows 原生进程

这时你只需要：

- 三台机器都安装 Go
- `WIN_A` 额外下载 `etcd` 的 Windows 发行包

本文档后面会分别给出：

- `6.1 Docker 方式启动 etcd`
- `6.2 原生进程方式启动 etcd`
- `10.1 Docker 方式启动 apiserver`
- `10.2 原生进程方式启动 apiserver`

你只需要二选一，不要同时跑两套。

### 2.4 网络与端口要求

三台 Windows 机器都需要：

- 能互相 `ping`
- 目标端口可达
- 防火墙允许本文档里的端口

建议先检查端口规划：

```text
WIN_A:
  2379
  18100, 30100
  21080-21082
  22080-22082

WIN_B:
  21180-21182
  22180-22182

WIN_C:
  21280-21282
  22280-22282
```

## 3. 约定目录与变量

下面统一假设仓库在：

```text
C:\ddb
```

你可以换成自己的实际路径。

### 3.1 WIN_A

在 `WIN_A` PowerShell 中执行：

```powershell
$env:WIN_A_IP="192.168.1.10"
$env:WIN_B_IP="192.168.1.11"
$env:WIN_C_IP="192.168.1.12"
$env:ETCD_ADDR="$env:WIN_A_IP:2379"
$env:API_URL="http://$($env:WIN_A_IP):18100"
$env:DDB_ROOT="C:\ddb"
$env:DDB_DATA_ROOT="C:\ddb-demo"
```

### 3.2 WIN_B

在 `WIN_B` PowerShell 中执行：

```powershell
$env:WIN_A_IP="192.168.1.10"
$env:WIN_B_IP="192.168.1.11"
$env:WIN_C_IP="192.168.1.12"
$env:ETCD_ADDR="$env:WIN_A_IP:2379"
$env:API_URL="http://$($env:WIN_A_IP):18100"
$env:DDB_ROOT="C:\ddb"
$env:DDB_DATA_ROOT="C:\ddb-demo"
```

### 3.3 WIN_C

在 `WIN_C` PowerShell 中执行：

```powershell
$env:WIN_A_IP="192.168.1.10"
$env:WIN_B_IP="192.168.1.11"
$env:WIN_C_IP="192.168.1.12"
$env:ETCD_ADDR="$env:WIN_A_IP:2379"
$env:API_URL="http://$($env:WIN_A_IP):18100"
$env:DDB_ROOT="C:\ddb"
$env:DDB_DATA_ROOT="C:\ddb-demo"
```

### 3.4 连通性预检查

完成上面的环境变量设置后，再跑仓库自带的连通性检查。

在 `WIN_A` 上：

```powershell
cd $env:DDB_ROOT
go run .\cmd\netcheck --peer=$env:WIN_B_IP --ports=21180,21181,21182,22180,22181,22182
go run .\cmd\netcheck --peer=$env:WIN_C_IP --ports=21280,21281,21282,22280,22281,22282
```

在 `WIN_B` 上：

```powershell
cd $env:DDB_ROOT
go run .\cmd\netcheck --peer=$env:WIN_A_IP --ports=2379,18100,21080,21081,21082,22080,22081,22082
go run .\cmd\netcheck --peer=$env:WIN_C_IP --ports=21280,21281,21282,22280,22281,22282
```

在 `WIN_C` 上：

```powershell
cd $env:DDB_ROOT
go run .\cmd\netcheck --peer=$env:WIN_A_IP --ports=2379,18100,21080,21081,21082,22080,22081,22082
go run .\cmd\netcheck --peer=$env:WIN_B_IP --ports=21180,21181,21182,22180,22181,22182
```

如果这里不通，不要继续启动集群，先解决网络问题。

## 4. 编译二进制

三台机器都执行：

```powershell
cd $env:DDB_ROOT
go build -o .\bin\ddb-server.exe .\cmd\server
go build -o .\bin\ddb-cli.exe .\cmd\cli
```

## 5. 创建数据目录

### 5.1 WIN_A

```powershell
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\api-1" | Out-Null
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\etcd" | Out-Null
New-Item -ItemType Directory -Force "$env:DDB_ROOT\tools" | Out-Null
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\g1-n1" | Out-Null
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\g2-n1" | Out-Null
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\g3-n1" | Out-Null
```

### 5.2 WIN_B

```powershell
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\g1-n2" | Out-Null
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\g2-n2" | Out-Null
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\g3-n2" | Out-Null
```

### 5.3 WIN_C

```powershell
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\g1-n3" | Out-Null
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\g2-n3" | Out-Null
New-Item -ItemType Directory -Force "$env:DDB_DATA_ROOT\g3-n3" | Out-Null
```

## 6. WIN_A 启动 etcd

### 6.1 方式 A：使用 Docker Desktop 启动 etcd

在 `WIN_A` 上执行：

```powershell
docker rm -f ddb-etcd 2>$null
docker run -d `
  --name ddb-etcd `
  --restart unless-stopped `
  -p 2379:2379 `
  quay.io/coreos/etcd:v3.5.9 `
  etcd `
  --advertise-client-urls=http://$env:WIN_A_IP:2379 `
  --listen-client-urls=http://0.0.0.0:2379
```

检查：

```powershell
Invoke-RestMethod -Uri "http://$($env:WIN_A_IP):2379/health"
```

预期返回里应该有 `health`。

### 6.2 方式 B：使用原生进程启动 etcd

如果 `WIN_A` 上不能使用 Docker Desktop，就在 `WIN_A` 上执行下面的命令下载并解压 `etcd`：

```powershell
cd $env:DDB_ROOT
Invoke-WebRequest `
  -Uri "https://github.com/etcd-io/etcd/releases/download/v3.5.9/etcd-v3.5.9-windows-amd64.zip" `
  -OutFile "$env:DDB_ROOT\tools\etcd-v3.5.9-windows-amd64.zip"

Expand-Archive `
  -Path "$env:DDB_ROOT\tools\etcd-v3.5.9-windows-amd64.zip" `
  -DestinationPath "$env:DDB_ROOT\tools" `
  -Force
```

下载完成后，在一个单独的 PowerShell 窗口里启动 `etcd`：

```powershell
cd "$env:DDB_ROOT\tools\etcd-v3.5.9-windows-amd64"
.\etcd.exe `
  --name=ddb-etcd `
  --data-dir="$env:DDB_DATA_ROOT\etcd" `
  --advertise-client-urls=http://$env:WIN_A_IP:2379 `
  --listen-client-urls=http://0.0.0.0:2379
```

健康检查：

```powershell
Invoke-RestMethod -Uri "http://$($env:WIN_A_IP):2379/health"
```

说明：

- 这个 `etcd` PowerShell 窗口需要一直保持打开
- 如果你关闭窗口，`etcd` 就会退出
- 后面的所有 `shard` 和 `apiserver` 都依赖这个 `etcd`

## 7. WIN_A 启动 3 个种子 shard 节点

注意启动顺序：先起各个 group 的 `n1`，它们负责各自 group 的 bootstrap。

### 7.1 WIN_A Terminal 1: g1-n1

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g1-n1 `
  --group-id=g1 `
  --http-addr=$env:WIN_A_IP:21080 `
  --raft-addr=$env:WIN_A_IP:22080 `
  --raft-dir="$env:DDB_DATA_ROOT\g1-n1\raft" `
  --db-path="$env:DDB_DATA_ROOT\g1-n1\data.db" `
  --bootstrap=true `
  --etcd=$env:ETCD_ADDR
```

### 7.2 WIN_A Terminal 2: g2-n1

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g2-n1 `
  --group-id=g2 `
  --http-addr=$env:WIN_A_IP:21081 `
  --raft-addr=$env:WIN_A_IP:22081 `
  --raft-dir="$env:DDB_DATA_ROOT\g2-n1\raft" `
  --db-path="$env:DDB_DATA_ROOT\g2-n1\data.db" `
  --bootstrap=true `
  --etcd=$env:ETCD_ADDR
```

### 7.3 WIN_A Terminal 3: g3-n1

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g3-n1 `
  --group-id=g3 `
  --http-addr=$env:WIN_A_IP:21082 `
  --raft-addr=$env:WIN_A_IP:22082 `
  --raft-dir="$env:DDB_DATA_ROOT\g3-n1\raft" `
  --db-path="$env:DDB_DATA_ROOT\g3-n1\data.db" `
  --bootstrap=true `
  --etcd=$env:ETCD_ADDR
```

## 8. WIN_B 启动 3 个 shard 节点

### 8.1 WIN_B Terminal 1: g1-n2

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g1-n2 `
  --group-id=g1 `
  --http-addr=$env:WIN_B_IP:21180 `
  --raft-addr=$env:WIN_B_IP:22180 `
  --raft-dir="$env:DDB_DATA_ROOT\g1-n2\raft" `
  --db-path="$env:DDB_DATA_ROOT\g1-n2\data.db" `
  --bootstrap=false `
  --join=http://$($env:WIN_A_IP):21080 `
  --etcd=$env:ETCD_ADDR
```

### 8.2 WIN_B Terminal 2: g2-n2

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g2-n2 `
  --group-id=g2 `
  --http-addr=$env:WIN_B_IP:21181 `
  --raft-addr=$env:WIN_B_IP:22181 `
  --raft-dir="$env:DDB_DATA_ROOT\g2-n2\raft" `
  --db-path="$env:DDB_DATA_ROOT\g2-n2\data.db" `
  --bootstrap=false `
  --join=http://$($env:WIN_A_IP):21081 `
  --etcd=$env:ETCD_ADDR
```

### 8.3 WIN_B Terminal 3: g3-n2

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g3-n2 `
  --group-id=g3 `
  --http-addr=$env:WIN_B_IP:21182 `
  --raft-addr=$env:WIN_B_IP:22182 `
  --raft-dir="$env:DDB_DATA_ROOT\g3-n2\raft" `
  --db-path="$env:DDB_DATA_ROOT\g3-n2\data.db" `
  --bootstrap=false `
  --join=http://$($env:WIN_A_IP):21082 `
  --etcd=$env:ETCD_ADDR
```

## 9. WIN_C 启动 3 个 shard 节点

### 9.1 WIN_C Terminal 1: g1-n3

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g1-n3 `
  --group-id=g1 `
  --http-addr=$env:WIN_C_IP:21280 `
  --raft-addr=$env:WIN_C_IP:22280 `
  --raft-dir="$env:DDB_DATA_ROOT\g1-n3\raft" `
  --db-path="$env:DDB_DATA_ROOT\g1-n3\data.db" `
  --bootstrap=false `
  --join=http://$($env:WIN_A_IP):21080 `
  --etcd=$env:ETCD_ADDR
```

### 9.2 WIN_C Terminal 2: g2-n3

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g2-n3 `
  --group-id=g2 `
  --http-addr=$env:WIN_C_IP:21281 `
  --raft-addr=$env:WIN_C_IP:22281 `
  --raft-dir="$env:DDB_DATA_ROOT\g2-n3\raft" `
  --db-path="$env:DDB_DATA_ROOT\g2-n3\data.db" `
  --bootstrap=false `
  --join=http://$($env:WIN_A_IP):21081 `
  --etcd=$env:ETCD_ADDR
```

### 9.3 WIN_C Terminal 3: g3-n3

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g3-n3 `
  --group-id=g3 `
  --http-addr=$env:WIN_C_IP:21282 `
  --raft-addr=$env:WIN_C_IP:22282 `
  --raft-dir="$env:DDB_DATA_ROOT\g3-n3\raft" `
  --db-path="$env:DDB_DATA_ROOT\g3-n3\data.db" `
  --bootstrap=false `
  --join=http://$($env:WIN_A_IP):21082 `
  --etcd=$env:ETCD_ADDR
```

## 10. 最后在 WIN_A 启动 apiserver

### 10.1 方式 A：使用 Docker Desktop 启动 apiserver

这里把 `apiserver` 放到 Docker 中，便于统一管理和后续直接打开 dashboard。

在 `WIN_A` 上执行：

```powershell
docker rm -f api-1 2>$null
docker run -d `
  --name api-1 `
  --restart unless-stopped `
  -p 18100:18100 `
  -p 30100:30100 `
  -v "${env:DDB_DATA_ROOT}\api-1:/data" `
  ghcr.io/yedou37/ddb:latest `
  --role=apiserver `
  --node-id=api-1 `
  --group-id=control `
  --http-addr=$env:WIN_A_IP:18100 `
  --raft-addr=$env:WIN_A_IP:30100 `
  --raft-dir=/data/raft `
  --db-path=/data/apiserver.db `
  --bootstrap=false `
  --etcd=$env:ETCD_ADDR
```

健康检查：

```powershell
Invoke-WebRequest -UseBasicParsing -Uri "$env:API_URL/health"
```

Dashboard：

```text
http://WIN_A_IP:18100/dashboard/
```

### 10.2 方式 B：使用原生进程启动 apiserver

如果你没有使用 Docker Desktop，就在 `WIN_A` 上单独开一个 PowerShell 窗口执行：

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=apiserver `
  --node-id=api-1 `
  --group-id=control `
  --http-addr=$env:WIN_A_IP:18100 `
  --raft-addr=$env:WIN_A_IP:30100 `
  --raft-dir="$env:DDB_DATA_ROOT\api-1\raft" `
  --db-path="$env:DDB_DATA_ROOT\api-1\apiserver.db" `
  --bootstrap=false `
  --etcd=$env:ETCD_ADDR
```

健康检查：

```powershell
Invoke-WebRequest -UseBasicParsing -Uri "$env:API_URL/health"
```

Dashboard：

```text
http://WIN_A_IP:18100/dashboard/
```

说明：

- 这个 `apiserver` PowerShell 窗口也需要一直保持打开
- 如果你选择 `10.2`，就不要再执行 `10.1`

## 11. 基础验收命令

下面的命令建议都在 `WIN_A` 上执行。

先进入仓库目录：

```powershell
cd $env:DDB_ROOT
```

### 11.1 控制面检查

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL control config
.\bin\ddb-cli.exe --node-url=$env:API_URL control groups
.\bin\ddb-cli.exe --node-url=$env:API_URL control shards
```

预期：

- 能看到 `g1`、`g2`、`g3`
- `shards` 返回有分片分配信息

### 11.2 建表

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)"
```

### 11.3 写入几条基础数据

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "INSERT INTO users VALUES (1, 'user-001')"
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "INSERT INTO users VALUES (2, 'user-002')"
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "INSERT INTO users VALUES (3, 'user-003')"
```

### 11.4 查询

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "SELECT * FROM users WHERE id = 1"
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "SELECT * FROM users WHERE id = 2"
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "SELECT * FROM users WHERE id = 3"
```

## 12. 参考单机脚本的 seed 测试

如果你想像单机 Windows 脚本那样一次性插入多条数据，可以直接在 PowerShell 跑：

```powershell
cd $env:DDB_ROOT
1..40 | ForEach-Object {
  $id = $_
  $name = "user-{0:D3}" -f $id
  .\bin\ddb-cli.exe --node-url=$env:API_URL sql "INSERT INTO users VALUES ($id, '$name')"
}
```

然后抽样验证：

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "SELECT * FROM users WHERE id = 1"
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "SELECT * FROM users WHERE id = 20"
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "SELECT * FROM users WHERE id = 40"
```

## 13. ShardDB 测试命令

### 13.1 查看 groups 和 shards

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL control groups
.\bin\ddb-cli.exe --node-url=$env:API_URL control shards
```

### 13.2 执行 rebalance

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL control rebalance g1 g2 g3
```

再次观察：

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL control shards
```

### 13.3 rebalance 后再次读写

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "SELECT * FROM users WHERE id = 1"
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "SELECT * FROM users WHERE id = 20"
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "INSERT INTO users VALUES (101, 'user-101')"
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "SELECT * FROM users WHERE id = 101"
```

### 13.4 手动 move-shard

先查看当前 shard 编号：

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL control shards
```

选一个 shard，例如 `6`，移动到 `g3`：

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL control move-shard 6 g3
```

再查：

```powershell
.\bin\ddb-cli.exe --node-url=$env:API_URL control shards
```

## 14. 观测命令

### 14.1 看 etcd 容器日志

```powershell
docker logs -f ddb-etcd
```

### 14.2 看 apiserver 容器日志

```powershell
docker logs -f api-1
```

### 14.3 看 Windows shard 进程

在任意一台机器上：

```powershell
Get-Process ddb-server -ErrorAction SilentlyContinue
```

### 14.4 按端口看监听

在 `WIN_A` 上：

```powershell
netstat -ano | findstr ":18100"
netstat -ano | findstr ":21080"
netstat -ano | findstr ":22080"
```

在 `WIN_B` 上：

```powershell
netstat -ano | findstr ":21180"
netstat -ano | findstr ":22180"
```

在 `WIN_C` 上：

```powershell
netstat -ano | findstr ":21280"
netstat -ano | findstr ":22280"
```

## 15. 故障测试

### 15.1 停掉一整台 follower 机器

三机部署时，推荐优先停 `WIN_B` 或 `WIN_C`，不要先停 `WIN_A`。

例如先停 `WIN_B`：

```powershell
Get-Process ddb-server -ErrorAction SilentlyContinue | Stop-Process -Force
```

然后在 `WIN_A` 上执行：

```powershell
cd $env:DDB_ROOT
.\bin\ddb-cli.exe --node-url=$env:API_URL control groups
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "SELECT * FROM users WHERE id = 1"
.\bin\ddb-cli.exe --node-url=$env:API_URL sql "INSERT INTO users VALUES (102, 'user-102')"
```

预期：

- 集群仍然可读写
- 每个 `group` 仍然保留 `WIN_A + WIN_C` 两个副本，多数派仍在
- Dashboard 中对应节点会显示异常或不可达

### 15.2 恢复一整台 follower 机器

以恢复 `WIN_B` 为例，重新在 `WIN_B` 启动原命令即可。

#### WIN_B Terminal 1: g1-n2

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g1-n2 `
  --group-id=g1 `
  --http-addr=$env:WIN_B_IP:21180 `
  --raft-addr=$env:WIN_B_IP:22180 `
  --raft-dir="$env:DDB_DATA_ROOT\g1-n2\raft" `
  --db-path="$env:DDB_DATA_ROOT\g1-n2\data.db" `
  --bootstrap=false `
  --join=http://$($env:WIN_A_IP):21080 `
  --etcd=$env:ETCD_ADDR
```

#### WIN_B Terminal 2: g2-n2

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g2-n2 `
  --group-id=g2 `
  --http-addr=$env:WIN_B_IP:21181 `
  --raft-addr=$env:WIN_B_IP:22181 `
  --raft-dir="$env:DDB_DATA_ROOT\g2-n2\raft" `
  --db-path="$env:DDB_DATA_ROOT\g2-n2\data.db" `
  --bootstrap=false `
  --join=http://$($env:WIN_A_IP):21081 `
  --etcd=$env:ETCD_ADDR
```

#### WIN_B Terminal 3: g3-n2

```powershell
cd $env:DDB_ROOT
.\bin\ddb-server.exe `
  --role=shard `
  --node-id=g3-n2 `
  --group-id=g3 `
  --http-addr=$env:WIN_B_IP:21182 `
  --raft-addr=$env:WIN_B_IP:22182 `
  --raft-dir="$env:DDB_DATA_ROOT\g3-n2\raft" `
  --db-path="$env:DDB_DATA_ROOT\g3-n2\data.db" `
  --bootstrap=false `
  --join=http://$($env:WIN_A_IP):21082 `
  --etcd=$env:ETCD_ADDR
```

如果只是普通宕机重启，并且数据目录还在，通常直接按原参数启动即可。

### 15.3 为什么不建议先停 WIN_A

因为在这套推荐拓扑里：

- `WIN_A` 上还有 `etcd`
- `WIN_A` 上还有统一入口 `apiserver`
- `WIN_A` 上还有三个 `group` 的 `n1`

所以如果直接停掉 `WIN_A`：

- 各个 `group` 本身通常仍有 `WIN_B + WIN_C` 两个副本，数据面多数派还在
- 但 `apiserver` 和 `etcd` 都会受影响
- 演示时统一 SQL 入口和控制命令入口会不可用

因此课堂或答辩演示里，建议先停 `WIN_B` 或 `WIN_C` 展示副本组容错。

## 16. 清理环境

### 16.1 停止 WIN_C 上所有 shard 进程

```powershell
Get-Process ddb-server -ErrorAction SilentlyContinue | Stop-Process -Force
```

### 16.2 停止 WIN_B 上所有 shard 进程

```powershell
Get-Process ddb-server -ErrorAction SilentlyContinue | Stop-Process -Force
```

### 16.3 停止 WIN_A 上原生进程

如果你在 `WIN_A` 上使用了原生进程方式启动 `apiserver` 或 `etcd`，直接在对应 PowerShell 窗口里 `Ctrl+C` 即可。

如果你也想一次性结束 `WIN_A` 上的原生 `ddb-server` 进程，可以执行：

```powershell
Get-Process ddb-server -ErrorAction SilentlyContinue | Stop-Process -Force
```

### 16.4 删除 Docker 容器

如果 `WIN_A` 使用了 Docker 方式启动 `etcd` / `apiserver`，执行：

```powershell
docker rm -f api-1 ddb-etcd
```

### 16.5 删除数据目录

三台机器都可以分别执行：

```powershell
Remove-Item -Recurse -Force $env:DDB_DATA_ROOT
```

## 17. 排障要点

- 跨物理机场景里，所有 `--http-addr`、`--raft-addr`、`--etcd` 都必须写真实物理机 IP，不要写 `127.0.0.1`
- `WIN_B` 和 `WIN_C` 节点虽然通过 `WIN_A` 的种子节点加入，但加入后组内副本彼此也要能访问对方的 `raft` 地址
- 启动顺序必须是：`etcd -> shard nodes -> apiserver`
- 如果 `WIN_B` 节点无法加入，先检查：
  - `Test-NetConnection $env:WIN_A_IP -Port 2379`
  - `Test-NetConnection $env:WIN_A_IP -Port 21080`
  - `Test-NetConnection $env:WIN_A_IP -Port 21081`
  - `Test-NetConnection $env:WIN_A_IP -Port 21082`
  - `Test-NetConnection $env:WIN_C_IP -Port 22280`
  - `Test-NetConnection $env:WIN_C_IP -Port 22281`
  - `Test-NetConnection $env:WIN_C_IP -Port 22282`
- 如果 `WIN_C` 节点无法加入，先检查：
  - `Test-NetConnection $env:WIN_A_IP -Port 2379`
  - `Test-NetConnection $env:WIN_A_IP -Port 21080`
  - `Test-NetConnection $env:WIN_A_IP -Port 21081`
  - `Test-NetConnection $env:WIN_A_IP -Port 21082`
  - `Test-NetConnection $env:WIN_B_IP -Port 22180`
  - `Test-NetConnection $env:WIN_B_IP -Port 22181`
  - `Test-NetConnection $env:WIN_B_IP -Port 22182`
- 如果 `apiserver` 起不来，先确认 `etcd` 健康且三个 group 的种子节点都已经起来
- 如果 Dashboard 页面没更新，优先查 `api-1` 容器日志

## 18. 建议的实际演示顺序

- 先做 `netcheck`
- 启动 `etcd`
- 启动 `g1-n1/g2-n1/g3-n1`
- 启动 `WIN_B` 上 3 个副本
- 启动 `WIN_C` 上 3 个副本
- 启动 `apiserver`
- 执行 `control groups` / `control shards`
- 建表并插入 40 条 `users`
- 执行一次 `rebalance`
- 再做查询与写入验证
- 停一个整机 follower，例如 `WIN_B`，验证仍可写
- 重启该机器上的 3 个节点，验证恢复
