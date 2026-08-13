---
implementation: IMP-content-community-backend
verified_at: 2026-08-13
verified_commit: f7beca9
commands:
  - make check
  - make test
  - make coverage
  - make engineering-lint
  - docker run clickhouse + clickhouse-client（手动 SQL 验证）
result: passed
---

# 2026-08-13 REL-020 去标识聚合 365 天留存实现验证

## 环境

- Go 1.26.1（GOTOOLCHAIN=go1.26.1）；分支 `task/rel020-daily-aggregates`；
  ClickHouse 23.8.16（docker 手动容器 + `xbh_analytics.sql` 初始化脚本）。

## 本批次改动

- `deploy/sql/xbh_analytics.sql`：
  - 修复既有 schema 的 TTL 建表错误——`behavior_events`/`behavior_dead_letters` 的
    `received_at` 为 `DateTime64(3)`，ClickHouse 23.8 拒绝在 DateTime64 结果上建 TTL
    （`BAD_TTL_EXPRESSION`），导致 **schema 无法初始化、全部 ClickHouse 集成测试从未
    真正跑通**；改为 `TTL toDateTime(received_at) + INTERVAL ... DAY DELETE`。
  - 新增 `daily_aggregates`（Date 列 TTL 365 天，ReplacingMergeTree(aggregated_at) 幂等）。
- `app/pipeline/behaviorlog`：
  - `ClickHouseStore.AggregateDaily`：读取 `behavior_events FINAL`（按 event_id 收敛，
    避免 at-least-once 重复计数）聚合写 `daily_aggregates`；计数查询用 `FINAL`。
  - 定时任务 `runDailyAggregation`（启动立即执行一次 + 周期执行，`AggregateIntervalSeconds`
    默认 86400，`AggregateBackfillDays` 默认 1 可回填存量）；配置与 yaml 同步。
  - 单测：`aggregateWindow` 窗口计算（含回填/clamp）；Mock store 扩展接口。
  - 集成测试 `TestClickHouseStoreAggregateDailyDedupesAndIsIdempotent`。

## 结果（手动真实 ClickHouse 验证）

- schema 初始化：7 张表/视图全部创建成功（含 daily_aggregates）。
- 聚合 SQL：同 event_id 重投经 FINAL 收敛为 1；窗口过滤正确；两次聚合后
  `SELECT count() ... FINAL` 恒为 3（幂等不重复累计）。
- TTL：`create_table_query` 渲染为 `TTL date + toIntervalDay(365)`。
- `make check`/`make test`/`make coverage`/`engineering-lint` 全绿。

## 未覆盖边界

- 本机 testcontainers ClickHouse HTTP 等待策略确定性超时（5 分钟，既有问题，
  未改动的旧集成测试同样失败）；集成测试代码已就绪，在 harness 可用的环境执行。
- 冻结评测集与月度 SLO 仍待人类/生产输入。
