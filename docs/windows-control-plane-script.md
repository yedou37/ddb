# Windows Control Plane Script

这份说明配合 `scripts/ddb-win-control.ps1` 使用，用于在 `WIN_A` 上统一启动：

- `etcd`
- `apiserver`

然后你再分别在三台机器上用 `scripts/ddb-win.ps1` 启动各自的 `shard` 节点。

## 1. 准备配置

先复制样例配置：

```powershell
copy .\configs\windows\three-machine\control-plane.sample.json .\configs\windows\control-plane.local.json
```

演示时一般只需要改：

- `project_root`
- `local_ip`

如果你要换端口或不用 Docker，再改 `etcd` / `apiserver` 子段。

## 2. 推荐工作流

### 2.1 WIN_A 启动控制平面

```powershell
.\scripts\ddb-win-control.ps1 -Action validate
.\scripts\ddb-win-control.ps1 -Action start
```

查看状态：

```powershell
.\scripts\ddb-win-control.ps1 -Action status
```

停止：

```powershell
.\scripts\ddb-win-control.ps1 -Action stop
```

重启：

```powershell
.\scripts\ddb-win-control.ps1 -Action restart
```

默认配置路径是：

```text
configs/windows/control-plane.local.json
```

如果你想显式指定配置文件：

```powershell
.\scripts\ddb-win-control.ps1 -Config .\configs\windows\three-machine\control-plane.sample.json -Action start
```

### 2.2 三台机器分别启动 shard 节点

假设三台机器都已经把各自样例复制成：

```text
configs/windows/local.json
```

那么分别执行：

```powershell
.\scripts\ddb-win.ps1 -Action start-all
```

如果你想演示“分别启动”的过程，也可以按节点名逐个启动：

```powershell
.\scripts\ddb-win.ps1 -Action start -Name g1-n1
.\scripts\ddb-win.ps1 -Action start -Name g2-n1
.\scripts\ddb-win.ps1 -Action start -Name g3-n1
```

或者显式指定配置：

```powershell
.\scripts\ddb-win.ps1 -Config .\configs\windows\three-machine\win-a.sample.json -Action start-all
```

## 3. 控制平面配置说明

样例文件：

- `configs/windows/three-machine/control-plane.sample.json`

关键字段：

- `project_root`: 仓库根目录
- `local_ip`: `WIN_A` 的实际 IP
- `etcd.runner`: `docker` 或 `native`
- `etcd.port`: 默认 `2379`
- `apiserver.http_port`: 默认 `18100`
- `apiserver.raft_port`: 默认 `30100`

默认行为：

- `etcd` 默认使用 Docker
- `apiserver` 默认使用本地 `ddb-server.exe`
- `apiserver` 的数据目录会自动落到 `project_root\.ddb-data\api-1`

如果 `etcd.runner = native`，你还需要准备：

```text
tools\etcd-v3.5.9-windows-amd64\etcd.exe
```
