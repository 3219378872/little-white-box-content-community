---
title: esx knowledge governance
owner: human
status: approved
protected_paths:
  - AGENTS.md
  - docs/INDEX.md
  - docs/knowledge/README.md
  - docs/knowledge/templates/
  - docs/knowledge/intent/
  - docs/knowledge/spec/
  - docs/active/
  - docs/DESIGN.md
  - docs/SECURITY.md
  - docs/RELIABILITY.md
  - docs/QUALITY_SCORE.md
legacy_upstream:
  - AGENTS.md
  - docs/active/api.md
  - docs/active/data.md
  - docs/active/operations.md
  - docs/active/rpc.md
  - docs/active/security.md
  - docs/active/testing.md
  - docs/DESIGN.md
  - docs/SECURITY.md
  - docs/RELIABILITY.md
  - docs/QUALITY_SCORE.md
---

# 四层知识总路由

正式知识只沿一个方向传递：

```text
意图（human） → 规范（human） → 设计（agent） → 实现（agent）
```

## 两类权威

- 规范权威回答“应该做什么”：已批准的[意图](intent/README.md)约束已批准的
  [规范](spec/README.md)，规范约束活跃[设计](design/README.md)。
- 事实权威回答“当前做了什么”：源码、配置、`.api`、`.proto` 和测试结果高于
  [实现说明](implementation/README.md)及其带日期证据。
- 当前代码不满足活跃设计时，实现状态必须为 `diverged`；任何下层都不能通过改写上层来消除偏离。

## 所有权

本文件 frontmatter 的 `protected_paths` 是人类维护面。本次引导提交完成后，agent 对这些路径只读，
不得新增、修改、删除或移动文件，也不得替人类把提案提升为正式内容。

agent 可以维护 `design/`、`implementation/`、`proposals/` 和 `TRANSITION.md`。对意图或规范的建议
只能进入[独立提案区](proposals/README.md)；提案不能作为设计或实现的正式上游。

## 加载与决策顺序

1. 从本页定位任务相关的已批准意图与规范，只读取所需页面。
2. 读取引用这些规范的活跃设计，再定位实现页、源码和证据。
3. 设计缺少已批准规范时，可以引用本页 `legacy_upstream` 中登记的只读过渡基线。
4. 正式上游缺失、相互冲突或无法给出可验证解释时，标记 `blocked` 并请求人类决定。
5. 人类发布对应正式文档后，agent 才能用 `INT-*` / `SPEC-*` 引用替换过渡引用。

## 文档契约

- 正式 ID 使用稳定的 kebab-case：`INT-<slug>`、`SPEC-<slug>`、`DES-<slug>`、`IMP-<slug>`。
- 正式页面使用 `id`、`layer`、`title`、`status`、`owner`、`upstream` frontmatter。
- 规范的 `upstream` 只能引用 `INT-*`，设计只能引用 `SPEC-*`，实现只能引用 `DES-*`。
- 过渡期设计可使用 `legacy_upstream`，值必须为 `legacy:<path>#<heading>`，且路径在本页白名单中。
- 实现页额外记录 `tracks`、`verified_at` 和 `verified_commit`；验证详情放在
  [implementation/evidence/](implementation/evidence/README.md)。
- 模板位于 `templates/`。README、模板和提案都不是正式项目要求。

旧文档的当前分类、已知漂移与后续迁移条件见 [TRANSITION.md](TRANSITION.md)。
