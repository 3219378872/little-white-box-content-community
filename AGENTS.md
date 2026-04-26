# AGENTS.md

This file provides guidance to Codex when working with code in this repository.

## MUST do at the start of any conversation
- use using-superpowers
- use zero-skills

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
├── proto/                # .proto 定义 (user/content/media/feed/interaction/message/recommend/search)
├── pkg/                  # 跨服务公共库
│   ├── errx/             # 业务错误码 (BizError + codes)
│   ├── jwtx/             # JWT 工具
│   ├── middleware/        # HTTP 中间件 (auth/cors)
│   ├── mqx/              # RocketMQ 封装
│   ├── cachex/           # 缓存键前缀
│   ├── result/           # 统一响应体 Result[T]
│   ├── validator/         # 输入校验 (手机号/密码/用户名)
│   ├── interceptor/       # gRPC 拦截器
│   └── util/             # 雪花ID、时间、哈希
├── deploy/               # docker-compose.middleware.yml + 监控配置
├── scripts/              # 辅助脚本
└── doc(s)/               # 文档
```

## 服务架构

```
Client → Gateway (REST :8888) → User RPC (:9090)
                               → Content RPC (:8088)
                               → Media RPC (:9008)
```

- **Gateway**：`app/gateway/gateway.go`，API 定义在 `app/gateway/gateway.api`
- **User**：`app/user/user.go`，proto 在 `proto/user/user.proto`
- **Content**：`app/content/content.go`，proto 在 `proto/content/content.proto`
- **Media**：`app/media/media.go`，proto 在 `proto/media/media.proto`

每个 RPC 服务遵循：`internal/config/` → `internal/svc/` → `internal/server/` → `internal/logic/` → `internal/model/`

## 错误码体系

定义在 `pkg/errx/codes.go`，按业务域分段：
- 通用 1-999 · 用户 1000-1999 · 内容 2000-2999 · 交互 3000-3999 · 媒体 4000-4999 · 搜索 5000-5999

Logic 层必须返回 `errx.New(code, msg)`，禁止裸 `errors.New()`。

---

## 技能使用契约

本项目使用 **Superpowers 工作流** + **zero-skills 技术知识库**。
Claude Code 在以下情况下**必须**按顺序调用对应技能，不允许跳步。

### 创意与设计阶段

**触发场景**：新服务 / 新功能 / 新接口 / 架构变更

```
1. superpowers:brainstorming
   ├─ 在「探索项目上下文」步骤自动加载：
   │   - zero-skills/best-practices/ 的相关模块
   │   - zero-skills/references/ 的约定文件
   └─ 必须获得用户对设计的批准
2. superpowers:writing-plans
   └─ 拆解步骤时参照 zero-skills/examples/ 的结构
```

### 实施阶段

| 任务 | 必须加载的 zero-skills 模块 | 流程技能 |
|------|---------------------------|---------|
| 新 Handler / Logic / Model | `references/rest-api-patterns.md`、`best-practices/overview.md` | `test-driven-development` |
| 新 `.api` 文件 / goctl 生成 | `references/goctl-commands.md` | `brainstorming` → `writing-plans` |
| 中间件 / JWT / Context 传递 | `references/rest-api-patterns.md`（Middleware 章节）、`references/security-patterns.md` | `test-driven-development` |
| 数据库操作 / 缓存 | `references/database-patterns.md` | `test-driven-development` |
| RPC 服务 / zrpc 客户端 | `references/rpc-patterns.md` | `brainstorming` → `TDD` |
| 错误处理 | `references/api-governance-patterns.md`（错误码章节） | - |
| 限流 / 熔断 / 降级 | `references/resilience-patterns.md` | `brainstorming` |
| 消息队列 | `references/event-driven-patterns.md` | `brainstorming` → `TDD` |

### 质量与调试阶段

**Bug / 测试失败 / 异常行为**：

```
1. superpowers:systematic-debugging
   ├─ Phase 1 根因调查完成后，查 zero-skills/troubleshooting/
   ├─ Phase 2 模式分析时对比 zero-skills/examples/
   └─ Phase 4 实施阶段用 TDD 先写失败用例
```

**完工前**：

```
1. superpowers:verification-before-completion
2. 逐项对照 zero-skills/checklists/ 中对应清单
3. 执行命令：
   - go test ./... -race -cover
   - go vet ./...
   - golangci-lint run
4. use subagent ($gozero-project-commands 审查本次git提交)
```

### 收尾阶段

```
1. superpowers:requesting-code-review
2. superpowers:finishing-a-development-branch
```

---

## go-zero 项目硬性约定

> 以下规则**不可违反**。若与 zero-skills 默认建议冲突，以本文档为准。

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

---

## 明确禁止的行为

Claude Code **不得**在本项目中：

1. 绕过 TDD 直接写生产代码（即使"很简单"也要 RED-GREEN-REFACTOR）
2. 在未走完 `brainstorming` 流程时修改架构层面代码（新 service / 分层变更 / 公共库）
3. 系统性调试时跳过 Phase 1 根因调查直接提修复方案
4. 完工前跳过 `zero-skills/checklists/` 对应清单就声明「完成」
5. 手动编辑 goctl 生成的文件（应修改 `.api` / `.proto` 后重新生成）
6. 为了通过测试修改测试本身（必须修代码让测试通过）
7. 在未经用户批准的情况下引入新依赖（`go get` 需先讨论）
8. 使用 `logx.Info/Error` 等不带 ctx 的日志函数
9. 追踪superpowers生成文档(本项目不追踪docs文档)
