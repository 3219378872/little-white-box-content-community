# 规格层

SPEC 由人类决定语义，定义正确性、安全、可靠性、性能、兼容、隐私和验收要求，不指定组件、存储、
内部字段或 UI 方案。agent 修改须有当前对话授权；只有人类明确批准才可标为 `approved`。未授权建议
进入 [proposals](../proposals/README.md)。新页面使用 [SPEC 模板](../templates/spec.md)。

## 当前规范

| 规范 | 范围 | 状态 |
| --- | --- | --- |
| [SPEC-community-core](SPEC-community-core.md) | 用户、内容、互动、关系和交流 | approved |
| [SPEC-content-discovery](SPEC-content-discovery.md) | 关注流、搜索和推荐 | approved |
| [SPEC-assistant-agent](SPEC-assistant-agent.md) | 社区辅助 Agent、检索引用与持久运行 | approved |
| [SPEC-agent-memory](SPEC-agent-memory.md) | 双文档自然语言记忆 | approved |
| [SPEC-agent-watch](SPEC-agent-watch.md) | Watch 主动 Assistant 私信 | approved |
| [SPEC-feedback-reliability](SPEC-feedback-reliability.md) | 行为闭环、SLO、观测与故障降级 | approved |
| [SPEC-grounded-assistant](SPEC-grounded-assistant.md) | 旧同步证据化回答 | retired |
| [SPEC-assistant-agent-mode](SPEC-assistant-agent-mode.md) | 旧同步模式化 Agent | retired |

六份 approved SPEC 均引用 `INT-content-community-backend`，其正文中的精确 requirement ID 构成当前
DES/IMP 覆盖全集。两份 retired SPEC 不能约束 current DES，也不能向当前台账贡献 requirement。

当前 Agent 语义以内容社区为主，复杂需求支持分轮澄清、站内多轮检索与授权的外部补充；检索回答的
实质信息须就近关联实际取得且支持表述的帖子/网页 URL，普通闲聊和澄清不强制引用。AGENT-A13 仍要求
既有人类冻结集质量门禁；LLM 或合成样例只能验证工具流程。实际实现状态见
[六域 IMP](../implementation/README.md)，不得从本索引或历史修订说明推断已经交付。
