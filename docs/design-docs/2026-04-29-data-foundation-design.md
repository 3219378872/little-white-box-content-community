# 搜推系统 — 数据基座与管线设计

## 一、设计目标与范围

### 1.1 范围边界

- **Go 服务内闭环**：核心实时链路用 Go 实现，MQ 消费者驱动数据流转
- **ClickHouse 分析层**：全量行为事件落库，供特征聚合查询和 Spark 离线训练
- **Spark 可选协作**：离线模型训练通过 ClickHouse → Spark → MySQL/Milvus 路径接入，非系统依赖

### 1.2 核心决策

| 决策 | 选择 | 理由 |
|------|------|------|
| 管线拓扑 | 事件驱动链式管线 | 故障隔离、独立扩缩容、与现有 MQ 消费者架构一致 |
| 漏斗架构 | 多路召回→粗排→精排→混排 | 搜索和推荐共享框架，差异化配置 |
| 用户画像 | MySQL 宽表 + Redis 近期特征 | 宽表避免 JOIN，Redis 低延迟但可重建 |
| Embedding | MQ 异步生成 | 解耦发布链路，模型调用失败不影响帖子创建 |
| 离线计算 | Spark 读 ClickHouse | 存储即接口，Go 不直接依赖 Spark |

---

## 二、整体架构

```
┌─────────────────────────────────────────────────────────┐
│                      业务服务层                          │
│  Content RPC    Interaction RPC    User RPC             │
│  (发帖/编辑)     (点赞/收藏/评论)    (关注/画像)            │
└──────┬──────────────┬─────────────────┬─────────────────┘
       │ 发 MQ 消息    │ 发 MQ 消息       │ 发 MQ 消息
       ▼               ▼                  ▼
┌─────────────────────────────────────────────────────────┐
│              消息队列 (RocketMQ)                         │
│  post-create/update/delete  │  like/favorite/comment    │
│  user-follow/unfollow       │  user-behavior            │
└────┬──────────────┬─────────────────┬─────────────────┘
     │              │                  │
     ▼              ▼                  ▼
┌─────────────────────────────────────────────────────────┐
│                 第一层：数据落库消费者                     │
│                                                         │
│  search-index-consumer   behavior-log-consumer          │
│  → ES 全文索引            → ClickHouse 行为表            │
│  embedding-consumer                                     │
│  → Milvus 向量                                           │
│  feed-fanout-consumer                                   │
│  → Feed Redis                                           │
└────┬──────────────────────┬────────────────────────────┘
     │                      │
     ▼                      ▼
┌─────────────────────────────────────────────────────────┐
│                 第二层：特征加工消费者                     │
│                                                         │
│  content-feature-consumer    user-feature-consumer      │
│  → 内容质量分 (规则引擎)       → 用户近期行为序列          │
│  → Redis post:{pid}:quality  → Redis user:{uid}:*       │
│  content-feature-mysql-consumer                         │
│  → MySQL content_quality 表  (独立消费者，慢路径隔离)     │
│                                                         │
│  content-stat-consumer       content-stat-mysql-consumer│
│  → 帖子热度分 + 互动计数       → MySQL 宽表计数同步        │
│  → Redis post:{pid}:stats    (独立消费者，慢路径隔离)     │
│  → Redis hot:posts ZSET                                 │
│                                                         │
│  content-cleanup-consumer                               │
│  → 帖子删除时清理 Redis/Feed                             │
└────────────────────────┬────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│                    特征服务层                            │
│  Redis: 近期特征 + 热榜 + 计数     MySQL: 用户画像宽表     │
│  ES: 全文索引                     Milvus: 内容/用户向量   │
│  ClickHouse: 行为事件日志                                │
└────────────────────────┬────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────┐
│              漏斗服务层 (Search / Recommend RPC)          │
│  多路召回 (ES+Milvus+Redis) → 粗排 → 精排 → 混排        │
└─────────────────────────────────────────────────────────┘

离线协作层 (Spark，可选):
  ClickHouse ──(JDBC)──→ Spark 训练 ──→ 模型/向量 ──→ MySQL/Milvus/Redis
```

**关键约束：**
- 业务服务不直接写 ES/Milvus/ClickHouse，只发 MQ 消息
- 漏斗服务统一从存储层读取，不依赖 MQ
- 每个消费者只做一件事，管道串联通过"消费者产出写入存储"实现
- **慢路径隔离**：MySQL 写入独立为单独消费者（content-stat-mysql-consumer、content-feature-mysql-consumer），避免阻塞 Redis 快路径
- **删除清理**：content-cleanup-consumer 统一处理帖子删除后的 Redis、Feed 数据清理

---

## 三、MQ Topic 与消费者设计

### 3.1 Topic 规划

复用现有 `pkg/mqx/topics.go` 中已定义的 Topic，不新建。搜索推荐数据管线订阅以下 Topic：

| 管线 | Topic | 来源 | 消费者 |
|------|-------|------|--------|
| 内容 | post-create | Content RPC | search-index, embedding, feed-fanout, content-feature, content-feature-mysql |
| 内容 | post-update | Content RPC | search-index, embedding, content-feature, content-feature-mysql |
| 内容 | post-delete | Content RPC | search-index (delete), embedding (delete vector), content-cleanup |
| 行为 | like / unlike | Interaction RPC | behavior-log, content-stat, content-stat-mysql |
| 行为 | favorite / unfavorite | Interaction RPC | behavior-log, content-stat, content-stat-mysql |
| 行为 | comment-create | Content RPC | behavior-log, content-stat, content-stat-mysql |
| 行为 | user-follow / unfollow | User RPC | behavior-log, user-feature |

### 3.2 消费者清单

| 消费者 | 层 | 订阅 Topic | 写入目标 | 触发策略 |
|--------|-----|-----------|---------|---------|
| search-index-consumer | L1 | post-create/update/delete | ES | 事件驱动，逐条，upsert/delete 天然幂等 |
| embedding-consumer | L1 | post-create/update/delete | Milvus（调模型 gRPC） | 事件驱动，逐条（可攒批），upsert/delete 幂等 |
| feed-fanout-consumer | L1 | post-create | Feed Redis | 事件驱动，逐条 |
| behavior-log-consumer | L1 | like/favorite/comment/follow | ClickHouse | 事件驱动，逐条，基于 event_id 去重 |
| content-feature-consumer | L2 | post-create/update | Redis post:{pid}:quality | 事件驱动，逐条 |
| content-feature-mysql-consumer | L2 | post-create/update | MySQL content_quality 表 | 事件驱动，逐条，独立隔离慢写入 |
| content-stat-consumer | L2 | like/favorite/comment | Redis post:{pid}:stats, hot:posts ZSET | 批量攒批 (5s/100条) |
| content-stat-mysql-consumer | L2 | like/favorite/comment | MySQL 宽表计数列 | 批量攒批 (10s/200条)，独立隔离慢写入 |
| content-cleanup-consumer | L2 | post-delete | Redis post:{pid}:*, hot:posts ZSET, tag:{name}:posts, Feed Redis | 事件驱动，逐条 |
| user-feature-consumer | L2 | like/favorite/comment/follow | Redis user:{uid}:*, MySQL 宽表 | 事件+定时 (30min 聚合) |

### 3.3 消费者触发策略

- **事件驱动（逐条）**：行为日志（不可丢）、内容质量分（发布即可算）、搜索索引（实时性要求高）、删除清理（及时清除脏数据）
- **批量攒批**：互动计数 Redis（量大、逐条写 Redis 浪费连接）、热度分（攒批减少写入）、MySQL 计数同步（独立攒批，容忍更高延迟）
- **事件+定时混合**：用户特征（近期行为事件驱动追加，标签权重每 30 分钟从 ClickHouse 聚合刷新）

### 3.4 幂等性保障

| 消费者 | 幂等机制 | 说明 |
|--------|---------|------|
| search-index-consumer | ES upsert (index by doc_id) | 相同 post_id 覆盖写，天然幂等 |
| embedding-consumer | Milvus upsert by post_id | 相同 post_id 覆盖向量，天然幂等 |
| feed-fanout-consumer | Redis ZADD (score=timestamp) | 重复添加不会产生多条 |
| behavior-log-consumer | **event_id 去重** | 每条事件携带唯一 event_id（Snowflake ID），ClickHouse 表增加 event_id 列，使用 ReplacingMergeTree(event_time) 以 event_id 为排序键末列，后台 merge 自动去重；消费端额外维护 Redis Bloom Filter `bf:behavior_events` 做前置去重（TTL 48h） |
| content-feature-consumer | Redis SET 覆盖写 | 同一 post_id 重复计算覆盖旧值 |
| content-feature-mysql-consumer | MySQL INSERT ON DUPLICATE KEY UPDATE | 同一 post_id 覆盖写，天然幂等 |
| content-stat-consumer | Redis HINCRBY 可重入 + 攒批窗口内去重 | 攒批窗口内按 (event_id) 去重；窗口间重复依赖 behavior-log 侧去重 |
| content-stat-mysql-consumer | 同 content-stat-consumer | 攒批窗口内 event_id 去重 |
| content-cleanup-consumer | Redis DEL/ZREM 幂等 | 删除不存在的 key 无副作用 |
| user-feature-consumer | Redis LIST 追加 + 裁剪 | 重复追加后裁剪不影响正确性；30min 聚合为全量刷新 |

### 3.5 L1/L2 最终一致性约束

L2 消费者对 L1 产出存储的依赖关系及一致性窗口：

| L2 消费者 | 依赖的 L1 产出 | 一致性保证 | 说明 |
|-----------|---------------|-----------|------|
| content-feature-consumer | 无（直接从 MQ 消息体取帖子内容） | 无依赖 | 不读 ES/Milvus，仅用消息体中的标题、正文等字段 |
| content-feature-mysql-consumer | 无（直接从 MQ 消息体取帖子内容） | 无依赖 | 与 content-feature-consumer 相同输入，独立计算后写 MySQL |
| content-stat-consumer | 无（只做增量计数） | 无依赖 | 从 MQ 消息读事件，不依赖 ClickHouse |
| user-feature-consumer (事件驱动) | 无（从 MQ 消息追加 LIST） | 无依赖 | 追加操作不读 ClickHouse |
| user-feature-consumer (30min 聚合) | behavior-log-consumer → ClickHouse | **最终一致，窗口 ≤ 30min** | 聚合查询 ClickHouse 时，behavior-log 写入可能有秒级延迟；30min 的聚合周期远大于写入延迟，不构成问题 |

**设计原则**：L2 消费者的事件驱动路径全部从 MQ 消息体获取数据，不依赖 L1 的写入完成。仅定时聚合路径依赖 ClickHouse，此时 L1 写入延迟（通常 < 5s）远小于聚合周期（30min），一致性有保障。

---

## 四、存储层 Schema

### 4.1 ClickHouse — 行为事件日志

唯一职责：全量行为事件落库，供 Spark 读取做离线训练，也供 Go 消费者做聚合查询。

```sql
-- 主表：以 user_id 为首列，优化用户维度聚合
CREATE TABLE behavior_events (
    event_id    Int64,                    -- Snowflake ID，用于去重
    event_time  DateTime64(3),
    user_id     Int64,
    action      LowCardinality(String),   -- like/favorite/comment/view/follow/share
    target_id   Int64,                    -- 帖子ID/用户ID
    target_type LowCardinality(String),   -- post/user/tag
    duration    Int32 DEFAULT 0,          -- 浏览时长(ms)
    scene       String DEFAULT '',        -- home/discover/search
    client_ip   String DEFAULT ''
) ENGINE = ReplacingMergeTree(event_time)
PARTITION BY toYYYYMMDD(event_time)
ORDER BY (user_id, action, event_time, event_id);

-- 物化视图：用户行为每日聚合（SummingMergeTree 自动累加 cnt 列）
CREATE MATERIALIZED VIEW user_action_daily
ENGINE = SummingMergeTree()
ORDER BY (user_id, action, target_type, date)
AS SELECT
    toDate(event_time) AS date,
    user_id, action, target_type,
    count() AS cnt
FROM behavior_events
GROUP BY date, user_id, action, target_type;

-- 物化视图：时间范围查询优化（供 Spark 按时间窗口批量读取）
CREATE MATERIALIZED VIEW behavior_events_by_time
ENGINE = ReplacingMergeTree(event_time)
PARTITION BY toYYYYMMDD(event_time)
ORDER BY (event_time, user_id, event_id)
AS SELECT * FROM behavior_events;
```

**设计说明：**
- 主表 ORDER BY 以 `user_id` 为首列，优化"查某用户近期行为"的在线查询场景
- `behavior_events_by_time` 物化视图以 `event_time` 为首列，优化 Spark 按时间范围批量读取场景，避免全表扫描
- 使用 `ReplacingMergeTree(event_time)` 配合 `event_id` 实现去重：相同 `event_id` 的重复行在后台 merge 时自动保留最新一条
- `SummingMergeTree` 的 `cnt` 列为可累加指标，引擎在 merge 时自动求和

### 4.2 MySQL — 用户画像宽表

一行包含所有标签和统计，漏斗服务一次查询拿到完整画像，避免 JOIN。

```sql
CREATE TABLE user_profile_wide (
    user_id         BIGINT PRIMARY KEY,
    -- 基础属性
    nickname        VARCHAR(64),
    avatar_url      VARCHAR(255),
    level           INT DEFAULT 0,
    -- 兴趣标签 (Spark 离线产出 + 实时增量更新)
    interest_tags   JSON,           -- ["游戏","科技","动漫"]
    tag_weights     JSON,           -- {"游戏":0.8,"科技":0.5}
    -- 行为统计 (content-stat-mysql-consumer 实时更新)
    follower_count  INT DEFAULT 0,
    following_count INT DEFAULT 0,
    post_count      INT DEFAULT 0,
    like_count      INT DEFAULT 0,
    -- 离线特征 (Spark 周期性写入)
    quality_score   FLOAT DEFAULT 0,
    active_level    VARCHAR(16) DEFAULT 'low',  -- high/medium/low
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

**设计说明：**
- **不存 Embedding**：用户向量（128/256 维 float 数组）以 JSON 存储在 MySQL 中会产生大体积行，批量查询时显著增加 IO。用户向量已在 Milvus 中存储一份，在线使用通过 Redis 缓存获取。MySQL 宽表只保留标量特征，保持行小、查询快
- 用户向量的读写路径：Spark 产出 → 写 Milvus + 写 Redis 缓存；精排读 Redis 缓存 → miss 时查 Milvus

### 4.2.1 MySQL — 内容质量分表

内容质量分的持久化副本，作为 Redis 丢失后的重建数据源。

```sql
CREATE TABLE content_quality (
    post_id         BIGINT PRIMARY KEY,
    quality_score   FLOAT NOT NULL DEFAULT 0,
    quality_tags    JSON,             -- ["quality_ok"] 或 ["title_bait","low_quality"]
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
);
```

**设计说明：**
- **写入方**：content-feature-mysql-consumer，通过 `INSERT ON DUPLICATE KEY UPDATE` 幂等覆盖
- **读取方**：Redis 重建脚本（全量扫描 → 批量写回 Redis）；精排降级时作为 fallback 数据源
- **与 Redis 的关系**：Redis `post:{pid}:quality` 是热路径缓存，MySQL `content_quality` 是持久化源。正常链路只读 Redis，Redis 故障或数据丢失时从 MySQL 重建

### 4.3 Redis — 近期特征 + 热数据

只存"最近"和"最热"，不存全量，重启可重建。

**重建路径：**
| Redis Key | 重建数据源 | 重建方式 |
|-----------|-----------|---------|
| `post:{pid}:quality` | MySQL `content_quality` 表 | 全量扫描 → 批量 SET 写回 Redis |
| `post:{pid}:stats` | MySQL 宽表计数列 | 全量扫描 → 批量 HSET 写回 Redis |
| `hot:posts:*` | 从重建后的 `post:{pid}:stats` 重算热度分 | 遍历计算 heat_score → ZADD |
| `user:{uid}:recent_actions` | ClickHouse `behavior_events` | 按 user_id 查最近 50 条 → LPUSH |

```
# 用户近期特征 (TTL 7天)
user:{uid}:recent_actions    LIST    最近 50 条行为 {action, target_id, target_type, time}
user:{uid}:session_tags      SET     当前会话交互过的标签 (TTL 30min，见 4.3.1)
user:{uid}:embedding         STRING  用户向量 JSON (预计算缓存, TTL 24h)

# 内容特征
post:{pid}:stats             HASH    {likes, comments, favorites, heat_score}
post:{pid}:quality           STRING  质量分 + 质量标签 JSON

# 热榜 (ZSet, score=热度分)
hot:posts:24h                ZSET    24h 热门帖子 (定时清理，见 4.3.2)
hot:tags:24h                 ZSET    24h 热门标签 (定时清理)
hot:posts:7d                 ZSET    7天热门帖子 (定时清理)

# 标签-内容映射
tag:{name}:posts             ZSET    标签下的帖子 (按时间排序)

# 去重辅助
bf:behavior_events           BloomFilter  behavior-log-consumer 前置去重 (TTL 48h)
```

#### 4.3.1 会话定义与 session_tags 生命周期

- **会话定义**：用户连续交互期间为一个会话；以 **30 分钟无交互** 作为会话结束的判定依据
- **实现方式**：`user:{uid}:session_tags` 设置 TTL = 30min；每次用户产生交互行为时，user-feature-consumer 执行 `SADD` 后 `EXPIRE 1800`（续期）；30 分钟无交互后 key 自动过期，下次交互自动开启新会话
- **使用场景**：混排阶段用于多样性保障——当前会话已交互过的标签降权，避免同质化推荐

#### 4.3.2 热榜 ZSET 淘汰策略

ZSET 不会自动过期成员，需要主动清理：

| 热榜 Key | 清理策略 | 执行方式 |
|----------|---------|---------|
| hot:posts:24h | ZREMRANGEBYSCORE 移除 score 中时间因子 > 24h 的成员 | content-stat-consumer 每次攒批写入后执行；另有 cron 每 10min 兜底清理 |
| hot:posts:7d | 同上，阈值 7 天 | cron 每 1h 清理 |
| hot:tags:24h | 同上，阈值 24h | cron 每 10min 清理 |

**ZSET 大小上限**：每个热榜 ZSET 保留 Top 10000 成员；cron 清理时若成员数超限，执行 `ZREMRANGEBYRANK 0 -(N-10000)` 截断尾部。

**热度分计算公式**（HN 风格，内嵌时间因子）：

```
heat_score = (likes * 3 + favorites * 5 + comments * 2) / pow(hours_since_publish + 2, 1.5)
```

content-stat-consumer 每次更新 `post:{pid}:stats` 时重算 `heat_score`，同时 `ZADD` 到对应热榜 ZSET。

### 4.4 存储职责汇总

| 存储 | 存什么 | 写入方 | 读取方 |
|------|--------|--------|--------|
| ES | 帖子全文索引 | search-index-consumer | 搜索-文本召回 |
| Milvus | 内容向量 + 用户向量 | embedding-consumer / Spark | 搜索-向量召回 / 推荐-相似内容 / 精排(miss) |
| ClickHouse | 全量行为事件 | behavior-log-consumer | user-feature-consumer / Spark |
| MySQL | 用户画像宽表（标量特征）+ 内容质量分表 + 内容元数据 | content-stat-mysql-consumer / content-feature-mysql-consumer / user-feature / Spark | 精排 / 混排 / Redis 重建 |
| Redis | 近期特征 + 热榜 + 计数 + 用户向量缓存 | content-feature / content-stat / user-feature / Spark | 全漏斗阶段 |

---

## 五、特征工程

### 5.1 实时特征（Go 消费者，秒~分钟级 → Redis）

| 特征 | 生产者 | 计算方法 | 存储位置 |
|------|--------|---------|---------|
| 内容质量分 | content-feature-consumer (Redis) + content-feature-mysql-consumer (MySQL) | 规则引擎：标题长度、敏感词、图片数量、文本长度 | Redis post:{pid}:quality / MySQL content_quality |
| 帖子热度分 | content-stat-consumer | HN 热度算法：点赞/评论/收藏/时间衰减加权 | Redis post:{pid}:stats |
| 互动计数 | content-stat-consumer (Redis) + content-stat-mysql-consumer (MySQL) | 批量增量更新，Redis 快路径与 MySQL 慢路径隔离 | Redis post:{pid}:stats / MySQL |
| 用户近期行为 | user-feature-consumer | 追加 LIST，裁剪到 50 条 | Redis user:{uid}:recent_actions |
| 会话标签 | user-feature-consumer | 当前会话交互的帖子标签去重，TTL 30min 续期 | Redis user:{uid}:session_tags |
| 热门榜单 | content-stat-consumer | ZSet 按热度分排序，cron + 攒批后双重淘汰 | Redis hot:posts:24h/7d |

### 5.2 离线特征（Spark，小时~天级 → MySQL/Milvus/Redis）

| 特征 | 计算方法 | 存储位置 |
|------|---------|---------|
| 用户兴趣标签 + 权重 | TF-IDF / 协同过滤，基于 ClickHouse 行为表 | MySQL user_profile_wide.tag_weights |
| 用户向量 | 聚合用户交互过的内容向量 (加权平均) | Milvus user_vectors + Redis user:{uid}:embedding 缓存 |
| CTR 预估模型 | Spark MLlib 训练 LR/GBDT，导出 PMML/ONNX | Go 侧加载推理 |
| 内容聚类 | 对 Milvus 向量做 K-Means 聚类 | Redis tag:{name}:posts |

### 5.3 特征查询路径

漏斗服务查询特征时：
1. **先查 Redis** → 命中返回（近期特征、热度、计数、用户向量缓存）
2. **Redis miss (标量特征)** → 查 MySQL 宽表（全量画像、长期特征）
3. **Redis miss (用户向量)** → 查 Milvus → 回填 Redis 缓存（TTL 24h）
4. 合并 Redis 实时特征 + MySQL/Milvus 离线特征 → 输入精排模型

---

## 六、与算法组协作的特征 Schema 规范

本节定义特征的数据契约，作为 Go 工程侧（特征生产）和算法侧（模型训练/推理）的接口规范。

### 6.1 通用约定

- **特征名**：`{domain}_{name}` 格式，如 `user_interest_tags`、`post_heat_score`
- **类型**：float64 / int64 / string / float64[] / string[]
- **缺失值**：float 填 0.0，int 填 0，string 填 ""，数组填空数组
- **版本**：特征表包含 `_v{N}` 后缀表示版本，如 `user_interest_tags_v2`
- **来源标注**：online（Go 实时）/ offline（Spark 离线）/ hybrid（双源合并）

### 6.2 特征版本管理

特征变更需要工程侧与算法侧协调，通过版本注册表管理：

```
# Redis 特征版本注册表
feature:versions    HASH    {feature_name: active_version}
                            例: {"user_interest_tags": "v2", "user_embedding": "v1"}

# 版本元信息 (MySQL)
CREATE TABLE feature_version_registry (
    feature_name    VARCHAR(128) NOT NULL,
    version         VARCHAR(16) NOT NULL,
    status          ENUM('staging', 'active', 'deprecated') DEFAULT 'staging',
    schema_desc     JSON,           -- 字段类型、维度等元信息
    created_by      VARCHAR(64),    -- spark / online
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    activated_at    DATETIME,
    PRIMARY KEY (feature_name, version)
);
```

**版本切换流程**：
1. Spark 产出新版本特征，写入 MySQL/Redis 时使用新版本 key（如 `user_interest_tags_v2`），status = staging
2. 算法侧用新版本特征训练模型，验证效果
3. 确认后，更新 `feature:versions` 中对应特征的 active_version → v2，status = active
4. 在线服务读取特征时，先查 `feature:versions` 获取 active_version，再按版本读取
5. 旧版本标记为 deprecated，保留 7 天后清理

### 6.3 用户特征 (User Features)

| 特征名 | 类型 | 维度 | 来源 | 更新频率 | 说明 |
|--------|------|------|------|---------|------|
| user_id | int64 | 1 | online | - | 用户唯一标识 |
| user_level | int64 | 1 | online | 实时 | 用户等级 |
| user_active_level | string | 1 | offline | 天级 | high/medium/low |
| user_quality_score | float64 | 1 | offline | 天级 | 用户质量分 (0-1) |
| user_interest_tags | string[] | ≤20 | hybrid | 天级+实时 | 兴趣标签列表 |
| user_tag_weights | float64[] | ≤20 | offline | 天级 | 标签权重，与 interest_tags 对齐 |
| user_embedding | float64[] | 128/256 | offline | 天级 | 用户向量（存储在 Milvus + Redis 缓存） |
| user_follower_count | int64 | 1 | online | 实时 | 粉丝数 |
| user_following_count | int64 | 1 | online | 实时 | 关注数 |
| user_post_count | int64 | 1 | online | 实时 | 发帖数 |
| user_like_count | int64 | 1 | online | 实时 | 累计获赞数 |
| user_recent_actions | json[] | ≤50 | online | 实时 | 最近 N 条行为，结构化 JSON 数组（见 6.3.1） |
| user_session_tags | string[] | ≤10 | online | 实时 | 当前会话交互标签 |

#### 6.3.1 user_recent_actions 格式

采用结构化 JSON 数组，便于算法侧直接解析，避免字符串拼接的解析成本：

```json
[
  {"action": "like", "target_type": "post", "target_id": 123456, "ts": 1714300000},
  {"action": "favorite", "target_type": "post", "target_id": 789012, "ts": 1714299000}
]
```

训练样本阶段 Spark 可直接 `explode` JSON 数组展开为行，无需正则解析。

### 6.4 内容特征 (Content/Post Features)

| 特征名 | 类型 | 维度 | 来源 | 更新频率 | 说明 |
|--------|------|------|------|---------|------|
| post_id | int64 | 1 | online | - | 帖子唯一标识 |
| author_id | int64 | 1 | online | - | 作者用户 ID |
| post_created_at | int64 | 1 | online | - | 发布时间 (Unix ms) |
| post_title_length | int64 | 1 | online | 实时 | 标题字符数 |
| post_body_length | int64 | 1 | online | 实时 | 正文字符数 |
| post_image_count | int64 | 1 | online | 实时 | 图片数量 |
| post_tags | string[] | ≤10 | online | 实时 | 帖子标签列表 |
| post_quality_score | float64 | 1 | online | 实时 | 质量分 (0-1)，规则引擎 |
| post_quality_tags | string[] | ≤5 | online | 实时 | 质量标签: title_bait/low_quality/quality_ok |
| post_heat_score | float64 | 1 | online | 实时 | 热度分 (HN算法) |
| post_like_count | int64 | 1 | online | 实时 | 点赞数 |
| post_comment_count | int64 | 1 | online | 实时 | 评论数 |
| post_favorite_count | int64 | 1 | online | 实时 | 收藏数 |
| post_embedding | float64[] | 128/256 | online | 发布时 | 内容向量 (text2vec) |

### 6.5 上下文特征 (Context Features)

| 特征名 | 类型 | 维度 | 来源 | 说明 |
|--------|------|------|------|------|
| ctx_hour_of_day | int64 | 1 | online | 请求小时 (0-23) |
| ctx_day_of_week | int64 | 1 | online | 请求星期 (0-6) |
| ctx_scene | string | 1 | online | home/discover/search |
| ctx_request_page | int64 | 1 | online | 请求页码 |
| ctx_recall_channel | string | 1 | online | 召回通道: es/vector/hot/tag/cf |

### 6.6 交叉特征 (Cross Features)

交叉特征由推荐服务在推理时实时计算，不预存。

| 特征名 | 类型 | 计算方法 | 说明 |
|--------|------|---------|------|
| cross_user_tag_match | float64 | user_tag_weights ∩ post_tags 的加权匹配度 | 用户-内容标签匹配 |
| cross_user_author_follow | int64 | 0/1，用户是否关注作者 | 社交关系 |
| cross_time_decay | float64 | exp(-λ * hours_since_publish) | 时间衰减因子 |
| cross_ctr_bucket | string | 用户历史 CTR 分桶: high/medium/low/cold | CTR 分层 |

### 6.7 特征存储与交付

```
训练样本交付:
  Spark 从 ClickHouse 读取行为日志（使用 behavior_events_by_time 视图按时间范围高效扫描）
  → 关联 MySQL 用户画像 + 内容特征
  → 生成训练样本 (label + 全量特征) → 保存为 Parquet (HDFS/S3)

在线推理交付:
  漏斗服务精排阶段:
    1. 查 Redis feature:versions 获取各特征 active_version
    2. 批量读取 Redis post:{pid}:stats + post:{pid}:quality
    3. 读取 Redis user:{uid}:recent_actions + session_tags
    4. 读取 Redis user:{uid}:embedding → miss 时查 Milvus 并回填
    5. 读取 MySQL user_profile_wide (标量画像)
    6. 实时计算交叉特征
    7. 组装特征向量 → 输入模型 → 得分
```

---

## 七、统一漏斗管线

搜索和推荐共享漏斗框架，通过配置差异化召回路数和精排权重。

### 7.1 四阶段漏斗

| 阶段 | 输入→输出 | 阶段超时 | 核心逻辑 | 数据来源 |
|------|----------|---------|---------|---------|
| **多路召回** | → ~5000 | 100ms | goroutine 并行，每路 Top-N，合并去重 | ES + Milvus + Redis |
| **粗排** | 5000 → ~500 | 20ms | 质量过滤 + 时效衰减 + 去重（纯内存） | Redis (预取，与召回并行) |
| **精排** | 500 → ~50 | 150ms | 多目标打分（CTR+互动+时长）加权融合 | Redis + Milvus + MySQL |
| **混排** | 50 → 20 | 20ms | MMR 多样性 + 作者打散 + 已读去重 | Redis user:{uid}:* |

### 7.2 端到端延迟预算

总超时上限 **500ms**，各阶段含网络开销与序列化的完整预算：

| 阶段 | 计算耗时 | 网络/IO | 合计 | 并行优化 |
|------|---------|---------|------|---------|
| 特征预取 | - | 30ms | 30ms | **与多路召回并行执行**，不占串行预算 |
| 多路召回 | 20ms 合并去重 | 80ms (ES/Milvus/Redis 并行最慢者) | 100ms | goroutine 并行，取最慢路 |
| 粗排 | 15ms 内存排序 | 5ms (Redis 预取已完成) | 20ms | 预取数据在召回阶段已就绪 |
| 精排 | 80ms 模型推理 | 70ms (Redis batch GET + MySQL 查询 + Milvus miss) | 150ms | Redis pipeline 批量读取 |
| 混排 | 15ms | 5ms (Redis 已读列表) | 20ms | - |
| 序列化/框架开销 | - | - | 30ms | - |
| **端到端合计** | - | - | **≤ 320ms (典型) / ≤ 500ms (P99)** | - |

**预取并行策略**：在召回阶段启动 goroutine 并行预取精排所需的 Redis 特征（post:stats、post:quality 按热榜 Top-N 预加载），召回完成后精排阶段只需补取未命中的条目。

**关键假设**：
- Redis 单次 Pipeline (100 key): ~3ms
- MySQL 单次查询 (user_profile_wide by PK): ~5ms
- Milvus ANN 检索 (Top-500): ~30ms
- ES 全文检索: ~50ms

### 7.3 搜索 vs 推荐差异化

| 阶段 | 搜索模式 | 推荐模式 |
|------|---------|---------|
| 多路召回 | ES + Milvus + 热门 (3路) | 协同过滤 + 内容相似 + 热门 + 新内容 + 标签 (5路) |
| 粗排 | 质量 + 时效 + 关键词匹配度 | 质量 + 时效 + 去重 |
| 精排 | 文本相关性权重 > 热度权重 | CTR 预估 + 兴趣匹配 > 热度 |
| 混排 | 相同逻辑 | 相同逻辑 |

### 7.4 降级策略

| 故障场景 | 降级行为 | 限流/熔断 |
|---------|---------|----------|
| 单路召回超时/失败 | 跳过该路，用已成功的结果继续 | 该路连续失败 3 次 → 熔断 30s |
| 精排模型加载失败 | 降级为规则打分（标签匹配 + 热度加权） | - |
| **Redis 不可用** | **不无条件 fallback MySQL**；启用熔断器，仅放行 10% 请求查 MySQL（含 content_quality 表），其余返回降级结果（热门缓存兜底） | 熔断器半开状态每 5s 试探 Redis 恢复；MySQL 并发限制 50 QPS |
| MySQL 不可用 | 精排仅用 Redis 特征（缺少长期画像），质量下降但可用 | - |
| Milvus 不可用 | 向量召回路跳过，其他召回路补量 | - |
| Spark 未接入 | 精排用规则模型，推荐仍可用 | - |

**Redis 故障详细说明**：Redis 承载全漏斗特征预取，故障时如果 500 个候选帖子特征全部 fallback 到 MySQL，瞬时 QPS 会从正常的 ~50 飙升到 ~5000，极易引发 MySQL 级联故障。因此采用"熔断 + 限流 + 兜底缓存"三层防护：
1. **熔断器**（go-zero breaker）：Redis 错误率 > 50% 时触发熔断，阻止请求打到 Redis
2. **MySQL 限流**：熔断期间仅允许 50 QPS 穿透到 MySQL，超出部分使用本地缓存兜底
3. **本地兜底缓存**：每 5 分钟从 Redis 快照 Top-1000 热门帖子特征到本地内存（sync.Map），Redis 故障时用于粗排和降级推荐

---

## 八、Embedding 向量生产

### 8.1 流程

```
帖子发布 → Content RPC 发 MQ → embedding-consumer 消费
  → 调用 Embedding gRPC 服务 (Python/sentence-transformers)
  → 写入 Milvus collection

帖子删除 → Content RPC 发 MQ → embedding-consumer 消费
  → 删除 Milvus collection 中对应向量
```

### 8.2 设计要点

- **异步解耦**：帖子创建不等待向量生成完成
- **失败重试**：MQ 消费者消费失败 → ConsumeRetryLater，天然重试
- **编辑同步**：post-update 触发重新生成向量，覆盖旧值（Milvus upsert 幂等）
- **删除清理**：post-delete 触发删除 Milvus 中对应 post_id 的向量
- **模型独立部署**：Embedding 模型部署为独立 Python/gRPC 服务，Go 侧通过 gRPC 调用
- **内容质量分使用规则引擎**：初期简单规则（标题长度、敏感词、图片数量），`QualityScorer` 接口预留后续切换为 NLP/多模态模型

```go
// 内容质量分接口（可替换实现）
type QualityScorer interface {
    Score(ctx context.Context, post PostContent) (float64, []string)
}

// 初期：规则引擎
type RuleBasedScorer struct{}

// 后期：模型
type ModelBasedScorer struct {
    client nlppb.NLPServiceClient
}
```

---

## 九、Spark 离线协作接口

### 9.1 数据流

```
ClickHouse ──(JDBC)──→ Spark 训练 ──→ 模型/特征 ──→ MySQL / Milvus / Redis
      │                    │
      │ (读 behavior_      ├── 用户兴趣标签 → MySQL user_profile_wide
      │  events_by_time    ├── 用户向量 → Milvus + Redis 缓存
      │  物化视图)          ├── CTR 模型 → PMML 文件 → Go 加载
                           └── 内容聚类 → Redis tag:{name}:posts
```

### 9.2 接口契约

| 接口 | 方向 | 格式 | 说明 |
|------|------|------|------|
| ClickHouse 读取 | Spark ← Go 管线 | JDBC (clickhouse-jdbc) | Spark 读 `behavior_events_by_time` 视图，按时间范围高效扫描 |
| MySQL 宽表写入 | Spark → Go 服务 | JDBC UPDATE | 标签/权重写 user_profile_wide 对应列（不写 embedding） |
| Milvus 向量写入 | Spark → Go 服务 | pymilvus / milvus-sdk-go | 产出向量写 Milvus collection |
| Redis 向量缓存 | Spark → Go 服务 | Go RPC (UpdateUserEmbedding) | 向量写入 Milvus 后同步刷新 Redis user:{uid}:embedding |
| 模型文件交付 | Spark → Go 服务 | PMML / ONNX 文件 | Go 侧加载用于精排推理 |
| Redis 缓存刷新 | Spark → Go 服务 | Go RPC (UpdateUserProfile) | 训练完成后触发画像缓存更新 |
| 特征版本注册 | Spark → MySQL | INSERT/UPDATE feature_version_registry | 新版本特征产出后注册 staging 版本 |

### 9.3 设计原则

- **存储即接口**：Spark 和 Go 不直接 RPC 通信，通过 ClickHouse/MySQL/Milvus 作为数据契约
- **Go 可独立运行**：Spark 未接入时，精排用规则模型，系统降级但不瘫痪
- **模型热替换**：CTRModel 接口统一规则模型和 PMML 模型，无需改调用方
- **Spark 读优化**：Spark 读 ClickHouse 时使用 `behavior_events_by_time` 视图（以 event_time 为首列排序），避免全表扫描

---

## 十、实施建议

### 10.1 分阶段交付

| 阶段 | 内容 | 开发周期 | 联调/压测 |
|------|------|---------|----------|
| Phase 1 | ClickHouse 部署 + behavior-log-consumer 实现 + behavior_events 表 + 物化视图 + Bloom Filter 去重 | 1 周 | 0.5 周 |
| Phase 2 | search-index-consumer (ES) + embedding-consumer (Milvus) + content-cleanup-consumer | 1.5 周 | 0.5 周 |
| Phase 3 | content-feature-consumer (质量分规则引擎→Redis) + content-feature-mysql-consumer (质量分→MySQL) + content-stat-consumer (热度+计数 Redis) + content-stat-mysql-consumer + 热榜清理 cron | 1 周 | 0.5 周 |
| Phase 4 | user-feature-consumer (近期行为+标签+会话) + MySQL 画像宽表 + 特征版本注册表 | 1 周 | 0.5 周 |
| Phase 5 | 漏斗管线实现（召回→粗排→精排→混排）+ Search/Recommend RPC 对接 + 降级/熔断策略 | 2 周 | 1 周 |
| Phase 6 | 全链路压测 + 灰度发布 + 监控告警配置 | - | 1.5 周 |
| **合计** | | **6.5 周** | **4.5 周** |

**总周期预估：~11 周**（含开发 6.5 周 + 联调压测灰度 4.5 周）

### 10.2 技术风险

| 风险 | 缓解 |
|------|------|
| ClickHouse 新增组件运维 | Docker Compose 单节点起步，后续可扩集群 |
| RocketMQ Go SDK 稳定性 | 已封装 `pkg/mqx`，消费者有重试机制 |
| Embedding 模型延迟 | 异步生成 + 攒批优化 |
| Spark 接入周期长 | Go 侧先跑通规则模型，Spark 按需引入 |
| Redis 大面积故障 | 熔断器 + MySQL 限流 + 本地兜底缓存三层防护 |
| 行为事件重复消费 | ReplacingMergeTree + Bloom Filter 双重去重 |
| 特征版本不一致 | 版本注册表 + staging/active 状态机，训练和在线统一读 active 版本 |
| 热榜 ZSET 膨胀 | 攒批后清理 + cron 兜底 + 成员数上限截断 |
