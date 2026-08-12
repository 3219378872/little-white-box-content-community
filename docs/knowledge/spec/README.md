# 规范层

本目录收录由人类开发者决定语义的项目规范。当前规范均为草案，不约束设计或实现。

- `owner: human` 表示语义决定权属于人类；agent 默认可以编辑和维护。
- 修改前只需获得当前对话中的人类自然语言授权，不需要授权文件、签名或额外审批记录。
- 授权只覆盖明确指示的内容和必要索引；需要补充新语义时必须再次询问。
- 规范描述可观察行为、约束、兼容边界和验收标准，不指定内部实现方案。
- 每份 `SPEC-*` 必须引用对应 `INT-*`；只有 `status: approved` 的规范可约束活跃设计。
- 只有人类明确接受、批准或要求发布正式规范时，agent 才能将状态设为 `approved`。
- 未获授权的 agent 建议必须写入 `../proposals/`。

创建页面时使用 `../templates/spec.md`，并在本页维护索引。

## 当前草案

| 规范 | 覆盖范围 | 状态 |
| --- | --- | --- |
| [SPEC-community-core](SPEC-community-core.md) | 用户、内容、互动、关系和交流 | draft |
| [SPEC-content-discovery](SPEC-content-discovery.md) | 关注流、搜索和个性化推荐 | draft |
| [SPEC-grounded-assistant](SPEC-grounded-assistant.md) | 基于已发布内容的可追溯回答 | draft |
| [SPEC-feedback-reliability](SPEC-feedback-reliability.md) | 行为数据闭环、可观测性和故障降级 | draft |

四份草案均引用 `INT-content-community-backend`，可以分别评审和批准。批准前，设计层不得把它们作为
正式上游；各页“待人类确认”中的问题也不构成要求。
