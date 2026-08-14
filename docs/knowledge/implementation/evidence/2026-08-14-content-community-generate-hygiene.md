---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 305ae40
commands:
  - make check
  - make test
  - bash scripts/generate.sh
result: passed
---

# 2026-08-14 生成卫生：make generate 不再留下死脚手架 + REL-A04 证据精确化

## 背景

- `gateway.api` 声明 `middleware: OptionalAuth`，goctl 每次 `make generate` 都会在
  `app/gateway/internal/middleware/` 重建空的 `OptionalAuthMiddleware` 桩
  （含误导性 TODO）；真实实现在 `pkg/middleware/optional_auth.go`，路由经
  `serverCtx.OptionalAuth` 装配。死桩此前已删除（790f8b6），但每次生成都会
  复活为未跟踪文件——工作树污染，且容易被误提交。
- 生成同步验证（本批）另确认：除 protoc 版本注释差异（本机 protoc 7.35 vs
  提交时 5.29，无语义变更）外，`make generate` 不产生任何 Go 差异，无生成漂移。

## 本批次改动

- `scripts/generate.sh`：gateway 生成后删除 goctl 重建的死桩
  optionalauth_middleware.go（位于 gateway 内部 middleware 目录，注释说明原因），
  保持生成后工作树干净。
- `docs/knowledge/implementation/IMP-content-community-backend.md`：REL-A04 行
  证据精确化——聚合 365 天 TTL 由 `daily_aggregates` 表 TTL 承担，指向
  `clickhouse_store_integration_test.go::TestClickHouseStoreAggregateDailyDedupesAndIsIdempotent`
  （重复执行幂等 + 365 天 TTL 断言）。

## 审查证据

- 重跑 `bash scripts/generate.sh`（goctl/protoc/grpc_tools）：生成后
  `git status` 无 optionalauth_middleware.go（死桩已被清理），仅剩 protoc 版本注释噪声
  （已还原，环境版本差异，非仓库漂移）。
- A 系列验收行全量核对：`IMP-content-community-backend.md` 中 CORE-A01~A06、
  DISC-A01~A06、ASST-A01~A04、REL-A01~A05 引用的测试文件/函数全部存在
  （仅 REL-A04 引用过泛，本批精确化）。
- `make check`：exit 0；`make test`：85 包 0 失败。

## 结果

- `bash scripts/generate.sh`：exit 0，工作树干净（仅环境 protoc 版本注释差异）。
- `make check` / `make test`：通过。

## 未覆盖边界

- 未在 CI 自动化「生成后 diff」检查；生成漂移仍依赖人工在 generate 后核对
  `git status`（AGENTS.md 规则）。
