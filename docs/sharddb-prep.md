# ShardDB 开发 Prompt

## 1. 你的身份与目标

你现在要把当前仓库从“单个 Raft 副本组上的分布式 MiniSQL”升级为“适合课程展示的 ShardDB MVP”。

目标不是做一个完美的工业级分布式数据库，而是做一个**可运行、可展示、可答辩、架构清晰**的课程项目实现。最终系统需要满足以下目标：

- 具备 `controller/apiserver + shard replica groups` 三层结构
- 具备一致性哈希驱动的分片路由
- 具备由控制面动态调整 `shard -> group` 分配关系的能力
- 每个 shard group 内部使用 Raft 保证一致性
- CLI 统一连接 apiserver
- 三台物理机可演示扩容、rebalance、故障下仍可服务

## 2. 核心约束

- 这是课程项目，不追求一次性支持完整 SQL
- 第一阶段不实现跨分片 Join
- 第一阶段只做单分片可路由请求
- 必须保留后续支持 Join 的架构空间
- etcd 负责服务发现，并作为当前版本的共享配置存储
- 当前演示版本使用单个 controller/apiserver 作为控制面入口
- 必须最大化复用当前仓库已有的稳定组件

## 3. 最终目标架构

```text
CLI
  -> API Server
       -> Controller Metadata
       -> Router / Coordinator
       -> Target Shard Group

Controller Service
  -> 管理 shard -> group 映射
  -> 管理 group 成员信息
  -> 管理配置版本
  -> 支持 move/rebalance

Shard Replica Groups
  -> 每组 3 节点
  -> 组内 Raft 保证一致性
  -> 仅负责自己承载的 shard 数据
```

推荐课程展示配置：

- `controller/apiserver`: 1 实例
- `shard groups`: 初始 2 组，展示时可增加到 3 组
- `replica per group`: 3 节点
- `total shards`: 8

## 4. 术语和层级

- `node`: 单个进程实例
- `replica`: 某份 shard 数据的一个副本
- `replica node`: 承载副本的节点
- `replica group`: 一组维护同一份 shard 数据的副本节点
- `controller service`: 管理 `shard -> group` 映射的控制面服务
- `apiserver`: 统一 SQL 和控制命令入口
- `shard`: 数据分片，是逻辑数据分布单元

层级关系：

```text
system
  -> apiserver
  -> controller service
  -> shard replica groups
       -> replica nodes
            -> local raft + storage
```

## 5. 数据流要求

### 5.1 写请求

写请求数据流必须满足：

```text
CLI
  -> API Server
  -> Router 根据 SQL + 主键定位 shard
  -> Controller 查询 shard -> group
  -> 转发到目标 shard group leader
  -> group 内 Raft replicate
  -> local storage apply
  -> 返回结果给 API Server
  -> 返回结果给 CLI
```

### 5.2 读请求

单分片读请求数据流必须满足：

```text
CLI
  -> API Server
  -> Router 根据 SQL + 主键定位 shard
  -> Controller 查询 shard -> group
  -> 转发到目标 shard group
  -> 返回局部结果
  -> API Server 返回最终结果
```

### 5.3 rebalance / move shard 请求

控制面操作数据流必须满足：

```text
CLI
  -> API Server
  -> Controller Service
  -> 更新共享配置
  -> 更新 shard -> group 映射和配置版本
  -> 触发 shard group 的配置刷新
  -> 返回最新配置
```

### 5.4 Join 的预留数据流

第一阶段不要求实现完整 Join，但架构必须允许未来扩展为：

```text
CLI
  -> API Server
  -> Planner / Coordinator
  -> 一个或多个 shard groups
  -> 收集局部结果
  -> API Server 做 merge / join
  -> 返回最终结果
```

## 6. 功能要求

### 6.1 第一阶段必须实现

- apiserver 统一入口
- controller service 管理 shard 配置
- shard group 内部 Raft 副本复制
- 一致性哈希把 `table + primary_key` 路由到 `0..7` 中某个 shard
- controller 维护 `shard -> group`
- 支持以下 SQL:
  - `CREATE TABLE`
  - `INSERT`
  - `SELECT ... WHERE pk = ?`
  - `DELETE ... WHERE pk = ?`
- 支持以下控制命令:
  - `show config`
  - `show shards`
  - `show groups`
  - `move shard`
  - `rebalance`

### 6.2 第一阶段明确不做

- 跨分片 Join
- 在线自动数据迁移
- 自动负载均衡策略优化
- 跨分片事务
- 通用复杂 SQL 执行计划

### 6.3 第二阶段预留能力

- 单分片 Join
- scatter-gather 查询
- 跨分片 Join 协调
- shard 迁移过程可视化

## 7. 模块划分与职责

### 模块 1: `internal/shardmeta`

职责：

- 定义 `ShardID`、`GroupID`、`ClusterConfig`、`ShardAssignment`、`ConfigVersion`
- 封装 shard 配置模型

验收标准：

- 能表达 8 个 shard 的配置
- 能表达一个 shard 对应哪个 group
- 能表达配置版本递增
- 能序列化/反序列化为持久化格式

### 模块 2: `internal/controller`

职责：

- 维护 shard 配置
- 维护 group 元数据
- 处理 `move shard`、`rebalance`
- 当前版本通过共享配置存储保证配置一致

验收标准：

- 能新增 group
- 能查询当前 shard 配置
- 能把一个 shard 从 group1 调整到 group2
- 能生成新的配置版本
- 多个控制面实例从同一配置源读取时保持一致

### 模块 3: `internal/apiserver`

职责：

- 提供统一 HTTP API
- 接收 SQL 和控制命令
- 调用 router、controller、coordinator
- 对外返回统一响应

验收标准：

- CLI 只需要连接 apiserver
- SQL 请求能被正确转发
- 控制命令能被正确转发到 controller/apiserver
- 请求失败时能给出明确错误信息

### 模块 4: `internal/router`

职责：

- 根据 SQL 和主键提取路由键
- 使用一致性哈希映射到 shard
- 再查询 controller 得到目标 group

验收标准：

- 相同 `table + primary_key` 必须稳定映射到同一个 shard
- 不同 key 能分布到多个 shard
- 对无主键、不可路由 SQL 返回明确错误
- 路由结果能输出 `shard id` 和 `group id`

### 模块 5: `internal/coordinator`

职责：

- 封装 apiserver 到 shard group 的请求协调
- 先实现单分片请求转发
- 后续扩展多分片 fan-out / merge

验收标准：

- 单分片读写可成功转发
- group leader 变更后仍能重试或重定向
- 返回结果格式统一

### 模块 6: `internal/group`

职责：

- 封装单个 shard replica group 的访问接口
- 封装对 group leader 的发现、请求发送、错误处理

验收标准：

- 能根据 group 元数据找到目标节点
- 能向目标 leader 发送 SQL/内部命令
- 失败时返回清晰错误

### 模块 7: `internal/storage`

职责：

- 在现有 BoltDB store 上增加 shard namespace
- 保证不同 shard 数据隔离

验收标准：

- 同表不同 shard 的数据不会混在一起
- 能按 shard 创建/查询/删除数据
- 不破坏单机存储正确性

### 模块 8: `internal/sql`

职责：

- 保留现有基础 SQL 解析
- 为 router 提供主键提取和路由所需信息

验收标准：

- 能识别 `CREATE/INSERT/SELECT(pk)/DELETE(pk)`
- 对不可路由语句给出明确错误
- 为后续 Join 扩展保留 AST 扩展点

### 模块 9: `internal/config`

职责：

- 新增角色化配置
- 支持 `controller`、`apiserver`、`shard node`

验收标准：

- 能配置 `role`
- 能配置 `group-id`
- 能配置 `controller-addrs`
- 能配置本节点承载的 shard group 信息

### 模块 10: `cmd/cli`

职责：

- 统一面向 apiserver
- 提供 SQL 命令和控制命令

验收标准：

- 支持执行基础 SQL
- 支持查看 shard 配置
- 支持 `move shard`
- 支持 `rebalance`
- 支持展示 group 状态

## 8. 代码复用评估

### 8.1 可直接复用

- `internal/raftnode`
  - 作为单个 replica group 的 Raft runtime
- `internal/discovery`
  - 继续作为 etcd 服务发现层
- `internal/config`
  - 保留基础解析框架，增加字段即可
- `internal/api`
  - 可复用 handler 组织方式
- `cmd/cli`
  - 可复用命令框架和 HTTP 请求框架

### 8.2 可部分复用

- `internal/storage`
  - 复用 BoltDB 与表存储逻辑，但必须引入 shard namespace
- `internal/sql`
  - 复用基础 parser，但必须补路由相关信息提取
- `internal/service/query_service`
  - 仅复用错误处理思路与服务编排经验，不能直接保留当前单组假设
- `internal/app`
  - 复用启动装配思想，但要重写为角色化启动

### 8.3 不应继续沿用的假设

- 全局只有一个 leader
- 所有节点持有同一份数据
- CLI 直接操作数据副本组
- 读请求可以不经过路由层直接本地执行

### 8.4 依赖建议

继续保留：

- `hashicorp/raft`
- `hashicorp/raft-boltdb`
- `bbolt`
- `etcd/client/v3`

暂不新增重型依赖：

- 第一阶段不要引入新的大型 SQL 引擎
- 真做 Join 时再评估是否切换到成熟 parser

## 9. 分片策略要求

### 9.1 总分片数

总分片数固定为 **`8`**。

原因：

- 足够展示负载均衡和分配变化
- 不会让迁移逻辑过于复杂
- 适合 `2 group -> 3 group` 的课程演示

### 9.2 分片路由

要求：

- 使用一致性哈希计算 `table + primary_key -> shard`
- shard 编号范围固定在 `0..7`
- controller 管理 `shard -> group`
- rebalance 只改变 `shard -> group` 映射，不改变总 shard 数

### 9.3 负载均衡展示目标

必须支持如下演示：

```text
初始:
  group1 -> shard 0,1,2,3
  group2 -> shard 4,5,6,7

加入 group3 后:
  group1 -> shard 0,1,2
  group2 -> shard 3,4,5
  group3 -> shard 6,7
```

验收标准：

- rebalance 后配置版本发生变化
- apiserver 能读取新配置
- 后续新请求落到新 group

## 10. 三台物理机展示要求

推荐部署：

```text
Machine A:
  etcd
  controller-1
  apiserver
  group1-node1
  group2-node1
  group3-node1

Machine B:
  group1-node2
  group2-node2
  group3-node2

Machine C:
  group1-node3
  group2-node3
  group3-node3
```

要求：

- CLI 可在任意一台机器额外开终端执行
- CLI 统一连接 apiserver
- etcd 和 controller/apiserver 部署在 `Machine A`
- `group3` 作为扩容演示组
- 至少要能演示一次 rebalance
- 当前演示重点是 shard group 的高可用，不强调控制面的高可用

## 11. 分阶段开发路线

### Phase 0: 领域模型与骨架

实现内容：

- 新增 `shardmeta`
- 扩展 `config`
- 定义 `NodeRole`、`GroupID`、`ShardID`
- 明确 controller/apiserver/shard node 的角色装配

验收标准：

- 项目能编译
- 新增模型完整
- 角色配置能被正确解析

### Phase 1: controller + 路由

实现内容：

- controller 保存 `shard -> group`
- router 基于一致性哈希定位 shard
- apiserver 能完成单分片请求路由

验收标准：

- 可查询当前 shard 配置
- 相同 key 路由稳定
- SQL 能被路由到目标 group

### Phase 2: shard group 数据执行

实现内容：

- 每个 group 独立启动 Raft
- group 内写入复制
- storage 按 shard 隔离

验收标准：

- group 内 3 副本数据一致
- 单节点故障后仍可写
- follower 重启后能追平

### Phase 3: rebalance 与演示能力

实现内容：

- `move shard`
- `rebalance`
- 配置版本切换
- CLI 展示当前 group 承载的 shard 数量

验收标准：

- 能手动移动 shard
- 能执行 rebalance
- rebalance 后新请求走新 group

### Phase 4: 查询增强

实现内容：

- 补充范围查询
- 补充多分片 fan-out 接口
- 预留 Join 协调层

验收标准：

- API Server 内存在清晰的 planner/coordinator 扩展点
- 单分片 Join 的设计接口清晰

## 12. 必须产出的测试

至少补以下测试：

- 一致性哈希路由稳定性测试
- `shard -> group` 配置读写测试
- 单分片写入转发测试
- group 内复制一致性测试
- rebalance 后路由变化测试
- follower 重启追平测试
- CLI 到 apiserver 的端到端测试

## 13. 开发时的实现原则

- 先让架构成立，再补功能
- 先支持单分片请求，再谈 Join
- 不在第一阶段引入不必要的大依赖
- 能复用的代码优先复用，但不能保留错误的单组假设
- 所有新增模块必须有清晰职责和最小验收测试

## 14. 立即开始的任务

按以下顺序开始实现：

1. 扩展 `internal/model` 或新增 `internal/shardmeta`
2. 扩展 `internal/config`
3. 新增 `internal/controller`
4. 新增 `internal/router`
5. 新增 `internal/apiserver`
6. 改造 `internal/storage` 支持 shard namespace
7. 改造 `internal/app` 支持角色化启动
8. 增加最小 e2e 测试
