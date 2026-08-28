---
implementation: IMP-content-community-backend
verified_at: 2026-08-28
verified_commit: 249f766b6ffa1ba2669e1bfa7400739ad8a85076
commands:
  - PATH=/tmp/xbh-gen-venv/bin:$PATH make generate
  - make check
  - make test
  - make coverage
  - make production-config
  - just e2e -q
result: passed
---

# 2026-08-28 审查修复验证

## 范围

- Gateway 以自有 RequiredAuth 替代会 dump 完整请求的框架 JWT handler，关闭 REST request dump；
  RPC 客户端改为只记录方法、耗时与错误。生产 Nginx 覆盖客户端 X-Forwarded-For。
- Agent 内置安全 system prompt 始终存在；Memory/Watch 作为结构化不可信用户数据注入。
- 个性化偏好不可查时 behavior 记忆 fail-closed；Memory PATCH 使用 optional 字段并校验更新值。
- Watch 回源校验当前 published/revision；execution 与 hit 同事务；列表和对话注入回源失败时脱敏。
- production compose 补 Agent/LLM/Tavily 环境，并在完整服务启动前重放幂等 SQL patches。

## 结果

- 完整生成、`make check` 和全量 race `make test` 通过。
- `make coverage` 通过：Handler 88.1%、Logic 79.7%、Model 12.4%、MQ consumer 74.6%、Shared 59.2%。
- production Compose 解析通过；assistant-rpc 明确含 Agent/LLM small/Tavily 变量，Watch 含 etcd/internal auth。
- 真实联调 E2E：116 passed、1 skipped（外部 LLM 条件）；业务断言与 teardown 均通过。
- JWT 与登录失败 sentinel 探针均返回预期 401；新增日志段 sentinel/request dump 出现数均为 0，
  SafeAccessLog 两条。

## 未覆盖边界

- 未启动整套 production profile，也未在生产持久卷执行迁移；仅验证 Compose 渲染和迁移入口。
- `discussion_spike` 的生产 LLM Judge 仍未接入，保持实现映射中的 partial。
- 人类冻结评测集与月度 SLO 不因本次工程修复自动升格。
