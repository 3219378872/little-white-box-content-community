---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 682b035
commands:
  - go test -count=1 ./app/interaction/...
  - make check
  - make test
result: passed
---

# 2026-08-14 业务路径全局日志修复（AGENTS.md ctx 规则）

## 缺陷

`get_counts_logic.go` 的 `parseInt64` 是包级工具函数，解析失败时用
`logx.Errorf`（全局 logger，不带 ctx）记日志——违反 AGENTS.md
「日志使用 logx.WithContext(ctx)」。该函数在 `GetCounts` 业务路径
（有请求 ctx）上被调用，日志缺失请求上下文。

## 修复

- `parseInt64(value, logger logx.Logger)`：改为接收 ctx 绑定 logger，
  调用方传 `l.Logger`（`logx.WithContext(ctx)` 注入）。
- 测试更新为传 `logx.WithContext(context.Background())`。

## 审查证据（本轮扫描）

- 全仓扫描 `l.Errorf`（9 处，interaction）：均通过
  `Logger: logx.WithContext(ctx)` 注入，**合规**。
- 其余全局日志（content/user/interaction/recommend main 的
  shutdown/relay 日志、media s3 初始化日志）均为最外层入口/启动路径，
  无请求 ctx，可接受。

## 补充（flaky 测试修复）

全量 `make test` 暴露 `TestGetRecommendPostsFaultInjectionOnlineInfer/
grpc_deadline` 间歇失败（单独跑必过）：fault-injection server 用
`net.Listen` 异步启动，并发下 `Rank` 可能在连接就绪前被调用而返回
`Unavailable`（误判为 infer-unavailable 而非 infer-timeout）。测试
client 增加 `grpc.WaitForReady(true)` 后消除时序竞态；`make test`
85 包 0 失败。

## 结果

- `go test ./app/interaction/...`、`./app/recommend/rpc/internal/logic/`
  全过；`make check` 通过；`make test` 85 包 0 失败（含 race）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
