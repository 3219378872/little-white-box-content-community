# 设计层

本目录由 agent 维护，说明如何满足已批准规范。

- 设计解释如何满足已批准规范，包括组件边界、数据流、取舍、失败模式和验证策略。
- `active` 设计必须引用已批准 `SPEC-*`，或在过渡期引用登记的旧基线。
- 上游缺失或冲突时使用 `blocked`，记录原因并请求人类决定，不能自行补写意图或规范。
- 新设计使用 `../templates/design.md`；被替代的设计标记 `superseded`，不改写历史结论。

## 当前设计

| 设计页 | 上游规范 | 状态 |
| --- | --- | --- |
| [DES-content-community-backend](DES-content-community-backend.md) | SPEC-community-core / SPEC-content-discovery / SPEC-grounded-assistant / SPEC-assistant-agent-mode / SPEC-feedback-reliability | active |
| [DES-assistant-agent-runtime](DES-assistant-agent-runtime.md) | SPEC-grounded-assistant / SPEC-assistant-agent-mode / SPEC-agent-memory / SPEC-agent-watch / SPEC-content-discovery / SPEC-feedback-reliability | active |
