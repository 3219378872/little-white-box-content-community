# 文档重组与 zero-powers 内联化迁移 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将项目文档重组为标准结构（design-docs/exec-plans/references/generated），将 zero-powers 插件内容内联到本地，重写 CLAUDE.md/AGENTS.md，卸载插件。

**Architecture:** 纯文档迁移，不涉及代码逻辑。通过 `git mv` 保留文件历史，zero-powers 参考文档直接复制到 `docs/references/`，CLAUDE.md/AGENTS.md 从 `.bak` 文件重写。

**Tech Stack:** bash, git

**Spec:** `docs/superpowers/specs/2026-06-04-docs-restructure-design.md`

---

## Task 1: 创建目标目录骨架

**Files:**
- Create: `docs/design-docs/` (目录)
- Create: `docs/exec-plans/active/` (目录)
- Create: `docs/exec-plans/completed/phases/` (目录)
- Create: `docs/references/checklists/` (目录)
- Create: `docs/generated/modules/` (目录)
- Create: `docs/generated/flows/` (目录)

- [ ] **Step 1: 创建所有目标目录**

```bash
mkdir -p docs/design-docs docs/exec-plans/active docs/exec-plans/completed/phases docs/references/checklists docs/generated/modules docs/generated/flows
```

- [ ] **Step 2: 验证目录结构**

```bash
find docs -type d | sort
```

Expected: 上述 7 个目录全部存在。

- [ ] **Step 3: Commit**

```bash
# 空目录不会被 git 追踪，暂不 commit。后续文件迁入后一并 commit。
```

---

## Task 2: 迁移 zero-powers 参考文档到 docs/references/

**Files:**
- Create: `docs/references/rest-api.md`
- Create: `docs/references/rpc.md`
- Create: `docs/references/database.md`
- Create: `docs/references/api-governance.md`
- Create: `docs/references/concurrency.md`
- Create: `docs/references/deployment.md`
- Create: `docs/references/event-driven.md`
- Create: `docs/references/goctl-commands.md`
- Create: `docs/references/observability.md`
- Create: `docs/references/resilience.md`
- Create: `docs/references/security.md`
- Create: `docs/references/testing.md`
- Create: `docs/references/best-practices.md`
- Create: `docs/references/troubleshooting.md`

zero-powers 插件路径前缀：`~/.claude/plugins/cache/work-plugin-marketplace/zero-powers/0.1.3`

- [ ] **Step 1: 复制 12 个 reference 文件（重命名去掉 `-patterns` 后缀）**

```bash
ZP="$HOME/.claude/plugins/cache/work-plugin-marketplace/zero-powers/0.1.3"
cp "$ZP/references/rest-api-patterns.md"          docs/references/rest-api.md
cp "$ZP/references/rpc-patterns.md"               docs/references/rpc.md
cp "$ZP/references/database-patterns.md"           docs/references/database.md
cp "$ZP/references/api-governance-patterns.md"     docs/references/api-governance.md
cp "$ZP/references/concurrency-patterns.md"        docs/references/concurrency.md
cp "$ZP/references/deployment-patterns.md"         docs/references/deployment.md
cp "$ZP/references/event-driven-patterns.md"       docs/references/event-driven.md
cp "$ZP/references/goctl-commands.md"              docs/references/goctl-commands.md
cp "$ZP/references/observability-patterns.md"      docs/references/observability.md
cp "$ZP/references/resilience-patterns.md"         docs/references/resilience.md
cp "$ZP/references/security-patterns.md"           docs/references/security.md
cp "$ZP/references/testing-patterns.md"            docs/references/testing.md
```

- [ ] **Step 2: 复制 best-practices 和 troubleshooting**

```bash
cp "$ZP/best-practices/overview.md"          docs/references/best-practices.md
cp "$ZP/troubleshooting/common-issues.md"    docs/references/troubleshooting.md
```

- [ ] **Step 3: 验证文件数量**

```bash
ls docs/references/*.md | wc -l
```

Expected: `14`

- [ ] **Step 4: Commit**

```bash
git add docs/references/*.md
git commit -m "docs: copy zero-powers reference docs to docs/references/

Migrates 12 pattern references + best-practices + troubleshooting from
zero-powers plugin v0.1.3 to local docs/references/."
```

---

## Task 3: 迁移 zero-powers checklists 到 docs/references/checklists/

**Files:**
- Create: `docs/references/checklists/db-migration.md`
- Create: `docs/references/checklists/new-api-definition.md`
- Create: `docs/references/checklists/new-handler.md`
- Create: `docs/references/checklists/new-middleware.md`
- Create: `docs/references/checklists/new-rpc-service.md`
- Create: `docs/references/checklists/production-readiness.md`

- [ ] **Step 1: 复制 6 个 checklist 文件（重命名去掉 `-checklist` 后缀）**

```bash
ZP="$HOME/.claude/plugins/cache/work-plugin-marketplace/zero-powers/0.1.3"
cp "$ZP/checklists/db-migration-checklist.md"         docs/references/checklists/db-migration.md
cp "$ZP/checklists/new-api-definition-checklist.md"   docs/references/checklists/new-api-definition.md
cp "$ZP/checklists/new-handler-checklist.md"           docs/references/checklists/new-handler.md
cp "$ZP/checklists/new-middleware-checklist.md"         docs/references/checklists/new-middleware.md
cp "$ZP/checklists/new-rpc-service-checklist.md"       docs/references/checklists/new-rpc-service.md
cp "$ZP/checklists/production-readiness-checklist.md"   docs/references/checklists/production-readiness.md
```

- [ ] **Step 2: 清理 checklist 文件头部的 superpowers 引用**

每个 checklist 文件头部有类似以下内容需要删除：

```
> 触发时机：`superpowers:verification-before-completion` 阶段
> 前置技能：`superpowers:test-driven-development`
```

对所有 checklist 文件执行：

```bash
sed -i '/superpowers:/d' docs/references/checklists/*.md
```

- [ ] **Step 3: 验证**

```bash
ls docs/references/checklists/*.md | wc -l
grep -r "superpowers:" docs/references/checklists/ || echo "OK: no superpowers references"
```

Expected: 6 个文件，无 superpowers 引用。

- [ ] **Step 4: Commit**

```bash
git add docs/references/checklists/
git commit -m "docs: copy zero-powers checklists to docs/references/checklists/

Migrates 6 checklist files, strips superpowers skill references from headers."
```

---

## Task 4: 迁移现有文档 — knowledge-base → generated

**Files:**
- Move: `docs/knowledge-base/modules/*.md` → `docs/generated/modules/`
- Move: `docs/knowledge-base/flows/*.md` → `docs/generated/flows/`
- Create: `docs/generated/INDEX.md` (合并 INDEX.md + README.md)

- [ ] **Step 1: git mv 模块文件（22 个）**

```bash
for f in docs/knowledge-base/modules/*.md; do
  git mv "$f" "docs/generated/modules/$(basename "$f")"
done
```

- [ ] **Step 2: git mv 流程文件（4 个）**

```bash
for f in docs/knowledge-base/flows/*.md; do
  git mv "$f" "docs/generated/flows/$(basename "$f")"
done
```

- [ ] **Step 3: 合并 INDEX.md 和 README.md 为 docs/generated/INDEX.md**

将 `docs/knowledge-base/INDEX.md` 的内容作为主体，把 `README.md` 中的维护说明（Page Frontmatter、Keeping It Current）追加为末尾的「维护指南」章节。同时更新所有内部链接路径（去掉 `modules/` 和 `flows/` 前缀，因为它们现在是相对于 `generated/` 的子目录）。

新文件内容结构：

```markdown
# Knowledge Base Index

（原 INDEX.md 内容，链接路径不变 — modules/ 和 flows/ 已保持相对路径）

---

## 维护指南

（从 README.md 摘录：Page Frontmatter 格式说明、Module Pages 固定节结构、Keeping It Current 的 CI 规则说明）
（更新引用：docs/superpowers/specs/ → docs/design-docs/，docs/agent-harness/ → 已删除）
```

- [ ] **Step 4: 删除旧的 knowledge-base 目录**

```bash
git rm docs/knowledge-base/INDEX.md docs/knowledge-base/README.md
# 目录已空，git 自动移除
```

- [ ] **Step 5: 验证**

```bash
ls docs/generated/modules/ | wc -l  # Expected: 22
ls docs/generated/flows/ | wc -l    # Expected: 4
test -f docs/generated/INDEX.md && echo "INDEX exists"
test -d docs/knowledge-base && echo "ERROR: old dir still exists" || echo "OK: old dir removed"
```

- [ ] **Step 6: Commit**

```bash
git add docs/generated/
git commit -m "docs: migrate knowledge-base/ to generated/

Moves 22 module docs + 4 flow docs, merges INDEX.md and README.md."
```

---

## Task 5: 迁移现有文档 — superpowers/specs → design-docs

**Files:**
- Move: `docs/superpowers/specs/*.md` (22 个) → `docs/design-docs/`
- Move: `docs/data-foundation.md` → `docs/design-docs/data-foundation.md`
- Move: `docs/pics/SPEC.md` → `docs/design-docs/architecture-diagrams.md`
- Create: `docs/design-docs/index.md`

- [ ] **Step 1: git mv 所有 specs 文件**

```bash
for f in docs/superpowers/specs/*.md; do
  git mv "$f" "docs/design-docs/$(basename "$f")"
done
```

- [ ] **Step 2: git mv 散落的设计文档**

```bash
git mv docs/data-foundation.md docs/design-docs/data-foundation.md
git mv docs/pics/SPEC.md docs/design-docs/architecture-diagrams.md
```

- [ ] **Step 3: 创建 docs/design-docs/index.md**

```markdown
# 设计文档索引

本目录包含所有设计规格文档（specs），按日期排序。

## 架构

- [architecture-diagrams.md](architecture-diagrams.md) — 系统架构与流程 Mermaid 图（17 个）
- [data-foundation.md](data-foundation.md) — 数据基础设施设计

## 按时间线

| 日期 | 文档 | 主题 |
|------|------|------|
| 2026-04-13 | [后端待补接口](2026-04-13-后端待补接口-design.md) | 后端接口补全 |
| 2026-04-17 | [media模块](2026-04-17-media模块-design.md) | 媒体模块设计 |
| 2026-04-21 | [interaction-service](2026-04-21-interaction-service-design.md) | 交互服务 |
| 2026-04-22 | [getpostlist-optional-auth](2026-04-22-getpostlist-optional-auth-design.md) | 可选鉴权 |
| 2026-04-23 | [architecture-compliance](2026-04-23-architecture-compliance-design.md) | 架构合规 |
| 2026-04-23 | [feed-week2](2026-04-23-feed-week2-design.md) | Feed 第二周 |
| 2026-04-23 | [interaction-rmw](2026-04-23-interaction-rmw-design.md) | 交互读改写 |
| 2026-04-23 | [media-cleanup](2026-04-23-media-cleanup-design.md) | 媒体清理 |
| 2026-04-23 | [query-performance](2026-04-23-query-performance-design.md) | 查询性能 |
| 2026-04-24 | [message-service](2026-04-24-message-service-design.md) | 消息服务 |
| 2026-04-26 | [codex-windows-notification](2026-04-26-codex-windows-notification-context-design.md) | Windows 通知上下文 |
| 2026-04-26 | [golangci-lint-cleanupx](2026-04-26-golangci-lint-cleanupx-refactor-design.md) | Lint 重构 |
| 2026-04-26 | [message-correctness](2026-04-26-message-module-correctness-repair-design.md) | 消息模块修复 |
| 2026-04-26 | [dtm-distributed-tx](2026-04-26-w4-dtm-distributed-transactions-design.md) | DTM 分布式事务 |
| 2026-04-26 | [mq-consumer-integration](2026-04-26-w5-mq-consumer-integration-design.md) | MQ 消费者集成 |
| 2026-04-27 | [rocketmq-init-topics](2026-04-27-rocketmq-init-topics-design.md) | RocketMQ Topic 初始化 |
| 2026-04-27 | [service-owned-mq](2026-04-27-w5-service-owned-mq-consumers-design.md) | 服务自有 MQ 消费者 |
| 2026-04-28 | [service-startup-issues](2026-04-28-fix-service-startup-issues-design.md) | 服务启动问题 |
| 2026-04-29 | [data-foundation](2026-04-29-data-foundation-design.md) | 数据基础设施 |
| 2026-04-29 | [testing-standards](2026-04-29-testing-standards-implementation-design.md) | 测试标准 |
| 2026-05-20 | [data-foundation-iteration](2026-05-20-data-foundation-iteration-plan.md) | 数据基础迭代 |
| 2026-06-04 | [docs-restructure](2026-06-04-docs-restructure-design.md) | 文档重组与迁移（本次） |
```

- [ ] **Step 4: 删除空的 docs/pics/ 目录**

```bash
rmdir docs/pics 2>/dev/null || true
```

- [ ] **Step 5: 验证**

```bash
ls docs/design-docs/*.md | wc -l  # Expected: 25 (22 specs + data-foundation + architecture-diagrams + index)
test -f docs/design-docs/index.md && echo "OK: index exists"
test -f docs/design-docs/architecture-diagrams.md && echo "OK: diagrams exist"
```

- [ ] **Step 6: Commit**

```bash
git add docs/design-docs/
git commit -m "docs: migrate superpowers/specs/ to design-docs/

Moves 22 design specs, data-foundation.md, and architecture diagrams.
Creates design-docs/index.md with full chronological listing."
```

---

## Task 6: 迁移现有文档 — superpowers/plans + phases → exec-plans

**Files:**
- Move: `docs/superpowers/plans/*.md` (28 个) → `docs/exec-plans/completed/`
- Move: `docs/phases/*.md` (6 个) → `docs/exec-plans/completed/phases/`
- Move: `docs/go-microservices-plan.md` → `docs/exec-plans/completed/go-microservices-plan.md`
- Create: `docs/exec-plans/tech-debt-tracker.md`

- [ ] **Step 1: git mv 所有 plans 文件**

```bash
for f in docs/superpowers/plans/*.md; do
  git mv "$f" "docs/exec-plans/completed/$(basename "$f")"
done
```

- [ ] **Step 2: git mv phases 文件**

```bash
for f in docs/phases/*.md; do
  git mv "$f" "docs/exec-plans/completed/phases/$(basename "$f")"
done
```

- [ ] **Step 3: git mv go-microservices-plan.md**

```bash
git mv docs/go-microservices-plan.md docs/exec-plans/completed/go-microservices-plan.md
```

- [ ] **Step 4: 创建 docs/exec-plans/tech-debt-tracker.md**

```markdown
# 技术债务追踪

记录已知技术债务，按优先级排序。

| 优先级 | 描述 | 关联文档 | 状态 |
|--------|------|----------|------|
| — | （暂无条目） | — | — |
```

- [ ] **Step 5: 验证**

```bash
ls docs/exec-plans/completed/*.md | wc -l          # Expected: 29 (28 plans + go-microservices-plan)
ls docs/exec-plans/completed/phases/*.md | wc -l    # Expected: 6
test -f docs/exec-plans/tech-debt-tracker.md && echo "OK"
```

- [ ] **Step 6: Commit**

```bash
git add docs/exec-plans/
git commit -m "docs: migrate plans and phases to exec-plans/

Moves 28 superpowers plans + 6 phase docs + go-microservices-plan.
Creates tech-debt-tracker.md placeholder."
```

---

## Task 7: 删除旧目录和过时文件

**Files:**
- Delete: `docs/agent-harness/` (整个目录)
- Delete: `docs/superpowers/` (已空)
- Delete: `docs/phases/` (已空)
- Delete: `CLAUDE.md.bak`
- Delete: `AGENTS.md.bak`

- [ ] **Step 1: 删除 agent-harness 目录**

```bash
git rm -r docs/agent-harness/
```

- [ ] **Step 2: 删除已空的旧目录残留**

如果 `docs/superpowers/` 和 `docs/phases/` 中还有残留文件（git 目录在所有文件移走后自动消失，但需确认）：

```bash
# 检查是否还有残留
find docs/superpowers docs/phases -type f 2>/dev/null
# 如果有残留，git rm 之
git rm -r docs/superpowers/ 2>/dev/null || true
git rm -r docs/phases/ 2>/dev/null || true
```

- [ ] **Step 3: 删除 .bak 文件**

```bash
rm CLAUDE.md.bak AGENTS.md.bak
```

- [ ] **Step 4: 验证**

```bash
test -d docs/agent-harness && echo "ERROR" || echo "OK: agent-harness removed"
test -d docs/superpowers && echo "ERROR" || echo "OK: superpowers removed"
test -d docs/phases && echo "ERROR" || echo "OK: phases removed"
test -f CLAUDE.md.bak && echo "ERROR" || echo "OK: CLAUDE.md.bak removed"
test -f AGENTS.md.bak && echo "ERROR" || echo "OK: AGENTS.md.bak removed"
```

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "docs: remove obsolete directories and backup files

Removes agent-harness/, empty superpowers/ and phases/ dirs,
and CLAUDE.md.bak / AGENTS.md.bak."
```

---

## Task 8: 创建顶层文档 — docs/ARCHITECTURE.md

**Files:**
- Create: `docs/ARCHITECTURE.md`

内容来源：CLAUDE.md.bak 的「服务架构」段 + `docs/design-docs/architecture-diagrams.md` 的组件图引用。

- [ ] **Step 1: 创建 docs/ARCHITECTURE.md**

```markdown
# 服务架构

## 概览

esx 是一个基于 go-zero 的社交内容平台微服务集群。

```
Client → Gateway (REST :8888) → User RPC (:9090)
                               → Content RPC (:8088)
                               → Media RPC (:9008)
```

## 服务清单

| 服务 | 类型 | 端口 | 入口文件 | 定义文件 |
|------|------|------|---------|---------|
| Gateway | REST API 网关 | :8888 | `app/gateway/gateway.go` | `app/gateway/gateway.api` |
| User | RPC 服务 | :9090 | `app/user/user.go` | `proto/user/user.proto` |
| Content | RPC 服务 | :8088 | `app/content/content.go` | `proto/content/content.proto` |
| Media | RPC 服务 | :9008 | `app/media/media.go` | `proto/media/media.proto` |

## RPC 服务分层

每个 RPC 服务遵循 go-zero 标准分层：

```
internal/config/   → 配置结构体
internal/svc/      → 依赖注入容器（ServiceContext）
internal/server/   → gRPC server 实现
internal/logic/    → 业务逻辑
internal/model/    → 数据访问层
```

## 服务间通信

- **Gateway → RPC**：通过 zrpc 客户端，经 etcd 服务发现
- **RPC → RPC**：Content 聚合 User（作者信息）和 Interaction（点赞/收藏状态）
- **RPC → MQ**：异步事件通过 RocketMQ（media-deleted、post-create 等）
- **DTM 二阶段消息**：Content 发帖使用 DTM 保证写库与 Feed Fanout 的最终一致性

## 详细架构图

参见 [architecture-diagrams.md](design-docs/architecture-diagrams.md) —— 包含 17 个 Mermaid 图覆盖系统全景、请求生命周期、事件总线、部署拓扑等。
```

- [ ] **Step 2: Commit**

```bash
git add docs/ARCHITECTURE.md
git commit -m "docs: create ARCHITECTURE.md top-level service architecture doc"
```

---

## Task 9: 创建顶层文档 — docs/DESIGN.md

**Files:**
- Create: `docs/DESIGN.md`

内容来源：CLAUDE.md.bak 的「go-zero 项目硬性约定」段提炼为设计原则。

- [ ] **Step 1: 创建 docs/DESIGN.md**

```markdown
# 设计原则与约定

本文档汇总 esx 项目的核心设计原则和硬性约定。CLAUDE.md / AGENTS.md 中的规则引用本文档。

## 三层架构

| 层 | 职责 | 禁止 |
|----|------|------|
| Handler | 参数绑定、调用 Logic、返回响应 | 写业务逻辑 |
| Logic | 业务逻辑，通过 `svc.ServiceContext` 获取资源 | 直接访问 `http.Request` |
| Model | 数据访问 | 跨 Model 直接调用 |
| Svc | 依赖注入容器（DB / Redis / RPC 客户端等） | — |

## Context 传递

- **必须** `logx.WithContext(ctx).Info(...)` — 禁止不带 ctx 的日志
- **必须** 所有 zrpc 调用透传入参 ctx
- **必须** goroutine 内使用 ctx 的拷贝
- **禁止** `context.Background()` 创建新 ctx（除非最外层入口）

## 错误处理

- Logic 层统一返回 `errx.New(code, msg)`
- 错误码集中定义于 `pkg/errx/codes.go`，按业务域分段
- HTTP 状态码映射由 errx 中间件统一处理
- **禁止** `return errors.New("xxx")` 裸字符串错误
- **禁止** Handler 层手动 `httpx.Error(w, err)`

## 配置管理

- 所有配置走 `etc/*.yaml` → `config.Config` 结构体
- 敏感值走环境变量，yaml 只放 `${ENV_VAR}` 占位
- **禁止** 硬编码任何配置值

## 代码生成

- Handler 和 types 由 goctl 生成，**禁止**手动编辑
- 修改 `.api` 后：`goctl api go -api xxx.api -dir . --style go_zero`
- 修改 `.proto` 后：运行对应 goctl rpc 命令

## 详细参考

- [REST API 模式](references/rest-api.md)
- [RPC 模式](references/rpc.md)
- [数据库模式](references/database.md)
- [错误治理模式](references/api-governance.md)
- [最佳实践速查](references/best-practices.md)
```

- [ ] **Step 2: Commit**

```bash
git add docs/DESIGN.md
git commit -m "docs: create DESIGN.md core design principles and conventions"
```

---

## Task 10: 创建顶层文档 — docs/SECURITY.md, RELIABILITY.md, QUALITY_SCORE.md

**Files:**
- Create: `docs/SECURITY.md`
- Create: `docs/RELIABILITY.md`
- Create: `docs/QUALITY_SCORE.md`

- [ ] **Step 1: 创建 docs/SECURITY.md**

```markdown
# 安全

esx 项目安全模式概要。详细模式与代码示例参见 [references/security.md](references/security.md)。

## 核心安全措施

- **JWT 鉴权**：`pkg/jwtx/` — HS256 签名，防 `alg=none` 攻击，context 透传 userId
- **可选鉴权中间件**：`pkg/middleware/` — 公开接口有 token 则解析，无 token 不拦截
- **CORS**：`pkg/middleware/` — 白名单 origin 控制
- **gRPC 拦截器**：`pkg/interceptor/` — 业务错误跨进程传播，不泄露内部堆栈
- **输入校验**：`pkg/validator/` — 手机号/密码/用户名格式校验

## 必须遵守

- 不硬编码 secret，敏感值走环境变量
- 所有用户输入经过 validator 校验
- 错误消息不泄露敏感数据（errx 统一包装）
- 新增依赖需经用户批准

## 详细参考

- [安全模式完整文档](references/security.md)
- [生产就绪检查清单](references/checklists/production-readiness.md)
```

- [ ] **Step 2: 创建 docs/RELIABILITY.md**

```markdown
# 弹性与可靠性

esx 项目弹性模式概要。详细模式与代码示例参见 [references/resilience.md](references/resilience.md)。

## go-zero 内置防护（默认启用）

```
Request → Load Shedding → Rate Limiting → Circuit Breaker → Timeout → Service
```

- **熔断器**：Google SRE 算法，自动保护 RPC/DB/Redis 调用
- **限流**：令牌桶，按服务/接口粒度配置
- **过载保护**：自适应降载，CPU/内存超阈值自动拒绝
- **超时控制**：zrpc 全链路超时透传

## 分布式事务

- DTM 二阶段消息保证发帖写库与 Feed Fanout 最终一致性
- 屏障表 `QueryPrepared` 实现幂等

## MQ 可靠消费

- RocketMQ SendOneWay 用于非关键异步事件（media-deleted）
- 消费者 ConsumeRetryLater 自动重试

## 详细参考

- [弹性模式完整文档](references/resilience.md)
- [并发模式](references/concurrency.md)
- [事件驱动模式](references/event-driven.md)
```

- [ ] **Step 3: 创建 docs/QUALITY_SCORE.md**

```markdown
# 测试与代码质量标准

## 测试要求

- **最低覆盖率**：80%
- **必须包含**：单元测试 + 集成测试（涉及 DB / Redis / RPC）
- **每个 Logic** 至少一个失败路径测试

## 测试策略

| 类型 | 工具 | 用途 |
|------|------|------|
| SQL 断言 | sqlmock | 纯 SQL 逻辑验证 |
| 集成测试 | testcontainers | 真实 DB/Redis 端到端 |
| RPC mock | gomock | 跨服务调用隔离 |

- **禁止** mock `sqlx.SqlConn`
- **推荐** testcontainers 跑真实数据库

## 代码质量

- 函数 < 50 行
- 文件 < 800 行
- 嵌套 < 4 层
- 不硬编码配置值
- 不静默吞错误

## CI 门禁

```bash
go test ./... -race -cover
go vet ./...
golangci-lint run
```

## 详细参考

- [测试模式完整文档](references/testing.md)
- [最佳实践](references/best-practices.md)
- [完工检查清单](references/checklists/)
```

- [ ] **Step 4: Commit**

```bash
git add docs/SECURITY.md docs/RELIABILITY.md docs/QUALITY_SCORE.md
git commit -m "docs: create SECURITY.md, RELIABILITY.md, QUALITY_SCORE.md top-level docs"
```

---

## Task 11: 重写 CLAUDE.md

**Files:**
- Create: `CLAUDE.md`

内容基于 `CLAUDE.md.bak`，关键变更：
1. 去除 `use skill zero-init`
2. 去除所有 `zero-powers:` skill 调用和 `use subagent gozero-architect`
3. 所有 `zero-skills/` 路径 → `docs/references/`
4. 更新目录结构为新布局
5. 服务架构段指向 `docs/ARCHITECTURE.md`

- [ ] **Step 1: 创建新 CLAUDE.md**

```markdown
# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目概述

- **项目名称**：esx (little-white-box)
- **Go module**：`esx`
- **业务域**：社交内容平台（用户管理、内容发布、媒体处理）
- **服务数量**：4（1 REST 网关 + 3 RPC 服务）
- **部署环境**：Docker Compose（开发环境）

## 技术栈

- **框架**：go-zero v1.10.1 · Go 1.26.1
- **数据库**：MySQL 8.0（go-zero sqlx + CachedConn）
- **缓存**：Redis 7
- **消息队列**：RocketMQ 5.1.0
- **注册中心**：etcd v3.5.5
- **对象存储**：MinIO / SeaweedFS（S3 兼容）
- **可观测性**：Jaeger（链路追踪）· Prometheus + Grafana（指标）· Loki（日志）
- **分布式事务**：DTM
- **向量数据库**：Milvus（搜索/推荐）
- **全文搜索**：Elasticsearch 8.8.0

## 目录结构

```
esx/
├── app/
│   ├── gateway/          # REST API 网关 (:8888)
│   ├── user/             # 用户 RPC 服务 (:9090)
│   ├── content/          # 内容 RPC 服务 (:8088)
│   └── media/            # 媒体 RPC 服务 (:9008)
├── proto/                # .proto 定义
├── pkg/                  # 跨服务公共库
├── deploy/               # docker-compose + 监控配置
├── scripts/              # 辅助脚本
└── docs/
    ├── ARCHITECTURE.md   # 服务架构总览
    ├── DESIGN.md         # 设计原则与硬性约定
    ├── SECURITY.md       # 安全模式概要
    ├── RELIABILITY.md    # 弹性模式概要
    ├── QUALITY_SCORE.md  # 测试与代码质量标准
    ├── design-docs/      # 设计规格文档
    ├── exec-plans/       # 执行计划（active/ + completed/）
    ├── references/       # 参考手册（go-zero 模式、检查清单）
    └── generated/        # 自动生成的模块/流程文档
```

## 服务架构

```
Client → Gateway (REST :8888) → User RPC (:9090)
                               → Content RPC (:8088)
                               → Media RPC (:9008)
```

详见 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)。

## 错误码体系

定义在 `pkg/errx/codes.go`，按业务域分段：
- 通用 1-999 · 用户 1000-1999 · 内容 2000-2999 · 交互 3000-3999 · 媒体 4000-4999 · 搜索 5000-5999

Logic 层必须返回 `errx.New(code, msg)`，禁止裸 `errors.New()`。

---

## 工作流契约

本项目使用 **Superpowers 工作流** + **本地参考文档**（`docs/references/`）。

### 创意与设计阶段

**触发场景**：新服务 / 新功能 / 新接口 / 架构变更

```
1. superpowers:brainstorming
   └─ 在「探索项目上下文」步骤加载 docs/references/ 相关文档
2. superpowers:writing-plans
```

### 实施阶段

| 任务 | 参考文档 | 流程技能 |
|------|---------|---------|
| 新 Handler / Logic / Model | `docs/references/rest-api.md`、`docs/references/best-practices.md` | `test-driven-development` |
| 新 `.api` 文件 / goctl 生成 | `docs/references/goctl-commands.md` | `brainstorming` → `writing-plans` |
| 中间件 / JWT / Context 传递 | `docs/references/rest-api.md`、`docs/references/security.md` | `test-driven-development` |
| 数据库操作 / 缓存 | `docs/references/database.md` | `test-driven-development` |
| RPC 服务 / zrpc 客户端 | `docs/references/rpc.md` | `brainstorming` → `TDD` |
| 错误处理 | `docs/references/api-governance.md` | - |
| 限流 / 熔断 / 降级 | `docs/references/resilience.md` | `brainstorming` |
| 消息队列 | `docs/references/event-driven.md` | `brainstorming` → `TDD` |

### 质量与调试阶段

**Bug / 测试失败 / 异常行为**：

```
1. superpowers:systematic-debugging
   └─ 参考 docs/references/troubleshooting.md
```

**完工前**：

```
1. superpowers:verification-before-completion
2. 逐项对照 docs/references/checklists/ 中对应清单
3. 执行命令：
   - go test ./... -race -cover
   - go vet ./...
   - golangci-lint run
```

### 收尾阶段

```
1. superpowers:requesting-code-review
2. superpowers:finishing-a-development-branch
```

---

## go-zero 项目硬性约定

> 以下规则**不可违反**。详细设计原则参见 [docs/DESIGN.md](docs/DESIGN.md)。

### 分层约定（三层架构）

- **Handler 层**：仅做参数绑定、调用 Logic、返回响应。**禁止**在 Handler 写业务逻辑
- **Logic 层**：业务逻辑，通过 `svc.ServiceContext` 获取资源。**禁止**直接访问 `http.Request`
- **Model 层**：数据访问。**禁止**跨 Model 直接调用，Logic 统一协调
- **Svc 层**：依赖注入容器，承载 DB / Redis / RPC 客户端等

### Context 传递（高优先级）

- **禁止** `context.Background()` 创建新 ctx（除非最外层入口）
- **禁止** `logx.Info() / logx.Error()` 不带 ctx 的日志
- **必须** `logx.WithContext(ctx).Info(...)`
- **必须** 所有 zrpc 调用透传入参 ctx
- **必须** goroutine 内使用 `ctx` 的拷贝

### 错误处理

- **禁止** `return errors.New("xxx")` 字符串错误
- **禁止** 在 Handler 层手动 `httpx.Error(w, err)` 设置状态码
- **必须** Logic 层返回 `errx.New(code, msg)` 统一包装
- **必须** 错误码走 `pkg/errx/codes.go` 集中定义
- **必须** HTTP 状态码映射由 `errx` 中间件统一处理

### 配置管理

- **禁止** 硬编码任何配置值
- **必须** 所有配置走 `etc/*.yaml` → `config.Config` 结构体
- **必须** 敏感值走环境变量，`etc/*.yaml` 只放 `${ENV_VAR}` 占位

### 测试要求

- **最低覆盖率**：80%
- **必须包含**：单元测试 + 集成测试（涉及 DB / Redis / RPC）
- **禁止** mock `sqlx.SqlConn`，集成测试用 testcontainers 跑真实数据库
- **推荐** 用 `sqlmock` 跑纯 SQL 断言型测试，用 testcontainers 跑端到端测试
- **必须** 每个 Logic 至少一个失败路径测试

### 代码生成

- **禁止** 手写 `internal/handler/*.go`、`internal/types/types.go`（由 goctl 生成）
- **必须** 修改 `.api` 文件后重新运行 `goctl api go -api xxx.api -dir . --style go_zero`
- **必须** 修改 `.proto` 后重新运行对应 goctl rpc 命令
- 手动修改生成代码会在下次 goctl 重新生成时被覆盖

---

## 明确禁止的行为

Claude Code **不得**在本项目中：

1. 绕过 TDD 直接写生产代码（即使"很简单"也要 RED-GREEN-REFACTOR）
2. 在未走完 `brainstorming` 流程时修改架构层面代码（新 service / 分层变更 / 公共库）
3. 系统性调试时跳过 Phase 1 根因调查直接提修复方案
4. 完工前跳过 `docs/references/checklists/` 对应清单就声明「完成」
5. 手动编辑 goctl 生成的文件（应修改 `.api` / `.proto` 后重新生成）
6. 为了通过测试修改测试本身（必须修代码让测试通过）
7. 在未经用户批准的情况下引入新依赖（`go get` 需先讨论）
8. 使用 `logx.Info/Error` 等不带 ctx 的日志函数
```

- [ ] **Step 2: 验证没有残留的 zero-powers/zero-skills 引用**

```bash
grep -n "zero-skills\|zero-powers\|zero-init\|gozero-architect" CLAUDE.md || echo "OK: no legacy references"
```

Expected: "OK: no legacy references"

- [ ] **Step 3: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: rewrite CLAUDE.md with local docs/references/ paths

Removes all zero-powers/zero-skills plugin references, inlines rules,
updates directory structure to new docs/ layout."
```

---

## Task 12: 重写 AGENTS.md

**Files:**
- Create: `AGENTS.md`

与 CLAUDE.md 同构，差异：去掉 superpowers skill 引用和 Claude Code 专属指令。

- [ ] **Step 1: 创建新 AGENTS.md**

```markdown
# AGENTS.md

This file provides guidance to Codex when working with code in this repository.

## RULES
- Obey the rules of skills
- When subagent-driven-development, obey the skill rules strictly

## 项目概述

- **项目名称**：esx (little-white-box)
- **Go module**：`esx`
- **业务域**：社交内容平台（用户管理、内容发布、媒体处理）
- **服务数量**：4（1 REST 网关 + 3 RPC 服务）
- **部署环境**：Docker Compose（开发环境）

## 技术栈

- **框架**：go-zero v1.10.1 · Go 1.26.1
- **数据库**：MySQL 8.0（go-zero sqlx + CachedConn）
- **缓存**：Redis 7
- **消息队列**：RocketMQ 5.1.0
- **注册中心**：etcd v3.5.5
- **对象存储**：MinIO / SeaweedFS（S3 兼容）
- **可观测性**：Jaeger（链路追踪）· Prometheus + Grafana（指标）· Loki（日志）
- **分布式事务**：DTM
- **向量数据库**：Milvus（搜索/推荐）
- **全文搜索**：Elasticsearch 8.8.0

## 目录结构

```
esx/
├── app/
│   ├── gateway/          # REST API 网关 (:8888)
│   ├── user/             # 用户 RPC 服务 (:9090)
│   ├── content/          # 内容 RPC 服务 (:8088)
│   └── media/            # 媒体 RPC 服务 (:9008)
├── proto/                # .proto 定义
├── pkg/                  # 跨服务公共库
├── deploy/               # docker-compose + 监控配置
├── scripts/              # 辅助脚本
└── docs/
    ├── ARCHITECTURE.md   # 服务架构总览
    ├── DESIGN.md         # 设计原则与硬性约定
    ├── SECURITY.md       # 安全模式概要
    ├── RELIABILITY.md    # 弹性模式概要
    ├── QUALITY_SCORE.md  # 测试与代码质量标准
    ├── design-docs/      # 设计规格文档
    ├── exec-plans/       # 执行计划（active/ + completed/）
    ├── references/       # 参考手册（go-zero 模式、检查清单）
    └── generated/        # 自动生成的模块/流程文档
```

## 服务架构

```
Client → Gateway (REST :8888) → User RPC (:9090)
                               → Content RPC (:8088)
                               → Media RPC (:9008)
```

详见 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)。

## 错误码体系

定义在 `pkg/errx/codes.go`，按业务域分段：
- 通用 1-999 · 用户 1000-1999 · 内容 2000-2999 · 交互 3000-3999 · 媒体 4000-4999 · 搜索 5000-5999

Logic 层必须返回 `errx.New(code, msg)`，禁止裸 `errors.New()`。

---

## 工作流参考文档

### 实施阶段

| 任务 | 参考文档 |
|------|---------|
| 新 Handler / Logic / Model | `docs/references/rest-api.md`、`docs/references/best-practices.md` |
| 新 `.api` 文件 / goctl 生成 | `docs/references/goctl-commands.md` |
| 中间件 / JWT / Context 传递 | `docs/references/rest-api.md`、`docs/references/security.md` |
| 数据库操作 / 缓存 | `docs/references/database.md` |
| RPC 服务 / zrpc 客户端 | `docs/references/rpc.md` |
| 错误处理 | `docs/references/api-governance.md` |
| 限流 / 熔断 / 降级 | `docs/references/resilience.md` |
| 消息队列 | `docs/references/event-driven.md` |

### 质量检查

完工前：
- 对照 `docs/references/checklists/` 中对应清单
- 参考 `docs/references/troubleshooting.md` 排查问题
- 执行命令：
  - `go test ./... -race -cover`
  - `go vet ./...`
  - `golangci-lint run`

---

## go-zero 项目硬性约定

> 以下规则**不可违反**。详细设计原则参见 [docs/DESIGN.md](docs/DESIGN.md)。

### 分层约定（三层架构）

- **Handler 层**：仅做参数绑定、调用 Logic、返回响应。**禁止**在 Handler 写业务逻辑
- **Logic 层**：业务逻辑，通过 `svc.ServiceContext` 获取资源。**禁止**直接访问 `http.Request`
- **Model 层**：数据访问。**禁止**跨 Model 直接调用，Logic 统一协调
- **Svc 层**：依赖注入容器，承载 DB / Redis / RPC 客户端等

### Context 传递（高优先级）

- **禁止** `context.Background()` 创建新 ctx（除非最外层入口）
- **禁止** `logx.Info() / logx.Error()` 不带 ctx 的日志
- **必须** `logx.WithContext(ctx).Info(...)`
- **必须** 所有 zrpc 调用透传入参 ctx
- **必须** goroutine 内使用 `ctx` 的拷贝

### 错误处理

- **禁止** `return errors.New("xxx")` 字符串错误
- **禁止** 在 Handler 层手动 `httpx.Error(w, err)` 设置状态码
- **必须** Logic 层返回 `errx.New(code, msg)` 统一包装
- **必须** 错误码走 `pkg/errx/codes.go` 集中定义
- **必须** HTTP 状态码映射由 `errx` 中间件统一处理

### 配置管理

- **禁止** 硬编码任何配置值
- **必须** 所有配置走 `etc/*.yaml` → `config.Config` 结构体
- **必须** 敏感值走环境变量，`etc/*.yaml` 只放 `${ENV_VAR}` 占位

### 测试要求

- **最低覆盖率**：80%
- **必须包含**：单元测试 + 集成测试（涉及 DB / Redis / RPC）
- **禁止** mock `sqlx.SqlConn`，集成测试用 testcontainers 跑真实数据库
- **推荐** 用 `sqlmock` 跑纯 SQL 断言型测试，用 testcontainers 跑端到端测试
- **必须** 每个 Logic 至少一个失败路径测试

### 代码生成

- **禁止** 手写 `internal/handler/*.go`、`internal/types/types.go`（由 goctl 生成）
- **必须** 修改 `.api` 文件后重新运行 `goctl api go -api xxx.api -dir . --style go_zero`
- **必须** 修改 `.proto` 后重新运行对应 goctl rpc 命令
- 手动修改生成代码会在下次 goctl 重新生成时被覆盖

---

## 明确禁止的行为

Agent **不得**在本项目中：

1. 绕过 TDD 直接写生产代码（即使"很简单"也要 RED-GREEN-REFACTOR）
2. 在未经设计评审时修改架构层面代码（新 service / 分层变更 / 公共库）
3. 系统性调试时跳过根因调查直接提修复方案
4. 完工前跳过 `docs/references/checklists/` 对应清单就声明「完成」
5. 手动编辑 goctl 生成的文件（应修改 `.api` / `.proto` 后重新生成）
6. 为了通过测试修改测试本身（必须修代码让测试通过）
7. 在未经用户批准的情况下引入新依赖（`go get` 需先讨论）
8. 使用 `logx.Info/Error` 等不带 ctx 的日志函数
```

- [ ] **Step 2: 验证没有残留引用**

```bash
grep -n "zero-skills\|zero-powers\|zero-init\|gozero-architect\|superpowers:" AGENTS.md || echo "OK: no legacy references"
```

Expected: "OK: no legacy references"

- [ ] **Step 3: Commit**

```bash
git add AGENTS.md
git commit -m "docs: rewrite AGENTS.md with local docs/references/ paths

Removes all zero-powers/zero-skills and superpowers references,
keeps all conventions and rules with local doc paths."
```

---

## Task 13: 全局验证 + 卸载 zero-powers 插件

**Files:**
- Modify: 全局 plugins 配置（卸载 zero-powers）

- [ ] **Step 1: 全项目 grep 确认无残留 zero-powers/zero-skills 引用**

```bash
grep -rn "zero-skills\|zero-powers\|zero-init" --include="*.md" . | grep -v "docs/design-docs/2026-06-04-docs-restructure-design.md" | grep -v "docs/exec-plans/completed/"
```

Expected: 无输出（设计文档和已完成计划中的历史引用可以保留）。

- [ ] **Step 2: 验证新目录结构完整性**

```bash
echo "=== Top-level ==="
test -f CLAUDE.md && echo "OK: CLAUDE.md" || echo "MISSING: CLAUDE.md"
test -f AGENTS.md && echo "OK: AGENTS.md" || echo "MISSING: AGENTS.md"

echo "=== docs/ top-level ==="
for f in ARCHITECTURE.md DESIGN.md SECURITY.md RELIABILITY.md QUALITY_SCORE.md; do
  test -f "docs/$f" && echo "OK: docs/$f" || echo "MISSING: docs/$f"
done

echo "=== docs/ subdirs ==="
for d in design-docs exec-plans/active exec-plans/completed exec-plans/completed/phases references references/checklists generated generated/modules generated/flows; do
  test -d "docs/$d" && echo "OK: docs/$d/" || echo "MISSING: docs/$d/"
done

echo "=== Key files ==="
test -f docs/design-docs/index.md && echo "OK: index.md" || echo "MISSING: index.md"
test -f docs/exec-plans/tech-debt-tracker.md && echo "OK: tech-debt-tracker.md" || echo "MISSING"
test -f docs/generated/INDEX.md && echo "OK: generated INDEX.md" || echo "MISSING"

echo "=== Old dirs should NOT exist ==="
for d in docs/knowledge-base docs/phases docs/superpowers docs/agent-harness docs/pics; do
  test -d "$d" && echo "ERROR: $d still exists" || echo "OK: $d removed"
done

echo "=== Old files should NOT exist ==="
test -f CLAUDE.md.bak && echo "ERROR" || echo "OK: no CLAUDE.md.bak"
test -f AGENTS.md.bak && echo "ERROR" || echo "OK: no AGENTS.md.bak"
```

- [ ] **Step 3: 卸载 zero-powers 插件**

```bash
claude plugins remove zero-powers
```

如果 `claude plugins remove` 不可用，手动从全局 plugins 配置中移除：

```bash
# 查找 plugins 配置文件
cat ~/.claude/plugins/installed.json 2>/dev/null || cat ~/.claude/plugins.json 2>/dev/null
# 编辑移除 zero-powers 条目
```

- [ ] **Step 4: 验证插件已卸载**

```bash
claude plugins list 2>/dev/null | grep zero-powers && echo "ERROR: still installed" || echo "OK: zero-powers removed"
```

- [ ] **Step 5: 最终 Commit（如有未提交变更）**

```bash
git status
# 如果有未提交的变更
git add -A
git commit -m "chore: remove zero-powers plugin dependency

Plugin content has been migrated to docs/references/.
All CLAUDE.md/AGENTS.md references updated to local paths."
```
