---
id: DES-agent-capability-governance
layer: design
title: Agent 能力与动态扩展治理
status: active
owner: agent
upstream:
  - SPEC-assistant-agent
  - SPEC-agent-memory
---

# Agent 能力与动态扩展治理

## 决策

Little 的 Agent 能力优先实现为仓库内、服务端持有的显式工具 adapter。工具必须经过统一 metadata、
用户授权、source 分组、availability、结果上限、journal/确认策略和测试后，才能进入新的 prompt epoch。
运行时不得从用户目录、网络地址或模型输出动态加载可执行代码。

provider route 同样是受控 capability：配置只引用运维提供的 endpoint/model/凭据，session 只持久化不含
secret 的 route id 和能力快照。fallback 不能扩大工具、数据地域、保留期或隐私边界。

## 暂不引入

- 不引入 Hermes 式 `HERMES_HOME` profile；Little 的隔离单位是认证用户和 MySQL 行级所有权。
- 不引入外部 Memory provider；MySQL、容量、version、undo、删除与审计保持唯一权威。
- 不引入通用 cron、subagent、delegation 或 MoA；Watch 继续使用领域事件与 bucket。
- 不向社区 Agent 提供 terminal、browser、code execution 或任意 MCP server。
- 不建设动态 plugin/skills marketplace；静态 registry 足以覆盖当前规模。

## 重新评估门槛

只有同时出现至少两个独立能力所有者、静态发布节奏成为明确瓶颈，并完成插件签名、进程/网络隔离、
权限声明、secret 作用域、资源配额、审计、撤销和崩溃恢复设计后，才可提出动态扩展规范。重新评估必须
先进入 human-owned spec，不能由实现层自行启用。
