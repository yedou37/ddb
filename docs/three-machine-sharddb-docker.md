# Three-Machine ShardDB Docker Deployment Guide

这份文档记录了在三台物理机上使用同一个 `ddb` 镜像启动 `ShardDB` 的推荐方式。

目标拓扑：

- `MACHINE_1`: `etcd` + `controller` + `apiserver` + `g1-n1` + `g2-n1` + `g3-n1`
- `MACHINE_2`: `g1-n2` + `g2-n2` + `g3-n2`
- `MACHINE_3`: `g1-n3` + `g2-n3` + `g3-n3`

并且：

- 每个 group 的 `n1` 在机器 1
- 每个 group 的 `n2` 在机器 2
- 每个 group 的 `n3` 在机器 3
- dashboard 通过机器 1 上的 `apiserver` 提供

## 1. 机器与地址规划

示例地址如下，请按你的真实局域网 IP 替换：

```text
MACHINE_1 = 192.168.1.10
MACHINE_2 = 192.168.1.11
MACHINE_3 = 192.168.1.12
ETCD      = 192.168.1.10:2379
API       = 192.168.1.10:18100
```

## 2. 前提条件

每台机器都需要：

- 安装 Docker 或 Docker Desktop
- 能访问镜像仓库，或提前 `docker load` 导入镜像
- 三台机器之间网络互通
- 对应端口未被防火墙拦截

建议先检查：

```bash
ping 192.168.1.10
ping 192.168.1.11
ping 192.168.1.12
```

## 3. 镜像

这里统一使用你的镜像地址：

```text
ghcr.io/yedou37/ddb:latest
```

下面所有命令都直接使用这个镜像名。

## 4. 端口规划

### MACHINE_1

```text
etcd          2379
controller    http 18080 / raft 30080
apiserver     http 18100 / raft 30100
g1-n1         http 21080 / raft 22080
g2-n1         http 21081 / raft 22081
g3-n1         http 21082 / raft 22082
```

### MACHINE_2

```text
g1-n2         http 21180 / raft 22180
g2-n2         http 21181 / raft 22181
g3-n2         http 21182 / raft 22182
```

### MACHINE_3

```text
g1-n3         http 21280 / raft 22280
g2-n3         http 21281 / raft 22281
g3-n3         http 21282 / raft 22282
```

## 5. 清理旧容器

如果之前跑过旧环境，先在三台机器分别执行：

```bash
docker rm -f ddb-etcd ctrl-1 api-1 g1-n1 g2-n1 g3-n1 g1-n2 g2-n2 g3-n2 g1-n3 g2-n3 g3-n3 2>/dev/null || true
```

## 6. MACHINE_1 启动 etcd

在 `MACHINE_1` 上执行：

```bash
docker rm -f ddb-etcd 2>/dev/null || true
docker run -d \
  --name ddb-etcd \
  --restart unless-stopped \
  -p 2379:2379 \
  quay.io/coreos/etcd:v3.5.9 \
  etcd \
  --advertise-client-urls=http://192.168.1.10:2379 \
  --listen-client-urls=http://0.0.0.0:2379
```

验证：

```bash
curl http://192.168.1.10:2379/health
```

## 7. MACHINE_1 启动 controller 与 apiserver

### 7.1 controller

```bash
docker run -d \
  --name ctrl-1 \
  --restart unless-stopped \
  -p 18080:18080 \
  -p 30080:30080 \
  -v /data/ddb/ctrl-1:/data \
  -e ROLE=controller \
  -e NODE_ID=ctrl-1 \
  -e GROUP_ID=control \
  -e HTTP_ADDR=192.168.1.10:18080 \
  -e RAFT_ADDR=192.168.1.10:30080 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/controller.db \
  -e BOOTSTRAP=false \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

### 7.2 apiserver

```bash
docker run -d \
  --name api-1 \
  --restart unless-stopped \
  -p 18100:18100 \
  -p 30100:30100 \
  -v /data/ddb/api-1:/data \
  -e ROLE=apiserver \
  -e NODE_ID=api-1 \
  -e GROUP_ID=control \
  -e HTTP_ADDR=192.168.1.10:18100 \
  -e RAFT_ADDR=192.168.1.10:30100 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/apiserver.db \
  -e BOOTSTRAP=false \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

dashboard 访问地址：

```text
http://192.168.1.10:18100/dashboard/
```

## 8. MACHINE_1 启动 g1-n1 / g2-n1 / g3-n1

### 8.1 g1-n1

```bash
docker run -d \
  --name g1-n1 \
  --restart unless-stopped \
  -p 21080:21080 \
  -p 22080:22080 \
  -v /data/ddb/g1-n1:/data \
  -e ROLE=shard \
  -e NODE_ID=g1-n1 \
  -e GROUP_ID=g1 \
  -e HTTP_ADDR=192.168.1.10:21080 \
  -e RAFT_ADDR=192.168.1.10:22080 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/data.db \
  -e BOOTSTRAP=true \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

### 8.2 g2-n1

```bash
docker run -d \
  --name g2-n1 \
  --restart unless-stopped \
  -p 21081:21081 \
  -p 22081:22081 \
  -v /data/ddb/g2-n1:/data \
  -e ROLE=shard \
  -e NODE_ID=g2-n1 \
  -e GROUP_ID=g2 \
  -e HTTP_ADDR=192.168.1.10:21081 \
  -e RAFT_ADDR=192.168.1.10:22081 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/data.db \
  -e BOOTSTRAP=true \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

### 8.3 g3-n1

```bash
docker run -d \
  --name g3-n1 \
  --restart unless-stopped \
  -p 21082:21082 \
  -p 22082:22082 \
  -v /data/ddb/g3-n1:/data \
  -e ROLE=shard \
  -e NODE_ID=g3-n1 \
  -e GROUP_ID=g3 \
  -e HTTP_ADDR=192.168.1.10:21082 \
  -e RAFT_ADDR=192.168.1.10:22082 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/data.db \
  -e BOOTSTRAP=true \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

## 9. MACHINE_2 启动 g1-n2 / g2-n2 / g3-n2

### 9.1 g1-n2

```bash
docker run -d \
  --name g1-n2 \
  --restart unless-stopped \
  -p 21180:21180 \
  -p 22180:22180 \
  -v /data/ddb/g1-n2:/data \
  -e ROLE=shard \
  -e NODE_ID=g1-n2 \
  -e GROUP_ID=g1 \
  -e HTTP_ADDR=192.168.1.11:21180 \
  -e RAFT_ADDR=192.168.1.11:22180 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/data.db \
  -e BOOTSTRAP=false \
  -e JOIN_ADDR=http://192.168.1.10:21080 \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

### 9.2 g2-n2

```bash
docker run -d \
  --name g2-n2 \
  --restart unless-stopped \
  -p 21181:21181 \
  -p 22181:22181 \
  -v /data/ddb/g2-n2:/data \
  -e ROLE=shard \
  -e NODE_ID=g2-n2 \
  -e GROUP_ID=g2 \
  -e HTTP_ADDR=192.168.1.11:21181 \
  -e RAFT_ADDR=192.168.1.11:22181 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/data.db \
  -e BOOTSTRAP=false \
  -e JOIN_ADDR=http://192.168.1.10:21081 \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

### 9.3 g3-n2

```bash
docker run -d \
  --name g3-n2 \
  --restart unless-stopped \
  -p 21182:21182 \
  -p 22182:22182 \
  -v /data/ddb/g3-n2:/data \
  -e ROLE=shard \
  -e NODE_ID=g3-n2 \
  -e GROUP_ID=g3 \
  -e HTTP_ADDR=192.168.1.11:21182 \
  -e RAFT_ADDR=192.168.1.11:22182 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/data.db \
  -e BOOTSTRAP=false \
  -e JOIN_ADDR=http://192.168.1.10:21082 \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

## 10. MACHINE_3 启动 g1-n3 / g2-n3 / g3-n3

### 10.1 g1-n3

```bash
docker run -d \
  --name g1-n3 \
  --restart unless-stopped \
  -p 21280:21280 \
  -p 22280:22280 \
  -v /data/ddb/g1-n3:/data \
  -e ROLE=shard \
  -e NODE_ID=g1-n3 \
  -e GROUP_ID=g1 \
  -e HTTP_ADDR=192.168.1.12:21280 \
  -e RAFT_ADDR=192.168.1.12:22280 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/data.db \
  -e BOOTSTRAP=false \
  -e JOIN_ADDR=http://192.168.1.10:21080 \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

### 10.2 g2-n3

```bash
docker run -d \
  --name g2-n3 \
  --restart unless-stopped \
  -p 21281:21281 \
  -p 22281:22281 \
  -v /data/ddb/g2-n3:/data \
  -e ROLE=shard \
  -e NODE_ID=g2-n3 \
  -e GROUP_ID=g2 \
  -e HTTP_ADDR=192.168.1.12:21281 \
  -e RAFT_ADDR=192.168.1.12:22281 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/data.db \
  -e BOOTSTRAP=false \
  -e JOIN_ADDR=http://192.168.1.10:21081 \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

### 10.3 g3-n3

```bash
docker run -d \
  --name g3-n3 \
  --restart unless-stopped \
  -p 21282:21282 \
  -p 22282:22282 \
  -v /data/ddb/g3-n3:/data \
  -e ROLE=shard \
  -e NODE_ID=g3-n3 \
  -e GROUP_ID=g3 \
  -e HTTP_ADDR=192.168.1.12:21282 \
  -e RAFT_ADDR=192.168.1.12:22282 \
  -e RAFT_DIR=/data/raft \
  -e DB_PATH=/data/data.db \
  -e BOOTSTRAP=false \
  -e JOIN_ADDR=http://192.168.1.10:21082 \
  -e ETCD_ADDR=192.168.1.10:2379 \
  ghcr.io/yedou37/ddb:latest
```

## 11. 启动后验证

在 `MACHINE_1` 上执行：

```bash
docker logs ctrl-1 --tail 50
docker logs api-1 --tail 50
docker logs g1-n1 --tail 50
docker logs g2-n1 --tail 50
docker logs g3-n1 --tail 50
```

然后验证控制面：

```bash
docker exec -it api-1 ddb-cli --node-url=http://192.168.1.10:18100 control config
docker exec -it api-1 ddb-cli --node-url=http://192.168.1.10:18100 control groups
docker exec -it api-1 ddb-cli --node-url=http://192.168.1.10:18100 control shards
```

打开 dashboard：

```bash
open http://192.168.1.10:18100/dashboard/
```

## 12. 创建表并写入测试数据

在 `MACHINE_1` 上执行：

```bash
docker exec -it api-1 ddb-cli --node-url=http://192.168.1.10:18100 sql "CREATE TABLE users (id INT PRIMARY KEY, name TEXT)"
docker exec -it api-1 ddb-cli --node-url=http://192.168.1.10:18100 sql "INSERT INTO users VALUES (1, 'alice')"
docker exec -it api-1 ddb-cli --node-url=http://192.168.1.10:18100 sql "INSERT INTO users VALUES (2, 'bob')"
docker exec -it api-1 ddb-cli --node-url=http://192.168.1.10:18100 sql "SELECT * FROM users"
```

## 13. 常见问题

### 13.1 为什么必须用物理机 IP

因为这里是三台独立物理机，不是同一个 Docker bridge 网络。

所以：

- `HTTP_ADDR`
- `RAFT_ADDR`
- `JOIN_ADDR`
- `ETCD_ADDR`

都必须写成其他机器能访问到的真实 IP 地址，不能写容器名。

### 13.2 为什么要挂载 `/data`

因为：

- `raft` 数据
- `BoltDB` 数据

都需要持久化。

如果不挂卷，容器删掉后节点状态会丢失。

## 14. 清理命令

### MACHINE_1

```bash
docker rm -f ddb-etcd ctrl-1 api-1 g1-n1 g2-n1 g3-n1 2>/dev/null || true
```

### MACHINE_2

```bash
docker rm -f g1-n2 g2-n2 g3-n2 2>/dev/null || true
```

### MACHINE_3

```bash
docker rm -f g1-n3 g2-n3 g3-n3 2>/dev/null || true
```
