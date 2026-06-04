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
