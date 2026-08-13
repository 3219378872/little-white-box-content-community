---
title: esx knowledge governance
owner: human
status: approved
agent_write_policy: human-authorized
authorization_mode: conversation
protected_paths:
  - AGENTS.md
  - docs/INDEX.md
  - docs/knowledge/README.md
  - docs/knowledge/templates/
  - docs/knowledge/intent/
  - docs/knowledge/spec/
legacy_upstream:
  - AGENTS.md
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

本文件 frontmatter 的 `protected_paths` 是人类语义所有权范围，不是永久只读列表。agent 默认具备
编辑和维护能力，但每次新增、修改、删除或移动这些路径前，必须获得人类开发者授权。

- 当前任务或对话中的自然语言指令就是有效授权；文字指令或口头指令的转录均可，不需要在仓库中
  创建授权文件、签名、frontmatter 记录或额外审批流程。
- 授权只覆盖指令明确的目标、内容和完成该修改所必需的索引或元数据联动。范围不清、出现冲突或
  需要 agent 新增产品语义时，必须停止并请求人类决定。
- `owner: human` 表示意图、规范和治理规则的最终语义决定权属于人类，不表示文件只能由人类编辑。
- 人类明确表示接受、批准或要求发布正式内容后，获授权的 agent 可以创建、维护或提升对应
  `INT-*` / `SPEC-*`；单纯要求起草或讨论时保持 `draft`。

agent 无需额外授权即可维护 `design/`、`implementation/`、`proposals/` 和 `TRANSITION.md`。未获
授权的意图或规范建议只能进入[独立提案区](proposals/README.md)；提案不能作为正式上游。

## 加载与决策顺序

1. 从本页定位任务相关的已批准意图与规范，只读取所需页面。
2. 读取引用这些规范的活跃设计，再定位实现页、源码和证据。
3. 设计缺少已批准规范时，可以引用本页 `legacy_upstream` 中登记的过渡基线。
4. 正式上游缺失、相互冲突或无法给出可验证解释时，标记 `blocked` 并请求人类决定。
5. 人类发布或授权 agent 发布对应正式文档后，agent 才能用 `INT-*` / `SPEC-*` 引用替换过渡引用。

## 文档契约

- 正式 ID 使用稳定的 kebab-case：`INT-<slug>`、`SPEC-<slug>`、`DES-<slug>`、`IMP-<slug>`。
- 正式页面使用 `id`、`layer`、`title`、`status`、`owner`、`upstream` frontmatter。
- `owner` 记录语义所有权，不记录实际执笔者；经授权由 agent 编辑的意图和规范仍使用 `human`。
- 规范的 `upstream` 只能引用 `INT-*`，设计只能引用 `SPEC-*`，实现只能引用 `DES-*`。
- 过渡期设计可使用 `legacy_upstream`，值必须为 `legacy:<path>#<heading>`，且路径在本页白名单中。
- 实现页额外记录 `tracks`、`verified_at` 和 `verified_commit`；验证详情放在
  [implementation/evidence/](implementation/evidence/README.md)。
- 模板位于 `templates/`。README、模板和提案都不是正式项目要求。

旧文档迁移状态与后续待办见 [TRANSITION.md](TRANSITION.md)。
