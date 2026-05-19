#set page(
  paper: "a4",
  margin: (
    top: 2.54cm,
    bottom: 2.54cm,
    left: 2.54cm,
    right: 2.54cm,
  ),
)

#let google-blue = rgb("#1A73E8")
#let google-text = rgb("#202124")
#let google-subtle = rgb("#5F6368")
#let google-border = rgb("#DADCE0")
#let google-header = rgb("#F1F3F4")
#let google-gray = rgb("#F8F9FA")

#let title-sans = ("Google Sans", "Google Sans Text", "PingFang SC", "Noto Sans CJK SC", "Source Han Sans SC")
#let body-sans = ("Google Sans Text", "Google Sans", "PingFang SC", "Noto Sans CJK SC", "Source Han Sans SC")

#set text(
  lang: "zh",
  region: "cn",
  font: body-sans,
  size: 10.5pt,
  fill: google-text,
)

#set par(
  justify: true,
  first-line-indent: 2em,
  leading: 1em,
)

#let fill-line(width: 7cm) = box(
  width: width,
  height: 1.15em,
  inset: 0pt,
  stroke: (bottom: 0.8pt + google-border),
)[]

#let toc-line(title, page) = grid(
  columns: (78%, 22%),
  column-gutter: 0pt,
  [
    #text(fill: google-text)[#title]
  ],
  [
    #align(right)[#text(fill: google-subtle)[#page]]
  ],
)

#let h1(title) = block(above: 1.8em, below: 1.3em)[
  #box(width: 100%, inset: (left: 10pt, top: 7pt, bottom: 7pt), fill: google-header, radius: 6pt)[
    #text(font: title-sans, size: 15.5pt, weight: "bold", fill: google-blue)[#title]
  ]
]

#let h2(title) = block(above: 2em, below: 1.15em)[
  #text(font: title-sans, size: 13pt, weight: "bold", fill: google-text)[#title]
]

#let h3(title) = block(above: 1.7em, below: 1em)[
  #text(font: title-sans, size: 11.5pt, weight: "semibold", fill: google-subtle)[#title]
]

#let body-line(text) = block(above: 0.08em, below: 0.38em)[#text]

#let ref-line(content) = block(above: 0.06em, below: 0.3em)[
  #set par(first-line-indent: 0pt, hanging-indent: 2em, leading: 1em)
  #text(font: body-sans, size: 10pt, fill: google-text)[#content]
]

#let divider(width: 100%) = box(
  width: width,
  height: 0pt,
  stroke: (bottom: 0.8pt + google-border),
)[]

#let fig(path, caption, width: 92%) = figure(
  image(path, width: width),
  caption: caption,
)

#align(center)[
  #v(18mm)
  #box(
    width: 100%,
    inset: (left: 16pt, right: 16pt, top: 16pt, bottom: 16pt),
    fill: google-gray,
    stroke: 0.8pt + google-border,
    radius: 12pt,
  )[
    #align(left)[
      #text(font: title-sans, size: 9pt, weight: "medium", fill: google-blue)[COURSE MODULE REPORT]
      #v(8pt)
      #text(font: title-sans, size: 24pt, weight: "bold", fill: google-text)[大规模信息系统构建技术导论]
      #v(10pt)
      #text(font: title-sans, size: 18pt, weight: "bold", fill: google-blue)[控制平面与 API 接入模块实现与测试报告]
      #v(12pt)
      #divider()
      #v(18pt)
      #grid(
        columns: (2.1cm, 1fr),
        row-gutter: 10pt,
        column-gutter: 10pt,
        align: (left, left),
        [#text(font: body-sans, size: 11pt, weight: "medium", fill: google-subtle)[姓名]], [#text(font: body-sans, size: 11pt)[叶容宇、胡治禾、杨佳利]],
        [#text(font: body-sans, size: 11pt, weight: "medium", fill: google-subtle)[学院]], [#text(font: body-sans, size: 11pt)[计算机科学与技术学院]],
        [#text(font: body-sans, size: 11pt, weight: "medium", fill: google-subtle)[系]],   [#fill-line(width: 100%)],
        [#text(font: body-sans, size: 11pt, weight: "medium", fill: google-subtle)[专业]], [#text(font: body-sans, size: 11pt)[软件工程]],
        [#text(font: body-sans, size: 11pt, weight: "medium", fill: google-subtle)[学号]], [#fill-line(width: 100%)],
      )
      #v(22pt)
      #align(right)[#text(font: body-sans, size: 11pt, fill: google-subtle)[2026 年 5 月 18 日]]
    ]
  ]
]

#pagebreak()
#counter(page).update(2)
#set page(
  footer: context align(center)[
    #box(
      width: 45%,
      height: 0pt,
      stroke: (bottom: 0.7pt + google-border),
    )[]
    #v(4pt)
    #text(font: body-sans, size: 9pt, fill: google-subtle)[#counter(page).display()]
  ],
)

#align(center)[#text(font: title-sans, size: 16pt, weight: "bold", fill: google-blue)[目录]]

#v(0.9em)
#divider()
#v(0.8em)
#toc-line([一．系统模块简介], [3])
#toc-line([二．控制平面元数据管理实现说明], [4])
#toc-line([2.1 模块组件设计], [4])
#toc-line([2.2 主要数据结构], [5])
#toc-line([2.3 流程图设计], [6])
#toc-line([三．API 接入与可观测面实现说明], [7])
#toc-line([3.1 模块组件设计], [7])
#toc-line([3.2 主要数据结构], [8])
#toc-line([3.3 流程图设计], [9])
#toc-line([四．测试结果], [10])
#toc-line([4.1 控制平面功能测试], [10])
#toc-line([4.2 API 接入与 Dashboard 测试], [11])
#toc-line([五．开发体会], [12])
#toc-line([参考文献], [13])

#pagebreak()

#h1([一．系统模块简介])
#body-line([本人在分布式 MiniSQL 系统中负责梳理控制平面与 API 接入模块，主要涉及 `internal/controller`、`internal/apiserver`、`internal/discovery`、`internal/coordinator`、`internal/app`、`cmd/server` 与 `cmd/cli` 等目录。该模块将底层 Raft 副本组组织成可路由、可迁移、可观测的分片数据库系统，是 ShardDB 相比单副本组 DDB 的主要扩展部分。])
#body-line([控制平面负责维护“分片到副本组”的全局元数据，API 接入层负责把用户 SQL、控制命令和 Dashboard 请求转换为对控制平面或数据平面的调用。二者共同形成系统的接入面：用户不再直接关心某条数据位于哪一个 Raft 组，而是通过统一 API Server 或 CLI 发起请求，由系统自动完成服务发现、分片路由、Leader 选择和异常重试。])
#body-line([本模块的具体功能如下：])
#body-line([（1）基于 etcd 实现服务发现，维护在线节点、节点角色、副本组、Leader 状态和 removed 标记。])
#body-line([（2）实现控制平面 Controller，维护 `ClusterConfig`，支持初始分片分配、MoveShard 和 Rebalance。])
#body-line([（3）实现配置持久化链路，将控制面配置保存到 etcd，并可附带保存到本地文件，使 API Server 重启后能够恢复最新配置。])
#body-line([（4）实现分片级迁移锁，在 MoveShard/Rebalance 期间阻止冲突读写，并通过 `503 + Retry-After` 反馈给客户端。])
#body-line([（5）实现 API Server 网关，提供 `/sql`、`/config`、`/shards`、`/groups`、`/move-shard`、`/rebalance` 等 HTTP 接口。])
#body-line([（6）实现 Dashboard 与 CLI 接入，支持拓扑展示、健康探测、分片查看、表数据浏览和控制命令调用。])

#fig("ddbarch.jpeg", [控制平面与 API 接入在系统架构中的位置], width: 95%)

#h1([二．控制平面元数据管理实现说明])
#h2([2.1 模块组件设计])
#body-line([控制平面的核心实现位于 `internal/controller/service.go`。它不直接处理 SQL 语句，而是管理分片配置、分片锁和配置持久化。上层 API Server 通过 Controller Service 获取当前配置、预览迁移动作、更新配置，并在迁移期间持有锁。])
#body-line([本模块包括以下几个部分：])
#body-line([（1）Controller Service。`Service` 持有当前 `ClusterConfig`、配置存储接口和 `lockedShardIDs` 集合，提供 `CurrentConfig`、`PreviewMoveShard`、`PreviewRebalance`、`UpdateConfig`、`LockShards`、`UnlockShards` 与 `WithLockedShards` 等方法。])
#body-line([（2）配置初始化。`NewBootstrapService` 优先从存储中加载已有配置；若配置不存在，则根据总分片数和启动时发现的 groupIDs 进行轮询式初始分配，并将配置保存。默认总分片数为 8。])
#body-line([（3）配置存储抽象。`ConfigStore` 定义 `Load` 和 `Save` 两个接口，具体实现包括内存存储、文件存储、基于 etcd 的 DiscoveryStore 以及 ChainStore。ChainStore 可按顺序读取多个存储，并在保存时写入多个后端。])
#body-line([（4）etcd 服务发现。`internal/discovery/etcd.go` 使用 `/ddb/nodes/` 保存带 Lease 的在线节点信息，使用 `/ddb/removed/` 保存被移除节点标记，使用 `/ddb/controller/config` 保存控制平面配置。节点注册使用 10 秒 Lease 与 KeepAlive，节点离线后注册键自动过期。])
#body-line([（5）应用装配。`internal/app/app.go` 根据启动角色选择不同服务：`controller` 与 `apiserver` 角色会装配 Controller Service、Router 和 Coordinator；`shard` 角色会装配 BoltDB、RaftNode 和普通数据节点 API。])
#body-line([（6）分片迁移编排。MoveShard/Rebalance 并不只修改元数据，而是在 `WithLockedShards` 保护下先调用 migrator 搬迁数据，再更新 `ClusterConfig`，最后释放锁。])

#h2([2.2 主要数据结构])
#body-line([控制平面主要数据结构定义在 `internal/shardmeta/types.go`、`internal/controller/service.go` 和 `internal/model/model.go` 中：])
#body-line([（1）`ClusterConfig`：全局分片配置，包含版本号、总分片数、分片分配列表和可选副本组信息。版本号在 MoveShard/Rebalance 后递增，用于表示配置演进。])
#body-line([（2）`ShardAssignment`：单个分片到副本组的映射，包含 `ShardID` 与 `GroupID`。系统通过该结构决定某个分片当前由哪个 Raft 副本组承载。])
#body-line([（3）`GroupInfo`：副本组信息，包含组 ID 和可选节点列表，用于描述数据平面的逻辑承载单元。])
#body-line([（4）`NodeRole`：节点角色枚举，包括 `shard`、`controller`、`apiserver`。角色化启动使同一个 `cmd/server` 可以运行不同职责的进程。])
#body-line([（5）`NodeInfo`：服务发现中的节点信息，包含节点 ID、HTTP 地址、Raft 地址、Leader 状态、角色和 GroupID。API Server 通过该结构挑选目标副本组的 Leader 或在线节点。])
#body-line([（6）`ShardMigrationError`：迁移锁冲突错误，封装具体分片 ID，并可 unwrap 到 `ErrShardMigrationInProgress`，便于 API 层转换为 503 响应。])

#h3([2.2.1 控制平面类图])
#figure(
  block(width: 100%, inset: 8pt, fill: google-gray, stroke: 0.8pt + google-border, radius: 6pt)[
```text
+-----------------------------+          +-----------------------------+
| controller.Service          |          | controller.ConfigStore      |
+-----------------------------+          +-----------------------------+
| config: ClusterConfig       | -------> | Load(ctx) ClusterConfig     |
| lockedShardIDs: map         |          | Save(ctx, config) error     |
+-----------------------------+          +-----------------------------+
| CurrentConfig()             |              ^        ^          ^
| PreviewMoveShard(id, group) |              |        |          |
| PreviewRebalance(groups)    |   +----------+   +----+-----+    +----------------+
| UpdateConfig(config)        |   |              |          |                     |
| WithLockedShards(ids, fn)   |   |              |          |                     |
+-----------------------------+   |              |          |                     |
                                  |              |          |                     |
+-----------------------------+   |  +------------------+  +-------------------+
| shardmeta.ClusterConfig     |   |  | MemoryStore      |  | FileStore         |
+-----------------------------+   |  +------------------+  +-------------------+
| Version: uint64             |   |
| TotalShards: int            |   |  +------------------+      +----------------+
| Assignments: []ShardAssign  |   +--| DiscoveryStore   | ---> | discovery.Client|
| Groups: []GroupInfo         |      +------------------+      +----------------+
+-----------------------------+                                | ListNodes()    |
         |                                                     | Register()     |
         | contains                                            | SaveConfig()   |
         v                                                     | LoadConfig()   |
+-----------------------------+                                +----------------+
| shardmeta.ShardAssignment   |
+-----------------------------+
| ShardID: ShardID            |
| GroupID: GroupID            |
+-----------------------------+
```
  ],
  caption: [控制平面元数据管理类图],
)

#h2([2.3 流程图设计])
#h3([2.3.1 控制平面启动流程])
#body-line([控制平面启动时，`cmd/server` 读取命令行参数和环境变量，生成 `ServerConfig`。若角色为 `controller` 或 `apiserver`，`NewServerApp` 会连接 etcd，读取当前在线 shard 节点并选择启动分组；随后创建 `DiscoveryStore + FileStore` 的 ChainStore，并调用 `NewBootstrapService` 加载或初始化 `ClusterConfig`。最后创建 Router 与 Coordinator，并将它们挂载到 API Server Handler。])
#h3([2.3.2 MoveShard / Rebalance 流程])
#body-line([MoveShard 流程首先读取当前配置，根据目标 shard 找到源副本组；然后 Controller 对该分片加锁，Coordinator 从源组读取表结构和行数据，将属于该分片的数据逐行写入目标组并从源组删除；迁移完成后 Controller 生成新配置、递增版本号、持久化配置并释放锁。Rebalance 与之类似，只是先根据新 group 列表计算多条分片移动计划，再按计划逐个迁移。])

#fig("7a26e3c261844642abe172d3d0195046.png", [分片迁移与控制平面锁保护示意图], width: 95%)

#h1([三．API 接入与可观测面实现说明])
#h2([3.1 模块组件设计])
#body-line([API 接入层分为两类：一类是 shard 数据节点上的普通 DDB API，直接服务于某个 Raft 副本组；另一类是 controller/apiserver 角色上的 ShardDB API Server，用于统一接收 SQL、控制命令和 Dashboard 请求。])
#body-line([本模块包括以下几个部分：])
#body-line([（1）数据节点 API。`internal/api/handler.go` 提供 `/health`、`/sql`、`/status`、`/leader`、`/members`、`/tables`、`/schema`、`/join`、`/remove` 与 `/rejoin`。这些接口主要围绕单个 Raft 副本组工作。])
#body-line([（2）ShardDB API Server。`internal/apiserver/handler.go` 提供 `/sql`、`/config`、`/shards`、`/groups`、`/move-shard`、`/rebalance`。它通过 Controller Service 读取元数据，通过 Coordinator 执行 SQL 或迁移。])
#body-line([（3）SQL 接入路径。API Server 收到 `/sql` 后，将请求交给 Coordinator。Coordinator 解析 SQL 后，根据语句类型选择单分片路由、广播、Scatter-Gather 或 JOIN 聚合策略，再通过 HTTP 调用目标 shard 节点。])
#body-line([（4）控制命令接入路径。`/move-shard` 和 `/rebalance` 先进行参数校验，再调用 Controller 预览配置变更，并在锁保护下执行迁移和配置更新。])
#body-line([（5）Dashboard。`internal/apiserver/dashboard.go` 使用 `go:embed` 内嵌静态资源，提供 `/dashboard/` 页面、`/dashboard/api/overview` 拓扑概览和 `/dashboard/api/table-data` 表数据浏览接口。])
#body-line([（6）CLI。`cmd/cli/main.go` 提供 `sql`、`cluster` 和 `control` 三类命令。CLI 可通过 `--node-url` 指定接入地址，也可通过 etcd 自动发现 API Server、Controller 或 Leader 节点。])

#h2([3.2 主要数据结构])
#body-line([（1）`MoveShardRequest`：`/move-shard` 请求体，包含目标 `shard_id` 与新的 `group_id`。])
#body-line([（2）`RebalanceRequest`：`/rebalance` 请求体，包含新的副本组 ID 列表，Controller 根据该列表重新均衡分片。])
#body-line([（3）`model.SQLRequest` 与 `model.SQLResponse`：SQL API 的统一输入输出结构。响应中包含 `success`、`result`、`error` 和可选 `leader` 字段。])
#body-line([（4）`model.ShardsResponse` 与 `model.GroupStatus`：控制面查询接口的输出结构，分别用于展示分片分配和副本组状态。])
#body-line([（5）`dashboardOverview`：Dashboard 聚合结构，包含生成时间、摘要、当前配置、分片状态、锁定分片、节点状态、组状态和错误列表。])
#body-line([（6）`dashboardNode` 与 `dashboardGroup`：Dashboard 展示层结构，额外记录节点可达性、表数量、Leader 信息、分组健康状态和迁移状态。])

#h3([3.2.1 API 接入类图])
#figure(
  block(width: 100%, inset: 8pt, fill: google-gray, stroke: 0.8pt + google-border, radius: 6pt)[
```text
+-----------------------------+        +-----------------------------+
| apiserver.Handler           |        | coordinator.Coordinator     |
+-----------------------------+        +-----------------------------+
| /sql                        | -----> | configReader: ConfigReader  |
| /config                     |        | nodeLister: NodeLister      |
| /shards                     |        | router: *router.Router      |
| /groups                     |        | httpClient: *http.Client    |
| /move-shard                 |        +-----------------------------+
| /rebalance                  |        | ExecuteSQL(ctx, sql)        |
+-----------------------------+        | MigrateShard(ctx, id, g1,g2)|
          |                            +-----------------------------+
          | uses                                      |
          v                                           | reads and routes
+-----------------------------+        +-----------------------------+
| controller.Service          |        | router.Router               |
+-----------------------------+        +-----------------------------+
| CurrentConfig()             |        | Route(table, key, config)   |
| WithLockedShards(ids, fn)   |        +-----------------------------+
+-----------------------------+                     |
          |                                         v
          | returns                         +-----------------------------+
          v                                 | model.SQLResponse           |
+-----------------------------+             +-----------------------------+
| model.ShardsResponse        |             | Success, Result, Error      |
| model.GroupStatus           |             | Leader                      |
+-----------------------------+             +-----------------------------+

+-----------------------------+        +-----------------------------+
| dashboardOverview           |        | dashboardNode/dashboardGroup|
+-----------------------------+        +-----------------------------+
| Summary, Config, Shards     | -----> | health, leader, tables      |
| LockedShards, Nodes, Groups |        | group status and shard list |
+-----------------------------+        +-----------------------------+
```
  ],
  caption: [API 接入与 Dashboard 聚合类图],
)

#h2([3.3 流程图设计])
#h3([3.3.1 SQL API 接入流程])
#body-line([SQL API 的核心流程为：客户端发送 `POST /sql` 到 API Server；API Server 解码 `SQLRequest` 并调用 Coordinator；Coordinator 解析 SQL 并读取当前 `ClusterConfig`；若语句带主键条件，则通过 Router 定位目标 shard 与 group；若是 CREATE TABLE 则广播到所有 group；若是无主键 SELECT 或 JOIN，则 Scatter 到多个 group 后聚合结果；最后 API Server 将统一的 `SQLResponse` 返回客户端。])

#fig("78b5598871104db7ab8998a0b3b3bdcf.png", [SQL API 接入与路由流程图], width: 95%)

#h3([3.3.2 Dashboard 数据聚合流程])
#body-line([Dashboard 概览接口不直接读取数据库全量数据，而是轻量聚合控制面和节点健康信息。`/dashboard/api/overview` 首先读取当前 `ClusterConfig` 和锁定分片，再通过 etcd 获取在线节点；随后对每个节点执行 `/health` 探测，对 shard 节点额外读取 `/status` 获取 Leader 和表列表；最后按 GroupID 聚合节点与分片，计算 healthy、degraded、offline、migrating 等状态。])
#body-line([表数据浏览接口 `/dashboard/api/table-data?table=...` 使用 Coordinator 执行 `SELECT * FROM 表名`，由统一 SQL 路径完成跨分片查询，避免 Dashboard 自己重复实现路由和聚合逻辑。])

#h1([四．测试结果])
#h2([4.1 控制平面功能测试])
#h3([4.1.1 测试用例])
#body-line([控制平面相关测试主要覆盖以下场景：])
#body-line([（1）`TestMoveShard`：验证单个分片从源组移动到目标组后，`ClusterConfig` 中的分片归属正确更新。])
#body-line([（2）`TestRebalance`：验证给定新的 group 列表后，系统能够重新生成均衡的分片分配。])
#body-line([（3）`TestSharedStorePropagatesConfigAcrossServices`：验证多个 Controller Service 通过共享存储看到一致的配置更新。])
#body-line([（4）`TestShardLocks`：验证分片迁移锁能够阻止重复加锁，并在释放后允许后续操作。])
#body-line([（5）`TestFileStorePersistsConfig` 与 `TestEmbeddedClientSavesAndLoadsControllerConfig`：验证配置可以持久化到文件和 etcd，并在重启后加载。])
#h3([4.1.2 测试结果])
#body-line([项目测试已全部通过。结果说明控制平面对分片配置的合法性校验、版本递增、共享配置传播、分片锁保护和持久化恢复均符合预期；端到端测试中的 ShardDB move-shard 与 API Server restart 场景也验证了控制面配置在真实进程环境中的可恢复性。])

#h2([4.2 API 接入与 Dashboard 测试])
#h3([4.2.1 测试用例])
#body-line([API 接入相关测试主要覆盖以下场景：])
#body-line([（1）`TestNewHandlerHealthConfigAndControlOperations`：验证 `/health`、`/config`、`/shards`、`/groups`、`/move-shard` 与 `/rebalance` 的正常返回。])
#body-line([（2）`TestNewHandlerControlErrors`：验证非法方法、未知 shard、空 rebalance group 列表等错误路径。])
#body-line([（3）`TestNewHandlerSQL`：验证 `/sql` 能够调用统一 SQL Executor。])
#body-line([（4）`TestNewHandlerSQLReturnsRetryDuringShardMigration`：验证迁移冲突时返回 HTTP 503 并带 `Retry-After`。])
#body-line([（5）`TestNewHandlerDashboardRoutes` 与 `TestNewHandlerDashboardTableData`：验证 Dashboard 静态页面、概览接口和表数据接口。])
#body-line([（6）端到端测试中的 `TestShardMoveMigratesRowsAndUpdatesControlPlane`、`TestScatterSelectAndEqualityJoinAcrossShards`、`TestShardMoveReturnsRetryDuringMigration`：验证 API、Coordinator、Controller 与数据节点协作路径。])
#h3([4.2.2 测试结果])
#body-line([项目测试已全部通过。测试结果表明，API Server 能够正确处理 SQL 查询、控制面查询、分片迁移、再平衡和 Dashboard 访问；在迁移期间，冲突请求会得到明确的 503 与 Retry-After 反馈，避免客户端误以为写入已经提交。])

#h1([五．开发体会])
#body-line([（1）控制平面给我的主要启发是，分布式系统不只需要“能写入数据”，还需要有地方回答“数据现在应该在哪里”。`ClusterConfig` 看起来只是一个小型元数据结构，但它实际上决定了所有 SQL 请求的路径和迁移后的系统拓扑。])
#body-line([（2）API 接入层的设计更接近用户视角。底层可以有多个 Raft 组、多个 Leader、多个迁移状态，但用户仍然希望通过一个稳定入口发 SQL 或控制命令。因此，API Server 的工作重点不是执行所有逻辑，而是把复杂性收拢到统一协议和统一错误语义里。])
#body-line([（3）迁移锁的引入让我意识到，控制操作本身也会和业务请求竞争。MoveShard 如果只更新配置而不约束并发写入，很容易出现数据已经搬走但请求还写入旧组的情况；`503 + Retry-After` 虽然简单，却给客户端提供了清晰的重试边界。])
#body-line([（4）Dashboard 的价值不在于替代命令行，而在于降低观察系统状态的成本。拓扑、分片、锁、节点健康和表数据放在同一个页面后，演示时可以更直观地说明一次路由或迁移背后发生了什么。])
#body-line([（5）后续如果继续做控制面，我会优先补充两类能力：一是控制操作审计和权限校验，避免误操作；二是更细的迁移进度与失败恢复机制，让 MoveShard 不只是一个同步接口，而是可观察、可恢复的后台任务。])

#h1([参考文献])
#ref-line([［1］ etcd contributors. etcd Documentation: A distributed, reliable key-value store. https:／／etcd.io／docs／, 2024.])
#ref-line([［2］ Karger D, Lehman E, Leighton T, et al. Consistent hashing and random trees: Distributed caching protocols for relieving hot spots on the World Wide Web[C]. 见：STOC. 1997.])
#ref-line([［3］ HashiCorp. HashiCorp Raft: A Go implementation of the Raft consensus protocol. https:／／github.com／hashicorp／raft, 2024.])
#ref-line([［4］ PingCAP. TiKV Documentation: A Distributed Transactional Key-Value Database. https:／／tikv.org／docs／, 2024.])
