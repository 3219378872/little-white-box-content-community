# 文档重组与 zero-powers 内联化迁移设计

**日期**：2026-06-04
**分支**：`feat/slim-CLAUDE.md`
**范围**：文档目录重组 + zero-powers 插件内容内联化 + CLAUDE.md/AGENTS.md 重写 + 插件卸载

---

## 1. 目标

1. 将项目文档组织为 `design-docs/`、`exec-plans/`、`references/`、`generated/` 的标准结构
2. 将 zero-powers 插件（v0.1.3）的全部有效内容迁移到本地 `docs/references/`，完全解除对插件的依赖
3. 重写 CLAUDE.md / AGENTS.md，去除所有 `zero-skills/`、`zero-powers:` 引用，改为本地路径
4. 卸载 zero-powers 插件

## 2. 目标目录结构

```
esx/
├── AGENTS.md
├── CLAUDE.md
├── docs/
│   ├── ARCHITECTURE.md
│   ├── DESIGN.md
│   ├── SECURITY.md
│   ├── RELIABILITY.md
│   ├── QUALITY_SCORE.md
│   │
│   ├── design-docs/
│   │   ├── index.md
│   │   └── (从 superpowers/specs/ 迁入的 20 个设计文档)
│   │
│   ├── exec-plans/
│   │   ├── active/
│   │   ├── completed/
│   │   │   ├── phases/        (从 docs/phases/ 迁入)
│   │   │   └── (从 superpowers/plans/ 迁入的 30 个计划)
│   │   └── tech-debt-tracker.md
│   │
│   ├── references/
│   │   ├── api-governance.md
│   │   ├── best-practices.md
│   │   ├── concurrency.md
│   │   ├── database.md
│   │   ├── deployment.md
│   │   ├── event-driven.md
│   │   ├── goctl-commands.md
│   │   ├── observability.md
│   │   ├── resilience.md
│   │   ├── rest-api.md
│   │   ├── rpc.md
│   │   ├── security.md
│   │   ├── testing.md
│   │   ├── troubleshooting.md
│   │   └── checklists/
│   │       ├── db-migration.md
│   │       ├── new-api-definition.md
│   │       ├── new-handler.md
│   │       ├── new-middleware.md
│   │       ├── new-rpc-service.md
│   │       └── production-readiness.md
│   │
│   └── generated/
│       ├── INDEX.md
│       ├── modules/           (从 knowledge-base/modules/ 迁入)
│       └── flows/             (从 knowledge-base/flows/ 迁入)
```

## 3. zero-powers 迁移映射

### 3a. 参考文档（直接复制，去掉插件 frontmatter）

| zero-powers 源 | 目标 |
|---|---|
| `references/rest-api-patterns.md` | `docs/references/rest-api.md` |
| `references/rpc-patterns.md` | `docs/references/rpc.md` |
| `references/database-patterns.md` | `docs/references/database.md` |
| `references/api-governance-patterns.md` | `docs/references/api-governance.md` |
| `references/concurrency-patterns.md` | `docs/references/concurrency.md` |
| `references/deployment-patterns.md` | `docs/references/deployment.md` |
| `references/event-driven-patterns.md` | `docs/references/event-driven.md` |
| `references/goctl-commands.md` | `docs/references/goctl-commands.md` |
| `references/observability-patterns.md` | `docs/references/observability.md` |
| `references/resilience-patterns.md` | `docs/references/resilience.md` |
| `references/security-patterns.md` | `docs/references/security.md` |
| `references/testing-patterns.md` | `docs/references/testing.md` |
| `best-practices/overview.md` | `docs/references/best-practices.md` |
| `troubleshooting/common-issues.md` | `docs/references/troubleshooting.md` |
| `checklists/*.md` (6个) | `docs/references/checklists/*.md` |

### 3b. 不迁移项

| 源 | 理由 |
|---|---|
| `agents/*.md` (3个) | 规则内联到 CLAUDE.md 硬性约定 |
| `skills/*/SKILL.md` (13个) | 触发规则内联到工作流契约表格 |
| `templates/specs/*.md` (5个) | 脚手架模板，约定已在硬性约定中 |
| `docs/superpowers/` (插件自带) | 插件开发文档，与本项目无关 |

## 4. 现有文档清理与重分类

| 现有路径 | 目标 | 理由 |
|---|---|---|
| `docs/knowledge-base/modules/*.md` | `docs/generated/modules/*.md` | 代码分析生成 |
| `docs/knowledge-base/flows/*.md` | `docs/generated/flows/*.md` | 代码分析生成 |
| `docs/knowledge-base/INDEX.md` + `README.md` | `docs/generated/INDEX.md` | 合并为一个索引 |
| `docs/phases/*.md` | `docs/exec-plans/completed/phases/` | 已完成的阶段计划 |
| `docs/superpowers/specs/*.md` | `docs/design-docs/` | 设计文档 |
| `docs/superpowers/plans/*.md` | `docs/exec-plans/completed/` | 已完成的执行计划 |
| `docs/agent-harness/` | 删除 | 已过时，git 历史可追溯 |
| `docs/data-foundation.md` | `docs/design-docs/data-foundation.md` | 设计文档 |
| `docs/go-microservices-plan.md` | `docs/exec-plans/completed/go-microservices-plan.md` | 已完成计划 |
| `docs/pics/SPEC.md` | `docs/design-docs/architecture-diagrams.md` | 17 个 Mermaid 架构/流程图，属于设计文档 |

## 5. CLAUDE.md 重写方案

新结构：

```
## 项目概述          （保留）
## 技术栈            （保留）
## 目录结构          （更新为新 docs/ 结构）
## 服务架构          （保留，指向 docs/ARCHITECTURE.md）
## 错误码体系        （保留）
## 工作流契约        （路径全部从 zero-skills/ → docs/references/）
  ### 创意与设计阶段
  ### 实施阶段       （表格引用本地路径）
  ### 质量与调试阶段 （引用 docs/references/troubleshooting.md 和 checklists/）
  ### 收尾阶段
## go-zero 项目硬性约定 （保留全部）
## 明确禁止的行为    （更新路径引用）
```

关键变更：
- 去除 `use skill zero-init`
- 去除所有 `zero-powers:` skill 调用
- 去除 `use subagent gozero-architect`
- 所有 `zero-skills/` 路径 → `docs/references/`

## 6. AGENTS.md 重写方案

与 CLAUDE.md 保持同构，差异：
- 去掉 `use using-superpowers`、`use zero-powers` 等 Claude Code 专属指令
- 工作流中不引用 `superpowers:*` skill 名
- 保留所有硬性约定和禁止行为的实质内容
- 去除"不追踪 docs 文档"规则

## 7. 顶层文档内容来源

| 新文件 | 来源 |
|---|---|
| `docs/ARCHITECTURE.md` | CLAUDE.md "服务架构"段 + 补充 |
| `docs/DESIGN.md` | CLAUDE.md "硬性约定"段提炼 |
| `docs/SECURITY.md` | `zero-powers/references/security-patterns.md` 精简 |
| `docs/RELIABILITY.md` | `zero-powers/references/resilience-patterns.md` 精简 |
| `docs/QUALITY_SCORE.md` | CLAUDE.md "测试要求" + `zero-powers/references/testing-patterns.md` 精简 |
| `docs/design-docs/index.md` | 新建，所有设计文档索引 |
| `docs/exec-plans/tech-debt-tracker.md` | 新建空文件 |

## 8. 清理项

- 删除 `CLAUDE.md.bak`、`AGENTS.md.bak`
- 删除 `docs/agent-harness/` 整个目录
- 删除迁移后的旧目录：`docs/knowledge-base/`、`docs/phases/`、`docs/superpowers/`
- 卸载 zero-powers 插件（从全局 plugins 配置移除）

## 9. 风险与缓解

| 风险 | 缓解 |
|---|---|
| 大量文件移动导致 git blame 丢失 | 使用 `git mv` 保留历史追踪 |
| 遗漏某个 zero-powers 引用 | 迁移后 grep 全项目确认无残留引用 |
| 卸载插件后其他项目受影响 | zero-powers 是本用户开发的插件，确认只有本项目使用 |
| docs/pics/ 目录迁移后残留 | 迁移后删除空目录 |
