# 五容器真实化 Mock 环境

这个目录提供一套更贴近真实验收方式的本地 mock：

- 只创建 5 个空 Linux 容器
- 固定容器名和内网 IP
- 默认看不到你本地仓库源码
- 默认看不到你本地刚编译出的二进制
- 不自动启动 `etcd`、`apiserver` 或任何 `ddb-server`



## 拓扑

- `t9-ct1` -> `10.10.9.1`
- `t9-ct2` -> `10.10.9.2`
- `t9-ct3` -> `10.10.9.3`
- `t9-ct4` -> `10.10.9.4`
- `t9-ct5` -> `10.10.9.5`

Docker bridge 的 gateway 固定为 `10.10.9.254`，这样 5 个容器可以稳定占用
`10.10.9.1` 到 `10.10.9.5`。

## 一键起空环境

主入口：

```bash
./mock/five-container/up.sh
```

如果你的 Docker 环境支持 Compose，也可以使用：

```bash
docker compose -f mock/five-container/docker-compose.yml up -d
```

## 这次和以前的区别

现在的 mock 不再把仓库挂进容器，所以容器里默认：

- 没有 `/workspace/ddb`
- 看不到本地源码
- 看不到本地编译好的 `ddb-server`、`ddb-cli`

这更接近真实验收：你需要显式上传产物，不能靠“本地一编译，容器里立刻可见”。

## 容器里现在有什么

每个容器启动后只有：

- 一个空的 Alpine Linux shell 环境
- `/data`
- `/logs`

你后续需要自己把二进制或配置传进去，比如放到：

- `/opt/ddb/bin`
- `/opt/ddb/configs`
- `/opt/ddb/logs`
- `/opt/ddb/data`

## 如何进入 5 台机器

```bash
docker exec -it t9-ct1 sh
docker exec -it t9-ct2 sh
docker exec -it t9-ct3 sh
docker exec -it t9-ct4 sh
docker exec -it t9-ct5 sh
```

容器内可做的基础检查：

```sh
hostname
ip addr
ping -c 1 10.10.9.1
ls /
```

## 如何模拟真实上传

推荐工作流：

1. 在宿主机本地编译 Linux 二进制
2. 把这些产物放到一个本地目录，例如 `./artifacts/mock-linux-amd64`
3. 用上传脚本显式复制到 5 个容器

上传脚本：

```bash
./mock/five-container/push-artifacts.sh <artifacts-dir>
```

例如：

```bash
./mock/five-container/push-artifacts.sh ./artifacts/mock-linux-amd64
```

这个目录建议包含：

- `ddb-server`
- `ddb-cli`
- `etcd`（可选）
- `configs/`（可选）

上传后，容器内默认会落到：

- `/opt/ddb/bin`
- `/opt/ddb/configs`

例如检查：

```bash
docker exec t9-ct1 sh -lc 'ls -R /opt/ddb'
docker exec t9-ct1 sh -lc '/opt/ddb/bin/ddb-cli --help'
```

## 为什么这样更像真实环境

因为真实验收里，通常不是：

- 本地仓库直接挂进远端容器
- 本地一编译，远端立刻看到

而更像是：

- 你先本地编译
- 再通过 `scp`、`rsync` 或别的手段上传
- 最后登录远端机器执行

当前 mock 用 `docker cp` 来模拟这个“上传”动作，语义上比共享挂载更接近真实。

## 镜像方案是否更好

如果真实验收环境里的 5 台机器本身还能再运行 Docker/Podman，那么“提前构建镜像，再把镜像传过去并运行”也是可行路线。

但如果真实情况是：

- 你只能 SSH 到那 5 个现成容器
- 容器里不能再运行 Docker

那么镜像方案就不成立，最后仍然要回到：

- 上传二进制
- 在容器里直接起进程

所以当前这套 mock 默认优先模拟的是更通用、也更稳妥的“上传二进制 + 直接运行进程”方案。

## 验证拓扑

在宿主机检查 5 个容器：

```bash
docker ps --format 'table {{.Names}}\t{{.Status}}'
docker inspect -f '{{.Name}} -> {{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' t9-ct1 t9-ct2 t9-ct3 t9-ct4 t9-ct5
```

## 清理

停止并移除空环境：

```bash
./mock/five-container/down.sh
```

如果还想删除本地运行期目录：

```bash
rm -rf .mock-data/five-container .mock-logs/five-container
```
