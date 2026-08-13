---
implementation: IMP-content-community-backend
verified_at: 2026-08-13
verified_commit: dd77473
commands:
  - make check
  - make test
  - make coverage
  - make integration-critical
  - make spec-evals-test
result: passed
---

# 2026-08-13 共享可见性适配与官方评测集守卫验证

## 环境

- Go 1.26.1（GOTOOLCHAIN=go1.26.1）；分支 `task/spec-alignment-batch2`，验证提交 `dd77473`
  （rebase 到 main 后 SHA 不变）；testcontainers 启动 MySQL 8.0 + Redis 7。

## 本批次改动

- 新增 `app/content/visibility`：把 Content `GetPostsByIds` 适配为 `visibilityx.Fetcher`
  （nil client / RPC 错误 / nil 响应均 fail-closed）；替换 assistant / feed / recommend /
  search / gateway-favorites 五处重复的 `fetchContentPosts` 闭包。
- 评论列表：SQL `status=1` 过滤后再内存二次过滤并回减本页 Total（CORE-016 纵深防御，
  与帖子列表模式一致）。
- `scripts/spec_evals.py`：`require_official_search/assistant` 官方数据集守卫——拒绝
  `eval/dev/*` 及未声明 `frozen=true`、双评审者、数量/类型混合不达标的文件（DISC-060、
  ASST-050）；`test_spec_evals.py` 新增 6 个守卫测试。

## 结果

- `make check` 通过（fmt、engineering-lint、vet、golangci-lint 0 issues）。
- `make test` 全部模块通过（含 race）。
- `make coverage` 通过：handler 89.3%、logic 78.8%、model 12.2%、mq_consumer 74.4%、
  shared 45.9%。
- `make integration-critical` 通过；`make spec-evals-test` 通过（19 测试）。

## 未覆盖边界

- 冻结评测集（DISC-060~063 / ASST-050~051）与月度 SLO（REL-030~043）仍待人类/生产输入，
  见 [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
