---
title: clickhousex
tracks:
  - pkg/clickhousex/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# clickhousex

## 职责
ClickHouse 客户端封装，供行为日志等分析型写入路径使用。

## 公开接口与契约
- `NewClient(...)` — 构造 ClickHouse 客户端（连接参数来自配置）。

## 上游
[pipeline](pipeline.md) 行为日志服务的 behavior store。

## 下游
ClickHouse 实例。

## 关键文件
- `client.go` — 客户端封装。
- `client_integration_test.go` — 基于 testcontainers 的集成测试。

## 注意事项与陷阱
- 集成测试依赖容器运行时；无 Docker 环境应跳过并在 verification 记录原因。
- 连接配置走 `etc/*.yaml` → `config.Config`，敏感值用环境变量占位。
