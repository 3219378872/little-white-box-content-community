# 实现层

本目录由 agent 维护，登记设计到代码的映射与验证证据。

- 源码、配置、`.api`、`.proto` 和测试是当前行为的权威事实。
- 实现页只记录设计到代码的映射、`aligned/diverged/unknown` 状态和最后验证点。
- `aligned` 或 `diverged` 页面必须引用活跃 `DES-*`，列出真实 `tracks`、提交和日期。
- 命令、结果和环境边界写入 [evidence/](evidence/README.md)，不得用一次验证证明永久同步。
- 新实现页使用 `../templates/implementation.md`。

## 当前实现映射

| 实现页 | 上游设计 | 状态 | 证据 |
| --- | --- | --- | --- |
| [IMP-content-community-backend](IMP-content-community-backend.md) | DES-content-community-backend | aligned | [2026-08-12](evidence/2026-08-12-content-community.md) |

## 待办（需外部输入）

- [TODO-blocked-gates](TODO-blocked-gates.md)：冻结评测集（DISC-060~063 / ASST-050~051）
  与月度生产观测数据（REL-030~043）两项收尾门禁。
