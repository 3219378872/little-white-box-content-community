---
implementation: IMP-content-community-backend
verified_at: 2026-08-20
verified_commit: a1e03b6
commands:
  - go test -count=1 ./deploy/ -run 'TestLoki'
result: passed
---

# 2026-08-20 Loki 3.7 与 30 天 retention（REL-022）

## 缺陷

`deploy/docker-compose.middleware.yml` 使用 `grafana/loki:latest`。本机
`latest` 解析为 3.7.6，启动校验拒绝仓库 v11 / boltdb-shipper 配置，并要求
retention 开启时设置 `compactor.delete_request_store`。容器 `Exited (1)`
循环，Loki 从未就绪。

## 改动

- 镜像钉为 `grafana/loki:3.7.6`。
- `loki-config.yaml` 改为 schema v13 + tsdb，compactor 增加
  `delete_request_store: filesystem`；`retention_period: 720h` 不变。
- `log_retention_test.go` 断言镜像钉死、schema/tsdb 与 delete store。

## 结果

- `go test ./deploy/ -run TestLoki` 通过。
- 一次性容器挂载新配置后 `GET /ready` 返回 `ready`，buildinfo `3.7.6`。

## 未覆盖

- 本机旧 Loki 数据卷若残留失败启动文件，recreate 后仍需确认 `/ready`。
- Grafana 尚未配置 Loki 数据源；本次只保证进程能起且 retention 配置有效。
- ClickHouse `daily_aggregates` 旧卷缺表是独立问题，由工作区 `stack.sh`
  在中间件启动时重放 `xbh_analytics.sql`，不在本提交。
