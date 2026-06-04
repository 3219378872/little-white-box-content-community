---
title: testutil
tracks:
  - pkg/testutil/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# testutil

## 职责
集成测试的环境装配：基于 testcontainers 拉起真实依赖（MySQL/Redis/ClickHouse），
并提供 schema 路径定位，支撑「禁止 mock SqlConn、用真实数据库跑集成测试」的约定。

## 公开接口与契约
- `SetupTestEnv` / `SetupTestEnvM` — 通用集成测试环境装配（后者用于 `TestMain`）。
- `SetupClickHouseEnv` / `SetupClickHouseEnvM` — ClickHouse 专用装配。
- `SchemaPath` / `ClickHouseSchemaPath` — 定位 schema 文件。

## 上游
各服务的集成测试（`*_integration_test.go`）。

## 下游
testcontainers 运行时与对应数据库镜像。

## 关键文件
- `integration.go` — 通用装配。
- `clickhouse.go` — ClickHouse 装配。

## 注意事项与陷阱
- 依赖 Docker；CI 与本地无容器时集成测试应被跳过并在 verification 记录原因。
- 纯 SQL 断言型测试用 sqlmock，端到端用本包；不要 mock `sqlx.SqlConn`。
