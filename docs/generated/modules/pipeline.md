---
title: pipeline
tracks:
  - app/pipeline/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# pipeline

## 职责
行为日志数据管道。消费用户行为事件，经 Bloom Filter 去重后批量写入 ClickHouse，供后续分析/推荐使用。

## 公开接口与契约
- 入口：`behaviorlog/main.go` + `behaviorlog/internal/config/config.go`。
- 核心 Logic：`behaviorlog/internal/logic/record_behavior.go`。
- 消费 [event](event.md) 定义的行为载荷，经 [mqx](mqx.md) 投递。

## 上游
产生行为事件的服务（经 RocketMQ）。

## 下游
- ClickHouse（经 [clickhousex](clickhousex.md)）。
- Redis（Bloom Filter 去重状态）。

## 关键文件
- `behaviorlog/main.go` — 消费者入口。
- `behaviorlog/internal/logic/record_behavior.go` — 去重 + 落库。

## 注意事项与陷阱
- 去重基于 Bloom Filter（Redis）：Redis 故障时的降级行为已由测试覆盖，改动需保持去重持久化语义。
- 落 ClickHouse 为批量写入；集成测试依赖容器，无 Docker 时跳过并记录。
- 详见流程页 [behavior-log-pipeline](../flows/behavior-log-pipeline.md)。
