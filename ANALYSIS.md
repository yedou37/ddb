# 参考仓库分析

## 1. 项目概述

**项目名称**: Distributed-MySQL  
**项目性质**: 浙江大学 大规模信息系统构建技术导论 课程项目  
**技术栈**: Java + ZooKeeper + MySQL  

## 2. 项目架构

这是一个基于 **主从架构 (Master-Slave)** 的分布式数据库系统，主要包含四个核心模块：

### 2.1 模块划分

```
Distributed-MySQL/
├── MasterServer/          # 主服务器模块
├── RegionServer/          # 区域服务器模块（从节点）
├── Client/                # 客户端模块
└── miniSQL/               # 单机 SQL 引擎（基于 miniSQL 项目）
```

### 2.2 核心功能模块

#### MasterServer（主服务器）
- 负责集群管理和元数据管理
- 监听端口：12345
- 使用 ZooKeeper 进行服务发现
- 管理 RegionServer 的注册和发现

#### RegionServer（区域服务器）
- 实际的数据存储节点
- 监听客户端请求端口：22222
- RegionServer 间通信端口：22223
- 包含 DatabaseManager 用于与底层 MySQL 交互
- 通过 ZooKeeper 向 MasterServer 注册

#### Client（客户端）
- 提供用户交互接口
- 连接 MasterServer 进行元数据查询
- 连接 RegionServer 进行数据操作

#### miniSQL
- 单机 SQL 引擎，提供：
  - SQL 解释器
  - 缓冲区管理
  - 目录管理
  - 索引管理
  - 记录管理

### 2.3 通信机制

| 通信路径 | 协议/方式 | 端口 |
|---------|----------|------|
| Client → MasterServer | Socket | 12345 |
| Client → RegionServer | Socket | 22222 |
| RegionServer → RegionServer | Socket | 22223 |
| RegionServer ↔ ZooKeeper | ZooKeeper 协议 | 2181 |
| MasterServer ↔ ZooKeeper | ZooKeeper 协议 | 2181 |

## 3. 能完成的任务

1. **分布式数据存储**：数据分布在多个 RegionServer 上
2. **服务发现**：通过 ZooKeeper 实现动态的服务注册和发现
3. **SQL 查询**：支持基本的 SQL 操作（基于 miniSQL）
4. **集群管理**：MasterServer 管理整个集群的状态

## 4. 架构优缺点分析

### 优点
- 架构清晰，职责分离明确
- 利用 ZooKeeper 简化了服务发现
- 基于成熟的 miniSQL 项目，降低了开发难度

### 缺点
- 主从架构存在单点故障（MasterServer）
- 没有明确的一致性保证机制
- 依赖 MySQL 作为底层存储，增加了部署复杂度
