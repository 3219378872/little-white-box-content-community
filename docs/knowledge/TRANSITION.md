---
title: knowledge transition register
owner: agent
status: active
observed_at: 2026-08-12
observed_commit: 72576fd6863b40cd5e31913d0e9dbf80d2fdf638
---

# 知识迁移登记

本页只记录迁移状态，不是意图、规范或设计。

## 只读过渡基线

在正式人类层补齐前，下列现有文档继续提供人类确定的约束，但不再由 agent 更新：

- 根规则 `AGENTS.md`；
- `docs/active/` 下的 API、RPC、数据、安全、运维和测试速查；
- [设计原则](../DESIGN.md)、[安全](../SECURITY.md)、[可靠性](../RELIABILITY.md)和
  [质量标准](../QUALITY_SCORE.md)。

过渡引用必须指向知识总路由 frontmatter 中登记的路径和真实标题。正式 `INT-*` / `SPEC-*`
发布后，相同主题的过渡引用应由 agent 在设计层替换，不自动猜测两者等价。

## 旧实现快照

- [服务架构](../ARCHITECTURE.md)和 [generated 索引](../generated/INDEX.md)保留原路径，本阶段不移动或改写。
- 它们只用于定位，不能覆盖源码、配置、契约和测试。
- 2026-08-12 在上述提交运行可选 `python3 scripts/knowledge_base.py check`，已知缺少
  `app/assistant/`、`app/behavior/` 和 `pkg/outboxx/` 的模块页；该失败不是当前 CI 门禁。

## 本阶段边界

- 只建立目录、模板、路由和结构校验，不把现有内容语义迁移到四层。
- 本地未跟踪文件不属于知识链，迁移流程不得自动收集、移动、归档或纳入提交。
- 不恢复 Knowledge Base Sync、代码/文档 co-change 或 GitHub 人工审批门禁。

## 后续迁移条件

1. 人类创建并批准主题对应的意图与规范。
2. agent 基于已批准规范编写或迁移设计，明确取舍和失败模式。
3. agent 建立实现映射和带日期证据，并将偏离标记为 `diverged`。
4. 人类确认主题不再依赖过渡基线后，才从治理入口移除相应旧路径。
