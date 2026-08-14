---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: e922dfb
commands:
  - python3 -m unittest scripts.test_spec_evals
  - make spec-evals-test
result: passed
---

# 2026-08-14 spec_evals CLI 子命令 dispatch 测试补齐（工具质量）

## 缺陷

`CLIDispatchTest` 只覆盖 `recommend`/`slo` 两个子命令的 dispatch；
`search`/`assistant` 子命令无测试——CLI 解析与数据集校验分支回归时
无测试保护。

## 改动

新增两个测试：
- `test_search_subcommand_rejects_invalid_qrels`：空/非法 qrels 在
  live 调用前被 `require_official_search` 拒绝，main 返回 1
  （DISC-060 守卫）。
- `test_assistant_subcommand_rejects_invalid_cases`：同上，ASST-050
  守卫返回 1。

## 结果

- `test_spec_evals` 28 项全过（原 26 + 新 2）；`make spec-evals-test`
  通过。

## 未覆盖边界

- live 执行分支（合法数据集 + 真实 Gateway）依赖运行中服务，属集成
  验证边界；外部输入门禁不变，见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
