---
implementation: IMP-content-community-backend
verified_at: 2026-08-13
verified_commit: 6d2c41e
commands:
  - make check
  - make test
  - make coverage
  - make integration-critical
  - make spec-evals-test
result: passed
---

# 2026-08-13 CORE-013/CORE-062 契约收敛（选项 B）验证

## 环境

- Go 1.26.1（GOTOOLCHAIN=go1.26.1）；分支 `task/core-v2-revision`，验证提交见 frontmatter
  （rebase 到 main 后 SHA 不变）；testcontainers 启动 MySQL 8.0 + Redis 7。

## 背景

人类于 2026-08-13 采纳 `../../proposals/PROP-20260813-core-revision-contract.md` 选项 B：
`/api/v2/post` 写接口强制 `expectedRevision`，`/api/v1` 维持迁移期语义并登记废弃计划。

## 本批次改动

- `app/gateway/gateway.api`：新增 `/api/v2/post` 写路由（create/update/delete）；
  `UpdatePostV2Req`/`DeletePostV2Req` 的 `expectedRevision` 为必填（无 `optional`）；
  v1 写路由加迁移期注释。
- goctl 重新生成网关产物（handler/types/routes），并删除生成的未使用
  `optionalauth_middleware.go` 脚手架（与 790f8b6 同因）。
- `internal/logic/posts/*_v2_logic.go`：Update/Delete v2 强制 `ExpectedRevision > 0`，
  否则 `ParamError`；创建 v2 契约与 v1 一致。
- 测试：REST 决策表新增 8 条 v2 用例（成功/缺失/0/409 冲突），success 计数 40→43；
  新增 v2 Logic 守卫单测。
- 守卫更新：`engineering-lint` 从 FORBIDDEN_ALIGNED_REQUIREMENTS 移除 CORE-013
  （人类决策已关闭该缺口），测试同步。
- 文档：DES 记录决策与废弃计划；IMP CORE-013 → aligned、CORE-062 补充说明；
  提案标记 closed。

## 结果

- `make check` 通过；`make test` 全部模块通过（含 race）。
- `make coverage` 通过：handler 89.3%（376/421）、logic 78.8%、model 12.2%。
- `make integration-critical`、`make spec-evals-test` 通过。

## 未覆盖边界

- `/api/v1` 帖子写接口迁移期结束时间表由客户端升级节奏决定（DES 废弃计划待执行）。
- 冻结评测集与月度 SLO 仍待人类/生产输入。
