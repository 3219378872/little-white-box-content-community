# Agent 文档入口

这是按需路由表，不要求全文阅读。治理规则见 [knowledge/README](knowledge/README.md)。

## 正式知识链

| 层 | 入口 | 作用 |
| --- | --- | --- |
| INT 意图 | [knowledge/intent](knowledge/intent/README.md) | 人类批准产品价值、能力与边界 |
| SPEC 规格 | [knowledge/spec](knowledge/spec/README.md) | 人类批准可验收工程要求 |
| DES 设计 | [knowledge/design](knowledge/design/README.md) | agent 说明如何满足 approved requirement |
| IMP 实现 | [knowledge/implementation](knowledge/implementation/README.md) | requirement 到当前代码与状态的唯一映射 |
| EVD 证据 | [knowledge/evidence](knowledge/evidence/README.md) | 对特定提交、命令和范围的事实记录 |

链路为 `INT -> SPEC -> DES -> IMP <-> EVD`。指南、状态页、提案和 legacy 证据不参与当前
`aligned` 判定。

## 任务路由

| 任务 | 先读 | 再核对 |
| --- | --- | --- |
| REST API、Handler、参数 | [开发指南](knowledge/guides/development-quickstart.md) | `app/gateway/*.api` 与 Handler/Logic |
| RPC、proto、服务调用 | [开发指南](knowledge/guides/development-quickstart.md) | `proto/**/*.proto` 与对应 `app/` 代码 |
| MySQL、Redis、事务、缓存 | [开发指南](knowledge/guides/development-quickstart.md) | Model、SQL patch 与配置 |
| 鉴权、错误、安全边界 | [工程约定](knowledge/guides/engineering-conventions.md) | `pkg/jwtx/`、`pkg/middleware/`、`pkg/errx/` |
| MQ、重试、降级、部署 | [开发指南](knowledge/guides/development-quickstart.md) | `deploy/`、消费者与运行配置 |
| 测试、质量、生成 | [开发指南](knowledge/guides/development-quickstart.md) | Make target、测试与 CI |
| 服务和数据流概览 | [架构指南](knowledge/guides/architecture.md) | 对应源码、配置与契约 |
| Agent、Memory、Watch | [Agent 规格](knowledge/spec/SPEC-assistant-agent.md) | 相关 DES、三个 domain IMP 与 EVD |
| 规格逐条对齐 | [实现索引](knowledge/implementation/README.md) | 六个 domain IMP |
| 外部/长期门禁 | [开放门禁](knowledge/status/open-gates.md) | domain IMP 与当前 EVD |

## 来源优先级

1. 应做什么：approved INT/SPEC 与 current DES。
2. 当前做了什么：源码、配置、API/proto、数据迁移与测试结果。
3. 是否有证明：唯一 IMP 行与 active EVD；证据只覆盖其提交、命令和 scope。
4. 指南、状态、提案、retired 页面与 legacy 记录只作导航或历史，不能覆盖前三项。
