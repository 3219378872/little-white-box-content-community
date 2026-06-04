# 数据基座实施 — 迭代主索引

> 父 spec：[2026-04-29-data-foundation-design.md](./2026-04-29-data-foundation-design.md)（已批准）
> 本文档：把父 spec 切分为可独立交付的迭代，每轮独立走 writing-plans → TDD → review。

## 一、背景与现状

父 spec 设计已完成且批准，但实施进度不均：

| Phase | 父 spec 范围 | 当前状态 |
|-------|------------|---------|
| Phase 1 | ClickHouse + behavior-log + Bloom 去重 | ✅ 代码完成，但 `xbh_analytics.sql` 与父 spec §4.1 偏离（`PARTITION BY cityHash64` / `ORDER BY event_id` / `AggregatingMergeTree`） |
| Phase 2 | L1 索引/向量/清理 | ⚠️ `search/mq` 仅骨架；`embedding-consumer`、`content-cleanup-consumer` 缺失；`feed/mq` 已存在但目录结构与新 L1 consumer 不对齐 |
| Phase 3 | L2 内容特征/热榜 | ❌ 未启动 |
| Phase 4 | 用户画像 + 特征版本 | ❌ 未启动 |
| Phase 5 | 漏斗管线（Search/Recommend RPC） | ❌ 未启动 |
| Phase 6 | 压测 / 灰度 / 告警 | ❌ 未启动 |

整体完成度约 25%。

## 二、关键决策

| 决策 | 选择 | 理由 |
|------|------|------|
| Schema 偏离处理 | 按 spec §4.1 修复 | 当前无真实数据沉淀，迁移成本最低；Phase 4 30min 聚合查询、Phase 6 Spark 时间范围扫描都依赖 spec 的 `ORDER BY (user_id, action, event_time, event_id)` |
| 迭代颗粒度 | 按父 spec Phase 切分 | 每轮 = 一个 Phase，独立 writing-plans + TDD + commit，与父 spec §10.1 对齐 |
| Embedding/Spark 占位 | NoopEmbedder + 跳过 Spark | 模型服务后续迭代补；规则模型先跑通漏斗 |
| feed-fanout-consumer | 重构对齐 | 与 search/embedding 等其他 L1 consumer 共用目录结构（`internal/mqs/*_consumer.go` + `internal/<store|indexer>/`） |

## 三、迭代序列

### Iter 0 — Schema 偏离修复（独立小迭代）

**目标**：让 ClickHouse schema 与父 spec §4.1 完全一致。

**范围**：
- `deploy/sql/xbh_analytics.sql` 改为父 spec §4.1：
  - `behavior_events`：`PARTITION BY toYYYYMMDD(event_time)`、`ORDER BY (user_id, action, event_time, event_id)`、`ReplacingMergeTree(event_time)`
  - `user_action_daily` MV：`SummingMergeTree() + count() AS cnt`
  - `behavior_events_by_time` MV：以 `event_time` 为首列 `ORDER BY`
- `app/pipeline/behaviorlog` 集成测试在新 schema 下绿
- 验证：`go test -race ./app/pipeline/... -run Integration`

**不在范围**：数据迁移（当前无真实数据沉淀）。

**估算**：0.5 天

---

### Iter 1 = Phase 2 — L1 索引/向量/清理

**目标**：完成所有 L1（数据落库）消费者。

**范围**：
- **search-index-consumer**
  - `app/search/mq/internal/indexer/elasticsearch_indexer.go`：接 ES 8.8（`go-elasticsearch/v8`）
  - 订阅 `post-create/update/delete`，按 `doc_id` upsert/delete
  - 索引 Mapping：title/body 用 IK 分词；tags/author_id keyword
- **embedding-consumer**（新建 `app/embedding/mq/`）
  - 订阅 `post-create/update/delete`
  - `Embedder` 接口 + `NoopEmbedder` 默认实现 + `MilvusWriter`
  - Milvus collection schema：post_id (pk) + embedding (float[256])
- **content-cleanup-consumer**（新建 `app/content/mq/cleanup/`）
  - 订阅 `post-delete`：清理 `post:{pid}:*`、`hot:posts ZSET`、`tag:{name}:posts`、Feed Redis
  - 父 spec §3.2 划在 L2，但因为它与 search-index/embedding 的 post-delete 处理同源（同一事件触发），统一在 Iter 1 落地以减少跨迭代上下文切换
- **feed-fanout-consumer 重构**
  - 重命名/移动到 `internal/mqs/post_publish_consumer.go` 已就位；补 `internal/fanout/store.go` 接口与 noop 测试夹具，与新 L1 consumer 目录结构对齐

**验收**：
- 单测 + testcontainers 集成测试（ES + Milvus + Redis）≥80% 覆盖
- 端到端：post-create MQ 消息 → ES 可查 + Milvus 可查 + Feed Redis 写入
- `go vet ./... && golangci-lint run`

**估算**：2 周

---

### Iter 2 = Phase 3 — L2 内容特征/热榜

**目标**：内容质量分 + 热度分 + 互动计数，Redis 快路径与 MySQL 慢路径隔离。

**范围**：
- **content-feature-consumer**：规则引擎打分 → Redis `post:{pid}:quality`
- **content-feature-mysql-consumer**：独立消费者写 MySQL `content_quality` 表
- **content-stat-consumer**：攒批 5s/100 条 → Redis `post:{pid}:stats` + 热榜 ZSET（含 HN 热度公式）
- **content-stat-mysql-consumer**：攒批 10s/200 条 → MySQL 宽表计数列
- **热榜 cron**：cron 每 10min / 1h 清理 `hot:posts:24h` / `hot:posts:7d` / `hot:tags:24h`，按 `ZREMRANGEBYSCORE` + `ZREMRANGEBYRANK` 截断到 Top 10000
- **MySQL schema**：新增 `deploy/sql/xbh_analytics.sql`（或新文件）`content_quality` 表

**验收**：
- testcontainers 验证 Redis 与 MySQL 双写最终一致
- 热度分公式与父 spec §4.3.2 一致
- 故障注入：MySQL 慢消费者卡住时 Redis 快路径不受影响

**估算**：1.5 周

---

### Iter 3 = Phase 4 — 用户画像 + 特征版本

**目标**：用户画像宽表 + 特征版本注册表。

**范围**：
- **user-feature-consumer**（替换 `app/recommend/mq` 的 NoopBehaviorStore）
  - 事件驱动：`SADD` session_tags + `EXPIRE 1800`，`LPUSH` recent_actions + 裁剪 50
  - 定时聚合（30min）：从 ClickHouse `user_action_daily` 聚合 → 写 MySQL `user_profile_wide.tag_weights`
- **MySQL schema**：新增 `user_profile_wide` 表（`xbh_user.sql` 或独立文件）+ `feature_version_registry` 表
- **Redis**：`feature:versions` HASH 初始化脚本
- 父 spec §6.3.1 的 `user_recent_actions` JSON 结构化格式

**验收**：
- 集成测试：MQ 消息 → Redis session_tags TTL 续期生效
- 集成测试：触发 30min 聚合 → MySQL 宽表 `tag_weights` 更新
- 特征版本切换：staging → active 流程跑通

**估算**：1.5 周

---

### Iter 4 = Phase 5 — 漏斗管线

**目标**：Search/Recommend RPC 漏斗端到端可用，规则模型先行。

**范围**：
- `proto/search/search.proto` → `goctl rpc protoc` 生成 `app/search/rpc/`
- `proto/recommend/recommend.proto` → 生成 `app/recommend/rpc/`
- 多路召回（goroutine 并行 ES + Milvus + Redis 热榜/标签）
- 粗排（质量过滤 + 时效衰减，纯内存）
- 精排（规则模型：标签匹配 + 热度加权，预留 `CTRModel` 接口）
- 混排（MMR 多样性 + 作者打散 + 已读去重）
- 降级三层：Redis 熔断器 + MySQL 50 QPS 限流 + 本地兜底缓存（每 5min 快照 Top-1000）
- 单路熔断 30s（go-zero `breaker`）
- 端到端延迟预算：典型 ≤320ms / P99 ≤500ms

**验收**：
- 端到端集成测试：Search/Recommend RPC 返回结构化结果
- 故障注入：单路 ES 超时 → 其他路补量
- 延迟基准：P99 ≤500ms（本地 docker 环境）

**估算**：3 周

---

### Iter 5 = Phase 6 — 压测 / 灰度 / 告警

**目标**：上线就绪。

**范围**：
- 全链路压测脚本（vegeta / k6）
- Prometheus 告警规则：消费者 lag、Redis 错误率、ClickHouse 写入失败率、P99 延迟
- Jaeger 链路覆盖审计（所有 zrpc、所有 MQ 消费者）
- Grafana dashboard：消费者 lag、热榜大小、特征命中率
- 灰度发布 runbook（Helm values + flag 切换）

**验收**：
- 压测报告：搜索 QPS、推荐 QPS、消费者吞吐
- 告警规则在 Prometheus 加载无错
- runbook 通过 review

**估算**：1.5 周

## 四、各轮 PR/commit 策略

每轮 = 独立 PR，commit 序列：

1. spec：本文档 + 父 spec（已就位）
2. plan：`docs/superpowers/plans/<date>-iter-N-<topic>.md`（由 writing-plans 产出）
3. infra：docker-compose / SQL / config 等基础设施
4. implementation：按 plan 切分的 N 个 commit，每个走 RED → GREEN → REFACTOR
5. tests：单测 + 集成测试达 ≥80%
6. review：用 superpowers:requesting-code-review

## 五、风险与缓解

| 风险 | 缓解 |
|------|------|
| ES/Milvus SDK 版本与容器版本不匹配 | Iter 1 起步先做依赖矩阵确认 |
| testcontainers 拉取 ES 8.8 镜像慢 | 预先 pull；CI 用 cache |
| Phase 4 30min 聚合查 ClickHouse 在压力下慢 | 父 spec §3.5 已说明窗口 ≤30min 可接受；监控加 ClickHouse 查询耗时 |
| Phase 5 漏斗规则模型质量低 | 父 spec §10.2 已声明降级方案；Iter 5 之前不交付推荐准确性 KPI |
| 多轮迭代上下文丢失 | 每轮 PR + 本索引文档持续更新进度章节 |

## 六、迭代进度（每轮完成时更新）

- [x] Iter 0 — Schema 偏离修复 (commit 9c5d03a)
- [ ] Iter 1 — Phase 2 L1 索引/向量/清理
- [ ] Iter 2 — Phase 3 L2 内容特征/热榜
- [ ] Iter 3 — Phase 4 用户画像 + 特征版本
- [ ] Iter 4 — Phase 5 漏斗管线
- [ ] Iter 5 — Phase 6 压测 / 灰度 / 告警

## 七、与其他文档的关系

- 父 spec：[2026-04-29-data-foundation-design.md](./2026-04-29-data-foundation-design.md)
- 阶段总览：[../../../doc/phases/README.md](../../../doc/phases/README.md)（Phase 3/4 文档与本迭代 Iter 4 一一对应）
- 测试标准：[2026-04-29-testing-standards-implementation-design.md](./2026-04-29-testing-standards-implementation-design.md)
- Phase 1 已实施 plan：[../plans/2026-04-29-phase1-clickhouse-behavior-log.md](../plans/2026-04-29-phase1-clickhouse-behavior-log.md)
