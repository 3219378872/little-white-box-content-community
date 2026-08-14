---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 2d7165e
commands:
  - make production-config PRODUCTION_ENV_FILE=/tmp/prod-env-test.env
result: passed
---

# 2026-08-14 production.env.example 变量补全与 DSN 可覆盖

## 缺陷

`deploy/production.env.example` 缺失 `CLICKHOUSE_DSN`（middleware compose
以 `${CLICKHOUSE_DSN:-http://clickhouse:8123/...}` 引用）与可选构建代理
变量（HTTP_PROXY/HTTPS_PROXY/NO_PROXY），运维配置生产环境时无记录可循；
production compose 的 `CLICKHOUSE_DSN` 为硬编码，无法环境覆盖。

## 改动

- `production.env.example`：补充 `CLICKHOUSE_DSN`（注明 middleware/
  production 默认 DSN）与可选代理变量。
- `docker-compose.production.yml`：`CLICKHOUSE_DSN` 改为
  `${CLICKHOUSE_DSN:-clickhouse://clickhouse:9000/xbh_analytics}`，
  默认值不变（native 协议，clickhouse-go/v2 正确）。

## 验证

- 变量对照：compose 引用的 55 个变量现全部在 example 中列出。
- `make production-config`（完整 env）通过。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
