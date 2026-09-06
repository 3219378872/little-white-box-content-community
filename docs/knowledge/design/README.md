# 设计层

本目录由 agent 维护，说明如何满足已批准规范。

- 设计解释如何满足已批准规范，包括组件边界、数据流、取舍、失败模式和验证策略。
- `active` / `blocked` 设计是 current DES，必须引用已批准 `SPEC-*`，并用 `tracks` 列出承接的
  精确 requirement ID；过渡基线只用于本治理页允许的旧边界。
- 上游缺失或冲突时使用 `blocked`，记录原因并请求人类决定，不能自行补写意图或规范。
- 新设计使用 `../templates/design.md`；被替代的设计标记 `superseded`，不改写历史结论。
- 所有 approved requirement 必须至少被一个 current DES 承接；实现所有权与验证状态分别见
  [implementation](../implementation/README.md) 和 [evidence](../evidence/README.md)。

## 当前设计

| 设计页 | 上游规范 | 状态 |
| --- | --- | --- |
| [DES-content-community-backend](DES-content-community-backend.md) | SPEC-community-core / SPEC-content-discovery / SPEC-assistant-agent / SPEC-agent-memory / SPEC-agent-watch / SPEC-feedback-reliability | active |
| [DES-assistant-agent-runtime](DES-assistant-agent-runtime.md) | SPEC-assistant-agent / SPEC-agent-memory / SPEC-agent-watch / SPEC-content-discovery / SPEC-feedback-reliability | active |
| [DES-agent-capability-governance](DES-agent-capability-governance.md) | SPEC-assistant-agent / SPEC-agent-memory | active |
| [DES-agent-community-research](DES-agent-community-research.md) | SPEC-assistant-agent / SPEC-agent-memory / SPEC-agent-watch / SPEC-content-discovery / SPEC-feedback-reliability | active |
