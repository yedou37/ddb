# ShardDB Animation Design (AI Coding + Framework Rendering)

这份文档用于指导一个“技术可解释”的 ShardDB 动画项目：

- 不是 AI 一键生成视频
- 而是 AI 辅助编码
- 由可控框架渲染视频输出

目标风格：

- 简洁、科普、工程化
- 重点展示高层设计与关键机制
- 可与真实录屏拼接

## 1. 技术路线

首选方案：

- 框架：`Motion Canvas` (TypeScript)
- 素材：`Figma` 导出 SVG/PNG
- AI 用途：代码生成、动效脚本、分镜与旁白草稿
- 输出：1080p/30fps 或 4K/30fps

备选方案：

- `Remotion`：更偏 React 组件式视频
- `Manim`：更偏数学可视化（适合一致性哈希段落）

为什么首选 Motion Canvas：

- 对“节点、路径、状态切换、时间线”控制精确
- 用代码描述分片迁移逻辑更自然
- 后续改拓扑、改分片数时可复用

## 2. 视觉语义规范

### 2.1 基本对象

- `etcd`：圆柱体（2.5D），颜色 `#3BC9DB`
- `apiserver`：入口矩形，颜色 `#4C6FFF`
- `group`：大容器框，标题 `g1/g2/g3`
- `node`：小矩形，`leader` 外发光，`follower` 常规描边
- `shard`：编号卡片 `S0..S11`
- `request`：光点/数据包
- `hash ring`：圆环 + 落点标记

### 2.2 状态颜色

- 正常：`#2ECC71`
- 迁移中：`#F5A524`
- 故障：`#E74C3C`
- 离线：`#6B7280`
- 控制面信号：`#7C3AED`

### 2.3 运动语义

- 请求流：细发光线 + 匀速粒子
- 路由切换：旧路径淡出 + 新路径淡入
- shard 迁移：卡片沿贝塞尔曲线移动
- leader 切换：边框脉冲 + 标签切换

## 3. 动画信息架构

建议成片时长：`2.5 ~ 4 分钟`

### Scene 1: Cluster Overview (15s)

- 展示：`etcd`、`apiserver`、`g1/g2/g3`、每组 3 节点
- 目标：让观众先建立组件关系

### Scene 2: Write Path (25s)

- 展示：客户端 SQL -> `apiserver` -> 目标 group leader -> followers
- 目标：解释“写入是如何落地并复制的”

### Scene 3: Sharding Decision (25s)

- 展示：`id=101` -> `hash(id)` -> `shard S6` -> `group g2`
- 目标：解释分片不是随机分发

### Scene 4: Consistent Hash Ring (30s)

- 展示：哈希环、key 落点、顺时针命中 owner
- 目标：解释映射规则与稳定性

### Scene 5: Rebalance Trigger (20s)

- 展示：rebalance 前的分布不均，控制面触发平衡
- 目标：交代为什么要 rebalance

### Scene 6: Rebalance Migration (35s)

- 展示：`S4/S6/S9` 从旧组迁移到新组
- 同时展示：路由更新时机与迁移状态
- 目标：解释“迁移过程中的一致性保障”

### Scene 7: Failure And Recovery (25s)

- 展示：一个 follower 下线，组内仍可写；恢复后 catch-up
- 目标：解释容错语义

### Scene 8: Real System Proof (20s)

- 拼接：真实 dashboard/CLI 录屏
- 目标：证明动画对应真实实现

## 4. 数据驱动模型

建议将拓扑与分片状态写成 JSON，让动画从数据读取。

```json
{
  "groups": [
    {"id": "g1", "nodes": ["g1-n1", "g1-n2", "g1-n3"], "leader": "g1-n1"},
    {"id": "g2", "nodes": ["g2-n1", "g2-n2", "g2-n3"], "leader": "g2-n1"},
    {"id": "g3", "nodes": ["g3-n1", "g3-n2", "g3-n3"], "leader": "g3-n1"}
  ],
  "shards_before": {
    "S0": "g1", "S1": "g1", "S2": "g1", "S3": "g1",
    "S4": "g2", "S5": "g2", "S6": "g2", "S7": "g2",
    "S8": "g2", "S9": "g1", "S10": "g3", "S11": "g3"
  },
  "shards_after": {
    "S0": "g1", "S1": "g1", "S2": "g1", "S3": "g2",
    "S4": "g3", "S5": "g2", "S6": "g3", "S7": "g2",
    "S8": "g2", "S9": "g3", "S10": "g3", "S11": "g1"
  },
  "migrating": ["S4", "S6", "S9"]
}
```

迁移差异计算逻辑：

- 遍历 `shards_before`
- 如果 `before[shard] != after[shard]`，则加入迁移队列
- 为每个迁移 shard 生成：
  - `fromGroup`
  - `toGroup`
  - `startTime`
  - `duration`

## 5. 工程目录建议

```text
animation/
  README.md
  package.json
  src/
    main.ts
    project.ts
    theme/
      colors.ts
      typography.ts
    data/
      topology.json
      rebalance-case-a.json
    components/
      EtcdCylinder.tsx
      ApiServerBox.tsx
      GroupPanel.tsx
      NodeCard.tsx
      ShardCard.tsx
      HashRing.tsx
      FlowArrow.tsx
    scenes/
      S01_Overview.tsx
      S02_WritePath.tsx
      S03_Sharding.tsx
      S04_HashRing.tsx
      S05_RebalancePlan.tsx
      S06_RebalanceFlow.tsx
      S07_FailureRecovery.tsx
      S08_RealProof.tsx
```

说明：

- `components` 做可复用组件
- `scenes` 只编排镜头逻辑
- `data` 控制 topology 和 migration，避免硬编码

## 6. AI 辅助编码方式

推荐用法：

- 让 AI 一次只生成一个 Scene 或一个组件
- 不要一次生成整片动画
- 每次生成后先在本地渲染预览，再迭代

Prompt 模板（组件级）：

```text
请用 Motion Canvas (TypeScript) 写一个 GroupPanel 组件：
- 输入：groupId, nodes[], leaderId, position
- 展示：一个带标题的容器，里面是 3 个 NodeCard
- leader 节点边框高亮并带轻微呼吸动画
- 输出完整 TSX 代码
```

Prompt 模板（镜头级）：

```text
请写 Motion Canvas Scene：
- 名称：S06_RebalanceFlow
- 输入：shards_before/shards_after
- 自动找出迁移 shard
- 让迁移 shard 卡片沿曲线从 fromGroup 移动到 toGroup
- 迁移完成后切换路由箭头状态
- 输出可运行 TSX 代码，包含注释
```

## 7. 与录屏拼接策略

建议：

- 动画讲“原理和机制”
- 录屏讲“真实实现和证据”

拼接点：

- Scene 6 之后切一段真实 `control rebalance` 命令
- Scene 7 之后切一段 dashboard 节点离线/恢复画面

字幕策略：

- 每个 Scene 只保留 1 句核心字幕
- 术语固定，不要频繁换词

## 8. 里程碑计划

M1 (1 天)：

- 完成视觉规范与 Scene 草图
- 建立 Motion Canvas 工程

M2 (1~2 天)：

- 完成 Scene 1~4（架构、写路径、分片、哈希环）

M3 (1~2 天)：

- 完成 Scene 5~7（rebalance 与故障恢复）

M4 (0.5~1 天)：

- 拼接真实录屏
- 配音、字幕、导出最终版

## 9. 风险与规避

- 风险：动画很炫但语义不准确
  - 规避：使用数据驱动，不手工硬拖迁移路径
- 风险：信息量过大导致看不懂
  - 规避：每个 Scene 只讲一个因果
- 风险：录屏与动画口径不一致
  - 规避：以实际 `groups/shards/rebalance` 输出反推动画数据

## 10. 下一步

建议先做一个最小可运行版本：

- 仅实现 Scene 1 + Scene 3 + Scene 6
- 输出 45~60 秒样片
- 验证“能否讲清楚分片与 rebalance”

样片通过后再扩展完整成片。
